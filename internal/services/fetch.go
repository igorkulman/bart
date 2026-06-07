package services

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/igorkulman/bart/internal/config"
)

var httpClient = &http.Client{}

type Badge struct {
	Text  string
	Color string
}

// SubtitleHTML is safe HTML for template rendering (may contain <i> tags for icons).
type SubtitleHTML = template.HTML

func newBadge(n int, color string) Badge {
	return Badge{Text: strconv.Itoa(n), Color: color}
}

func endpointFor(item config.Item) string {
	u := item.ApiURL
	if u == "" {
		u = item.URL
	}
	return strings.TrimRight(u, "/")
}

func getJSON(url string, headers map[string]string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
