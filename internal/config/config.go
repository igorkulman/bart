package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type SysInfoConfig struct {
	Enabled bool   `yaml:"enabled"`
	Disk    string `yaml:"disk"`
}

type WeatherConfig struct {
	Enabled   bool    `yaml:"enabled"`
	APIKey    string  `yaml:"apiKey"`
	Latitude  float64 `yaml:"latitude"`
	Longitude float64 `yaml:"longitude"`
	Units     string  `yaml:"units"` // metric, imperial, standard
}

type Config struct {
	Title        string        `yaml:"title"`
	Columns      int           `yaml:"columns"`
	DockerSocket string        `yaml:"dockerSocket"`
	SysInfo      SysInfoConfig `yaml:"sysInfo"`
	Weather      WeatherConfig `yaml:"weather"`
	Services     []Group       `yaml:"services"`
}

type Group struct {
	Name  string `yaml:"name"`
	Icon  string `yaml:"icon"`
	Items []Item `yaml:"items"`
}

type Item struct {
	Name             string   `yaml:"name"`
	Type             string   `yaml:"type"`
	URL              string   `yaml:"url"`
	ApiURL           string   `yaml:"apiUrl"`
	Logo             string   `yaml:"logo"`
	Icon             string   `yaml:"icon"`
	ApiKey           string   `yaml:"apikey"`
	Username         string   `yaml:"username"`
	Password         string   `yaml:"password"`
	Container        string   `yaml:"container"`
	UpdateIntervalMs int      `yaml:"updateIntervalMs"`
	Target           string   `yaml:"target"`
	Slug             string   `yaml:"slug"`
	Sensors          []Sensor `yaml:"sensors"`
	Site             string   `yaml:"site"`
	Style            string   `yaml:"style"`
	Subtitle         string   `yaml:"subtitle"`
	ShowUnits        *bool    `yaml:"showUnits"`
}

type Sensor struct {
	ID   string `yaml:"id"`
	Icon string `yaml:"icon"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Columns == 0 {
		cfg.Columns = 3
	}
	if cfg.DockerSocket == "" {
		cfg.DockerSocket = "/var/run/docker.sock"
	}
	if cfg.SysInfo.Disk == "" {
		cfg.SysInfo.Disk = "/"
	}
	if cfg.Weather.Units == "" {
		cfg.Weather.Units = "metric"
	}
	for i := range cfg.Services {
		for j := range cfg.Services[i].Items {
			if cfg.Services[i].Items[j].UpdateIntervalMs == 0 {
				cfg.Services[i].Items[j].UpdateIntervalMs = 30000
			}
		}
	}
	return &cfg, nil
}
