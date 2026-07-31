package stormcharge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

const (
	PhaseIdle      = "idle"
	PhaseStarting  = "starting"
	PhaseCharging  = "charging"
	PhaseRestoring = "restoring"
	PhaseCompleted = "completed"
	PhaseCancelled = "cancelled"
	PhaseTimedOut  = "timed_out"
	PhaseError     = "error"
)

var managedRegisters = map[string]struct{}{
	"battery_charge_cutoff_soc":  {},
	"mains_charge_current_limit": {},
}

const (
	timedChargeAddress = 0xE026
	timedChargeWords   = 7
)

type RegisterWriter interface {
	WriteRegister(id string, value any) error
	ReadHoldingWords(address, count uint16) ([]uint16, error)
	WriteHoldingWords(address uint16, values []uint16) error
}

type Settings struct {
	TargetSOC   int             `yaml:"target_soc" json:"targetSoc"`
	MaxCurrentA float64         `yaml:"max_current_a" json:"maxCurrentA"`
	Timeout     config.Duration `yaml:"timeout" json:"timeout"`
}

type PreviousSettings struct {
	BatteryChargeCutoffSOC int      `yaml:"battery_charge_cutoff_soc" json:"batteryChargeCutoffSoc"`
	MainsChargeCurrentA    float64  `yaml:"mains_charge_current_a" json:"mainsChargeCurrentA"`
	TimedChargeSchedule    []uint16 `yaml:"timed_charge_schedule" json:"timedChargeSchedule"`
}

type Runtime struct {
	Version   int              `yaml:"version"`
	Active    bool             `yaml:"active"`
	Phase     string           `yaml:"phase"`
	Settings  Settings         `yaml:"settings"`
	Previous  PreviousSettings `yaml:"previous"`
	StartedAt time.Time        `yaml:"started_at,omitempty"`
	Deadline  time.Time        `yaml:"deadline,omitempty"`
	UpdatedAt time.Time        `yaml:"updated_at"`
	Reason    string           `yaml:"reason,omitempty"`
	LastError string           `yaml:"last_error,omitempty"`
}

type Status struct {
	Active              bool             `json:"active"`
	Phase               string           `json:"phase"`
	Settings            Settings         `json:"settings"`
	Previous            PreviousSettings `json:"previousSettings,omitempty"`
	StartedAt           *time.Time       `json:"startedAt,omitempty"`
	Deadline            *time.Time       `json:"deadline,omitempty"`
	Remaining           string           `json:"remaining,omitempty"`
	CurrentSOC          float64          `json:"currentSoc"`
	BatteryVoltage      float64          `json:"batteryVoltage"`
	EstimatedPowerWatts int              `json:"estimatedPowerWatts"`
	BMSChargeLimitA     float64          `json:"bmsChargeLimitA,omitempty"`
	Reason              string           `json:"reason,omitempty"`
	LastError           string           `json:"lastError,omitempty"`
	UpdatedAt           time.Time        `json:"updatedAt"`
}

type Manager struct {
	mu            sync.Mutex
	statePath     string
	writer        RegisterWriter
	state         *state.Store
	runtime       Runtime
	resumePending bool
	now           func() time.Time
}

func New(statePath string, writer RegisterWriter, runtimeState *state.Store) (*Manager, error) {
	manager := &Manager{
		statePath: statePath,
		writer:    writer,
		state:     runtimeState,
		now:       func() time.Time { return time.Now().UTC() },
		runtime: Runtime{
			Version: 2,
			Phase:   PhaseIdle,
		},
	}

	if err := manager.load(); err != nil {
		return nil, err
	}
	return manager, nil
}

func RuntimePath(configPath string) string {
	return configPath + ".storm-charge-state.yaml"
}

func IsManagedRegister(id string) bool {
	_, ok := managedRegisters[id]
	return ok
}

