package sonos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sonos-playlist/internal/ai"
	"sonos-playlist/internal/storage"
)

type QueueResult struct {
	Song    ai.Song
	Success bool
	Error   string
	TrackID int
}

type itunesSearchResult struct {
	ResultCount int           `json:"resultCount"`
	Results     []itunesTrack `json:"results"`
}

type itunesTrack struct {
	TrackID      int    `json:"trackId"`
	TrackName    string `json:"trackName"`
	ArtistName   string `json:"artistName"`
	Collection   string `json:"collectionName"`
	IsStreamable *bool  `json:"isStreamable"`
}

type sonosState struct {
	PlaybackState string       `json:"playbackState"`
	CurrentTrack  trackSummary `json:"currentTrack"`
	Currenttrack  trackSummary `json:"currenttrack"`
}

type trackSummary struct {
	Title      string `json:"title"`
	TrackName  string `json:"trackName"`
	Artist     string `json:"artist"`
	ArtistName string `json:"artistName"`
	Creator    string `json:"creator"`
}

var unwantedKeywords = []string{
	"lullaby", "lullabies", "karaoke", "tribute", "cover", "instrumental",
	"kids", "baby", "babies", "nursery", "toddler", "children",
	"made famous", "in the style of", "originally performed",
	"8-bit", "8 bit", "piano version", "acoustic cover",
	"ringtone", "workout", "fitness",
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

type searchCacheEntry struct {
	candidates []int
	expiresAt  time.Time
}

var (
	itunesHTTPClient = &http.Client{Timeout: 8 * time.Second}
	searchCacheMu    sync.RWMutex
	searchCache      = map[string]searchCacheEntry{}
)

const searchCacheTTL = 10 * time.Minute

func debugf(format string, args ...any) {
	if os.Getenv("DEBUG") == "1" {
		fmt.Printf(format+"\n", args...)
	}
}

func isUnwantedVersion(trackName, artistName, collectionName, wantedArtist string) bool {
	trackLower := strings.ToLower(trackName)
	artistLower := strings.ToLower(artistName)
	albumLower := strings.ToLower(collectionName)
	wantedLower := strings.ToLower(wantedArtist)

	for _, kw := range unwantedKeywords {
		if strings.Contains(trackLower, kw) || strings.Contains(artistLower, kw) || strings.Contains(albumLower, kw) {
			return true
		}
	}

	if !strings.Contains(artistLower, wantedLower) && !strings.Contains(wantedLower, artistLower) {
		if !strings.Contains(artistLower, "feat") && !strings.Contains(artistLower, "&") && !strings.Contains(artistLower, "with ") {
			return true
		}
	}
	return false
}

func scoreMatch(trackName, artistName string, song ai.Song) int {
	score := 0
	trackTitle := strings.ToLower(trackName)
	trackArtist := strings.ToLower(artistName)
	wantedTitle := strings.ToLower(song.Title)
	wantedArtist := strings.ToLower(song.Artist)

	if trackArtist == wantedArtist {
		score += 100
	} else if strings.Contains(trackArtist, wantedArtist) {
		score += 50
	} else if strings.Contains(wantedArtist, trackArtist) {
		score += 40
	}

	if trackTitle == wantedTitle {
		score += 100
	} else if strings.Contains(trackTitle, wantedTitle) {
		score += 50
	} else if strings.Contains(wantedTitle, trackTitle) {
		score += 40
	}

	if strings.Contains(trackTitle, "remix") {
		score -= 20
	}
	if strings.Contains(trackTitle, "live") {
		score -= 10
	}
	if strings.Contains(trackTitle, "remaster") {
		score -= 5
	}
	if strings.Contains(trackTitle, "edit") {
		score -= 15
	}
	if strings.Contains(trackTitle, "version") {
		score -= 10
	}
	return score
}

func normalizeText(v string) string {
	v = strings.ToLower(v)
	v = nonAlphaNum.ReplaceAllString(v, " ")
	return strings.TrimSpace(v)
}

func stateMatchesSong(state sonosState, song ai.Song) bool {
	current := state.CurrentTrack
	if current.Title == "" && current.TrackName == "" && current.Artist == "" && current.ArtistName == "" && current.Creator == "" {
		current = state.Currenttrack
	}
	stateTitle := current.Title
	if stateTitle == "" {
		stateTitle = current.TrackName
	}
	stateArtist := current.Artist
	if stateArtist == "" {
		stateArtist = current.ArtistName
	}
	if stateArtist == "" {
		stateArtist = current.Creator
	}

	wantTitle := normalizeText(song.Title)
	wantArtist := normalizeText(song.Artist)
	gotTitle := normalizeText(stateTitle)
	gotArtist := normalizeText(stateArtist)
	if gotTitle == "" || gotArtist == "" {
		return false
	}
	return strings.Contains(gotTitle, wantTitle) && strings.Contains(gotArtist, wantArtist)
}

func isPlayingState(state sonosState) bool {
	p := strings.ToUpper(state.PlaybackState)
	return p == "PLAYING" || p == "TRANSITIONING"
}

func waitForExpectedTrack(ctx context.Context, client *Client, room string, song ai.Song, timeout time.Duration) bool {
	encodedRoom := url.PathEscape(room)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false
		}
		var state sonosState
		err := client.RequestJSON(ctx, "/"+encodedRoom+"/state", &state)
		if err == nil && stateMatchesSong(state, song) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func waitForExpectedTrackPlaying(ctx context.Context, client *Client, room string, song ai.Song, timeout time.Duration) bool {
	encodedRoom := url.PathEscape(room)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return false
		}
		var state sonosState
		err := client.RequestJSON(ctx, "/"+encodedRoom+"/state", &state)
		if err == nil && stateMatchesSong(state, song) && isPlayingState(state) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func getCoordinatorUUID(ctx context.Context, client *Client, room string) string {
	var zones []Zone
	if err := client.RequestJSON(ctx, "/zones", &zones); err != nil {
		return ""
	}
	for _, z := range zones {
		if z.Coordinator.RoomName == room {
			return z.Coordinator.UUID
		}
		for _, m := range z.Members {
			if m.RoomName == room {
				return z.Coordinator.UUID
			}
		}
	}
	return ""
}

func searchITunesCandidates(ctx context.Context, song ai.Song) []int {
	cacheKey := normalizeText(song.Artist) + ":::" + normalizeText(song.Title)
	now := time.Now()

	searchCacheMu.RLock()
	entry, ok := searchCache[cacheKey]
	searchCacheMu.RUnlock()

	var baseCandidates []int
	if ok && now.Before(entry.expiresAt) {
		baseCandidates = append([]int(nil), entry.candidates...)
	} else {
		query := url.QueryEscape(song.Artist + " " + song.Title)
		u := "https://itunes.apple.com/search?media=music&limit=25&entity=song&term=" + query

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := itunesHTTPClient.Do(req)
		if err != nil {
			debugf("    [DEBUG] iTunes search error: %v", err)
			return []int{}
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			debugf("    [DEBUG] iTunes fetch failed: %d", resp.StatusCode)
			return []int{}
		}
		var data itunesSearchResult
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return []int{}
		}
		debugf("    [DEBUG] iTunes returned %d results", data.ResultCount)
		if data.ResultCount == 0 {
			return []int{}
		}

		type candidate struct {
			trackID int
			score   int
		}

		afterStreamable := make([]itunesTrack, 0, len(data.Results))
		for _, t := range data.Results {
			if t.IsStreamable != nil && !*t.IsStreamable {
				continue
			}
			afterStreamable = append(afterStreamable, t)
		}
		afterUnwanted := make([]itunesTrack, 0, len(afterStreamable))
		for _, t := range afterStreamable {
			if !isUnwantedVersion(t.TrackName, t.ArtistName, t.Collection, song.Artist) {
				afterUnwanted = append(afterUnwanted, t)
			}
		}
		debugf("    [DEBUG] After streamable filter: %d", len(afterStreamable))
		debugf("    [DEBUG] After unwanted filter: %d", len(afterUnwanted))

		scored := make([]candidate, 0, len(afterUnwanted))
		for _, t := range afterUnwanted {
			sc := scoreMatch(t.TrackName, t.ArtistName, song)
			scored = append(scored, candidate{trackID: t.TrackID, score: sc})
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })

		positive := make([]int, 0, len(scored))
		for _, c := range scored {
			if c.score > 0 {
				positive = append(positive, c.trackID)
			}
		}
		if len(positive) > 0 {
			baseCandidates = positive
			debugf("    [DEBUG] Found %d positive matches, best ID: %d", len(positive), positive[0])
		} else {
			for _, t := range afterUnwanted {
				baseCandidates = append(baseCandidates, t.TrackID)
			}
			if len(baseCandidates) > 0 {
				debugf("    [DEBUG] Using fallback: %d", baseCandidates[0])
			} else {
				debugf("    [DEBUG] Using fallback: none")
			}
		}

		searchCacheMu.Lock()
		searchCache[cacheKey] = searchCacheEntry{
			candidates: append([]int(nil), baseCandidates...),
			expiresAt:  time.Now().Add(searchCacheTTL),
		}
		searchCacheMu.Unlock()
	}

	blocked := storage.GetBlockedTrackIDs()
	replacements := []int{}
	for _, id := range storage.GetSongReplacementTrackIDs(song.Artist, song.Title) {
		if _, ok := blocked[id]; !ok {
			replacements = append(replacements, id)
		}
	}
	filtered := make([]int, 0, len(baseCandidates))
	for _, id := range baseCandidates {
		if _, ok := blocked[id]; ok {
			continue
		}
		filtered = append(filtered, id)
	}
	debugf("    [DEBUG] After blocklist filter: %d", len(filtered))

	preferred := dedupeInts(append(replacements, filtered...))
	if len(replacements) > 0 {
		debugf("    [DEBUG] Found %d stored replacements", len(replacements))
	}
	return preferred
}

