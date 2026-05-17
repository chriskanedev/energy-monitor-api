package poller

import (
	"context"
	"testing"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/config"
	"github.com/chriskanedev/energy-monitor-api/internal/energy"
	"github.com/chriskanedev/energy-monitor-api/internal/shelly"
)

type fakeShellyClient struct {
	statuses map[string]shelly.Status
	errs     map[string]error
}

func (f fakeShellyClient) FetchStatus(_ context.Context, host string) (shelly.Status, error) {
	if err := f.errs[host]; err != nil {
		return shelly.Status{}, err
	}
	return f.statuses[host], nil
}

func TestPollerBuildsSnapshotAndToleratesMissingSecondShelly(t *testing.T) {
	cfg := config.Config{
		PollInterval:    time.Second,
		RequestTimeout:  time.Second,
		StaleAfter:      10 * time.Second,
		SolarCapacityKw: 6.15,
		RetentionDays:   30,
		Devices: map[string]config.DeviceConfig{
			"first":  {Host: "first-host"},
			"second": {Host: ""},
		},
		Roles: map[energy.Role]config.RoleConfig{
			energy.RoleGrid:     {Device: "first", Channel: 0},
			energy.RoleSolar:    {Device: "first", Channel: 1},
			energy.RoleHotWater: {Device: "second", Channel: 0},
		},
	}
	client := fakeShellyClient{statuses: map[string]shelly.Status{
		"first-host": {
			EMeters: []shelly.EMeter{
				{Power: 2500, IsValid: true},
				{Power: 4300, IsValid: true},
			},
		},
	}}

	p := New(cfg, client, nil, nil)
	p.poll(context.Background())
	snapshot := p.Latest()

	if snapshot.GridKw != 2.5 {
		t.Fatalf("GridKw = %f", snapshot.GridKw)
	}
	if snapshot.SolarKw != 4.3 {
		t.Fatalf("SolarKw = %f", snapshot.SolarKw)
	}
	if snapshot.HotWaterKw != 0 {
		t.Fatalf("HotWaterKw = %f", snapshot.HotWaterKw)
	}
	if snapshot.HouseKw != 6.8 {
		t.Fatalf("HouseKw = %f", snapshot.HouseKw)
	}
	if snapshot.Sources[energy.RoleHotWater].Available {
		t.Fatalf("hotWater source should be unavailable: %+v", snapshot.Sources[energy.RoleHotWater])
	}
	if snapshot.Sources[energy.RoleHotWater].Error != "host not configured" {
		t.Fatalf("hotWater error = %q", snapshot.Sources[energy.RoleHotWater].Error)
	}
}