func ValidateSettings(settings Settings) error {
	if settings.TargetSOC < 50 || settings.TargetSOC > 100 {
		return errors.New("target SOC must be between 50 and 100 percent")
	}
	if settings.MaxCurrentA <= 0 || settings.MaxCurrentA > 120 {
		return errors.New("maximum grid charge current must be greater than 0 and at most 120 A")
	}
	if settings.Timeout.Duration < 5*time.Minute || settings.Timeout.Duration > 24*time.Hour {
		return errors.New("timeout must be between 5 minutes and 24 hours")
	}
	return nil
}

func SettingsFromConfig(cfg config.StormChargeConfig) Settings {
	return Settings{
		TargetSOC:   cfg.TargetSOC,
		MaxCurrentA: cfg.MaxCurrentA,
		Timeout:     cfg.Timeout,
	}
}

func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	m.evaluate()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.evaluate()
		}
	}
}

func (m *Manager) Start(settings Settings) error {
	if err := ValidateSettings(settings); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runtime.Active {
		return fmt.Errorf("storm charge is already active")
	}

	snapshot := m.state.Snapshot()
	currentSOC, err := requiredNumeric(snapshot, "battery_soc")
	if err != nil {
		return err
	}
	if currentSOC >= float64(settings.TargetSOC) {
		return fmt.Errorf("battery SOC is already %.0f%%, target must be higher", currentSOC)
	}
	if err := requireGrid(snapshot); err != nil {
		return err
	}
	if limit, ok := optionalNumeric(snapshot, "maximum_charge_current"); ok && limit > 0 && settings.MaxCurrentA > limit {
		return fmt.Errorf("requested %.1f A exceeds the current BMS charge limit of %.1f A", settings.MaxCurrentA, limit)
	}

	previous, err := m.previousSettings(snapshot)
	if err != nil {
		return err
	}

	now := m.now()
	m.runtime = Runtime{
		Version:   2,
		Active:    true,
		Phase:     PhaseStarting,
		Settings:  settings,
		Previous:  previous,
		StartedAt: now,
		Deadline:  now.Add(settings.Timeout.Duration),
		UpdatedAt: now,
	}
	if err := m.saveLocked(); err != nil {
		m.runtime = Runtime{Version: 2, Phase: PhaseIdle}
		return err
	}

	if err := m.applyStormSettingsLocked(); err != nil {
		m.runtime.Phase = PhaseRestoring
		m.runtime.Reason = "start_failed"
		m.runtime.LastError = err.Error()
		m.runtime.UpdatedAt = m.now()
		_ = m.saveLocked()
		if restoreErr := m.restoreLocked(PhaseError, "start_failed"); restoreErr != nil {
			return fmt.Errorf("start storm charge: %v; restore previous settings: %w", err, restoreErr)
		}
		return fmt.Errorf("start storm charge: %w", err)
	}

	m.runtime.Phase = PhaseCharging
	m.runtime.UpdatedAt = m.now()
	m.runtime.LastError = ""
	if err := m.saveLocked(); err != nil {
		m.runtime.Phase = PhaseRestoring
		m.runtime.Reason = "persistence_failed"
		m.runtime.LastError = err.Error()
		if restoreErr := m.restoreLocked(PhaseError, "persistence_failed"); restoreErr != nil {
			return fmt.Errorf("persist active storm charge: %v; restore previous settings: %w", err, restoreErr)
		}
		return fmt.Errorf("persist active storm charge: %w", err)
	}
	log.Printf("storm charge started target_soc=%d max_current=%.1fA timeout=%s", settings.TargetSOC, settings.MaxCurrentA, settings.Timeout.Duration)
	return nil
}

func (m *Manager) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.runtime.Active {
		return nil
	}
	return m.restoreLocked(PhaseCancelled, "manual_cancel")
}

func (m *Manager) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtime.Active
}

