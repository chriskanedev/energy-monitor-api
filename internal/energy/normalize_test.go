package energy

import (
	"testing"
	"time"
)

func TestNormalizerCalculatesDashboardValues(t *testing.T) {
	now := time.Now()
	snapshot := Normalizer{SolarCapacityKw: 6.15}.Snapshot([]RoleInput{
		{
			Role: RoleGrid,
			Reading: RawReading{
				Role:      RoleGrid,
				Device:    "first",
				Channel:   0,
				PowerW:    2500,
				VoltageV:  238.59,
				Available: true,
				ReadAt:    now,
			},
		},
		{
			Role: RoleSolar,
			Reading: RawReading{
				Role:      RoleSolar,
				Device:    "first",
				Channel:   1,
				PowerW:    4300,
				Available: true,
				ReadAt:    now,
			},
		},
		{
			Role: RoleHotWater,
			Reading: RawReading{
				Role:      RoleHotWater,
				Device:    "second",
				Channel:   0,
				PowerW:    2100,
				Available: true,
				ReadAt:    now,
			},
		},
	}, nil, nil, now.UnixMilli())

	if snapshot.GridKw != 2.5 {
		t.Fatalf("GridKw = %f", snapshot.GridKw)
	}
	if snapshot.GridVoltageV != 238.6 {
		t.Fatalf("GridVoltageV = %f", snapshot.GridVoltageV)
	}
	if snapshot.SolarKw != 4.3 {
		t.Fatalf("SolarKw = %f", snapshot.SolarKw)
	}
	if snapshot.HotWaterKw != 2.1 {
		t.Fatalf("HotWaterKw = %f", snapshot.HotWaterKw)
	}
	if snapshot.HouseKw != 4.7 {
		t.Fatalf("HouseKw = %f", snapshot.HouseKw)
	}
	if snapshot.SolarCapacityPct != 70 {
		t.Fatalf("SolarCapacityPct = %d", snapshot.SolarCapacityPct)
	}
}

func TestNormalizerZeroesUnavailableRole(t *testing.T) {
	now := time.Now()
	snapshot := Normalizer{SolarCapacityKw: 6.15}.Snapshot([]RoleInput{
		{
			Role: RoleHotWater,
			Reading: RawReading{
				Role:      RoleHotWater,
				Device:    "second",
				Channel:   0,
				PowerW:    2100,
				Available: false,
				Stale:     true,
				Error:     "host not configured",
				ReadAt:    now,
			},
		},
	}, nil, nil, now.UnixMilli())

	if snapshot.HotWaterKw != 0 {
		t.Fatalf("HotWaterKw = %f", snapshot.HotWaterKw)
	}
	if snapshot.Sources[RoleHotWater].Error != "host not configured" {
		t.Fatalf("hotWater source = %+v", snapshot.Sources[RoleHotWater])
	}
}
