package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parsePlaylistResponse(text string) ([]Song, error) {
	if songs, ok := parseSongsJSON(text); ok {
		return songs, nil
	}
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start >= 0 && end > start {
		if songs, ok := parseSongsJSON(text[start : end+1]); ok {
			return songs, nil
		}
	}
	return nil, fmt.Errorf("failed to parse playlist from ai response")
}

func parseSongsJSON(raw string) ([]Song, bool) {
	var anyArr []map[string]any
	if err := json.Unmarshal([]byte(raw), &anyArr); err != nil {
		return nil, false
	}
	out := make([]Song, 0, len(anyArr))
	for _, item := range anyArr {
		title, _ := item["title"].(string)
		artist, _ := item["artist"].(string)
		if title == "" || artist == "" {
			continue
		}
		out = append(out, Song{Title: title, Artist: artist})
	}
	return out, true
}
