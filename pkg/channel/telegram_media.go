package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ── Media resolution ──────────────────────────────────────────────────────

// resolveMedia downloads relevant media from a message, returning:
//   - media: list of MediaInput (images/PDFs to pass to LLM)
//   - extraText: placeholder text for non-downloadable media
func (b *TelegramBot) resolveMedia(ctx context.Context, msg *TelegramMessage) ([]MediaInput, string, error) {
	var media []MediaInput
	var extras []string

	// Photo: download highest resolution (current message)
	if len(msg.Photo) > 0 {
		best := msg.Photo[len(msg.Photo)-1]
		log.Printf("[telegram] downloading photo fileID=%s size=%d", best.FileID, best.FileSize)
		data, ct, err := b.downloadFileByID(ctx, best.FileID)
		if err != nil {
			log.Printf("[telegram] photo download error: %v", err)
			extras = append(extras, "[📷 图片]")
		} else {
			log.Printf("[telegram] photo downloaded: %d bytes, contentType=%q", len(data), ct)
			media = append(media, MediaInput{Data: data, ContentType: ct, FileName: "photo.jpg"})
		}
	}

	// Reply-to photo: if the user replied to a photo message, also download that image.
	// This allows the bot to see images when the user replies to a photo with a question.
	if msg.ReplyToMessage != nil && len(msg.ReplyToMessage.Photo) > 0 && len(msg.Photo) == 0 {
		replyPhotos := msg.ReplyToMessage.Photo
		best := replyPhotos[len(replyPhotos)-1]
		log.Printf("[telegram] downloading reply-to photo fileID=%s", best.FileID)
		data, ct, err := b.downloadFileByID(ctx, best.FileID)
		if err != nil {
			log.Printf("[telegram] reply-to photo download error: %v", err)
		} else {
			log.Printf("[telegram] reply-to photo downloaded: %d bytes, contentType=%q", len(data), ct)
			media = append(media, MediaInput{Data: data, ContentType: ct, FileName: "replied_photo.jpg"})
		}
	}

	// Video
	if msg.Video != nil {
		extras = append(extras, "[📹 视频消息]")
	}

	// Audio
	if msg.Audio != nil {
		extras = append(extras, "[🎵 音频消息]")
	}

	// Voice
	if msg.Voice != nil {
		extras = append(extras, "[🎤 语音消息]")
	}

	// VideoNote
	if msg.VideoNote != nil {
		extras = append(extras, "[🎥 视频笔记]")
	}

	// Document
	if msg.Document != nil {
		doc := msg.Document
		if doc.MimeType == "application/pdf" {
			data, ct, err := b.downloadFileByID(ctx, doc.FileID)
			if err != nil {
				log.Printf("[telegram] pdf download error: %v", err)
				extras = append(extras, "[📎 文件: "+doc.FileName+"]")
			} else {
				media = append(media, MediaInput{Data: data, ContentType: ct, FileName: doc.FileName})
			}
		} else {
			name := doc.FileName
			if name == "" {
				name = "文件"
			}
			extras = append(extras, "[📎 文件: "+name+"]")
		}
	}

	// Sticker
	if msg.Sticker != nil {
		s := msg.Sticker
		if s.IsAnimated || s.IsVideo {
			emoji := s.Emoji
			if emoji == "" {
				emoji = "贴纸"
			}
			extras = append(extras, "[贴纸: "+emoji+"]")
		} else {
			// Static WEBP sticker — download as image
			data, ct, err := b.downloadFileByID(ctx, s.FileID)
			if err != nil {
				log.Printf("[telegram] sticker download error: %v", err)
				emoji := s.Emoji
				if emoji == "" {
					emoji = "贴纸"
				}
				extras = append(extras, "[贴纸: "+emoji+"]")
			} else {
				media = append(media, MediaInput{Data: data, ContentType: ct, FileName: "sticker.webp"})
			}
		}
	}

	// Animation / GIF
	if msg.Animation != nil {
		extras = append(extras, "[🎞 动图]")
	}

	extraText := strings.Join(extras, " ")
	return media, extraText, nil
}

// downloadFileByID uses getFile to get the file path, then downloads it.
func (b *TelegramBot) downloadFileByID(ctx context.Context, fileID string) ([]byte, string, error) {
	filePath, err := b.getFilePath(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	return b.downloadTelegramFile(ctx, filePath)
}

// getFilePath calls Telegram getFile to resolve a file_id to a file path.
func (b *TelegramBot) getFilePath(ctx context.Context, fileID string) (string, error) {
	body, err := b.apiPost("getFile", map[string]any{"file_id": fileID})
	if err != nil {
		return "", fmt.Errorf("getFile request: %w", err)
	}
	var result struct {
		OK     bool   `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
			FileSize int    `json:"file_size"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("getFile parse: %w", err)
	}
	if !result.OK {
		return "", fmt.Errorf("getFile: %s", result.Description)
	}
	// 20MB limit
	const maxSize = 20 * 1024 * 1024
	if result.Result.FileSize > maxSize {
		return "", fmt.Errorf("file too large: %d bytes (max 20MB)", result.Result.FileSize)
	}
	return result.Result.FilePath, nil
}

// downloadTelegramFile fetches a file from Telegram servers.
// Returns: raw bytes, content-type, error.
func (b *TelegramBot) downloadTelegramFile(ctx context.Context, filePath string) ([]byte, string, error) {
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, filePath)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	const maxDownload = 20 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	// Strip content-type parameters (e.g. "image/jpeg; charset=binary" → "image/jpeg")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	// Normalize and guess from extension if needed
	lower := strings.ToLower(filePath)
	switch strings.ToLower(ct) {
	case "image/jpeg", "image/jpg":
		ct = "image/jpeg"
	case "image/png":
		ct = "image/png"
	case "image/webp":
		ct = "image/webp"
	case "image/gif":
		ct = "image/gif"
	case "application/pdf":
		ct = "application/pdf"
	default:
		// Guess from file extension
		switch {
		case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg"):
			ct = "image/jpeg"
		case strings.HasSuffix(lower, ".png"):
			ct = "image/png"
		case strings.HasSuffix(lower, ".webp"):
			ct = "image/webp"
		case strings.HasSuffix(lower, ".pdf"):
			ct = "application/pdf"
		case strings.HasSuffix(lower, ".gif"):
			ct = "image/gif"
		default:
			ct = "image/jpeg" // sensible default for Telegram photos
		}
	}

	return data, ct, nil
}

