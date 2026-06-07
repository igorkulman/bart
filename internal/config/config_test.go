package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	yml := `title: Test
services:
  - name: Group
    items:
      - name: Item
`
	if err := os.WriteFile(p, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Columns != 3 {
		t.Errorf("Columns = %d, want 3", c.Columns)
	}
	if c.DockerSocket != "/var/run/docker.sock" {
		t.Errorf("DockerSocket = %q, want default socket", c.DockerSocket)
	}
	if got := c.Services[0].Items[0].UpdateIntervalMs; got != 30000 {
		t.Errorf("UpdateIntervalMs = %d, want 30000", got)
	}
}

func TestLoadExplicitValuesKept(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	yml := `title: Test
columns: 4
dockerSocket: /tmp/custom.sock
services:
  - name: Group
    items:
      - name: Item
        updateIntervalMs: 5000
`
	if err := os.WriteFile(p, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Columns != 4 {
		t.Errorf("Columns = %d, want 4", c.Columns)
	}
	if c.DockerSocket != "/tmp/custom.sock" {
		t.Errorf("DockerSocket = %q, want /tmp/custom.sock", c.DockerSocket)
	}
	if got := c.Services[0].Items[0].UpdateIntervalMs; got != 5000 {
		t.Errorf("UpdateIntervalMs = %d, want 5000", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml")); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
