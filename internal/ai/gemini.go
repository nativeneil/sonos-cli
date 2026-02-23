package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func GeneratePlaylistGemini(ctx context.Context, apiKey, prompt string, count int) ([]Song, error) {
	fullPrompt := fmt.Sprintf(`You are a music expert. Generate a playlist of exactly %d songs for: "%s"

Return ONLY a JSON array of songs, no other text. Each song should have "title" and "artist" fields.
Example: [{"title": "Blue in Green", "artist": "Miles Davis"}, {"title": "Take Five", "artist": "Dave Brubeck"}]

Return only the JSON array, no explanation or markdown formatting.`, count, prompt)

	payload := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": fullPrompt}}},
		},
		"generationConfig": map[string]int{"maxOutputTokens": 2048},
	}
	buf, _ := json.Marshal(payload)
	u := "https://generativelanguage.googleapis.com/v1beta/models/gemini-3-flash-preview:generateContent?key=" + url.QueryEscape(apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini api error: %d - %s", resp.StatusCode, string(body))
	}

	var data struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	text := ""
	if len(data.Candidates) > 0 && len(data.Candidates[0].Content.Parts) > 0 {
		text = data.Candidates[0].Content.Parts[0].Text
	}
	return parsePlaylistResponse(text)
}
