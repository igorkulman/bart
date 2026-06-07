package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/igorkulman/bart/internal/config"
)

func TestEndpointFor(t *testing.T) {
	tests := []struct {
		name string
		item config.Item
		want string
	}{
		{"apiUrl preferred over url", config.Item{URL: "http://u:1", ApiURL: "http://a:2"}, "http://a:2"},
		{"falls back to url", config.Item{URL: "http://u:1"}, "http://u:1"},
		{"trailing slash trimmed", config.Item{ApiURL: "http://a:2/"}, "http://a:2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := endpointFor(tt.item); got != tt.want {
				t.Errorf("endpointFor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchJellyfin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Sessions" {
			t.Errorf("requested %s, want /Sessions", r.URL.Path)
		}
		if r.Header.Get("X-Emby-Token") != "secret" {
			t.Errorf("missing/incorrect token header")
		}
		_, _ = w.Write([]byte(`[{"NowPlayingItem":{"Name":"x"}},{"NowPlayingItem":null}]`))
	}))
	defer srv.Close()

	badges, _, err := FetchJellyfin(config.Item{ApiURL: srv.URL, ApiKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if len(badges) != 1 || badges[0].Text != "1" {
		t.Errorf("badges = %+v, want a single badge with text \"1\"", badges)
	}
}

func TestFetchArr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" {
			t.Errorf("missing X-Api-Key header on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/api/v3/queue":
			_, _ = w.Write([]byte(`{"totalRecords":3}`))
		case "/api/v3/wanted/missing":
			_, _ = w.Write([]byte(`{"totalRecords":0}`))
		case "/api/v3/health":
			_, _ = w.Write([]byte(`[{"type":"warning"},{"type":"error"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	badges, _, err := FetchArr(config.Item{ApiURL: srv.URL, ApiKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	// queue=3 (blue), missing=0 (none), health: 1 error (red) + 1 warning (orange)
	if len(badges) != 3 {
		t.Fatalf("got %d badges, want 3: %+v", len(badges), badges)
	}
}
