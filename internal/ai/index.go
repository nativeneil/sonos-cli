package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

func songKey(song Song) string {
	return strings.ToLower(song.Artist) + ":::" + strings.ToLower(song.Title)
}

func GeneratePlaylistAllProviders(ctx context.Context, keys APIKeys, prompt string, countPerProvider int) ([]RankedSong, error) {
	type providerCall struct {
		name string
		key  string
		fn   func(context.Context, string, string, int) ([]Song, error)
	}
	calls := []providerCall{}
	if keys.Anthropic != "" {
		calls = append(calls, providerCall{name: "claude", key: keys.Anthropic, fn: GeneratePlaylistClaude})
	}
	if keys.OpenAI != "" {
		calls = append(calls, providerCall{name: "openai", key: keys.OpenAI, fn: GeneratePlaylistOpenAI})
	}
	if keys.Google != "" {
		calls = append(calls, providerCall{name: "gemini", key: keys.Google, fn: GeneratePlaylistGemini})
	}
	if keys.XAI != "" {
		calls = append(calls, providerCall{name: "grok", key: keys.XAI, fn: GeneratePlaylistGrok})
	}
	if len(calls) == 0 {
		return nil, fmt.Errorf("no api keys configured")
	}

	all := make([]Song, 0, len(calls)*countPerProvider)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, c := range calls {
		call := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			songs, err := call.fn(ctx, call.key, prompt, countPerProvider)
			if err != nil {
				return
			}
			mu.Lock()
			all = append(all, songs...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	counts := map[string]RankedSong{}
	for _, song := range all {
		key := songKey(song)
		existing, ok := counts[key]
		if ok {
			existing.Votes++
			counts[key] = existing
		} else {
			counts[key] = RankedSong{Song: song, Votes: 1}
		}
	}

	ranked := make([]RankedSong, 0, len(counts))
	for _, s := range counts {
		ranked = append(ranked, s)
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Votes > ranked[j].Votes
	})
	return ranked, nil
}

func GenerateMoreSongs(ctx context.Context, keys APIKeys, prompt string, existingSongs []Song, count int) ([]Song, error) {
	existing := map[string]struct{}{}
	for _, s := range existingSongs {
		existing[songKey(s)] = struct{}{}
	}

	exclude := make([]string, 0, min(10, len(existingSongs)))
	for i := 0; i < len(existingSongs) && i < 10; i++ {
		exclude = append(exclude, existingSongs[i].Artist+" - "+existingSongs[i].Title)
	}
	newPrompt := prompt + ". Exclude these songs: " + strings.Join(exclude, ", ") + ". Give me different songs."

	providers := []struct {
		key string
		fn  func(context.Context, string, string, int) ([]Song, error)
	}{
		{key: keys.Anthropic, fn: GeneratePlaylistClaude},
		{key: keys.OpenAI, fn: GeneratePlaylistOpenAI},
		{key: keys.Google, fn: GeneratePlaylistGemini},
		{key: keys.XAI, fn: GeneratePlaylistGrok},
	}

	for _, p := range providers {
		if p.key == "" {
			continue
		}
		songs, err := p.fn(ctx, p.key, newPrompt, count)
		if err != nil {
			continue
		}
		out := make([]Song, 0, len(songs))
		for _, s := range songs {
			if _, ok := existing[songKey(s)]; !ok {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return []Song{}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
