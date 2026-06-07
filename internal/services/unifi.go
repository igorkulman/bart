package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/igorkulman/bart/internal/config"
)

type unifiClient struct {
	http *http.Client
}

var (
	unifiClients = map[string]*unifiClient{}
	unifiMu      sync.Mutex
)

func getUnifiClient(key string) *unifiClient {
	unifiMu.Lock()
	defer unifiMu.Unlock()
	if c, ok := unifiClients[key]; ok {
		return c
	}
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	c := &unifiClient{http: &http.Client{Jar: jar, Transport: transport, Timeout: 10 * time.Second}}
	unifiClients[key] = c
	return c
}

func FetchUnifi(item config.Item) ([]Badge, SubtitleHTML, error) {
	base := endpointFor(item)
	site := item.Site
	if site == "" {
		site = "default"
	}

	client := getUnifiClient(base)

	loginBody, _ := json.Marshal(map[string]string{
		"username": item.Username,
		"password": item.Password,
	})
	resp, err := client.http.Post(base+"/api/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		return nil, "", err
	}
	// Drain so the connection can be reused; the session cookie is in the headers.
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unifi login: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			NumSta int    `json:"num_sta"`
			Type   string `json:"type"`
		} `json:"data"`
	}
	devResp, err := client.http.Get(base + "/api/s/" + site + "/stat/device")
	if err != nil {
		return nil, "", err
	}
	defer devResp.Body.Close()
	if err := json.NewDecoder(devResp.Body).Decode(&result); err != nil {
		return nil, "", err
	}

	totalClients := 0
	accessPoints := 0
	for _, d := range result.Data {
		totalClients += d.NumSta
		if strings.ToLower(d.Type) == "uap" {
			accessPoints++
		}
	}
	otherDevices := len(result.Data) - accessPoints

	subtitle := SubtitleHTML(fmt.Sprintf(
		`<i class="fas fa-users"></i> %d  <i class="fas fa-wifi"></i> %d  <i class="fas fa-network-wired"></i> %d`,
		totalClients, accessPoints, otherDevices,
	))
	return nil, subtitle, nil
}