func dedupeInts(in []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func ClearQueue(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, "/"+url.PathEscape(room)+"/clearqueue")
}

func AddSongToQueue(ctx context.Context, client *Client, room string, song ai.Song, playNow bool) QueueResult {
	candidates := searchITunesCandidates(ctx, song)
	if len(candidates) == 0 {
		debugf("    [DEBUG] No track ID found for %s - %s", song.Artist, song.Title)
		return QueueResult{Song: song, Success: false, Error: "Not found on iTunes"}
	}
	trackID := candidates[0]
	encodedRoom := url.PathEscape(room)
	endpoint := fmt.Sprintf("/%s/applemusic/queue/song:%d", encodedRoom, trackID)
	debugf("    [DEBUG] Queueing via: %s", endpoint)

	if err := client.RequestNoResponse(ctx, endpoint); err != nil {
		debugf("    [DEBUG] Queue request failed: %v", err)
		return QueueResult{Song: song, Success: false, Error: err.Error(), TrackID: trackID}
	}

	if playNow {
		if coordinatorUUID := getCoordinatorUUID(ctx, client, room); coordinatorUUID != "" {
			queueURI := url.QueryEscape("x-rincon-queue:" + coordinatorUUID + "#0")
			_ = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/setavtransporturi/%s", encodedRoom, queueURI))
		}
		_ = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/trackseek/1", encodedRoom))
		_ = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/play", encodedRoom))

		startedExpected := waitForExpectedTrackPlaying(ctx, client, room, song, 5*time.Second)
		if !startedExpected {
			debugf("    [DEBUG] Queue start verification failed, forcing now/play fallback")
			_ = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/applemusic/now/song:%d", encodedRoom, trackID))
			_ = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/play", encodedRoom))
			startedExpected = waitForExpectedTrackPlaying(ctx, client, room, song, 5*time.Second)
		}

		if !startedExpected {
			isExpectedTrack := waitForExpectedTrack(ctx, client, room, song, 1500*time.Millisecond)
			if isExpectedTrack {
				storage.BlockTrack(strconv.Itoa(trackID), song.Artist, song.Title, "")
				return QueueResult{
					Song:    song,
					Success: false,
					Error:   "Track appears unavailable (stays stopped)",
					TrackID: trackID,
				}
			}
		}
	}

	debugf("    [DEBUG] Queue request succeeded")
	return QueueResult{Song: song, Success: true, TrackID: trackID}
}

func AddAlternateSongToQueue(ctx context.Context, client *Client, room string, song ai.Song, triedTrackIDs map[int]struct{}) QueueResult {
	candidates := searchITunesCandidates(ctx, song)
	trackID := 0
	for _, id := range candidates {
		if _, ok := triedTrackIDs[id]; !ok {
			trackID = id
			break
		}
	}
	if trackID == 0 {
		return QueueResult{Song: song, Success: false, Error: "No alternate track found"}
	}
	encodedRoom := url.PathEscape(room)
	err := client.RequestNoResponse(ctx, fmt.Sprintf("/%s/applemusic/queue/song:%d", encodedRoom, trackID))
	if err != nil {
		return QueueResult{Song: song, Success: false, Error: err.Error(), TrackID: trackID}
	}
	return QueueResult{Song: song, Success: true, TrackID: trackID}
}

func StartPlayback(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, "/"+url.PathEscape(room)+"/play")
}

func PlayFromStart(ctx context.Context, client *Client, room string) error {
	encodedRoom := url.PathEscape(room)
	_ = client.RequestNoResponse(ctx, "/"+encodedRoom+"/trackseek/1")
	return client.RequestNoResponse(ctx, "/"+encodedRoom+"/play")
}
