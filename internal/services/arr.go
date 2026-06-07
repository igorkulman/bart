package services

import (
	"github.com/igorkulman/bart/internal/config"
)

func FetchArr(item config.Item) ([]Badge, SubtitleHTML, error) {
	base := endpointFor(item)
	headers := map[string]string{"X-Api-Key": item.ApiKey}

	var badges []Badge

	type countResp struct {
		TotalRecords int `json:"totalRecords"`
	}
	addCount := func(path, color string) {
		var r countResp
		if err := getJSON(base+path, headers, &r); err == nil && r.TotalRecords > 0 {
			badges = append(badges, newBadge(r.TotalRecords, color))
		}
	}
	addCount("/api/v3/queue", "blue")
	addCount("/api/v3/wanted/missing", "purple")

	var health []struct {
		Type string `json:"type"`
	}
	if err := getJSON(base+"/api/v3/health", headers, &health); err == nil {
		warnings, errors := 0, 0
		for _, h := range health {
			switch h.Type {
			case "warning":
				warnings++
			case "error":
				errors++
			}
		}
		if errors > 0 {
			badges = append(badges, newBadge(errors, "red"))
		}
		if warnings > 0 {
			badges = append(badges, newBadge(warnings, "orange"))
		}
	}

	return badges, "", nil
}
