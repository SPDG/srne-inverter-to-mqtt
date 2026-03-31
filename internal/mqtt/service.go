package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

type ConfigProvider interface {
	GetConfig() config.Config
	WriteRegister(id string, value any) error
}

type Service struct {
	provider ConfigProvider
	state    *state.Store
	build    buildinfo.Info
	catalog  []registers.Register

	mu            sync.Mutex
	key           string
	client        paho.Client
	discoveryKey  string
	lastPublished map[string]string
}

func NewService(provider ConfigProvider, runtimeState *state.Store, build buildinfo.Info) *Service {
	return &Service{
		provider:      provider,
		state:         runtimeState,
		build:         build,
		catalog:       registers.Catalog(),
		lastPublished: make(map[string]string),
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.state.SetServiceStatus("mqtt", "starting", false, "", time.Time{})

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	s.sync()

	for {
		select {
		case <-ctx.Done():
			s.disconnect()
			return nil
		case <-ticker.C:
			s.sync()
		}
	}
}

func (s *Service) sync() {
	cfg := s.provider.GetConfig()
	if strings.TrimSpace(cfg.MQTT.Broker) == "" {
		s.disconnect()
		s.state.SetServiceStatus("mqtt", "disabled", false, "mqtt broker is empty", time.Time{})
		return
	}

	client := s.ensureClient(cfg)
	if client == nil {
		return
	}

	if !client.IsConnectionOpen() {
		s.state.SetServiceStatus("mqtt", "connecting", false, "", time.Time{})
		return
	}

	if err := s.publishAvailability(cfg, "online"); err != nil {
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
		return
	}

	if s.discoveryKey != mqttKey(cfg) {
		if err := s.publishDiscovery(cfg); err != nil {
			s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
			return
		}
		s.discoveryKey = mqttKey(cfg)
	}

	if err := s.publishTelemetry(cfg); err != nil {
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
		return
	}

	s.state.SetServiceStatus("mqtt", "connected", true, "", time.Now().UTC())
}

func (s *Service) ensureClient(cfg config.Config) paho.Client {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := mqttKey(cfg)
	if s.client != nil && s.key == key {
		return s.client
	}

	if s.client != nil {
		s.client.Disconnect(250)
	}

	opts := paho.NewClientOptions().
		AddBroker(cfg.MQTT.Broker).
		SetClientID(cfg.MQTT.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectTimeout(3 * time.Second).
		SetConnectRetryInterval(5 * time.Second).
		SetOrderMatters(false).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetCleanSession(true).
		SetDefaultPublishHandler(func(_ paho.Client, _ paho.Message) {})

	if cfg.MQTT.Username != "" {
		opts.SetUsername(cfg.MQTT.Username)
	}
	if cfg.MQTT.Password != "" {
		opts.SetPassword(cfg.MQTT.Password)
	}

	opts.OnConnect = func(client paho.Client) {
		if err := s.subscribeCommands(client, cfg); err != nil {
			log.Printf("mqtt subscribe failed: %v", err)
			s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
			return
		}
		s.state.SetServiceStatus("mqtt", "connected", true, "", time.Now().UTC())
	}
	opts.OnConnectionLost = func(_ paho.Client, err error) {
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
	}

	client := paho.NewClient(opts)
	token := client.Connect()
	if ok := token.WaitTimeout(5 * time.Second); !ok {
		s.state.SetServiceStatus("mqtt", "error", false, "mqtt connect timeout", time.Time{})
		return nil
	}
	if err := token.Error(); err != nil {
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
		return nil
	}

	s.client = client
	s.key = key
	s.discoveryKey = ""
	s.lastPublished = make(map[string]string)
	return s.client
}

func (s *Service) publishAvailability(cfg config.Config, payload string) error {
	return s.publish(cfg, availabilityTopic(cfg), payload, true)
}

func (s *Service) publishDiscovery(cfg config.Config) error {
	deviceID := sanitizeID(cfg.Device.Name)
	for _, reg := range s.catalog {
		sensorConfigTopic := discoveryTopic(cfg, "sensor", deviceID, reg.ID)
		if reg.WriteOnly {
			if err := s.publish(cfg, sensorConfigTopic, "", true); err != nil {
				return err
			}
		} else {
			payload := map[string]any{
				"name":               reg.Name,
				"unique_id":          fmt.Sprintf("%s_%s", deviceID, reg.ID),
				"state_topic":        stateTopic(cfg, reg.ID),
				"availability_topic": availabilityTopic(cfg),
				"icon":               reg.Icon,
				"device": map[string]any{
					"identifiers":  []string{deviceID},
					"name":         cfg.Device.Name,
					"manufacturer": "SRNE",
					"model":        "SRNE Inverter",
					"sw_version":   s.build.Version,
				},
			}

			if reg.Unit != "" {
				payload["unit_of_measurement"] = reg.Unit
			}
			if reg.DeviceClass != "" {
				payload["device_class"] = reg.DeviceClass
			}
			if reg.StateClass != "" {
				payload["state_class"] = reg.StateClass
			}
			if reg.EntityCategory != "" {
				payload["entity_category"] = reg.EntityCategory
			}

			body, err := json.Marshal(payload)
			if err != nil {
				return err
			}

			if err := s.publish(cfg, sensorConfigTopic, string(body), true); err != nil {
				return err
			}
		}

		if !reg.Writable {
			continue
		}

		controlPayload, component := writableDiscoveryPayload(cfg, s.build, deviceID, reg)
		body, err := json.Marshal(controlPayload)
		if err != nil {
			return err
		}

		if component == "button" {
			// Remove stale select/number entities after changing a write-only action to a button.
			for _, legacyComponent := range []string{"select", "number"} {
				if err := s.publish(cfg, discoveryTopic(cfg, legacyComponent, deviceID, controlObjectID(reg)), "", true); err != nil {
					return err
				}
			}
		}

		controlTopic := discoveryTopic(cfg, component, deviceID, controlObjectID(reg))
		if err := s.publish(cfg, controlTopic, string(body), true); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) publishTelemetry(cfg config.Config) error {
	snapshot := s.state.Snapshot()
	for _, value := range snapshot.Telemetry {
		topic := stateTopic(cfg, value.ID)
		if last, ok := s.getLastPublished(topic); ok && last == value.Rendered {
			continue
		}

		if err := s.publish(cfg, topic, value.Rendered, cfg.MQTT.Retain); err != nil {
			return err
		}
		s.setLastPublished(topic, value.Rendered)
	}

	return nil
}

func (s *Service) publish(cfg config.Config, topic, payload string, retained bool) error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	if client == nil {
		return fmt.Errorf("mqtt client is not initialized")
	}

	token := client.Publish(topic, 0, retained, payload)
	if ok := token.WaitTimeout(10 * time.Second); !ok {
		return fmt.Errorf("mqtt publish timeout for %s", topic)
	}
	return token.Error()
}

func (s *Service) subscribeCommands(client paho.Client, cfg config.Config) error {
	for _, reg := range s.catalog {
		if !reg.Writable {
			continue
		}

		topic := commandTopic(cfg, reg.ID)
		token := client.Subscribe(topic, 0, s.handleCommand(reg))
		if ok := token.WaitTimeout(10 * time.Second); !ok {
			return fmt.Errorf("mqtt subscribe timeout for %s", topic)
		}
		if err := token.Error(); err != nil {
			return fmt.Errorf("mqtt subscribe %s: %w", topic, err)
		}
	}

	return nil
}

func (s *Service) handleCommand(reg registers.Register) paho.MessageHandler {
	return func(_ paho.Client, msg paho.Message) {
		payload := strings.TrimSpace(string(msg.Payload()))
		if payload == "" {
			log.Printf("mqtt command ignored id=%s reason=empty-payload", reg.ID)
			return
		}

		log.Printf("mqtt command received id=%s payload=%q", reg.ID, payload)
		if err := s.provider.WriteRegister(reg.ID, payload); err != nil {
			log.Printf("mqtt command failed id=%s payload=%q err=%v", reg.ID, payload, err)
			return
		}

		cfg := s.provider.GetConfig()
		if err := s.publishCurrentValue(cfg, reg.ID); err != nil {
			log.Printf("mqtt state publish after command failed id=%s err=%v", reg.ID, err)
			return
		}

		log.Printf("mqtt command applied id=%s payload=%q", reg.ID, payload)
	}
}

func (s *Service) publishCurrentValue(cfg config.Config, registerID string) error {
	snapshot := s.state.Snapshot()
	for _, value := range snapshot.Telemetry {
		if value.ID != registerID {
			continue
		}

		topic := stateTopic(cfg, value.ID)
		if err := s.publish(cfg, topic, value.Rendered, cfg.MQTT.Retain); err != nil {
			return err
		}
		s.setLastPublished(topic, value.Rendered)
		return nil
	}

	return nil
}

func (s *Service) disconnect() {
	cfg := s.provider.GetConfig()

	s.mu.Lock()
	client := s.client
	s.client = nil
	s.key = ""
	s.discoveryKey = ""
	s.lastPublished = make(map[string]string)
	s.mu.Unlock()

	if client != nil && client.IsConnectionOpen() {
		token := client.Publish(availabilityTopic(cfg), 0, true, "offline")
		token.WaitTimeout(3 * time.Second)
		client.Disconnect(250)
	}
}

func mqttKey(cfg config.Config) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%t",
		cfg.MQTT.Broker,
		cfg.MQTT.ClientID,
		cfg.MQTT.Username,
		cfg.MQTT.TopicPrefix,
		cfg.MQTT.DiscoveryPrefix,
		cfg.MQTT.Retain,
	)
}

