package stormcharge

import (
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

type recordedWrite struct {
	id    string
	value any
}

type recordingWriter struct {
	mu       sync.Mutex
	writes   []recordedWrite
	failID   string
	failures int
}

func (w *recordingWriter) WriteRegister(id string, value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, recordedWrite{id: id, value: value})
	if id == w.failID && w.failures > 0 {
		w.failures--
		return errTestWrite
	}
	return nil
}

var errTestWrite = &testError{"test write failure"}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }

func TestStartAndCancelRestoresPreviousSettings(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	writer := &recordingWriter{}
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), writer, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	manager.sleep = func(time.Duration) {}

	settings := testSettings()
	if err := manager.Start(settings); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status := manager.Status(settings); !status.Active || status.Phase != PhaseCharging {
		t.Fatalf("active status = %#v", status)
	}
	if err := manager.Cancel(); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	status := manager.Status(settings)
	if status.Active || status.Phase != PhaseCancelled || status.Reason != "manual_cancel" {
		t.Fatalf("cancelled status = %#v", status)
	}
	if status.Deadline != nil {
		t.Fatalf("cancelled deadline = %v, want nil", status.Deadline)
	}
	assertWriteIDs(t, writer.writes, []string{
		"battery_charge_cutoff_soc",
		"mains_charge_current_limit",
		"output_source_priority",
		"charger_source_priority",
		"charger_source_priority",
		"output_source_priority",
		"mains_charge_current_limit",
		"battery_charge_cutoff_soc",
	})
	if got := writer.writes[4].value; got != int64(3) {
		t.Fatalf("restored charger source = %#v, want raw 3", got)
	}
	if got := writer.writes[3].value; got != int64(2) {
		t.Fatalf("storm charger source = %#v, want Hybrid raw 2", got)
	}
	if got := writer.writes[5].value; got != int64(2) {
		t.Fatalf("restored output source = %#v, want raw 2", got)
	}
	if got := writer.writes[6].value; got != float64(10) {
		t.Fatalf("restored mains current = %#v, want 10", got)
	}
	if got := writer.writes[7].value; got != 100 {
		t.Fatalf("restored charge cutoff = %#v, want 100", got)
	}
}

func TestTargetReachedCompletesAndRestores(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	writer := &recordingWriter{}
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), writer, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager.now = func() time.Time { return time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC) }
	manager.sleep = func(time.Duration) {}
	settings := testSettings()
	if err := manager.Start(settings); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	runtimeState.UpsertTelemetry([]registers.DecodedValue{numericValue("battery_soc", 96, "%")})

	manager.evaluate()
	status := manager.Status(settings)
	if status.Active || status.Phase != PhaseCompleted || status.Reason != "target_reached" {
		t.Fatalf("completed status = %#v", status)
	}
}

func TestTimeoutRestoresPreviousSettings(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	writer := &recordingWriter{}
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), writer, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	manager.sleep = func(time.Duration) {}
	settings := testSettings()
	if err := manager.Start(settings); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	now = now.Add(settings.Timeout.Duration + time.Second)
	manager.evaluate()
	status := manager.Status(settings)
	if status.Active || status.Phase != PhaseTimedOut || status.Reason != "timeout" {
		t.Fatalf("timed out status = %#v", status)
	}
	if got := len(writer.writes); got != 8 {
		t.Fatalf("write count = %d, want 8", got)
	}
}

func TestStartFailureRollsBackAppliedSettings(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	writer := &recordingWriter{failID: "charger_source_priority", failures: 1}
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), writer, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager.now = func() time.Time { return time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC) }
	manager.sleep = func(time.Duration) {}

	if err := manager.Start(testSettings()); err == nil {
		t.Fatal("Start() expected error")
	}
	status := manager.Status(testSettings())
	if status.Active || status.Phase != PhaseError {
		t.Fatalf("failed start status = %#v", status)
	}
	if len(writer.writes) < 7 {
		t.Fatalf("write count = %d, expected start writes plus rollback", len(writer.writes))
	}
}

func TestActiveSessionIsReappliedAfterRestart(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "runtime.yaml")
	runtimeState := readyState()
	firstWriter := &recordingWriter{}
	first, err := New(statePath, firstWriter, runtimeState)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	first.now = func() time.Time { return now }
	first.sleep = func(time.Duration) {}
	if err := first.Start(testSettings()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	secondWriter := &recordingWriter{}
	second, err := New(statePath, secondWriter, runtimeState)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}
	second.now = func() time.Time { return now.Add(time.Minute) }
	second.sleep = func(time.Duration) {}
	second.evaluate()

	if len(secondWriter.writes) != 4 {
		t.Fatalf("recovery write count = %d, want 4", len(secondWriter.writes))
	}
	if status := second.Status(testSettings()); !status.Active || status.Phase != PhaseCharging {
		t.Fatalf("recovered status = %#v", status)
	}
}

