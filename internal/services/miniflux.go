package services

import (
	"github.com/igorkulman/bart/internal/config"
)

func FetchMiniflux(item config.Item) ([]Badge, SubtitleHTML, error) {
	var counters struct {
		Unreads map[string]int `json:"unreads"`
	}
	err := getJSON(
		endpointFor(item)+"/v1/feeds/counters",
		map[string]string{"X-Auth-Token": item.ApiKey},
		&counters,
	)
	if err != nil {
		return nil, "", err
	}

	total := 0
	for _, v := range counters.Unreads {
		total += v
	}

	var badges []Badge
	if total > 0 {
		badges = append(badges, newBadge(total, "blue"))
	}
	return badges, "", nil
}
