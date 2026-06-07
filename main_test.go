package main

import (
	"testing"

	"github.com/igorkulman/bart/internal/config"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Home":             "home",
		"Photos & Data":    "photos-data",
		"Plex!":            "plex",
		"  Uptime  Kuma ":  "uptime-kuma",
		"UniFi Controller": "unifi-controller",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIntervalSec(t *testing.T) {
	cases := map[int]int{0: 1, 500: 1, 999: 1, 1000: 1, 5000: 5, 30000: 30}
	for in, want := range cases {
		if got := intervalSec(in); got != want {
			t.Errorf("intervalSec(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestResolveGroupsUniqueSlugs(t *testing.T) {
	cfg := &config.Config{Services: []config.Group{
		{Name: "Media", Items: []config.Item{{Name: "Plex"}, {Name: "Plex"}}},
		{Name: "Media", Items: []config.Item{{Name: "Sonarr"}}},
	}}

	seen := map[string]bool{}
	count := 0
	for _, g := range resolveGroups(cfg) {
		for _, it := range g.Items {
			key := g.Slug + "/" + it.Slug
			if seen[key] {
				t.Errorf("duplicate tile slug %q", key)
			}
			seen[key] = true
			count++
		}
	}
	if count != 3 {
		t.Errorf("got %d tiles, want 3", count)
	}
}