func availabilityTopic(cfg config.Config) string {
	return fmt.Sprintf("%s/availability", strings.TrimSuffix(cfg.MQTT.TopicPrefix, "/"))
}

func stateTopic(cfg config.Config, entityID string) string {
	return fmt.Sprintf("%s/state/%s", strings.TrimSuffix(cfg.MQTT.TopicPrefix, "/"), entityID)
}

func commandTopic(cfg config.Config, entityID string) string {
	return fmt.Sprintf("%s/command/%s", strings.TrimSuffix(cfg.MQTT.TopicPrefix, "/"), entityID)
}

func discoveryTopic(cfg config.Config, component, deviceID, objectID string) string {
	return fmt.Sprintf("%s/%s/%s/%s/config", cfg.MQTT.DiscoveryPrefix, component, deviceID, objectID)
}

func (s *Service) getLastPublished(topic string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.lastPublished[topic]
	return value, ok
}

func (s *Service) setLastPublished(topic, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPublished[topic] = value
}

func controlObjectID(reg registers.Register) string {
	return reg.ID + "_control"
}

func writableDiscoveryPayload(cfg config.Config, build buildinfo.Info, deviceID string, reg registers.Register) (map[string]any, string) {
	payload := map[string]any{
		"name":               reg.Name,
		"unique_id":          fmt.Sprintf("%s_%s_control", deviceID, reg.ID),
		"command_topic":      commandTopic(cfg, reg.ID),
		"availability_topic": availabilityTopic(cfg),
		"icon":               reg.Icon,
		"device": map[string]any{
			"identifiers":  []string{deviceID},
			"name":         cfg.Device.Name,
			"manufacturer": "SRNE",
			"model":        "SRNE Inverter",
			"sw_version":   build.Version,
		},
	}

	if reg.EntityCategory != "" {
		payload["entity_category"] = reg.EntityCategory
	} else if reg.Entity == "config" {
		payload["entity_category"] = "config"
	}

	if reg.WriteOnly && len(reg.Enum) == 1 {
		if raw, ok := singleEnumRaw(reg.Enum); ok {
			payload["payload_press"] = strconv.FormatInt(raw, 10)
		}
		payload["device_class"] = "restart"
		return payload, "button"
	}

	payload["state_topic"] = stateTopic(cfg, reg.ID)

	if len(reg.Enum) > 0 {
		payload["options"] = sortedEnumOptions(reg.Enum)
		return payload, "select"
	}

	if reg.Unit != "" {
		payload["unit_of_measurement"] = reg.Unit
	}
	if reg.DeviceClass != "" {
		payload["device_class"] = reg.DeviceClass
	}
	payload["min"] = reg.WriteMin
	payload["max"] = reg.WriteMax
	payload["step"] = reg.WriteStep
	payload["mode"] = "box"
	return payload, "number"
}

func singleEnumRaw(mapping map[int64]string) (int64, bool) {
	if len(mapping) != 1 {
		return 0, false
	}
	for raw := range mapping {
		return raw, true
	}
	return 0, false
}

func sortedEnumOptions(mapping map[int64]string) []string {
	keys := make([]int64, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, mapping[key])
	}
	return labels
}

func sanitizeID(input string) string {
	builder := strings.Builder{}
	for _, r := range strings.ToLower(input) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}

	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "srne_device"
	}
	return result
}
