package services

import (
	"github.com/igorkulman/bart/internal/config"
)

func FetchJellyfin(item config.Item) ([]Badge, SubtitleHTML, error) {
	var sessions []struct {
		NowPlayingItem interface{} `json:"NowPlayingItem"`
	}
	err := getJSON(
		endpointFor(item)+"/Sessions",
		map[string]string{"X-Emby-Token": item.ApiKey},
		&sessions,
	)
	if err != nil {
		return nil, "", err
	}

	count := 0
	for _, s := range sessions {
		if s.NowPlayingItem != nil {
			count++
		}
	}

	var badges []Badge
	if count > 0 {
		badges = append(badges, newBadge(count, "blue"))
	}
	return badges, "", nil
}