func (m *Manager) Status(defaults Settings) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	runtime := m.runtime
	settings := defaults
	if runtime.Active {
		settings = runtime.Settings
	}

	snapshot := m.state.Snapshot()
	soc, _ := optionalNumeric(snapshot, "battery_soc")
	voltage, _ := optionalNumeric(snapshot, "battery_voltage")
	bmsLimit, _ := optionalNumeric(snapshot, "maximum_charge_current")
	status := Status{
		Active:              runtime.Active,
		Phase:               runtime.Phase,
		Settings:            settings,
		Previous:            runtime.Previous,
		CurrentSOC:          soc,
		BatteryVoltage:      voltage,
		EstimatedPowerWatts: int(settings.MaxCurrentA*voltage + 0.5),
		BMSChargeLimitA:     bmsLimit,
		Reason:              runtime.Reason,
		LastError:           runtime.LastError,
		UpdatedAt:           runtime.UpdatedAt,
	}
	if !runtime.StartedAt.IsZero() {
		status.StartedAt = &runtime.StartedAt
	}
	if runtime.Active && !runtime.Deadline.IsZero() {
		status.Deadline = &runtime.Deadline
		remaining := runtime.Deadline.Sub(m.now())
		if remaining < 0 {
			remaining = 0
		}
		status.Remaining = remaining.Round(time.Second).String()
	}
	return status
}

func (m *Manager) evaluate() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.runtime.Active {
		return
	}
	snapshot := m.state.Snapshot()
	modbus, ok := snapshot.Services["modbus"]
	if !ok || !modbus.Connected {
		if m.resumePending {
			return
		}
		if m.runtime.Phase != PhaseRestoring {
			m.runtime.Phase = PhaseRestoring
			m.runtime.Reason = "modbus_lost"
			m.runtime.LastError = "waiting for Modbus to restore previous settings"
			m.runtime.UpdatedAt = m.now()
			_ = m.saveLocked()
		}
		return
	}
	if m.runtime.Phase == PhaseRestoring {
		if err := m.restoreLocked(finalPhaseForReason(m.runtime.Reason), m.runtime.Reason); err != nil {
			m.runtime.LastError = err.Error()
			m.runtime.UpdatedAt = m.now()
			_ = m.saveLocked()
		}
		return
	}

	now := m.now()
	if !m.runtime.Deadline.IsZero() && !now.Before(m.runtime.Deadline) {
		if err := m.restoreLocked(PhaseTimedOut, "timeout"); err != nil {
			log.Printf("storm charge timeout restore failed: %v", err)
		}
		return
	}

	if !hasRequiredTelemetry(snapshot) {
		return
	}
	if m.resumePending {
		if err := m.applyStormSettingsLocked(); err != nil {
			m.runtime.Phase = PhaseRestoring
			m.runtime.Reason = "resume_failed"
			m.runtime.LastError = err.Error()
			m.runtime.UpdatedAt = now
			_ = m.saveLocked()
			return
		}
		m.resumePending = false
		m.runtime.Phase = PhaseCharging
		m.runtime.LastError = ""
		m.runtime.UpdatedAt = now
		_ = m.saveLocked()
	}
	if fault, id, ok := activeFault(snapshot); ok {
		if err := m.restoreLocked(PhaseError, fmt.Sprintf("%s_%d", id, fault)); err != nil {
			log.Printf("storm charge fault restore failed: %v", err)
		}
		return
	}
	if err := requireGrid(snapshot); err != nil {
		if restoreErr := m.restoreLocked(PhaseCancelled, "grid_lost"); restoreErr != nil {
			log.Printf("storm charge grid-loss restore failed: %v", restoreErr)
		}
		return
	}
	soc, err := requiredNumeric(snapshot, "battery_soc")
	if err != nil {
		return
	}
	if soc >= float64(m.runtime.Settings.TargetSOC) {
		if err := m.restoreLocked(PhaseCompleted, "target_reached"); err != nil {
			log.Printf("storm charge completion restore failed: %v", err)
		}
	}
}

