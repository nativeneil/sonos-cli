package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func GeneratePlaylistGrok(ctx context.Context, apiKey, prompt string, count int) ([]Song, error) {
	systemPrompt := `You are a music expert. Generate playlists based on user requests.
Return ONLY a JSON array of songs, no other text. Each song should have "title" and "artist" fields.
Example: [{"title": "Blue in Green", "artist": "Miles Davis"}, {"title": "Take Five", "artist": "Dave Brubeck"}]`

	userPrompt := fmt.Sprintf("Generate a playlist of exactly %d songs for: \"%s\"\nReturn only the JSON array, no explanation.", count, prompt)

	payload := map[string]any{
		"model": "grok-4-1-fast-reasoning",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"max_tokens": 2048,
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.x.ai/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("xai api error: %d - %s", resp.StatusCode, string(body))
	}

	var data struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	text := ""
	if len(data.Choices) > 0 {
		text = data.Choices[0].Message.Content
	}
	return parsePlaylistResponse(text)
}
