package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/igorkulman/bart/internal/config"
)

var (
	transmissionSessions = map[string]string{}
	transmissionMu       sync.Mutex
)

func FetchTransmission(item config.Item) ([]Badge, SubtitleHTML, error) {
	base := endpointFor(item)

	transmissionMu.Lock()
	sessionID := transmissionSessions[base]
	transmissionMu.Unlock()

	badges, err := fetchTransmissionStats(item, base, sessionID)
	return badges, "", err
}

func fetchTransmissionStats(item config.Item, base, sessionID string) ([]Badge, error) {
	body, _ := json.Marshal(map[string]string{"method": "session-stats"})
	req, err := http.NewRequest("POST", base+"/transmission/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-Transmission-Session-Id", sessionID)
	}
	if item.Username != "" {
		creds := base64.StdEncoding.EncodeToString([]byte(item.Username + ":" + item.Password))
		req.Header.Set("Authorization", "Basic "+creds)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		newID := resp.Header.Get("X-Transmission-Session-Id")
		transmissionMu.Lock()
		transmissionSessions[base] = newID
		transmissionMu.Unlock()
		return fetchTransmissionStats(item, base, newID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Arguments struct {
			ActiveTorrentCount int `json:"activeTorrentCount"`
		} `json:"arguments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var badges []Badge
	if result.Arguments.ActiveTorrentCount > 0 {
		badges = append(badges, newBadge(result.Arguments.ActiveTorrentCount, "blue"))
	}
	return badges, nil
}
