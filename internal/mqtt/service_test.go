package mqtt

import (
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
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
		ID:          "reset_machine",
		Name:        "Reset Machine",
		Writable:    true,
		WriteOnly:   true,
		ButtonClass: "restart",
		Icon:        "mdi:restart-alert",
		Entity:      "config",
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

func TestWritableDiscoveryPayloadSkipsButtonClassWhenUnset(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Device: config.DeviceConfig{Name: "srne-main"},
		MQTT: config.MQTTConfig{
			TopicPrefix: "srne/srne-main",
		},
	}
	reg := registers.Register{
		ID:        "maintenance_ping",
		Name:      "Maintenance Ping",
		Writable:  true,
		WriteOnly: true,
		Icon:      "mdi:wrench",
		Entity:    "config",
		Enum: map[int64]string{
			1: "Ping",
		},
	}

	payload, component := writableDiscoveryPayload(cfg, buildinfo.Info{Version: "test"}, "srne_main", reg)

	if component != "button" {
		t.Fatalf("component = %q, want %q", component, "button")
	}
	if _, ok := payload["device_class"]; ok {
		t.Fatal("button payload should not define device_class when ButtonClass is empty")
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

func TestPublishCommandStatePublishesOptimisticState(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		MQTT: config.MQTTConfig{
			TopicPrefix: "srne/srne-main",
			Retain:      true,
		},
	}
	reg := registers.Register{
		ID:       "charger_source_priority",
		Name:     "Charger Source Priority",
		Writable: true,
		Count:    1,
		Type:     registers.TypeUint16,
		Group:    registers.GroupSlow,
		Enum: map[int64]string{
			0: "PV Priority",
			1: "Utility Priority",
			2: "Hybrid",
			3: "PV Only",
		},
	}

	fakeClient := &recordingClient{}
	service := &Service{
		state:         state.New(),
		client:        fakeClient,
		lastPublished: make(map[string]string),
	}

	if err := service.publishCommandState(cfg, reg, "Utility Priority"); err != nil {
		t.Fatalf("publishCommandState() error = %v", err)
	}

	if len(fakeClient.publishes) != 1 {
		t.Fatalf("publish count = %d, want 1", len(fakeClient.publishes))
	}
	got := fakeClient.publishes[0]
	if got.topic != "srne/srne-main/state/charger_source_priority" {
		t.Fatalf("topic = %q", got.topic)
	}
	if got.payload != "Utility Priority" {
		t.Fatalf("payload = %q", got.payload)
	}
}

type publishedMessage struct {
	topic    string
	payload  string
	retained bool
}

type recordingClient struct {
	mu        sync.Mutex
	publishes []publishedMessage
}

func (c *recordingClient) IsConnected() bool      { return true }
func (c *recordingClient) IsConnectionOpen() bool { return true }
func (c *recordingClient) Connect() paho.Token    { return immediateToken{} }

func (c *recordingClient) Disconnect(uint) {}

func (c *recordingClient) Publish(topic string, qos byte, retained bool, payload interface{}) paho.Token {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.publishes = append(c.publishes, publishedMessage{
		topic:    topic,
		payload:  payload.(string),
		retained: retained,
	})
	return immediateToken{}
}

func (c *recordingClient) Subscribe(string, byte, paho.MessageHandler) paho.Token {
	return immediateToken{}
}

func (c *recordingClient) SubscribeMultiple(map[string]byte, paho.MessageHandler) paho.Token {
	return immediateToken{}
}

func (c *recordingClient) Unsubscribe(...string) paho.Token     { return immediateToken{} }
func (c *recordingClient) AddRoute(string, paho.MessageHandler) {}
func (c *recordingClient) OptionsReader() paho.ClientOptionsReader {
	return paho.ClientOptionsReader{}
}

type immediateToken struct{}

func (immediateToken) Wait() bool                     { return true }
func (immediateToken) WaitTimeout(time.Duration) bool { return true }
func (immediateToken) Done() <-chan struct{}          { ch := make(chan struct{}); close(ch); return ch }
func (immediateToken) Error() error                   { return nil }
