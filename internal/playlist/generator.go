package playlist

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sonos-playlist/internal/ai"
	"sonos-playlist/internal/output"
	"sonos-playlist/internal/sonos"
	"sonos-playlist/internal/storage"
)

type GeneratorOptions struct {
	Keys             ai.APIKeys
	Prompt           string
	Room             string
	DryRun           bool
	Client           *sonos.Client
	Monitor          bool
	CountPerProvider int
	Output           *output.Output
}

type Result struct {
	Room            string `json:"room"`
	DryRun          bool   `json:"dryRun"`
	QueuedSongs     int    `json:"queuedSongs"`
	FailedSongs     int    `json:"failedSongs"`
	PlaybackStarted bool   `json:"playbackStarted"`
	Monitored       bool   `json:"monitored"`
}

const (
	minSongs         = 30
	maxRetries       = 5
	altRetryCooldown = 30 * time.Second
	fastStartTimeout = 10 * time.Second
)

func songKey(song ai.Song) string {
	return strings.ToLower(song.Artist) + ":::" + strings.ToLower(song.Title)
}

func generateFastStartSongs(ctx context.Context, keys ai.APIKeys, prompt string) []ai.Song {
	if keys.Google == "" {
		return []ai.Song{}
	}
	fctx, cancel := context.WithTimeout(ctx, fastStartTimeout)
	defer cancel()
	songs, err := ai.GeneratePlaylistGemini(fctx, keys.Google, prompt, 3)
	if err != nil {
		return []ai.Song{}
	}
	return songs
}