func (m *Manager) applyStormSettingsLocked() error {
	writes := []struct {
		id    string
		value any
	}{
		{id: "battery_charge_cutoff_soc", value: m.runtime.Settings.TargetSOC},
		{id: "mains_charge_current_limit", value: m.runtime.Settings.MaxCurrentA},
	}
	for _, write := range writes {
		if err := m.writer.WriteRegister(write.id, write.value); err != nil {
			return fmt.Errorf("write %s: %w", write.id, err)
		}
	}

	// Near-full-day utility windows avoid depending on the inverter's current RTC.
	// This firmware validates all three slot pairs when the function is enabled,
	// rejects zero-length unused slots, and normalizes a 00:00 start to 00:01.
	schedule := []uint16{0x0001, 0x173B, 0x0001, 0x173B, 0x0001, 0x173B, 1}
	if err := m.writeTimedChargeSchedule(schedule); err != nil {
		return fmt.Errorf("enable timed utility charging: %w", err)
	}
	return nil
}

func (m *Manager) restoreLocked(finalPhase, reason string) error {
	m.runtime.Phase = PhaseRestoring
	m.runtime.Reason = reason
	m.runtime.UpdatedAt = m.now()
	_ = m.saveLocked()

	errorsFound := make([]string, 0)
	if len(m.runtime.Previous.TimedChargeSchedule) == timedChargeWords {
		if err := m.writeTimedChargeSchedule(m.runtime.Previous.TimedChargeSchedule); err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("timed utility charging: %v", err))
		}
	}

	writes := []struct {
		id    string
		value any
	}{
		{id: "mains_charge_current_limit", value: m.runtime.Previous.MainsChargeCurrentA},
		{id: "battery_charge_cutoff_soc", value: m.runtime.Previous.BatteryChargeCutoffSOC},
	}

	for _, write := range writes {
		if err := m.writer.WriteRegister(write.id, write.value); err != nil {
			errorsFound = append(errorsFound, fmt.Sprintf("%s: %v", write.id, err))
		}
	}
	if len(errorsFound) > 0 {
		m.runtime.LastError = strings.Join(errorsFound, "; ")
		m.runtime.UpdatedAt = m.now()
		_ = m.saveLocked()
		return errors.New(m.runtime.LastError)
	}

	m.runtime.Active = false
	m.runtime.Phase = finalPhase
	m.runtime.LastError = ""
	m.runtime.Deadline = time.Time{}
	m.runtime.UpdatedAt = m.now()
	if err := m.saveLocked(); err != nil {
		m.runtime.Active = true
		m.runtime.Phase = PhaseRestoring
		m.runtime.LastError = err.Error()
		return err
	}
	log.Printf("storm charge stopped phase=%s reason=%s", finalPhase, reason)
	return nil
}

func (m *Manager) writeTimedChargeSchedule(values []uint16) error {
	if len(values) != timedChargeWords {
		return fmt.Errorf("timed utility charging requires %d words", timedChargeWords)
	}

	// Disable first when restoring an inactive schedule, then restore all slot
	// values. For an active schedule, enable only after every slot is valid.
	if values[6] == 0 {
		if err := m.writer.WriteHoldingWords(timedChargeAddress+6, values[6:7]); err != nil {
			return fmt.Errorf("write OnTimeChargeEn: %w", err)
		}
	}
	for offset := 0; offset < 6; offset++ {
		if err := m.writer.WriteHoldingWords(timedChargeAddress+uint16(offset), values[offset:offset+1]); err != nil {
			return fmt.Errorf("write timed charge slot register 0x%04X: %w", timedChargeAddress+uint16(offset), err)
		}
	}
	if values[6] != 0 {
		if err := m.writer.WriteHoldingWords(timedChargeAddress+6, values[6:7]); err != nil {
			return fmt.Errorf("write OnTimeChargeEn: %w", err)
		}
	}
	return nil
}

func finalPhaseForReason(reason string) string {
	switch reason {
	case "target_reached":
		return PhaseCompleted
	case "timeout":
		return PhaseTimedOut
	case "manual_cancel", "grid_lost":
		return PhaseCancelled
	default:
		return PhaseError
	}
}

