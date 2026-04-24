package state

import (
	"math"
	"sort"
	"strconv"
	"strings"
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

type switchEvent struct {
	At        time.Time
	SOC       float64
	UpdatedAt time.Time
}

type Store struct {
	mu             sync.RWMutex
	services       map[string]ServiceStatus
	values         map[string]registers.DecodedValue
	lastSwitchGrid switchEvent
	lastSwitchBatt switchEvent
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

	previousMachineState := s.values["machine_state"].Rendered
	for _, value := range values {
		s.values[value.ID] = value
	}

	currentMachineState := s.values["machine_state"].Rendered
	if previousMachineState == "" || currentMachineState == "" || currentMachineState == previousMachineState {
		return
	}

	event := s.switchEventLocked(values, s.values["machine_state"])
	switch currentMachineState {
	case "AC Power Operation":
		s.lastSwitchGrid = event
	case "Inverter Operation":
		s.lastSwitchBatt = event
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

	telemetry = append(telemetry, derivedTelemetry(telemetry, s.lastSwitchGrid, s.lastSwitchBatt)...)

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

func derivedTelemetry(values []registers.DecodedValue, lastSwitchGrid, lastSwitchBatt switchEvent) []registers.DecodedValue {
	index := make(map[string]registers.DecodedValue, len(values))
	for _, value := range values {
		index[value.ID] = value
	}

	derived := make([]registers.DecodedValue, 0, 8)

	imported, okImport := numericValue(index["total_energy_import"])
	consumed, okConsumed := numericValue(index["total_load_consumption"])
	if okImport && okConsumed && imported > 0 {
		updatedAt := newestUpdatedAt(index["total_energy_import"], index["total_load_consumption"])
		losses := roundFloat(imported-consumed, 1)
		efficiency := roundFloat((consumed/imported)*100, 1)

		derived = append(derived,
			registers.DecodedValue{
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
			registers.DecodedValue{
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
		)
	}

	derived = append(derived, batteryEnergyTelemetry(index)...)

	derived = append(derived, switchTelemetry(
		"last_switch_to_grid_at",
		"Last Switch To Grid At",
		0xFFF2,
		"mdi:transmission-tower-import",
		lastSwitchGrid,
	)...)
	derived = append(derived, switchSOCTelemetry(
		"last_switch_to_grid_soc",
		"Last Switch To Grid SOC",
		0xFFF3,
		"mdi:battery-arrow-down",
		lastSwitchGrid,
	)...)
	derived = append(derived, switchTelemetry(
		"last_switch_to_battery_at",
		"Last Switch To Battery At",
		0xFFF4,
		"mdi:battery-arrow-up",
		lastSwitchBatt,
	)...)
	derived = append(derived, switchSOCTelemetry(
		"last_switch_to_battery_soc",
		"Last Switch To Battery SOC",
		0xFFF5,
		"mdi:battery-heart-variant",
		lastSwitchBatt,
	)...)

	return derived
}

func batteryEnergyTelemetry(index map[string]registers.DecodedValue) []registers.DecodedValue {
	nominalVoltage, ok := nominalBatteryVoltage(index["battery_type"])
	if !ok {
		return nil
	}

	chargedAh, okCharge := numericValue(index["total_battery_charge_ah"])
	dischargedAh, okDischarge := numericValue(index["total_battery_discharge_ah"])
	if !okCharge && !okDischarge {
		return nil
	}

	derived := make([]registers.DecodedValue, 0, 2)
	if okCharge {
		derived = append(derived, batteryEnergyValue(
			"battery_charge_energy_total_estimate",
			"Battery Charge Energy Total Estimate",
			0xFFF6,
			"mdi:battery-plus",
			chargedAh,
			nominalVoltage,
			index["total_battery_charge_ah"].UpdatedAt,
		))
	}
	if okDischarge {
		derived = append(derived, batteryEnergyValue(
			"battery_discharge_energy_total_estimate",
			"Battery Discharge Energy Total Estimate",
			0xFFF7,
			"mdi:battery-minus",
			dischargedAh,
			nominalVoltage,
			index["total_battery_discharge_ah"].UpdatedAt,
		))
	}

	return derived
}

func batteryEnergyValue(id, name string, address uint16, icon string, ampHours, nominalVoltage float64, updatedAt time.Time) registers.DecodedValue {
	kwh := roundFloat((ampHours*nominalVoltage)/1000, 1)
	return registers.DecodedValue{
		ID:          id,
		Name:        name,
		Address:     address,
		Group:       registers.GroupSlow,
		Component:   "sensor",
		Entity:      "diagnostic",
		Unit:        "kWh",
		DeviceClass: "energy",
		StateClass:  "total_increasing",
		Icon:        icon,
		Value:       kwh,
		Rendered:    strconv.FormatFloat(kwh, 'f', 1, 64),
		UpdatedAt:   updatedAt,
	}
}

func nominalBatteryVoltage(value registers.DecodedValue) (float64, bool) {
	label := value.Rendered
	if label == "" {
		if text, ok := value.Value.(string); ok {
			label = text
		}
	}

	fields := strings.Fields(label)
	if len(fields) < 2 {
		return 0, false
	}

	cellToken := fields[len(fields)-1]
	if !strings.HasPrefix(cellToken, "x") {
		return 0, false
	}
	cells, err := strconv.Atoi(strings.TrimPrefix(cellToken, "x"))
	if err != nil || cells <= 0 {
		return 0, false
	}

	switch {
	case strings.HasPrefix(label, "LFP "):
		return float64(cells) * 3.2, true
	case strings.HasPrefix(label, "Ternary Li "):
		return float64(cells) * 3.7, true
	default:
		return 0, false
	}
}

func (s *Store) switchEventLocked(values []registers.DecodedValue, machineState registers.DecodedValue) switchEvent {
	soc, _ := numericValue(s.values["battery_soc"])
	for _, value := range values {
		if value.ID != "battery_soc" {
			continue
		}
		if parsed, ok := numericValue(value); ok {
			soc = parsed
		}
		break
	}

	updatedAt := newestUpdatedAt(machineState, s.values["battery_soc"])
	if updatedAt.IsZero() {
		updatedAt = machineState.UpdatedAt
	}

	return switchEvent{
		At:        machineState.UpdatedAt,
		SOC:       soc,
		UpdatedAt: updatedAt,
	}
}

func switchTelemetry(id, name string, address uint16, icon string, event switchEvent) []registers.DecodedValue {
	if event.At.IsZero() {
		return nil
	}

	rendered := event.At.Format(time.RFC3339)
	return []registers.DecodedValue{{
		ID:          id,
		Name:        name,
		Address:     address,
		Group:       registers.GroupSlow,
		Component:   "sensor",
		Entity:      "diagnostic",
		DeviceClass: "timestamp",
		Icon:        icon,
		Value:       rendered,
		Rendered:    rendered,
		UpdatedAt:   event.UpdatedAt,
	}}
}

func switchSOCTelemetry(id, name string, address uint16, icon string, event switchEvent) []registers.DecodedValue {
	if event.At.IsZero() {
		return nil
	}

	soc := roundFloat(event.SOC, 0)
	return []registers.DecodedValue{{
		ID:          id,
		Name:        name,
		Address:     address,
		Group:       registers.GroupSlow,
		Component:   "sensor",
		Entity:      "diagnostic",
		Unit:        "%",
		DeviceClass: "battery",
		Icon:        icon,
		Value:       soc,
		Rendered:    strconv.FormatFloat(soc, 'f', 0, 64),
		UpdatedAt:   event.UpdatedAt,
	}}
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
