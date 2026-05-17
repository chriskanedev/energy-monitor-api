package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/energy"
)

func TestLoadExpandsEnvAndValidatesRoles(t *testing.T) {
	t.Setenv("SERVER_ADDR", ":9090")
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "energy.db"))
	t.Setenv("SHELLY_FIRST_HOST", "192.168.1.173")
	t.Setenv("SHELLY_SECOND_HOST", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
pollInterval: 2s
requestTimeout: 1500ms
staleAfter: 10s
solarCapacityKw: 6.15
retentionDays: 30
server:
  addr: ${SERVER_ADDR}
  corsAllowedOrigins: ${CORS_ALLOWED_ORIGINS}
storage:
  sqlitePath: ${SQLITE_PATH}
devices:
  first:
    host: ${SHELLY_FIRST_HOST}
  second:
    host: ${SHELLY_SECOND_HOST}
roles:
  grid:
    device: first
    channel: 0
  solar:
    device: first
    channel: 1
  hotWater:
    device: second
    channel: 0
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Fatalf("Server.Addr = %q", cfg.Server.Addr)
	}
	if cfg.PollInterval != 2*time.Second {
		t.Fatalf("PollInterval = %s", cfg.PollInterval)
	}
	if cfg.RequestTimeout != 1500*time.Millisecond {
		t.Fatalf("RequestTimeout = %s", cfg.RequestTimeout)
	}
	if got := cfg.Roles[energy.RoleHotWater]; got.Device != "second" || got.Channel != 0 {
		t.Fatalf("hotWater role = %+v", got)
	}
	if cfg.Devices["second"].Host != "" {
		t.Fatalf("second Shelly host should be empty for degraded startup")
	}
	if len(cfg.Server.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins = %#v", cfg.Server.CORSAllowedOrigins)
	}
}

func TestLoadRejectsMissingRequiredRole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(`
pollInterval: 2s
requestTimeout: 1500ms
staleAfter: 10s
solarCapacityKw: 6.15
retentionDays: 30
server:
  addr: :8080
storage:
  sqlitePath: ./data/energy.db
devices:
  first:
    host: 192.168.1.173
roles:
  grid:
    device: first
    channel: 0
  solar:
    device: first
    channel: 1
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load() expected missing hotWater role error")
	}
}
