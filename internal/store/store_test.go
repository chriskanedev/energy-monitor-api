package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/energy"
)

func TestStoreInsertHistoryAndPrune(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	now := time.Now()
	snapshot := energy.Snapshot{
		SolarKw:          1.2,
		HouseKw:          2.3,
		HotWaterKw:       0.4,
		GridKw:           1.5,
		SolarCapacityPct: 20,
		UpdatedAt:        now.UnixMilli(),
	}
	readings := []energy.RawReading{{
		Role:      energy.RoleGrid,
		Device:    "first",
		Channel:   0,
		PowerW:    1500,
		Available: true,
		ReadAt:    now,
	}}

	if err := db.InsertPoll(context.Background(), snapshot, readings); err != nil {
		t.Fatalf("InsertPoll() error = %v", err)
	}

	home, grid, err := db.RecentHistory(context.Background(), 24)
	if err != nil {
		t.Fatalf("RecentHistory() error = %v", err)
	}
	if len(home) != 1 || home[0].Value != 2.3 {
		t.Fatalf("home history = %#v", home)
	}
	if len(grid) != 1 || grid[0].Value != 1.5 {
		t.Fatalf("grid history = %#v", grid)
	}

	if err := db.Prune(context.Background(), 30); err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
}
