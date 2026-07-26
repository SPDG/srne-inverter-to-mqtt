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
		{ID: "battery_discharge_cutoff_soc", Raw: 15},
		{ID: "battery_low_soc_alarm", Raw: 20},
		{ID: "battery_discharge_stop", Raw: 10},
		{ID: "battery_discharge_start", Raw: 25},
	})

	cutoffReg, _ := registers.FindByID("battery_discharge_cutoff_soc")
	alarmReg, _ := registers.FindByID("battery_low_soc_alarm")
	stopReg, _ := registers.FindByID("battery_discharge_stop")
	startReg, _ := registers.FindByID("battery_discharge_start")

	if err := service.validateWriteLocked(config.Config{}, cutoffReg, 16); err == nil {
		t.Fatal("expected discharge cut-off versus low SOC alarm validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, alarmReg, 19); err == nil {
		t.Fatal("expected low SOC alarm versus discharge cut-off validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, alarmReg, 21); err == nil {
		t.Fatal("expected low SOC alarm versus inverter threshold validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, stopReg, 26); err == nil {
		t.Fatal("expected stop threshold validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, startReg, 9); err == nil {
		t.Fatal("expected start threshold validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, cutoffReg, 15); err != nil {
		t.Fatalf("unexpected cut-off validation error: %v", err)
	}
	if err := service.validateWriteLocked(config.Config{}, alarmReg, 20); err != nil {
		t.Fatalf("unexpected low SOC alarm validation error: %v", err)
	}
	if err := service.validateWriteLocked(config.Config{}, stopReg, 10); err != nil {
		t.Fatalf("unexpected stop validation error: %v", err)
	}
	if err := service.validateWriteLocked(config.Config{}, startReg, 24); err == nil {
		t.Fatal("expected start threshold versus low SOC alarm validation error")
	}
	if err := service.validateWriteLocked(config.Config{}, startReg, 25); err != nil {
		t.Fatalf("unexpected start validation error: %v", err)
	}
}