func TestRestartWaitsForInitialModbusConnection(t *testing.T) {
	t.Parallel()

	statePath := filepath.Join(t.TempDir(), "runtime.yaml")
	runtimeState := readyState()
	first, err := New(statePath, &recordingWriter{}, runtimeState)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	first.now = func() time.Time { return now }
	first.sleep = func(time.Duration) {}
	if err := first.Start(testSettings()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	runtimeState.SetServiceStatus("modbus", "starting", false, "", time.Time{})
	secondWriter := &recordingWriter{}
	second, err := New(statePath, secondWriter, runtimeState)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}
	second.now = func() time.Time { return now.Add(time.Minute) }
	second.sleep = func(time.Duration) {}
	second.evaluate()
	if got := len(secondWriter.writes); got != 0 {
		t.Fatalf("writes before initial Modbus connection = %d, want 0", got)
	}
	if status := second.Status(testSettings()); !status.Active || status.Phase != PhaseCharging {
		t.Fatalf("waiting status = %#v", status)
	}

	runtimeState.SetServiceStatus("modbus", "connected", true, "", time.Now().UTC())
	second.evaluate()
	if got := len(secondWriter.writes); got != 4 {
		t.Fatalf("writes after initial Modbus connection = %d, want 4", got)
	}
	if status := second.Status(testSettings()); !status.Active || status.Phase != PhaseCharging {
		t.Fatalf("resumed status = %#v", status)
	}
}

func TestStartRejectsUnavailableGrid(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	runtimeState.UpsertTelemetry([]registers.DecodedValue{numericValue("grid_voltage_phase_b", 0, "V")})
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), &recordingWriter{}, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager.sleep = func(time.Duration) {}
	if err := manager.Start(testSettings()); err == nil {
		t.Fatal("Start() expected grid validation error")
	}
}

func TestRestoreWaitsUntilModbusReconnects(t *testing.T) {
	t.Parallel()

	runtimeState := readyState()
	writer := &recordingWriter{}
	manager, err := New(filepath.Join(t.TempDir(), "runtime.yaml"), writer, runtimeState)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager.now = func() time.Time { return time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC) }
	manager.sleep = func(time.Duration) {}
	if err := manager.Start(testSettings()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	runtimeState.SetServiceStatus("modbus", "error", false, "disconnected", time.Time{})
	manager.evaluate()
	manager.evaluate()
	if got := len(writer.writes); got != 4 {
		t.Fatalf("writes while Modbus disconnected = %d, want 4 initial writes", got)
	}
	if status := manager.Status(testSettings()); !status.Active || status.Phase != PhaseRestoring {
		t.Fatalf("disconnected status = %#v", status)
	}

	runtimeState.SetServiceStatus("modbus", "connected", true, "", time.Now().UTC())
	manager.evaluate()
	if got := len(writer.writes); got != 8 {
		t.Fatalf("writes after Modbus reconnect = %d, want 8", got)
	}
	if status := manager.Status(testSettings()); status.Active || status.Phase != PhaseError || status.Reason != "modbus_lost" {
		t.Fatalf("restored status = %#v", status)
	}
}

func readyState() *state.Store {
	runtimeState := state.New()
	runtimeState.SetServiceStatus("modbus", "connected", true, "", time.Now().UTC())
	runtimeState.UpsertTelemetry([]registers.DecodedValue{
		numericValue("battery_soc", 80, "%"),
		numericValue("battery_voltage", 53.2, "V"),
		numericValue("maximum_charge_current", 150, "A"),
		numericValue("battery_charge_cutoff_soc", 100, "%"),
		numericValue("mains_charge_current_limit", 10, "A"),
		rawValue("charger_source_priority", 3, "PV Only"),
		rawValue("output_source_priority", 2, "Solar, Battery, Utility"),
		numericValue("grid_voltage_phase_a", 234, "V"),
		numericValue("grid_voltage_phase_b", 233, "V"),
		numericValue("grid_voltage_phase_c", 235, "V"),
		numericValue("grid_frequency", 50, "Hz"),
		rawValue("fault_code_1", 0, "0"),
		rawValue("fault_code_2", 0, "0"),
	})
	return runtimeState
}

func numericValue(id string, value float64, unit string) registers.DecodedValue {
	return registers.DecodedValue{
		ID: id, Value: value, Raw: int64(value), Rendered: strconvFloat(value), Unit: unit,
	}
}

func rawValue(id string, raw int64, rendered string) registers.DecodedValue {
	return registers.DecodedValue{ID: id, Value: rendered, Raw: raw, Rendered: rendered}
}

func strconvFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func testSettings() Settings {
	return Settings{
		TargetSOC:   95,
		MaxCurrentA: 50,
		Timeout:     config.Duration{Duration: 12 * time.Hour},
	}
}

func assertWriteIDs(t *testing.T, writes []recordedWrite, want []string) {
	t.Helper()
	if len(writes) != len(want) {
		t.Fatalf("write count = %d, want %d: %#v", len(writes), len(want), writes)
	}
	for index, id := range want {
		if writes[index].id != id {
			t.Fatalf("write[%d] = %q, want %q", index, writes[index].id, id)
		}
	}
}
