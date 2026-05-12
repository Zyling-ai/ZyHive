// pkg/channel/feishu_avatar.go — fetch and cache contact avatars from Feishu.
//
// Flow:
//  1. GET /contact/v3/users/{openID}?user_id_type=open_id (tenant token)
//     → data.user.avatar.avatar_origin = direct CDN URL
//  2. GET that URL → image bytes
//  3. SaveAvatar(...) on the per-agent network.Store
//
// All work happens on a goroutine, never blocking the inbound message stream.
// Failures (auth, 4xx, oversized, network) are silently swallowed with a log.
//
// Added 26.5.12v1 (E-01).

package channel

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/Zyling-ai/zyhive/pkg/network"
)

// feishuAvatarFetched tracks (openID → true) to avoid re-fetching on every
// message. Process-local; restarts trigger a re-fetch if the agent hasn't
// persisted an avatar yet.
var feishuAvatarFetched sync.Map

// fetchAndCacheFeishuAvatar is fire-and-forget: looks up avatar URL via the
// Feishu user info API, downloads it, and writes via SaveAvatar.
//
// Caller invokes this *after* a successful store.Resolve so we know the
// contact file already exists. Caller must own b (FeishuBot) — we read
// b.refreshToken / b.apiBase / b.client / b.agentDir.
func (b *FeishuBot) fetchAndCacheFeishuAvatar(openID, contactID string) {
	if openID == "" || contactID == "" || b.agentDir == "" {
		return
	}
	// Per-process dedupe: only one attempt per openID per restart.
	if _, attempted := feishuAvatarFetched.LoadOrStore(openID, true); attempted {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[feishu/avatar] panic openID=%s: %v", openID, r)
			}
		}()
		url, err := b.fetchFeishuAvatarURL(openID)
		if err != nil {
			log.Printf("[feishu/avatar] resolve URL openID=%s: %v", openID, err)
			return
		}
		if url == "" {
			return
		}
		data, ct, err := b.downloadAvatarBytes(url)
		if err != nil {
			log.Printf("[feishu/avatar] download openID=%s: %v", openID, err)
			return
		}
		wsDir := filepath.Join(b.agentDir, "workspace")
		store := network.NewStore(wsDir)
		if err := store.SaveAvatar(contactID, data, ct); err != nil {
			log.Printf("[feishu/avatar] save openID=%s: %v", openID, err)
			return
		}
		log.Printf("[feishu/avatar] cached openID=%s contact=%s bytes=%d ct=%s",
			openID, contactID, len(data), ct)
	}()
}

// fetchFeishuAvatarURL returns the avatar_origin URL from /contact/v3/users.
// Empty string on success means "no avatar configured" (not an error).
func (b *FeishuBot) fetchFeishuAvatarURL(openID string) (string, error) {
	token, err := b.refreshToken()
	if err != nil {
		return "", fmt.Errorf("refreshToken: %w", err)
	}
	req, _ := http.NewRequest("GET",
		b.apiBase()+"/contact/v3/users/"+openID+"?user_id_type=open_id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// Read more than 4 KiB — the v3/users response with avatar block can be
	// quite long.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			User struct {
				Avatar struct {
					AvatarOrigin string `json:"avatar_origin"`
					Avatar640    string `json:"avatar_640"`
					Avatar240    string `json:"avatar_240"`
					Avatar72     string `json:"avatar_72"`
				} `json:"avatar"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("feishu code=%d msg=%q", result.Code, result.Msg)
	}
	av := result.Data.User.Avatar
	// Prefer 240 (small enough not to bloat disk, large enough to look ok at 52px).
	if av.Avatar240 != "" {
		return av.Avatar240, nil
	}
	if av.Avatar640 != "" {
		return av.Avatar640, nil
	}
	if av.AvatarOrigin != "" {
		return av.AvatarOrigin, nil
	}
	return av.Avatar72, nil
}

// downloadAvatarBytes pulls an image URL and returns (bytes, content-type).
// Hard-capped at MaxAvatarBytes+1 so SaveAvatar can reject oversized images
// without us holding huge buffers.
func (b *FeishuBot) downloadAvatarBytes(url string) ([]byte, string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, network.MaxAvatarBytes+1))
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	return data, ct, nil
}
