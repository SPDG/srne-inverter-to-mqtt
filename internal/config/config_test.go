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

	if loaded.Serial.Port != cfg.Serial.Port {
		t.Fatalf("serial port mismatch: got %q want %q", loaded.Serial.Port, cfg.Serial.Port)
	}

	if loaded.Polling.FastInterval.Duration != cfg.Polling.FastInterval.Duration {
		t.Fatalf("fast interval mismatch: got %s want %s", loaded.Polling.FastInterval, cfg.Polling.FastInterval)
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
