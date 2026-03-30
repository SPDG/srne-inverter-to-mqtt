package state

import (
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
)

type ServiceStatus struct {
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	Connected   bool      `json:"connected"`
	LastError   string    `json:"lastError,omitempty"`
	LastSuccess time.Time `json:"lastSuccess,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Snapshot struct {
	Services  map[string]ServiceStatus `json:"services"`
	Telemetry []registers.DecodedValue `json:"telemetry"`
}

type Store struct {
	mu       sync.RWMutex
	services map[string]ServiceStatus
	values   map[string]registers.DecodedValue
}

func New() *Store {
	return &Store{
		services: make(map[string]ServiceStatus),
		values:   make(map[string]registers.DecodedValue),
	}
}

func (s *Store) SetServiceStatus(name, status string, connected bool, lastError string, lastSuccess time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := s.services[name]
	if lastSuccess.IsZero() {
		lastSuccess = existing.LastSuccess
	}

	s.services[name] = ServiceStatus{
		Name:        name,
		Status:      status,
		Connected:   connected,
		LastError:   lastError,
		LastSuccess: lastSuccess,
		UpdatedAt:   time.Now().UTC(),
	}
}

func (s *Store) UpsertTelemetry(values []registers.DecodedValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, value := range values {
		s.values[value.ID] = value
	}
}

func (s *Store) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make(map[string]ServiceStatus, len(s.services))
	for key, value := range s.services {
		services[key] = value
	}

	telemetry := make([]registers.DecodedValue, 0, len(s.values))
	for _, value := range s.values {
		telemetry = append(telemetry, value)
	}

	telemetry = append(telemetry, derivedTelemetry(telemetry)...)

	sort.Slice(telemetry, func(i, j int) bool {
		if telemetry[i].Group == telemetry[j].Group {
			return telemetry[i].Address < telemetry[j].Address
		}
		return telemetry[i].Group < telemetry[j].Group
	})

	return Snapshot{
		Services:  services,
		Telemetry: telemetry,
	}
}

func derivedTelemetry(values []registers.DecodedValue) []registers.DecodedValue {
	index := make(map[string]registers.DecodedValue, len(values))
	for _, value := range values {
		index[value.ID] = value
	}

	imported, okImport := numericValue(index["total_energy_import"])
	consumed, okConsumed := numericValue(index["total_load_consumption"])
	if !okImport || !okConsumed || imported <= 0 {
		return nil
	}

	updatedAt := newestUpdatedAt(index["total_energy_import"], index["total_load_consumption"])
	losses := roundFloat(imported-consumed, 1)
	efficiency := roundFloat((consumed/imported)*100, 1)

	return []registers.DecodedValue{
		{
			ID:          "system_energy_losses_total",
			Name:        "System Energy Losses Total",
			Address:     0xFFF0,
			Group:       registers.GroupSlow,
			Component:   "sensor",
			Entity:      "diagnostic",
			Unit:        "kWh",
			DeviceClass: "energy",
			StateClass:  "total_increasing",
			Icon:        "mdi:transmission-tower-off",
			Value:       losses,
			Rendered:    strconv.FormatFloat(losses, 'f', 1, 64),
			UpdatedAt:   updatedAt,
		},
		{
			ID:        "system_energy_efficiency_total",
			Name:      "System Energy Efficiency Total",
			Address:   0xFFF1,
			Group:     registers.GroupSlow,
			Component: "sensor",
			Entity:    "diagnostic",
			Unit:      "%",
			Icon:      "mdi:percent-outline",
			Value:     efficiency,
			Rendered:  strconv.FormatFloat(efficiency, 'f', 1, 64),
			UpdatedAt: updatedAt,
		},
	}
}

func numericValue(value registers.DecodedValue) (float64, bool) {
	switch typed := value.Value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func newestUpdatedAt(values ...registers.DecodedValue) time.Time {
	var newest time.Time
	for _, value := range values {
		if value.UpdatedAt.After(newest) {
			newest = value.UpdatedAt
		}
	}
	return newest
}

func roundFloat(value float64, precision int) float64 {
	if precision < 0 {
		return value
	}
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}
