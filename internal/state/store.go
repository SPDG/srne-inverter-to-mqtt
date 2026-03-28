package state

import (
	"sort"
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
