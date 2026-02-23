package sonos

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"sonos-playlist/internal/ai"
	"sonos-playlist/internal/storage"
)

type MonitorOptions struct {
	PollMS       int
	MinPlaySecs  int
	StallSecs    int
	OnUnplayable func(song ai.Song, trackID string) error
	OnStateLog   func(state any)
	UseTimer     bool
}

type TrackInfo struct {
	Song            *ai.Song
	PositionSeconds *int
	TrackID         string
	Album           string
}

type monitorState struct {
	CurrentTrack map[string]any `json:"currentTrack"`
	Currenttrack map[string]any `json:"currenttrack"`
	Position     any            `json:"position"`
}

func parseTimeToSeconds(v any) *int {
	switch t := v.(type) {
	case float64:
		i := int(t)
		return &i
	case int:
		i := t
		return &i
	case string:
		parts := strings.Split(t, ":")
		if len(parts) == 0 {
			return nil
		}
		seconds := 0
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil
			}
			seconds = seconds*60 + n
		}
		return &seconds
	default:
		return nil
	}
}

func extractTrack(raw map[string]any) TrackInfo {
	current, _ := raw["currentTrack"].(map[string]any)
	if len(current) == 0 {
		current, _ = raw["currenttrack"].(map[string]any)
	}
	title := firstString(current, "title", "trackName", "name", "track")
	artist := firstString(current, "artist", "artistName", "creator", "albumArtist")
	album, _ := current["album"].(string)
	if title == "" || artist == "" {
		return TrackInfo{}
	}
	position := parseTimeToSeconds(firstAny(current["position"], current["positionSeconds"], raw["position"]))
	uri := firstString(current, "uri", "trackUri", "albumArtUri")
	trackID := storage.ExtractTrackIDFromURI(uri)
	song := ai.Song{Title: title, Artist: artist}
	return TrackInfo{Song: &song, PositionSeconds: position, TrackID: trackID, Album: album}
}

func firstAny(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func songKey(song ai.Song) string {
	return strings.ToLower(song.Artist) + ":::" + strings.ToLower(song.Title)
}

type MonitorHandle struct {
	Stop func()
	Done <-chan struct{}
}

func StartPlaybackMonitor(ctx context.Context, client *Client, room string, options MonitorOptions) MonitorHandle {
	pollMS := options.PollMS
	if pollMS == 0 {
		pollMS = 4000
	}
	stallMS := options.StallSecs * 1000
	if stallMS == 0 {
		stallMS = 10000
	}
	encodedRoom := url.PathEscape(room)

	var stopped atomic.Bool
	done := make(chan struct{})

	go func() {
		defer close(done)
		var lastSong *ai.Song
		lastSongKey := ""
		lastTrackID := ""
		lastAlbum := ""
		var lastPosition *int
		lastPositionAt := time.Now()
		startedPlaying := false
		sawStall := false

		for !stopped.Load() {
			if options.UseTimer && !storage.ShouldMonitorContinue() {
				break
			}
			var raw map[string]any
			err := client.RequestJSON(ctx, "/"+encodedRoom+"/state", &raw)
			if err != nil {
				time.Sleep(time.Duration(pollMS) * time.Millisecond)
				continue
			}
			if options.OnStateLog != nil {
				options.OnStateLog(raw)
			}
			track := extractTrack(raw)
			if track.Song == nil {
				time.Sleep(time.Duration(pollMS) * time.Millisecond)
				continue
			}
			key := songKey(*track.Song)
			now := time.Now()

			if lastSong == nil {
				lastSong = track.Song
				lastSongKey = key
				lastTrackID = track.TrackID
				lastAlbum = track.Album
				lastPosition = track.PositionSeconds
				lastPositionAt = now
				startedPlaying = track.PositionSeconds != nil && *track.PositionSeconds >= 3
				sawStall = false
				time.Sleep(time.Duration(pollMS) * time.Millisecond)
				continue
			}

			if key != lastSongKey {
				if !startedPlaying || sawStall {
					if lastTrackID != "" {
						storage.BlockTrack(lastTrackID, lastSong.Artist, lastSong.Title, lastAlbum)
					}
					if options.OnUnplayable != nil {
						_ = options.OnUnplayable(*lastSong, lastTrackID)
					}
				}
				lastSong = track.Song
				lastSongKey = key
				lastTrackID = track.TrackID
				lastAlbum = track.Album
				lastPosition = track.PositionSeconds
				lastPositionAt = now
				startedPlaying = track.PositionSeconds != nil && *track.PositionSeconds >= 3
				sawStall = false
				time.Sleep(time.Duration(pollMS) * time.Millisecond)
				continue
			}

			if track.PositionSeconds != nil {
				if lastPosition != nil && *track.PositionSeconds == *lastPosition {
					if now.Sub(lastPositionAt) > time.Duration(stallMS)*time.Millisecond {
						sawStall = true
					}
				} else {
					if *track.PositionSeconds >= 3 {
						startedPlaying = true
					}
					lastPosition = track.PositionSeconds
					lastPositionAt = now
				}
			}

			time.Sleep(time.Duration(pollMS) * time.Millisecond)
		}
	}()

	return MonitorHandle{
		Stop: func() { stopped.Store(true) },
		Done: done,
	}
}
