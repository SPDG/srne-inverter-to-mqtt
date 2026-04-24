package modbus

import (
	"testing"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

func TestRawFromSnapshotFindsUint16Value(t *testing.T) {
	t.Parallel()

	snapshot := state.Snapshot{
		Telemetry: []registers.DecodedValue{
			{ID: "battery_discharge_stop", Raw: 20},
		},
	}

	value, ok := rawFromSnapshot(snapshot, "battery_discharge_stop")
	if !ok {
		t.Fatal("expected value to be found")
	}
	if value != 20 {
		t.Fatalf("value = %d, want 20", value)
	}
}

func TestValidateBatteryDischargeThresholdsFromSnapshot(t *testing.T) {
	t.Parallel()

	service := &Service{
		state: state.New(),
	}
	service.state.UpsertTelemetry([]registers.DecodedValue{
		{ID: "battery_discharge_stop", Raw: 20},
		{ID: "battery_discharge_start", Raw: 95},
	})

	stopReg, _ := registers.FindByID("battery_discharge_stop")
	startReg, _ := registers.FindByID("battery_discharge_start")

	if err := service.validateWriteLocked(config.Config{}, stopReg, 96); err == nil {
		t.Fatal("expected stop threshold validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, startReg, 15); err == nil {
		t.Fatal("expected start threshold validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, stopReg, 20); err != nil {
		t.Fatalf("unexpected stop validation error: %v", err)
	}
	if err := service.validateWriteLocked(config.Config{}, startReg, 25); err != nil {
		t.Fatalf("unexpected start validation error: %v", err)
	}
}
