// pkg/channel/telegram_avatar.go — fetch and cache Telegram user avatars.
//
// Flow:
//  1. getUserProfilePhotos(user_id=N, limit=1) → photos[0][largest].file_id
//  2. getFile(file_id) → file_path
//  3. downloadTelegramFile(file_path) → bytes + content-type
//  4. store.SaveAvatar(...)
//
// Runs on a goroutine; failures are silent (logged only). Per-user de-dupe
// via in-memory sync.Map (one fetch attempt per restart).
//
// Added 26.5.12v1 (E-01).

package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/network"
)

var telegramAvatarFetched sync.Map

// fetchAndCacheTelegramAvatar is fire-and-forget. Caller invokes it after
// store.Resolve, passing the canonical contact ID.
func (b *TelegramBot) fetchAndCacheTelegramAvatar(userID int64, contactID string) {
	if userID == 0 || contactID == "" || b.agentDir == "" {
		return
	}
	key := fmt.Sprintf("%d", userID)
	if _, attempted := telegramAvatarFetched.LoadOrStore(key, true); attempted {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[telegram/avatar] panic userID=%d: %v", userID, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fileID, err := b.fetchTelegramAvatarFileID(userID)
		if err != nil {
			log.Printf("[telegram/avatar] getUserProfilePhotos userID=%d: %v", userID, err)
			return
		}
		if fileID == "" {
			return
		}
		data, ct, err := b.downloadFileByID(ctx, fileID)
		if err != nil {
			log.Printf("[telegram/avatar] download userID=%d: %v", userID, err)
			return
		}
		if len(data) > network.MaxAvatarBytes {
			log.Printf("[telegram/avatar] userID=%d skipped (size %d > %d)",
				userID, len(data), network.MaxAvatarBytes)
			return
		}
		wsDir := filepath.Join(b.agentDir, "workspace")
		store := network.NewStore(wsDir)
		if err := store.SaveAvatar(contactID, data, ct); err != nil {
			log.Printf("[telegram/avatar] save userID=%d: %v", userID, err)
			return
		}
		log.Printf("[telegram/avatar] cached userID=%d contact=%s bytes=%d ct=%s",
			userID, contactID, len(data), ct)
	}()
}

// fetchTelegramAvatarFileID returns the file_id of the user's biggest photo,
// or "" if the user has no profile photo. Picks the highest resolution from
// the photo grid (TG returns multiple sizes).
func (b *TelegramBot) fetchTelegramAvatarFileID(userID int64) (string, error) {
	body, err := b.apiPost("getUserProfilePhotos", map[string]any{
		"user_id": userID,
		"limit":   1,
	})
	if err != nil {
		return "", err
	}
	type photoSize struct {
		FileID   string `json:"file_id"`
		FileSize int    `json:"file_size"`
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			TotalCount int           `json:"total_count"`
			Photos     [][]photoSize `json:"photos"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", fmt.Errorf("getUserProfilePhotos: %s", result.Description)
	}
	if result.Result.TotalCount == 0 || len(result.Result.Photos) == 0 {
		return "", nil // no avatar set
	}
	row := result.Result.Photos[0]
	if len(row) == 0 {
		return "", nil
	}
	// Pick the largest size (last element is usually the highest resolution).
	best := row[len(row)-1]
	for _, p := range row {
		if p.FileSize > best.FileSize {
			best = p
		}
	}
	return best.FileID, nil
}
