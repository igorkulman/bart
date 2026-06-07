package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Client struct {
	http *http.Client
}

type Status struct {
	Running bool
	Known   bool
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{http: &http.Client{Transport: transport, Timeout: 5 * time.Second}}
}

func (c *Client) ContainerStatus(name string) Status {
	resp, err := c.http.Get(fmt.Sprintf("http://localhost/containers/%s/json", name))
	if err != nil {
		return Status{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Status{}
	}

	var data struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Status{}
	}
	return Status{Running: data.State.Running, Known: true}
}
