package services

import (
	"fmt"

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
	feeds := 0
	for _, v := range counters.Unreads {
		total += v
		feeds++
	}

	var badges []Badge
	var subtitle SubtitleHTML
	if total > 0 {
		badges = append(badges, newBadge(total, "blue"))
		if feeds >= 2 {
			subtitle = SubtitleHTML(fmt.Sprintf("%d unread in %d feeds", total, feeds))
		} else {
			subtitle = SubtitleHTML(fmt.Sprintf("%d unread", total))
		}
	}
	return badges, subtitle, nil
}
