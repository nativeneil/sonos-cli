package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var storageDir string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		storageDir = "."
		return
	}
	storageDir = filepath.Join(home, ".sonos-playlist")
}

func ensureStorageDir() {
	_ = os.MkdirAll(storageDir, 0o755)
}

type BlockedTrack struct {
	TrackID            string `json:"trackId"`
	Artist             string `json:"artist"`
	Title              string `json:"title"`
	Album              string `json:"album,omitempty"`
	ReplacementTrackID string `json:"replacementTrackId,omitempty"`
	FailedAt           string `json:"failedAt"`
}

func blocklistFile() string { return filepath.Join(storageDir, "blocklist.json") }

func GetBlocklist() map[string]BlockedTrack {
	file := blocklistFile()
	b, err := os.ReadFile(file)
	if err != nil {
		return map[string]BlockedTrack{}
	}
	var out map[string]BlockedTrack
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]BlockedTrack{}
	}
	if out == nil {
		return map[string]BlockedTrack{}
	}
	return out
}

func IsTrackBlocked(trackID string) bool {
	_, ok := GetBlocklist()[trackID]
	return ok
}

func BlockTrack(trackID, artist, title, album string) {
	ensureStorageDir()
	b := GetBlocklist()
	b[trackID] = BlockedTrack{
		TrackID:  trackID,
		Artist:   artist,
		Title:    title,
		Album:    album,
		FailedAt: time.Now().UTC().Format(time.RFC3339),
	}
	persistBlocklist(b)
}

func SetTrackReplacement(trackID, replacementTrackID string) {
	ensureStorageDir()
	b := GetBlocklist()
	item, ok := b[trackID]
	if !ok {
		return
	}
	item.ReplacementTrackID = replacementTrackID
	b[trackID] = item
	persistBlocklist(b)
}

func persistBlocklist(data map[string]BlockedTrack) {
	buf, _ := json.MarshalIndent(data, "", "  ")
	_ = os.WriteFile(blocklistFile(), buf, 0o644)
}

func GetSongReplacementTrackIDs(artist, title string) []int {
	wantArtist := strings.ToLower(strings.TrimSpace(artist))
	wantTitle := strings.ToLower(strings.TrimSpace(title))
	seen := map[int]struct{}{}
	for _, blocked := range GetBlocklist() {
		if blocked.ReplacementTrackID == "" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(blocked.Artist)) == wantArtist &&
			strings.ToLower(strings.TrimSpace(blocked.Title)) == wantTitle {
			id, err := strconv.Atoi(blocked.ReplacementTrackID)
			if err == nil {
				seen[id] = struct{}{}
			}
		}
	}
	out := make([]int, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func GetBlockedTrackIDs() map[int]struct{} {
	ids := map[int]struct{}{}
	for key := range GetBlocklist() {
		id, err := strconv.Atoi(key)
		if err == nil {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func monitorUntilFile() string { return filepath.Join(storageDir, "monitor-until.txt") }

func SetMonitorUntil(ts int64) {
	ensureStorageDir()
	_ = os.WriteFile(monitorUntilFile(), []byte(strconv.FormatInt(ts, 10)), 0o644)
}

func GetMonitorUntil() *int64 {
	b, err := os.ReadFile(monitorUntilFile())
	if err != nil {
		return nil
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return nil
	}
	return &ts
}

func ExtendMonitorBy(ms int64) {
	now := time.Now().UnixMilli()
	base := now
	if current := GetMonitorUntil(); current != nil && *current > now {
		base = *current
	}
	SetMonitorUntil(base + ms)
}

func ShouldMonitorContinue() bool {
	until := GetMonitorUntil()
	if until == nil {
		return false
	}
	return time.Now().UnixMilli() < *until
}

var trackIDRe = regexp.MustCompile(`song[:%]3[Aa]?(\d+)`)

func ExtractTrackIDFromURI(uri string) string {
	m := trackIDRe.FindStringSubmatch(uri)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
