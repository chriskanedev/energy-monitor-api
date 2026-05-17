package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/energy"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS raw_readings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	observed_at TEXT NOT NULL,
	role TEXT NOT NULL,
	device TEXT NOT NULL,
	channel INTEGER NOT NULL,
	power_w REAL NOT NULL,
	reactive_var REAL NOT NULL,
	voltage_v REAL NOT NULL,
	power_factor REAL NOT NULL,
	total_kwh REAL NOT NULL,
	total_returned_kwh REAL NOT NULL,
	available INTEGER NOT NULL,
	stale INTEGER NOT NULL,
	error TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_raw_readings_observed_at ON raw_readings(observed_at);

CREATE TABLE IF NOT EXISTS snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	observed_at TEXT NOT NULL,
	updated_at_ms INTEGER NOT NULL,
	solar_kw REAL NOT NULL,
	house_kw REAL NOT NULL,
	hot_water_kw REAL NOT NULL,
	grid_kw REAL NOT NULL,
	solar_capacity_pct INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_observed_at ON snapshots(observed_at);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	return nil
}

func (s *Store) InsertPoll(ctx context.Context, snapshot energy.Snapshot, readings []energy.RawReading) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	observedAt := time.UnixMilli(snapshot.UpdatedAt).UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `
INSERT INTO snapshots (observed_at, updated_at_ms, solar_kw, house_kw, hot_water_kw, grid_kw, solar_capacity_pct)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		observedAt,
		snapshot.UpdatedAt,
		snapshot.SolarKw,
		snapshot.HouseKw,
		snapshot.HotWaterKw,
		snapshot.GridKw,
		snapshot.SolarCapacityPct,
	); err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO raw_readings (
	observed_at, role, device, channel, power_w, reactive_var, voltage_v, power_factor,
	total_kwh, total_returned_kwh, available, stale, error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare raw reading insert: %w", err)
	}
	defer stmt.Close()

	for _, reading := range readings {
		readAt := reading.ReadAt
		if readAt.IsZero() {
			readAt = time.UnixMilli(snapshot.UpdatedAt)
		}
		if _, err = stmt.ExecContext(ctx,
			readAt.UTC().Format(time.RFC3339Nano),
			string(reading.Role),
			reading.Device,
			reading.Channel,
			reading.PowerW,
			reading.ReactiveVAR,
			reading.VoltageV,
			reading.PowerFactor,
			reading.TotalKWh,
			reading.TotalReturnedKWh,
			boolInt(reading.Available),
			boolInt(reading.Stale),
			reading.Error,
		); err != nil {
			return fmt.Errorf("insert raw reading %s: %w", reading.Role, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit poll: %w", err)
	}

	return nil
}

func (s *Store) RecentHistory(ctx context.Context, limit int) ([]energy.Point, []energy.Point, error) {
	if limit <= 0 {
		limit = 24
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT updated_at_ms, house_kw, ABS(grid_kw)
FROM (
	SELECT updated_at_ms, house_kw, grid_kw
	FROM snapshots
	ORDER BY updated_at_ms DESC
	LIMIT ?
)
ORDER BY updated_at_ms ASC`, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("query recent history: %w", err)
	}
	defer rows.Close()

	home := make([]energy.Point, 0, limit)
	grid := make([]energy.Point, 0, limit)
	for rows.Next() {
		var ts int64
		var houseKw float64
		var gridKw float64
		if err := rows.Scan(&ts, &houseKw, &gridKw); err != nil {
			return nil, nil, fmt.Errorf("scan recent history: %w", err)
		}
		home = append(home, energy.Point{Time: ts, Value: houseKw})
		grid = append(grid, energy.Point{Time: ts, Value: gridKw})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate recent history: %w", err)
	}

	return home, grid, nil
}

func (s *Store) Prune(ctx context.Context, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM raw_readings WHERE observed_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune raw readings: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM snapshots WHERE observed_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune snapshots: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
