package mqtt

import (
	"testing"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
)

func TestWritableDiscoveryPayloadUsesButtonForWriteOnlySingleOption(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Device: config.DeviceConfig{Name: "srne-main"},
		MQTT: config.MQTTConfig{
			TopicPrefix: "srne/srne-main",
		},
	}
	reg := registers.Register{
		ID:        "reset_machine",
		Name:      "Reset Machine",
		Writable:  true,
		WriteOnly: true,
		Icon:      "mdi:restart-alert",
		Entity:    "config",
		Enum: map[int64]string{
			1: "Reset",
		},
	}

	payload, component := writableDiscoveryPayload(cfg, buildinfo.Info{Version: "test"}, "srne_main", reg)

	if component != "button" {
		t.Fatalf("component = %q, want %q", component, "button")
	}
	if got := payload["payload_press"]; got != "1" {
		t.Fatalf("payload_press = %#v, want %q", got, "1")
	}
	if got := payload["device_class"]; got != "restart" {
		t.Fatalf("device_class = %#v, want %q", got, "restart")
	}
	if _, ok := payload["state_topic"]; ok {
		t.Fatal("button payload should not define state_topic")
	}
}

func TestWritableDiscoveryPayloadUsesSelectForRegularEnum(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Device: config.DeviceConfig{Name: "srne-main"},
		MQTT: config.MQTTConfig{
			TopicPrefix: "srne/srne-main",
		},
	}
	reg := registers.Register{
		ID:       "output_source_priority",
		Name:     "Output Source Priority",
		Writable: true,
		Icon:     "mdi:home-switch-outline",
		Entity:   "config",
		Enum: map[int64]string{
			0: "Solar",
			1: "Utility",
		},
	}

	payload, component := writableDiscoveryPayload(cfg, buildinfo.Info{Version: "test"}, "srne_main", reg)

	if component != "select" {
		t.Fatalf("component = %q, want %q", component, "select")
	}
	if got := payload["state_topic"]; got != "srne/srne-main/state/output_source_priority" {
		t.Fatalf("state_topic = %#v, want %q", got, "srne/srne-main/state/output_source_priority")
	}
}
