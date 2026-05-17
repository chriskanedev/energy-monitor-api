package energy

import "time"

type Role string

const (
	RoleGrid     Role = "grid"
	RoleSolar    Role = "solar"
	RoleHotWater Role = "hotWater"
)

var RequiredRoles = []Role{RoleGrid, RoleSolar, RoleHotWater}

type Point struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type SourceStatus struct {
	Available      bool       `json:"available"`
	Stale          bool       `json:"stale"`
	Device         string     `json:"device"`
	Channel        int        `json:"channel"`
	LastUpdateTime *time.Time `json:"lastUpdateTime"`
	Error          string     `json:"error,omitempty"`
}

type Snapshot struct {
	SolarKw          float64               `json:"solarKw"`
	HouseKw          float64               `json:"houseKw"`
	HotWaterKw       float64               `json:"hotWaterKw"`
	GridKw           float64               `json:"gridKw"`
	GridVoltageV     float64               `json:"gridVoltageV"`
	SolarCapacityPct int                   `json:"solarCapacityPct"`
	HomeHistory      []Point               `json:"homeHistory"`
	GridHistory      []Point               `json:"gridHistory"`
	UpdatedAt        int64                 `json:"updatedAt"`
	Sources          map[Role]SourceStatus `json:"sources"`
}

type RawReading struct {
	Role             Role
	Device           string
	Channel          int
	PowerW           float64
	ReactiveVAR      float64
	VoltageV         float64
	PowerFactor      float64
	TotalKWh         float64
	TotalReturnedKWh float64
	Available        bool
	Stale            bool
	Error            string
	ReadAt           time.Time
}
