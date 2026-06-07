package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

type Client struct {
	http *http.Client
}

type DockerStatus struct {
	Running bool
	Known   bool
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	return &Client{http: &http.Client{Transport: transport}}
}

func (c *Client) ContainerStatus(name string) DockerStatus {
	resp, err := c.http.Get(fmt.Sprintf("http://localhost/containers/%s/json", name))
	if err != nil {
		return DockerStatus{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DockerStatus{}
	}

	var data struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return DockerStatus{}
	}
	return DockerStatus{Running: data.State.Running, Known: true}
}
