package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func GeneratePlaylistClaude(ctx context.Context, apiKey, prompt string, count int) ([]Song, error) {
	systemPrompt := `You are a music expert. Generate playlists based on user requests.
Return ONLY a JSON array of songs, no other text. Each song should have "title" and "artist" fields.
Example: [{"title": "Blue in Green", "artist": "Miles Davis"}, {"title": "Take Five", "artist": "Dave Brubeck"}]`

	userPrompt := fmt.Sprintf("Generate a playlist of exactly %d songs for: \"%s\"\nReturn only the JSON array, no explanation.", count, prompt)

	payload := map[string]any{
		"model":      "claude-sonnet-4-5",
		"max_tokens": 2048,
		"system":     systemPrompt,
		"messages": []map[string]any{
			{"role": "user", "content": userPrompt},
		},
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claude api error: %d - %s", resp.StatusCode, string(body))
	}

	var data struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	text := ""
	if len(data.Content) > 0 {
		text = data.Content[0].Text
	}
	return parsePlaylistResponse(text)
}
