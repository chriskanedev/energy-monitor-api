package energy

import (
	"math"
	"time"
)

type RoleInput struct {
	Role    Role
	Reading RawReading
	Invert  bool
}

type Normalizer struct {
	SolarCapacityKw float64
}

func (n Normalizer) Snapshot(inputs []RoleInput, homeHistory []Point, gridHistory []Point, updatedAtMillis int64) Snapshot {
	values := map[Role]float64{
		RoleGrid:     0,
		RoleSolar:    0,
		RoleHotWater: 0,
	}
	sources := make(map[Role]SourceStatus, len(inputs))

	for _, input := range inputs {
		powerW := input.Reading.PowerW
		if input.Invert {
			powerW = -powerW
		}
		powerKw := roundOne(powerW / 1000)
		if !input.Reading.Available || input.Reading.Stale {
			powerKw = 0
		}

		switch input.Role {
		case RoleGrid:
			values[RoleGrid] = powerKw
		case RoleSolar:
			values[RoleSolar] = roundOne(math.Max(0, powerKw))
		case RoleHotWater:
			values[RoleHotWater] = roundOne(math.Max(0, powerKw))
		}

		var lastUpdate *time.Time
		if !input.Reading.ReadAt.IsZero() {
			cloned := input.Reading.ReadAt
			lastUpdate = &cloned
		}

		sources[input.Role] = SourceStatus{
			Available:      input.Reading.Available,
			Stale:          input.Reading.Stale,
			Device:         input.Reading.Device,
			Channel:        input.Reading.Channel,
			LastUpdateTime: lastUpdate,
			Error:          input.Reading.Error,
		}
	}

	houseKw := roundOne(math.Max(0, values[RoleSolar]+values[RoleGrid]-values[RoleHotWater]))
	solarCapacityPct := 0
	if n.SolarCapacityKw > 0 {
		solarCapacityPct = int(math.Round((values[RoleSolar] / n.SolarCapacityKw) * 100))
		if solarCapacityPct < 0 {
			solarCapacityPct = 0
		}
		if solarCapacityPct > 100 {
			solarCapacityPct = 100
		}
	}

	return Snapshot{
		SolarKw:          values[RoleSolar],
		HouseKw:          houseKw,
		HotWaterKw:       values[RoleHotWater],
		GridKw:           values[RoleGrid],
		SolarCapacityPct: solarCapacityPct,
		HomeHistory:      homeHistory,
		GridHistory:      gridHistory,
		UpdatedAt:        updatedAtMillis,
		Sources:          sources,
	}
}

func roundOne(value float64) float64 {
	return math.Round(value*10) / 10
}
