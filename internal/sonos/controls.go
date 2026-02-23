package sonos

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type playerState struct {
	Volume   int `json:"volume"`
	PlayMode struct {
		Repeat  string `json:"repeat"`
		Shuffle bool   `json:"shuffle"`
	} `json:"playMode"`
}

func getState(ctx context.Context, client *Client, room string) (playerState, error) {
	var state playerState
	err := client.RequestJSON(ctx, "/"+url.PathEscape(room)+"/state", &state)
	return state, err
}

func VolumeUp(ctx context.Context, client *Client, room string) (int, error) {
	state, err := getState(ctx, client, room)
	if err != nil {
		return 0, err
	}
	current := state.Volume
	newVolume := 0
	if current%5 == 0 {
		newVolume = min(100, current+5)
	} else {
		newVolume = min(100, ((current/5)+1)*5)
	}
	err = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/volume/%d", url.PathEscape(room), newVolume))
	return newVolume, err
}

func VolumeDown(ctx context.Context, client *Client, room string) (int, error) {
	state, err := getState(ctx, client, room)
	if err != nil {
		return 0, err
	}
	current := state.Volume
	newVolume := 0
	if current%5 == 0 {
		newVolume = max(0, current-5)
	} else {
		newVolume = max(0, (current/5)*5)
	}
	err = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/volume/%d", url.PathEscape(room), newVolume))
	return newVolume, err
}

func VolumeSet(ctx context.Context, client *Client, room string, level int) (int, error) {
	newVolume := max(0, min(100, level))
	err := client.RequestNoResponse(ctx, fmt.Sprintf("/%s/volume/%d", url.PathEscape(room), newVolume))
	return newVolume, err
}

func VolumeHigh(ctx context.Context, client *Client, room string) (int, error) {
	return VolumeSet(ctx, client, room, 60)
}

func VolumeLow(ctx context.Context, client *Client, room string) (int, error) {
	return VolumeSet(ctx, client, room, 10)
}

func GetVolume(ctx context.Context, client *Client, room string) (int, error) {
	state, err := getState(ctx, client, room)
	if err != nil {
		return 0, err
	}
	return state.Volume, nil
}

func Play(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, fmt.Sprintf("/%s/play", url.PathEscape(room)))
}

func Pause(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, fmt.Sprintf("/%s/pause", url.PathEscape(room)))
}

func Skip(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, fmt.Sprintf("/%s/next", url.PathEscape(room)))
}

func Previous(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, fmt.Sprintf("/%s/previous", url.PathEscape(room)))
}

func Restart(ctx context.Context, client *Client, room string) error {
	return client.RequestNoResponse(ctx, fmt.Sprintf("/%s/seek/0", url.PathEscape(room)))
}

func Repeat(ctx context.Context, client *Client, room string, mode string) (string, error) {
	encodedRoom := url.PathEscape(room)
	if mode != "" {
		err := client.RequestNoResponse(ctx, fmt.Sprintf("/%s/repeat/%s", encodedRoom, mode))
		return mode, err
	}
	state, err := getState(ctx, client, room)
	if err != nil {
		return "", err
	}
	current := strings.ToLower(state.PlayMode.Repeat)
	next := "none"
	switch current {
	case "none":
		next = "all"
	case "all":
		next = "one"
	default:
		next = "none"
	}
	err = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/repeat/%s", encodedRoom, next))
	return next, err
}

func Shuffle(ctx context.Context, client *Client, room string, enabled *bool) (bool, error) {
	encodedRoom := url.PathEscape(room)
	if enabled != nil {
		val := "off"
		if *enabled {
			val = "on"
		}
		err := client.RequestNoResponse(ctx, fmt.Sprintf("/%s/shuffle/%s", encodedRoom, val))
		return *enabled, err
	}
	state, err := getState(ctx, client, room)
	if err != nil {
		return false, err
	}
	newState := !state.PlayMode.Shuffle
	val := "off"
	if newState {
		val = "on"
	}
	err = client.RequestNoResponse(ctx, fmt.Sprintf("/%s/shuffle/%s", encodedRoom, val))
	return newState, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
