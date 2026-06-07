package services

import (
	"github.com/igorkulman/bart/internal/config"
)

func FetchUptimeKuma(item config.Item) ([]Badge, SubtitleHTML, error) {
	slug := item.Slug
	if slug == "" {
		slug = "default"
	}

	var result struct {
		HeartbeatList map[string][]struct {
			Status int `json:"status"`
		} `json:"heartbeatList"`
	}
	err := getJSON(
		endpointFor(item)+"/api/status-page/heartbeat/"+slug,
		nil,
		&result,
	)
	if err != nil {
		return nil, "", err
	}

	down := 0
	for _, beats := range result.HeartbeatList {
		if len(beats) > 0 && beats[len(beats)-1].Status != 1 {
			down++
		}
	}

	var badges []Badge
	if down > 0 {
		badges = append(badges, newBadge(down, "red"))
	}
	return badges, "", nil
}
