package httpapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chriskanedev/energy-monitor-api/internal/energy"
)

type fakeProvider struct {
	snapshot energy.Snapshot
	ch       chan energy.Snapshot
}

func (f *fakeProvider) Latest() energy.Snapshot {
	return f.snapshot
}

func (f *fakeProvider) Subscribe() (<-chan energy.Snapshot, func()) {
	f.ch <- f.snapshot
	return f.ch, func() {}
}

func TestCurrentReturnsSnapshotWithCORS(t *testing.T) {
	provider := &fakeProvider{snapshot: energy.Snapshot{GridKw: 1.2}}
	server := httptest.NewServer(New(provider, []string{"http://localhost:5173"}).Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/energy/current", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET current error = %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("CORS header = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	var snapshot energy.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot error = %v", err)
	}
	if snapshot.GridKw != 1.2 {
		t.Fatalf("GridKw = %f", snapshot.GridKw)
	}
}

func TestStreamEmitsEnergyEvent(t *testing.T) {
	provider := &fakeProvider{
		snapshot: energy.Snapshot{GridKw: 1.2},
		ch:       make(chan energy.Snapshot, 1),
	}
	server := httptest.NewServer(New(provider, nil).Handler())
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/api/energy/stream")
	if err != nil {
		t.Fatalf("GET stream error = %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	lines := make([]string, 0, 2)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= 2 {
			break
		}
	}

	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "event: energy") {
		t.Fatalf("stream did not include energy event: %q", got)
	}
	if !strings.Contains(got, `"gridKw":1.2`) {
		t.Fatalf("stream did not include snapshot data: %q", got)
	}
}
