package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Default()
	cfg.Device.Name = "test-device"
	cfg.Device.InverterType = "three_phase"
	cfg.Serial.Port = "/dev/ttyUSB7"
	cfg.Polling.FastInterval = Duration{Duration: 20 * time.Second}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Device.Name != cfg.Device.Name {
		t.Fatalf("device name mismatch: got %q want %q", loaded.Device.Name, cfg.Device.Name)
	}
	if loaded.Device.InverterType != cfg.Device.InverterType {
		t.Fatalf("inverter type mismatch: got %q want %q", loaded.Device.InverterType, cfg.Device.InverterType)
	}

	if loaded.Serial.Port != cfg.Serial.Port {
		t.Fatalf("serial port mismatch: got %q want %q", loaded.Serial.Port, cfg.Serial.Port)
	}

	if loaded.Polling.FastInterval.Duration != cfg.Polling.FastInterval.Duration {
		t.Fatalf("fast interval mismatch: got %s want %s", loaded.Polling.FastInterval, cfg.Polling.FastInterval)
	}
	if loaded.StormCharge != cfg.StormCharge {
		t.Fatalf("storm charge mismatch: got %#v want %#v", loaded.StormCharge, cfg.StormCharge)
	}
}

func TestStormChargeDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if cfg.StormCharge.TargetSOC != 95 || cfg.StormCharge.MaxCurrentA != 50 || cfg.StormCharge.Timeout.Duration != 12*time.Hour {
		t.Fatalf("unexpected storm charge defaults: %#v", cfg.StormCharge)
	}

	cfg.StormCharge.MaxCurrentA = 121
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected maximum storm charge current error")
	}
}

func TestDeviceInverterTypeNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{input: "", want: "single_phase"},
		{input: "single", want: "single_phase"},
		{input: "1p", want: "single_phase"},
		{input: "three", want: "spi_h3p"},
		{input: "3p", want: "spi_h3p"},
		{input: "three-phase", want: "spi_h3p"},
		{input: "spi-h3p", want: "spi_h3p"},
		{input: "hesp", want: "hesp_sh3"},
		{input: "hesp-sh3", want: "hesp_sh3"},
	}

	for _, tc := range cases {
		cfg := Default()
		cfg.Device.InverterType = tc.input
		got, err := cfg.Device.NormalizedInverterType()
		if err != nil {
			t.Fatalf("NormalizedInverterType(%q) error = %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizedInverterType(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestValidateRejectsInvalidInverterType(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Device.InverterType = "split_phase"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for invalid inverter type")
	}
}

func TestValidateRejectsInvalidParity(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Serial.Parity = "X"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for invalid parity")
	}
}

func TestConnectionModeDefaultsTCPPortToRTUOverTCP(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Serial.Port = "tcp://192.168.6.50:502"
	cfg.Serial.NetworkProtocol = "rtu"

	mode, err := cfg.Serial.ConnectionMode()
	if err != nil {
		t.Fatalf("ConnectionMode() error = %v", err)
	}
	if mode != ConnectionModeRTUOverTCP {
		t.Fatalf("ConnectionMode() = %q, want %q", mode, ConnectionModeRTUOverTCP)
	}
}

func TestConnectionModeSupportsModbusTCP(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Serial.Port = "tcp://192.168.6.50:502"
	cfg.Serial.NetworkProtocol = "modbus_tcp"

	mode, err := cfg.Serial.ConnectionMode()
	if err != nil {
		t.Fatalf("ConnectionMode() error = %v", err)
	}
	if mode != ConnectionModeModbusTCP {
		t.Fatalf("ConnectionMode() = %q, want %q", mode, ConnectionModeModbusTCP)
	}
}

func TestConnectionModeRejectsTCPProtocolForLocalPort(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Serial.Port = "/dev/ttyUSB0"
	cfg.Serial.NetworkProtocol = "modbus_tcp"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for modbus_tcp on a local serial port")
	}
}

func TestConnectionModeRejectsEmptyTCPEndpoint(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.Serial.Port = "tcp://"

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for empty TCP endpoint")
	}
}
