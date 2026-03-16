package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Zyling-ai/zyhive/pkg/llm"
)

// VisionCaller is a function that calls the LLM with images for visual analysis.
// Injected from pool.go so the image tool can make sub-LLM calls.
type VisionCaller func(ctx context.Context, prompt string, imagePaths []string) (string, error)

var imageAnalyzeToolDef = llm.ToolDef{
	Name:        "image",
	Description: "Analyze one or more images using the vision model. Accepts file paths or URLs. Use when you need to understand, describe, or extract information from images.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"image":{"type":"string","description":"Single image path or URL"},
			"images":{"type":"array","items":{"type":"string"},"description":"Multiple image paths or URLs (up to 10)"},
			"prompt":{"type":"string","description":"What to analyze or extract from the image(s)"}
		},
		"required":["prompt"]
	}`),
}

// WithVisionCaller registers the image analysis tool using the provided LLM vision caller.
// If caller is nil, the tool is not registered.
func (r *Registry) WithVisionCaller(caller VisionCaller) {
	if caller == nil {
		return
	}
	r.register(imageAnalyzeToolDef, func(ctx context.Context, input json.RawMessage) (string, error) {
		return handleImageAnalyze(ctx, input, caller)
	})
}

func handleImageAnalyze(ctx context.Context, input json.RawMessage, caller VisionCaller) (string, error) {
	var p struct {
		Image  string   `json:"image"`
		Images []string `json:"images"`
		Prompt string   `json:"prompt"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", fmt.Errorf("image: invalid input: %v", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("image: prompt is required")
	}

	var allImages []string
	if p.Image != "" {
		allImages = append(allImages, p.Image)
	}
	allImages = append(allImages, p.Images...)

	if len(allImages) == 0 {
		return "", fmt.Errorf("image: at least one image (image or images) is required")
	}
	if len(allImages) > 10 {
		allImages = allImages[:10]
	}

	return caller(ctx, p.Prompt, allImages)
}

// BuildVisionCaller creates a VisionCaller using the given LLM client and model.
// Images can be file paths (base64-encoded) or HTTP(S) URLs.
func BuildVisionCaller(client llm.Client, model, apiKey string) VisionCaller {
	return func(ctx context.Context, prompt string, images []string) (string, error) {
		// Build content array with image blocks + text prompt
		type imageSourceURL struct {
			Type string `json:"type"` // "url"
			URL  string `json:"url"`
		}
		type imageSourceBase64 struct {
			Type      string `json:"type"` // "base64"
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		}
		type imageBlockURL struct {
			Type   string         `json:"type"` // "image"
			Source imageSourceURL `json:"source"`
		}
		type imageBlockBase64 struct {
			Type   string            `json:"type"` // "image"
			Source imageSourceBase64 `json:"source"`
		}
		type textBlock struct {
			Type string `json:"type"` // "text"
			Text string `json:"text"`
		}

		var parts []any
		for _, img := range images {
			if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
				parts = append(parts, imageBlockURL{
					Type:   "image",
					Source: imageSourceURL{Type: "url", URL: img},
				})
			} else {
				// Local file → base64
				data, err := os.ReadFile(img)
				if err != nil {
					return "", fmt.Errorf("image: read %s: %v", img, err)
				}
				ext := strings.ToLower(filepath.Ext(img))
				mediaType := "image/jpeg"
				switch ext {
				case ".png":
					mediaType = "image/png"
				case ".gif":
					mediaType = "image/gif"
				case ".webp":
					mediaType = "image/webp"
				}
				parts = append(parts, imageBlockBase64{
					Type: "image",
					Source: imageSourceBase64{
						Type:      "base64",
						MediaType: mediaType,
						Data:      base64.StdEncoding.EncodeToString(data),
					},
				})
			}
		}
		parts = append(parts, textBlock{Type: "text", Text: prompt})

		contentJSON, err := json.Marshal(parts)
		if err != nil {
			return "", fmt.Errorf("image: marshal content: %v", err)
		}

		req := &llm.ChatRequest{
			Model:     model,
			APIKey:    apiKey,
			MaxTokens: 1024,
			Messages: []llm.ChatMessage{
				{Role: "user", Content: contentJSON},
			},
		}

		ctxTimeout, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		ch, err := client.Stream(ctxTimeout, req)
		if err != nil {
			return "", fmt.Errorf("image: LLM stream: %v", err)
		}

		var sb strings.Builder
		for ev := range ch {
			switch ev.Type {
			case llm.EventTextDelta:
				sb.WriteString(ev.Text)
			case llm.EventError:
				if ev.Err != nil && sb.Len() == 0 {
					return "", fmt.Errorf("image: LLM error: %v", ev.Err)
				}
			}
		}
		if sb.Len() == 0 {
			return "No analysis result.", nil
		}
		return sb.String(), nil
	}
}

// fetchURLToTempFile downloads a URL to a temp file and returns its path.
// Used internally if we need to normalize URL images.
func fetchURLToTempFile(imageURL string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ext := ".jpg"
	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "png"):
		ext = ".png"
	case strings.Contains(ct, "gif"):
		ext = ".gif"
	case strings.Contains(ct, "webp"):
		ext = ".webp"
	}

	f, err := os.CreateTemp("", "zyhive-img-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
