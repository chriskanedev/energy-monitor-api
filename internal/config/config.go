package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/energy"
	"gopkg.in/yaml.v3"
)

type Config struct {
	PollInterval    time.Duration
	RequestTimeout  time.Duration
	StaleAfter      time.Duration
	SolarCapacityKw float64
	RetentionDays   int
	Server          ServerConfig
	Storage         StorageConfig
	Devices         map[string]DeviceConfig
	Roles           map[energy.Role]RoleConfig
}

type ServerConfig struct {
	Addr               string
	CORSAllowedOrigins []string
}

type StorageConfig struct {
	SQLitePath string
}

type DeviceConfig struct {
	Host string
}

type RoleConfig struct {
	Device  string
	Channel int
	Invert  bool
}

type rawConfig struct {
	PollInterval    string                  `yaml:"pollInterval"`
	RequestTimeout  string                  `yaml:"requestTimeout"`
	StaleAfter      string                  `yaml:"staleAfter"`
	SolarCapacityKw float64                 `yaml:"solarCapacityKw"`
	RetentionDays   int                     `yaml:"retentionDays"`
	Server          rawServerConfig         `yaml:"server"`
	Storage         StorageConfig           `yaml:"storage"`
	Devices         map[string]DeviceConfig `yaml:"devices"`
	Roles           map[string]RoleConfig   `yaml:"roles"`
}

type rawServerConfig struct {
	Addr               string `yaml:"addr"`
	CORSAllowedOrigins string `yaml:"corsAllowedOrigins"`
}

func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(content))
	var raw rawConfig
	if err := yaml.Unmarshal([]byte(expanded), &raw); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	cfg, err := raw.toConfig()
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (raw rawConfig) toConfig() (Config, error) {
	pollInterval, err := parseDuration(raw.PollInterval, "pollInterval")
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := parseDuration(raw.RequestTimeout, "requestTimeout")
	if err != nil {
		return Config{}, err
	}
	staleAfter, err := parseDuration(raw.StaleAfter, "staleAfter")
	if err != nil {
		return Config{}, err
	}

	roles := make(map[energy.Role]RoleConfig, len(raw.Roles))
	for roleName, roleCfg := range raw.Roles {
		role := energy.Role(roleName)
		roles[role] = roleCfg
	}

	cfg := Config{
		PollInterval:    pollInterval,
		RequestTimeout:  requestTimeout,
		StaleAfter:      staleAfter,
		SolarCapacityKw: raw.SolarCapacityKw,
		RetentionDays:   raw.RetentionDays,
		Server: ServerConfig{
			Addr:               defaultString(raw.Server.Addr, ":8080"),
			CORSAllowedOrigins: splitCSV(raw.Server.CORSAllowedOrigins),
		},
		Storage: StorageConfig{
			SQLitePath: defaultString(raw.Storage.SQLitePath, "./data/energy.db"),
		},
		Devices: raw.Devices,
		Roles:   roles,
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.PollInterval <= 0 {
		return errors.New("pollInterval must be positive")
	}
	if cfg.RequestTimeout <= 0 {
		return errors.New("requestTimeout must be positive")
	}
	if cfg.StaleAfter <= 0 {
		return errors.New("staleAfter must be positive")
	}
	if cfg.SolarCapacityKw <= 0 {
		return errors.New("solarCapacityKw must be positive")
	}
	if cfg.RetentionDays <= 0 {
		return errors.New("retentionDays must be positive")
	}
	if cfg.Storage.SQLitePath == "" {
		return errors.New("storage.sqlitePath is required")
	}
	if len(cfg.Devices) == 0 {
		return errors.New("at least one device is required")
	}

	for _, role := range energy.RequiredRoles {
		roleCfg, ok := cfg.Roles[role]
		if !ok {
			return fmt.Errorf("role %q is required", role)
		}
		if roleCfg.Device == "" {
			return fmt.Errorf("role %q must reference a device", role)
		}
		if roleCfg.Channel < 0 {
			return fmt.Errorf("role %q channel must be >= 0", role)
		}
		if _, ok := cfg.Devices[roleCfg.Device]; !ok {
			return fmt.Errorf("role %q references unknown device %q", role, roleCfg.Device)
		}
	}

	return nil
}

func parseDuration(value string, field string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", field, err)
	}

	return duration, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func EnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