func (m *Manager) previousSettings(snapshot state.Snapshot) (PreviousSettings, error) {
	cutoff, err := requiredNumeric(snapshot, "battery_charge_cutoff_soc")
	if err != nil {
		return PreviousSettings{}, err
	}
	current, err := requiredNumeric(snapshot, "mains_charge_current_limit")
	if err != nil {
		return PreviousSettings{}, err
	}
	schedule, err := m.writer.ReadHoldingWords(timedChargeAddress, timedChargeWords)
	if err != nil {
		return PreviousSettings{}, fmt.Errorf("read timed utility charging settings: %w", err)
	}
	return PreviousSettings{
		BatteryChargeCutoffSOC: int(cutoff),
		MainsChargeCurrentA:    current,
		TimedChargeSchedule:    schedule,
	}, nil
}

func requireGrid(snapshot state.Snapshot) error {
	for _, id := range []string{"grid_voltage_phase_a", "grid_voltage_phase_b", "grid_voltage_phase_c"} {
		voltage, err := requiredNumeric(snapshot, id)
		if err != nil {
			return err
		}
		if voltage < 170 || voltage > 280 {
			return fmt.Errorf("grid is unavailable: %s is %.1f V", id, voltage)
		}
	}
	frequency, err := requiredNumeric(snapshot, "grid_frequency")
	if err != nil {
		return err
	}
	if frequency < 45 || frequency > 65 {
		return fmt.Errorf("grid is unavailable: frequency is %.2f Hz", frequency)
	}
	return nil
}

func hasRequiredTelemetry(snapshot state.Snapshot) bool {
	for _, id := range []string{
		"battery_soc",
		"battery_charge_cutoff_soc",
		"mains_charge_current_limit",
		"grid_voltage_phase_a",
		"grid_voltage_phase_b",
		"grid_voltage_phase_c",
		"grid_frequency",
	} {
		found := false
		for _, value := range snapshot.Telemetry {
			if value.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func activeFault(snapshot state.Snapshot) (int64, string, bool) {
	for _, id := range []string{"fault_code_1", "fault_code_2"} {
		for _, value := range snapshot.Telemetry {
			if value.ID == id && value.Raw != 0 {
				return value.Raw, id, true
			}
		}
	}
	return 0, "", false
}

func requiredNumeric(snapshot state.Snapshot, id string) (float64, error) {
	value, ok := optionalNumeric(snapshot, id)
	if !ok {
		return 0, fmt.Errorf("telemetry %s is not available", id)
	}
	return value, nil
}

func optionalNumeric(snapshot state.Snapshot, id string) (float64, bool) {
	for _, value := range snapshot.Telemetry {
		if value.ID != id {
			continue
		}
		switch typed := value.Value.(type) {
		case float64:
			return typed, true
		case int64:
			return float64(typed), true
		case int:
			return float64(typed), true
		}
		parsed, err := strconv.ParseFloat(value.Rendered, 64)
		return parsed, err == nil
	}
	return 0, false
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read storm charge runtime state: %w", err)
	}
	if err := yaml.Unmarshal(data, &m.runtime); err != nil {
		return fmt.Errorf("parse storm charge runtime state: %w", err)
	}
	if m.runtime.Version == 1 && !m.runtime.Active {
		m.runtime.Version = 2
	}
	if m.runtime.Version != 2 {
		return fmt.Errorf("unsupported storm charge runtime state version %d", m.runtime.Version)
	}
	if m.runtime.Phase == "" {
		m.runtime.Phase = PhaseIdle
	}
	m.resumePending = m.runtime.Active
	return nil
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return fmt.Errorf("create storm charge state directory: %w", err)
	}
	data, err := yaml.Marshal(&m.runtime)
	if err != nil {
		return fmt.Errorf("marshal storm charge runtime state: %w", err)
	}
	tmpPath := m.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write storm charge runtime state: %w", err)
	}
	if err := os.Rename(tmpPath, m.statePath); err != nil {
		return fmt.Errorf("replace storm charge runtime state: %w", err)
	}
	return nil
}
