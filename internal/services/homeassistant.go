package services

import (
	"fmt"
	"html/template"
	"strings"
	"sync"

	"github.com/igorkulman/bart/internal/config"
)

func FetchHomeAssistant(item config.Item) ([]Badge, SubtitleHTML, error) {
	base := endpointFor(item)
	headers := map[string]string{
		"Authorization": "Bearer " + item.ApiKey,
	}

	type sensorResult struct {
		icon  string
		value string
	}

	results := make([]sensorResult, len(item.Sensors))
	var wg sync.WaitGroup
	for i, sensor := range item.Sensors {
		wg.Add(1)
		go func(idx int, s config.Sensor) {
			defer wg.Done()
			var state struct {
				State      string `json:"state"`
				Attributes struct {
					UnitOfMeasurement string `json:"unit_of_measurement"`
				} `json:"attributes"`
			}
			if err := getJSON(base+"/api/states/"+s.ID, headers, &state); err != nil {
				return
			}
			unit := state.Attributes.UnitOfMeasurement
			if item.ShowUnits != nil && !*item.ShowUnits {
				unit = ""
			}
			results[idx] = sensorResult{icon: s.Icon, value: fmt.Sprintf("%s%s", state.State, unit)}
		}(i, sensor)
	}
	wg.Wait()

	var parts []string
	for _, r := range results {
		if r.value == "" {
			continue
		}
		if r.icon != "" {
			parts = append(parts, fmt.Sprintf(`<i class="%s"></i> %s`,
				template.HTMLEscapeString(r.icon), template.HTMLEscapeString(r.value)))
		} else {
			parts = append(parts, template.HTMLEscapeString(r.value))
		}
	}

	return nil, SubtitleHTML(strings.Join(parts, "  ")), nil
}
