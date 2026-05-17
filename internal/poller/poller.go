package poller

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/config"
	"github.com/chriskanedev/energy-monitor-api/internal/energy"
	"github.com/chriskanedev/energy-monitor-api/internal/shelly"
	"github.com/chriskanedev/energy-monitor-api/internal/store"
)

const historyLimit = 24

type ShellyClient interface {
	FetchStatus(ctx context.Context, host string) (shelly.Status, error)
}

type Poller struct {
	cfg    config.Config
	client ShellyClient
	store  *store.Store
	logger *slog.Logger

	mu          sync.RWMutex
	latest      energy.Snapshot
	lastGood    map[energy.Role]energy.RawReading
	homeHistory []energy.Point
	gridHistory []energy.Point
	subscribers map[chan energy.Snapshot]struct{}
}

func New(cfg config.Config, client ShellyClient, store *store.Store, logger *slog.Logger) *Poller {
	if logger == nil {
		logger = slog.Default()
	}
	now := time.Now()
	latest := energy.Snapshot{
		UpdatedAt: now.UnixMilli(),
		Sources:   defaultSources(cfg, true),
	}
	return &Poller{
		cfg:         cfg,
		client:      client,
		store:       store,
		logger:      logger,
		latest:      latest,
		lastGood:    make(map[energy.Role]energy.RawReading),
		subscribers: make(map[chan energy.Snapshot]struct{}),
	}
}

func (p *Poller) SeedHistory(home []energy.Point, grid []energy.Point) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.homeHistory = keepLatest(home, historyLimit)
	p.gridHistory = keepLatest(grid, historyLimit)
	p.latest.HomeHistory = append([]energy.Point(nil), p.homeHistory...)
	p.latest.GridHistory = append([]energy.Point(nil), p.gridHistory...)
}

func (p *Poller) Run(ctx context.Context) {
	p.poll(ctx)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) Latest() energy.Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return cloneSnapshot(p.latest)
}

func (p *Poller) Subscribe() (<-chan energy.Snapshot, func()) {
	ch := make(chan energy.Snapshot, 8)
	p.mu.Lock()
	p.subscribers[ch] = struct{}{}
	latest := cloneSnapshot(p.latest)
	p.mu.Unlock()

	ch <- latest

	cancel := func() {
		p.mu.Lock()
		if _, ok := p.subscribers[ch]; ok {
			delete(p.subscribers, ch)
			close(ch)
		}
		p.mu.Unlock()
	}

	return ch, cancel
}

func (p *Poller) poll(parent context.Context) {
	now := time.Now()
	statuses := p.fetchStatuses(parent)
	readings := p.buildReadings(statuses, now)
	inputs := p.inputs(readings, now)

	p.mu.Lock()
	homeHistory := appendHistory(p.homeHistory, energy.Point{Time: now.UnixMilli(), Value: calculateHouseKw(inputs)})
	gridHistory := appendHistory(p.gridHistory, energy.Point{Time: now.UnixMilli(), Value: math.Abs(calculateGridKw(inputs))})
	snapshot := energy.Normalizer{SolarCapacityKw: p.cfg.SolarCapacityKw}.Snapshot(inputs, homeHistory, gridHistory, now.UnixMilli())
	p.homeHistory = homeHistory
	p.gridHistory = gridHistory
	p.latest = snapshot
	subscribers := make([]chan energy.Snapshot, 0, len(p.subscribers))
	for subscriber := range p.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	p.mu.Unlock()

	if p.store != nil {
		if err := p.store.InsertPoll(parent, snapshot, readings); err != nil {
			p.logger.Warn("failed to store poll", "error", err)
		}
		if err := p.store.Prune(parent, p.cfg.RetentionDays); err != nil {
			p.logger.Warn("failed to prune old readings", "error", err)
		}
	}

	for _, subscriber := range subscribers {
		select {
		case subscriber <- cloneSnapshot(snapshot):
		default:
		}
	}
}

type deviceResult struct {
	status shelly.Status
	err    error
}

