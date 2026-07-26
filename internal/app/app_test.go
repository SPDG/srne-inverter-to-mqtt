package app

import (
	"testing"
	"time"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

func TestShouldTriggerWatchdogAfterStall(t *testing.T) {
	runtimeState := state.New()
	lastSuccess := time.Date(2026, time.March, 29, 12, 0, 0, 0, time.UTC)
	runtimeState.SetServiceStatus("modbus", "error", false, "serial: timeout", lastSuccess)

	app := &App{
		cfg: config.Config{
			Polling: config.PollingConfig{
				FastInterval: config.Duration{Duration: 15 * time.Second},
				SlowInterval: config.Duration{Duration: 60 * time.Second},
			},
		},
		runtimeState: runtimeState,
	}

	trigger, stalledFor, service := app.shouldTriggerWatchdog(lastSuccess.Add(3 * time.Minute))
	if !trigger {
		t.Fatalf("expected watchdog trigger, got false")
	}
	if stalledFor < 2*time.Minute {
		t.Fatalf("expected stalled duration to be above threshold, got %s", stalledFor)
	}
	if service.Status != "error" {
		t.Fatalf("expected modbus error status, got %q", service.Status)
	}
}

func TestShouldNotTriggerWatchdogWithoutSuccessfulRead(t *testing.T) {
	runtimeState := state.New()
	runtimeState.SetServiceStatus("modbus", "error", false, "serial: timeout", time.Time{})

	app := &App{
		cfg: config.Config{
			Polling: config.PollingConfig{
				FastInterval: config.Duration{Duration: 15 * time.Second},
				SlowInterval: config.Duration{Duration: 60 * time.Second},
			},
		},
		runtimeState: runtimeState,
	}

	trigger, _, _ := app.shouldTriggerWatchdog(time.Now().UTC().Add(10 * time.Minute))
	if trigger {
		t.Fatalf("expected watchdog to ignore startup failures without prior success")
	}
}

func TestModbusWatchdogThresholdHasFloor(t *testing.T) {
	cfg := config.Config{
		Polling: config.PollingConfig{
			SlowInterval: config.Duration{Duration: 20 * time.Second},
		},
	}

	got := modbusWatchdogThreshold(cfg)
	if got != 90*time.Second {
		t.Fatalf("expected 90s watchdog floor, got %s", got)
	}
}
