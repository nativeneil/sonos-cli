package sonos

import (
	"context"
	"fmt"
	"sort"
)

type Zone struct {
	Coordinator struct {
		RoomName string `json:"roomName"`
		UUID     string `json:"uuid"`
	} `json:"coordinator"`
	Members []struct {
		RoomName string `json:"roomName"`
		UUID     string `json:"uuid"`
	} `json:"members"`
}

func DiscoverRooms(ctx context.Context, client *Client) ([]string, error) {
	var zones []Zone
	if err := client.RequestJSON(ctx, "/zones", &zones); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, zone := range zones {
		if zone.Coordinator.RoomName != "" {
			seen[zone.Coordinator.RoomName] = struct{}{}
		}
		for _, member := range zone.Members {
			if member.RoomName != "" {
				seen[member.RoomName] = struct{}{}
			}
		}
	}
	rooms := make([]string, 0, len(seen))
	for room := range seen {
		rooms = append(rooms, room)
	}
	sort.Strings(rooms)
	return rooms, nil
}

func DefaultRoom(ctx context.Context, client *Client) (string, error) {
	var zones []Zone
	if err := client.RequestJSON(ctx, "/zones", &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 || zones[0].Coordinator.RoomName == "" {
		return "", fmt.Errorf("no sonos speakers found")
	}
	return zones[0].Coordinator.RoomName, nil
}