func (p *Poller) fetchStatuses(parent context.Context) map[string]deviceResult {
	results := make(map[string]deviceResult, len(p.cfg.Devices))
	var mu sync.Mutex
	var wg sync.WaitGroup

	deviceIDs := make([]string, 0, len(p.cfg.Devices))
	for deviceID := range p.cfg.Devices {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	for _, deviceID := range deviceIDs {
		deviceID := deviceID
		deviceCfg := p.cfg.Devices[deviceID]
		if deviceCfg.Host == "" {
			mu.Lock()
			results[deviceID] = deviceResult{err: fmt.Errorf("host not configured")}
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(parent, p.cfg.RequestTimeout)
			defer cancel()
			status, err := p.client.FetchStatus(ctx, deviceCfg.Host)
			mu.Lock()
			results[deviceID] = deviceResult{status: status, err: err}
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results
}

func (p *Poller) buildReadings(results map[string]deviceResult, now time.Time) []energy.RawReading {
	readings := make([]energy.RawReading, 0, len(energy.RequiredRoles))
	for _, role := range energy.RequiredRoles {
		roleCfg := p.cfg.Roles[role]
		reading := energy.RawReading{
			Role:    role,
			Device:  roleCfg.Device,
			Channel: roleCfg.Channel,
			Stale:   true,
		}

		result := results[roleCfg.Device]
		if result.err != nil {
			reading.Error = result.err.Error()
			readings = append(readings, p.withLastGood(reading, now))
			continue
		}
		if roleCfg.Channel >= len(result.status.EMeters) {
			reading.Error = fmt.Sprintf("channel %d not present", roleCfg.Channel)
			readings = append(readings, p.withLastGood(reading, now))
			continue
		}

		emeter := result.status.EMeters[roleCfg.Channel]
		if !emeter.IsValid {
			reading.Error = "emeter reading is invalid"
			readings = append(readings, p.withLastGood(reading, now))
			continue
		}

		reading.PowerW = emeter.Power
		reading.ReactiveVAR = emeter.Reactive
		reading.VoltageV = emeter.Voltage
		reading.PowerFactor = emeter.PowerFactor
		reading.TotalKWh = emeter.Total
		reading.TotalReturnedKWh = emeter.TotalReturned
		reading.Available = true
		reading.Stale = false
		reading.Error = ""
		reading.ReadAt = now
		p.lastGood[role] = reading
		readings = append(readings, reading)
	}

	return readings
}

func (p *Poller) withLastGood(reading energy.RawReading, now time.Time) energy.RawReading {
	last, ok := p.lastGood[reading.Role]
	if !ok {
		return reading
	}

	age := now.Sub(last.ReadAt)
	last.Available = false
	last.Stale = age > p.cfg.StaleAfter
	last.Error = reading.Error
	last.Device = reading.Device
	last.Channel = reading.Channel
	last.Role = reading.Role
	if last.Stale {
		last.PowerW = 0
		last.ReactiveVAR = 0
		last.VoltageV = 0
		last.PowerFactor = 0
		last.TotalKWh = 0
		last.TotalReturnedKWh = 0
	}
	return last
}

func (p *Poller) inputs(readings []energy.RawReading, now time.Time) []energy.RoleInput {
	inputs := make([]energy.RoleInput, 0, len(readings))
	for _, reading := range readings {
		roleCfg := p.cfg.Roles[reading.Role]
		if !reading.Available && !reading.Stale && now.Sub(reading.ReadAt) > p.cfg.StaleAfter {
			reading.Stale = true
		}
		inputs = append(inputs, energy.RoleInput{
			Role:    reading.Role,
			Reading: reading,
			Invert:  roleCfg.Invert,
		})
	}
	return inputs
}

func calculateGridKw(inputs []energy.RoleInput) float64 {
	for _, input := range inputs {
		if input.Role == energy.RoleGrid && input.Reading.Available && !input.Reading.Stale {
			power := input.Reading.PowerW
			if input.Invert {
				power = -power
			}
			return math.Round((power/1000)*10) / 10
		}
	}
	return 0
}

func calculateHouseKw(inputs []energy.RoleInput) float64 {
	values := map[energy.Role]float64{
		energy.RoleGrid:     0,
		energy.RoleSolar:    0,
		energy.RoleHotWater: 0,
	}
	for _, input := range inputs {
		if !input.Reading.Available || input.Reading.Stale {
			continue
		}
		power := input.Reading.PowerW
		if input.Invert {
			power = -power
		}
		kw := math.Round((power/1000)*10) / 10
		switch input.Role {
		case energy.RoleGrid:
			values[energy.RoleGrid] = kw
		case energy.RoleSolar:
			values[energy.RoleSolar] = math.Max(0, kw)
		case energy.RoleHotWater:
			values[energy.RoleHotWater] = math.Max(0, kw)
		}
	}
	return math.Round(math.Max(0, values[energy.RoleSolar]+values[energy.RoleGrid]-values[energy.RoleHotWater])*10) / 10
}

func appendHistory(history []energy.Point, point energy.Point) []energy.Point {
	return keepLatest(append(history, point), historyLimit)
}

func keepLatest(points []energy.Point, limit int) []energy.Point {
	if len(points) <= limit {
		return append([]energy.Point(nil), points...)
	}
	return append([]energy.Point(nil), points[len(points)-limit:]...)
}

func defaultSources(cfg config.Config, stale bool) map[energy.Role]energy.SourceStatus {
	sources := make(map[energy.Role]energy.SourceStatus, len(energy.RequiredRoles))
	for _, role := range energy.RequiredRoles {
		roleCfg := cfg.Roles[role]
		sources[role] = energy.SourceStatus{
			Available: false,
			Stale:     stale,
			Device:    roleCfg.Device,
			Channel:   roleCfg.Channel,
		}
	}
	return sources
}

func cloneSnapshot(snapshot energy.Snapshot) energy.Snapshot {
	cloned := snapshot
	cloned.HomeHistory = append([]energy.Point(nil), snapshot.HomeHistory...)
	cloned.GridHistory = append([]energy.Point(nil), snapshot.GridHistory...)
	cloned.Sources = make(map[energy.Role]energy.SourceStatus, len(snapshot.Sources))
	for role, source := range snapshot.Sources {
		cloned.Sources[role] = source
	}
	return cloned
}