func GenerateAndPlay(ctx context.Context, options GeneratorOptions) (Result, error) {
	keys := options.Keys
	prompt := options.Prompt
	room := options.Room
	dryRun := options.DryRun
	client := options.Client
	monitor := options.Monitor
	countPerProvider := options.CountPerProvider
	out := options.Output

	playbackStarted := false
	var fastStartSong *ai.Song
	existingKeys := map[string]struct{}{}
	triedTrackIDsBySong := map[string]map[int]struct{}{}
	lastAltAttemptAt := map[string]time.Time{}
	var monitorHandle *sonos.MonitorHandle

	handleUnplayable := func(song ai.Song, trackID string) error {
		key := songKey(song)
		if trackID != "" {
			out.Warn(fmt.Sprintf("Detected unplayable track: %s - %s (ID: %s, now blocked)", song.Artist, song.Title, trackID))
		} else {
			out.Warn(fmt.Sprintf("Detected unplayable track: %s - %s", song.Artist, song.Title))
		}
		now := time.Now()
		if last, ok := lastAltAttemptAt[key]; ok && now.Sub(last) < altRetryCooldown {
			return nil
		}
		lastAltAttemptAt[key] = now

		tried, ok := triedTrackIDsBySong[key]
		if !ok {
			tried = map[int]struct{}{}
			triedTrackIDsBySong[key] = tried
		}
		if trackID != "" {
			if id, err := strconv.Atoi(trackID); err == nil {
				tried[id] = struct{}{}
			}
		}

		alt := sonos.AddAlternateSongToQueue(ctx, client, room, song, tried)
		if alt.Success && alt.TrackID != 0 {
			tried[alt.TrackID] = struct{}{}
			if trackID != "" {
				storage.SetTrackReplacement(trackID, strconv.Itoa(alt.TrackID))
			}
			out.Warn(fmt.Sprintf("Unavailable track replaced with alternate: %s - %s", song.Artist, song.Title))
		} else {
			out.Warn(fmt.Sprintf("No alternate found for: %s - %s", song.Artist, song.Title))
		}
		return nil
	}

	ensureMonitor := func() {
		if !monitor || monitorHandle != nil {
			return
		}
		h := sonos.StartPlaybackMonitor(ctx, client, room, sonos.MonitorOptions{
			OnUnplayable: handleUnplayable,
			UseTimer:     false,
		})
		monitorHandle = &h
		out.Info(out.Gray("Monitoring playback for unavailable tracks (Ctrl+C to stop)..."))
	}

	var songs []ai.RankedSong
	if !dryRun {
		out.Info("Generating fast-start songs (Gemini 3 Flash)...")
		fastStartCandidates := generateFastStartSongs(ctx, keys, prompt)

		if err := sonos.Pause(ctx, client, room); err != nil {
			// ignore pause failures
		}
		if err := sonos.ClearQueue(ctx, client, room); err != nil {
			return Result{}, err
		}

		if len(fastStartCandidates) == 0 {
			out.Warn("Fast-start unavailable; continuing")
		} else {
			for _, candidate := range fastStartCandidates {
				result := sonos.AddSongToQueue(ctx, client, room, candidate, true)
				if !result.Success {
					continue
				}
				key := songKey(candidate)
				if result.TrackID != 0 {
					tried := triedTrackIDsBySong[key]
					if tried == nil {
						tried = map[int]struct{}{}
						triedTrackIDsBySong[key] = tried
					}
					tried[result.TrackID] = struct{}{}
				}
				existingKeys[key] = struct{}{}
				playbackStarted = true
				c := candidate
				fastStartSong = &c
				out.Success(fmt.Sprintf("  -> Now playing: %s - %s", candidate.Artist, candidate.Title))
				ensureMonitor()
				break
			}
			if !playbackStarted {
				out.Warn("Fast-start songs queued but none were playable")
			}
		}
	}

	out.Info(fmt.Sprintf("Generating playlist from all providers for: %q", prompt))
	ranked, err := ai.GeneratePlaylistAllProviders(ctx, keys, prompt, countPerProvider)
	if err != nil {
		return Result{}, err
	}
	songs = ranked
	out.Success(fmt.Sprintf("Generated %d unique songs from all providers", len(songs)))

	out.Print(out.Bold("Initial playlist:"))
	for i, song := range songs {
		voteIndicator := ""
		if song.Votes > 1 {
			voteIndicator = out.Yellow(fmt.Sprintf(" [%d]", song.Votes))
		}
		out.Print(fmt.Sprintf("  %d. %s - %s%s", i+1, song.Artist, song.Title, voteIndicator))
	}
	out.Print("")

	if dryRun {
		out.Warn("Dry run - not queueing or playing")
		return Result{Room: room, DryRun: true, QueuedSongs: 0, FailedSongs: 0, PlaybackStarted: false, Monitored: false}, nil
	}

	if playbackStarted {
		out.Info("Fast-start playing; appending generated songs to the queue...")
	}

	queuedSongs := []ai.Song{}
	failedSongs := []ai.Song{}
	retryCount := 0

	for len(queuedSongs) < minSongs && retryCount < maxRetries {
		if retryCount > 0 {
			out.Info(fmt.Sprintf("Generating more songs (attempt %d)...", retryCount+1))
			existing := make([]ai.Song, 0, len(queuedSongs)+len(failedSongs)+1)
			existing = append(existing, queuedSongs...)
			existing = append(existing, failedSongs...)
			if fastStartSong != nil {
				existing = append(existing, *fastStartSong)
			}
			moreSongs, err := ai.GenerateMoreSongs(ctx, keys, prompt, existing, countPerProvider)
			if err != nil {
				return Result{}, err
			}
			if len(moreSongs) == 0 {
				out.Warn("Could not generate more unique songs")
				break
			}
			songs = make([]ai.RankedSong, 0, len(moreSongs))
			for _, s := range moreSongs {
				songs = append(songs, ai.RankedSong{Song: s, Votes: 1})
			}
			out.Success(fmt.Sprintf("Generated %d more songs", len(songs)))
		}

		out.Info("Adding songs to queue...")
		for _, song := range songs {
			if len(queuedSongs) >= minSongs {
				break
			}
			key := songKey(song.Song)
			if _, ok := existingKeys[key]; ok {
				continue
			}
			out.Write(out.Gray(fmt.Sprintf("  Searching: %s - %s... ", song.Artist, song.Title)))
			shouldPlayNow := !playbackStarted
			result := sonos.AddSongToQueue(ctx, client, room, song.Song, shouldPlayNow)
			if result.Success {
				out.Print(out.Green("found"))
				queuedSongs = append(queuedSongs, song.Song)
				existingKeys[key] = struct{}{}
				if result.TrackID != 0 {
					tried := triedTrackIDsBySong[key]
					if tried == nil {
						tried = map[int]struct{}{}
						triedTrackIDsBySong[key] = tried
					}
					tried[result.TrackID] = struct{}{}
				}
				if shouldPlayNow {
					playbackStarted = true
					out.Success("  -> Playback started!")
					ensureMonitor()
				}
			} else {
				out.Print(out.Red("not found"))
				failedSongs = append(failedSongs, song.Song)
			}
		}
		retryCount++
	}

	out.Print("")
	out.Print(out.Bold(fmt.Sprintf("Queued %d songs on %s", len(queuedSongs), room)))
	if len(failedSongs) > 0 {
		out.Warn(fmt.Sprintf("%d songs not found on Apple Music", len(failedSongs)))
	}
	if !playbackStarted {
		out.Warn("No songs were queued, nothing to play")
	}

	if !monitor {
		out.Info(out.Gray("Queueing complete. Exiting now (use --monitor to keep running)."))
		return Result{
			Room: room, DryRun: false, QueuedSongs: len(queuedSongs), FailedSongs: len(failedSongs),
			PlaybackStarted: playbackStarted, Monitored: false,
		}, nil
	}

	if monitorHandle != nil {
		<-monitorHandle.Done
	}
	return Result{
		Room: room, DryRun: false, QueuedSongs: len(queuedSongs), FailedSongs: len(failedSongs),
		PlaybackStarted: playbackStarted, Monitored: true,
	}, nil
}
