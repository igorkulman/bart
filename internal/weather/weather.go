package weather

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Config struct {
	APIKey    string
	Latitude  float64
	Longitude float64
	Units     string
}

type Data struct {
	Temp        string
	Description string
	IconCode    string // raw OWM code, e.g. "01d"
}

type Fetcher struct {
	cfg       Config
	mu        sync.Mutex
	cached    *Data
	fetchedAt time.Time
	client    *http.Client
}

func New(cfg Config) *Fetcher {
	return &Fetcher{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Fetcher) Current() (*Data, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cached != nil && time.Since(f.fetchedAt) < 15*time.Minute {
		return f.cached, nil
	}

	data, err := f.fetch()
	if err != nil {
		if f.cached != nil {
			return f.cached, nil
		}
		return nil, err
	}
	f.cached = data
	f.fetchedAt = time.Now()
	return data, nil
}

type owmResponse struct {
	Weather []struct {
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
	} `json:"main"`
	Name string `json:"name"`
	Sys  struct {
		Country string `json:"country"`
	} `json:"sys"`
}

func (f *Fetcher) fetch() (*Data, error) {
	units := f.cfg.Units
	if units == "" {
		units = "metric"
	}
	url := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/weather?lat=%f&lon=%f&appid=%s&units=%s",
		f.cfg.Latitude, f.cfg.Longitude, f.cfg.APIKey, units,
	)

	resp, err := f.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OWM API returned %d", resp.StatusCode)
	}

	var r owmResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}

	unit := "°C"
	if units == "imperial" {
		unit = "°F"
	} else if units == "standard" {
		unit = "K"
	}

	desc := ""
	iconCode := ""
	if len(r.Weather) > 0 {
		d := r.Weather[0].Description
		if d != "" {
			desc = strings.ToUpper(d[:1]) + d[1:]
		}
		iconCode = r.Weather[0].Icon
	}

	return &Data{
		Temp:        fmt.Sprintf("%d%s", int(r.Main.Temp), unit),
		Description: desc,
		IconCode:    iconCode,
	}, nil
}
