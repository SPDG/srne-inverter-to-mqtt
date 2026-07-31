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
	"github.com/tomasz/srne-inverter-to-mqtt/internal/stormcharge"
)

type ConfigProvider interface {
	GetConfig() config.Config
	WriteRegister(id string, value any) error
	GetStormChargeStatus() stormcharge.Status
	UpdateStormChargeSettings(stormcharge.Settings) error
	StartStormCharge(stormcharge.Settings) error
	CancelStormCharge() error
}

type Service struct {
	provider ConfigProvider
	state    *state.Store
	build    buildinfo.Info

	mu             sync.Mutex
	key            string
	client         paho.Client
	discoveryKey   string
	lastPublished  map[string]string
	unhealthySince time.Time
}

const mqttClientResetAfter = 2 * time.Minute

func NewService(provider ConfigProvider, runtimeState *state.Store, build buildinfo.Info) *Service {
	return &Service{
		provider:      provider,
		state:         runtimeState,
		build:         build,
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
		stalledFor := s.markUnhealthy(time.Now().UTC())
		if stalledFor >= mqttClientResetAfter {
			s.resetClient(fmt.Sprintf("connection not open for %s", stalledFor.Round(time.Second)), client)
		}
		s.state.SetServiceStatus("mqtt", "connecting", false, "", time.Time{})
		return
	}

	if err := s.publishAvailability(cfg, "online"); err != nil {
		s.recordError("publish availability", err)
		return
	}

	if s.discoveryKey != mqttKey(cfg) {
		if err := s.publishDiscovery(cfg); err != nil {
			s.recordError("publish discovery", err)
			return
		}
		s.discoveryKey = mqttKey(cfg)
	}

	if err := s.publishTelemetry(cfg); err != nil {
		s.recordError("publish telemetry", err)
		return
	}
	if err := s.publishStormCharge(cfg); err != nil {
		s.recordError("publish storm charge", err)
		return
	}

	s.markHealthy()
	s.state.SetServiceStatus("mqtt", "connected", true, "", time.Now().UTC())
}

func (s *Service) ensureClient(cfg config.Config) paho.Client {
	key := mqttKey(cfg)

	s.mu.Lock()
	if s.client != nil && s.key == key {
		client := s.client
		s.mu.Unlock()
		return client
	}

	oldClient := s.client
	s.client = nil
	s.key = ""
	s.discoveryKey = ""
	s.lastPublished = make(map[string]string)
	s.unhealthySince = time.Time{}
	s.mu.Unlock()

	if oldClient != nil {
		oldClient.Disconnect(250)
	}

	client := paho.NewClient(s.clientOptions(cfg))

	s.mu.Lock()
	s.client = client
	s.key = key
	s.discoveryKey = ""
	s.lastPublished = make(map[string]string)
	s.unhealthySince = time.Now().UTC()
	s.mu.Unlock()

	token := client.Connect()
	if ok := token.WaitTimeout(5 * time.Second); !ok {
		err := fmt.Errorf("mqtt connect timeout")
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
		s.resetClient(err.Error(), client)
		return nil
	}
	if err := token.Error(); err != nil {
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
		s.resetClient(fmt.Sprintf("connect failed: %v", err), client)
		return nil
	}

	return client
}

func (s *Service) clientOptions(cfg config.Config) *paho.ClientOptions {
	opts := paho.NewClientOptions().
		AddBroker(cfg.MQTT.Broker).
		SetClientID(cfg.MQTT.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(false).
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
			s.markUnhealthy(time.Now().UTC())
			go s.resetClient(fmt.Sprintf("subscribe failed: %v", err), client)
			return
		}
		s.markHealthy()
		s.state.SetServiceStatus("mqtt", "connected", true, "", time.Now().UTC())
	}
	opts.OnConnectionLost = func(_ paho.Client, err error) {
		s.markUnhealthy(time.Now().UTC())
		s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
	}

	return opts
}

func (s *Service) publishAvailability(cfg config.Config, payload string) error {
	return s.publish(cfg, availabilityTopic(cfg), payload, true)
}

func (s *Service) publishDiscovery(cfg config.Config) error {
	deviceID := sanitizeID(cfg.Device.Name)
	for _, topic := range staleDiscoveryTopics(cfg, deviceID) {
		if err := s.publish(cfg, topic, "", true); err != nil {
			return err
		}
	}

	for _, reg := range catalogForConfig(cfg) {
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
	inverterType, _ := cfg.Device.NormalizedInverterType()
	if inverterType == registers.InverterTypeSPIH3P {
		if err := s.publishStormChargeDiscovery(cfg, deviceID); err != nil {
			return err
		}
	} else {
		if err := s.clearStormChargeDiscovery(cfg, deviceID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) publishStormChargeDiscovery(cfg config.Config, deviceID string) error {
	device := map[string]any{
		"identifiers":  []string{deviceID},
		"name":         cfg.Device.Name,
		"manufacturer": "SRNE",
		"model":        "SRNE Inverter",
		"sw_version":   s.build.Version,
	}
	entities := []struct {
		component string
		objectID  string
		payload   map[string]any
	}{
		{
			component: "switch",
			objectID:  "storm_charge",
			payload: map[string]any{
				"name": "Storm Charge", "icon": "mdi:weather-lightning",
				"state_topic": stateTopic(cfg, "storm_charge_active"), "command_topic": commandTopic(cfg, "storm_charge"),
				"payload_on": "ON", "payload_off": "OFF", "state_on": "ON", "state_off": "OFF",
				"entity_category": "config",
			},
		},
		{
			component: "number",
			objectID:  "storm_charge_target_soc",
			payload: map[string]any{
				"name": "Storm Charge Target SOC", "icon": "mdi:battery-charging-100",
				"state_topic": stateTopic(cfg, "storm_charge_target_soc"), "command_topic": commandTopic(cfg, "storm_charge_target_soc"),
				"min": 50, "max": 100, "step": 1, "mode": "box", "unit_of_measurement": "%",
				"entity_category": "config",
			},
		},
		{
			component: "number",
			objectID:  "storm_charge_max_current",
			payload: map[string]any{
				"name": "Storm Charge Maximum Current", "icon": "mdi:current-dc",
				"state_topic": stateTopic(cfg, "storm_charge_max_current"), "command_topic": commandTopic(cfg, "storm_charge_max_current"),
				"min": 1, "max": 120, "step": 1, "mode": "box", "unit_of_measurement": "A",
				"device_class": "current", "entity_category": "config",
			},
		},
		{
			component: "number",
			objectID:  "storm_charge_timeout_minutes",
			payload: map[string]any{
				"name": "Storm Charge Timeout", "icon": "mdi:timer-alert-outline",
				"state_topic": stateTopic(cfg, "storm_charge_timeout_minutes"), "command_topic": commandTopic(cfg, "storm_charge_timeout_minutes"),
				"min": 5, "max": 1440, "step": 5, "mode": "box", "unit_of_measurement": "min",
				"entity_category": "config",
			},
		},
		{
			component: "sensor",
			objectID:  "storm_charge_status",
			payload: map[string]any{
				"name": "Storm Charge Status", "icon": "mdi:weather-lightning-rainy", "state_topic": stateTopic(cfg, "storm_charge_status"),
			},
		},
		{
			component: "sensor",
			objectID:  "storm_charge_estimated_power",
			payload: map[string]any{
				"name": "Storm Charge Estimated Power", "icon": "mdi:flash",
				"state_topic": stateTopic(cfg, "storm_charge_estimated_power"), "unit_of_measurement": "W",
				"device_class": "power", "state_class": "measurement",
			},
		},
		{
			component: "sensor",
			objectID:  "storm_charge_deadline",
			payload: map[string]any{
				"name": "Storm Charge Deadline", "icon": "mdi:calendar-clock",
				"state_topic": stateTopic(cfg, "storm_charge_deadline"), "device_class": "timestamp",
			},
		},
		{
			component: "sensor",
			objectID:  "storm_charge_remaining",
			payload: map[string]any{
				"name": "Storm Charge Remaining", "icon": "mdi:timer-sand", "state_topic": stateTopic(cfg, "storm_charge_remaining"),
			},
		},
		{
			component: "sensor",
			objectID:  "storm_charge_reason",
			payload: map[string]any{
				"name": "Storm Charge Last Result", "icon": "mdi:information-outline",
				"state_topic": stateTopic(cfg, "storm_charge_reason"), "entity_category": "diagnostic",
			},
		},
	}

	for _, entity := range entities {
		entity.payload["unique_id"] = fmt.Sprintf("%s_%s", deviceID, entity.objectID)
		entity.payload["availability_topic"] = availabilityTopic(cfg)
		entity.payload["device"] = device
		body, err := json.Marshal(entity.payload)
		if err != nil {
			return err
		}
		if err := s.publish(cfg, discoveryTopic(cfg, entity.component, deviceID, entity.objectID), string(body), true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) clearStormChargeDiscovery(cfg config.Config, deviceID string) error {
	for _, entity := range []struct {
		component string
		objectID  string
	}{
		{component: "switch", objectID: "storm_charge"},
		{component: "number", objectID: "storm_charge_target_soc"},
		{component: "number", objectID: "storm_charge_max_current"},
		{component: "number", objectID: "storm_charge_timeout_minutes"},
		{component: "sensor", objectID: "storm_charge_status"},
		{component: "sensor", objectID: "storm_charge_estimated_power"},
		{component: "sensor", objectID: "storm_charge_deadline"},
		{component: "sensor", objectID: "storm_charge_remaining"},
		{component: "sensor", objectID: "storm_charge_reason"},
	} {
		if err := s.publish(cfg, discoveryTopic(cfg, entity.component, deviceID, entity.objectID), "", true); err != nil {
			return err
		}
	}
	return nil
}

func staleDiscoveryTopics(cfg config.Config, deviceID string) []string {
	inverterType, err := cfg.Device.NormalizedInverterType()
	if err != nil || inverterType != registers.InverterTypeSPIH3P {
		return nil
	}

	controls := []struct {
		id        string
		component string
	}{
		{id: "grid_operating_mode", component: "select"},
		{id: "on_grid_max_power", component: "number"},
		{id: "zero_export_power", component: "number"},
	}

	topics := make([]string, 0, len(controls)*2)
	for _, control := range controls {
		topics = append(topics,
			discoveryTopic(cfg, "sensor", deviceID, control.id),
			discoveryTopic(cfg, control.component, deviceID, control.id+"_control"),
		)
	}
	return topics
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

func (s *Service) publishStormCharge(cfg config.Config) error {
	status := s.provider.GetStormChargeStatus()
	values := map[string]string{
		"storm_charge_active":          map[bool]string{true: "ON", false: "OFF"}[status.Active],
		"storm_charge_status":          status.Phase,
		"storm_charge_target_soc":      strconv.Itoa(status.Settings.TargetSOC),
		"storm_charge_max_current":     strconv.FormatFloat(status.Settings.MaxCurrentA, 'f', 1, 64),
		"storm_charge_timeout_minutes": strconv.FormatFloat(status.Settings.Timeout.Duration.Minutes(), 'f', 0, 64),
		"storm_charge_estimated_power": strconv.Itoa(status.EstimatedPowerWatts),
		"storm_charge_remaining":       status.Remaining,
		"storm_charge_reason":          status.Reason,
	}
	if status.Deadline != nil {
		values["storm_charge_deadline"] = status.Deadline.UTC().Format(time.RFC3339)
	} else {
		values["storm_charge_deadline"] = ""
	}

	for id, payload := range values {
		topic := stateTopic(cfg, id)
		if last, ok := s.getLastPublished(topic); ok && last == payload {
			continue
		}
		if err := s.publish(cfg, topic, payload, cfg.MQTT.Retain); err != nil {
			return err
		}
		s.setLastPublished(topic, payload)
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
	subscriptions := make(map[string]byte)
	routes := make(map[string]registers.Register)

	for _, reg := range catalogForConfig(cfg) {
		if !reg.Writable {
			continue
		}

		topic := commandTopic(cfg, reg.ID)
		subscriptions[topic] = 0
		routes[topic] = reg
	}
	inverterType, _ := cfg.Device.NormalizedInverterType()
	if inverterType == registers.InverterTypeSPIH3P {
		for _, id := range []string{
			"storm_charge",
			"storm_charge_target_soc",
			"storm_charge_max_current",
			"storm_charge_timeout_minutes",
		} {
			subscriptions[commandTopic(cfg, id)] = 0
		}
	}

	if len(subscriptions) == 0 {
		return nil
	}

	token := client.SubscribeMultiple(subscriptions, func(_ paho.Client, msg paho.Message) {
		reg, ok := routes[msg.Topic()]
		if ok {
			s.handleCommand(reg)(client, msg)
			return
		}
		if s.handleStormChargeCommand(cfg, msg) {
			return
		}
		log.Printf("mqtt command ignored topic=%s reason=unknown-topic", msg.Topic())
	})
	if ok := token.WaitTimeout(10 * time.Second); !ok {
		return fmt.Errorf("mqtt subscribe timeout for command topics")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt subscribe command topics: %w", err)
	}

	return nil
}

func (s *Service) handleStormChargeCommand(cfg config.Config, msg paho.Message) bool {
	payload := strings.TrimSpace(string(msg.Payload()))
	if payload == "" {
		return true
	}

	var err error
	switch msg.Topic() {
	case commandTopic(cfg, "storm_charge"):
		switch strings.ToUpper(payload) {
		case "ON":
			err = s.provider.StartStormCharge(s.provider.GetStormChargeStatus().Settings)
		case "OFF":
			err = s.provider.CancelStormCharge()
		default:
			err = fmt.Errorf("storm_charge accepts ON or OFF")
		}
	case commandTopic(cfg, "storm_charge_target_soc"):
		settings := s.provider.GetStormChargeStatus().Settings
		value, parseErr := strconv.Atoi(payload)
		if parseErr != nil {
			err = fmt.Errorf("invalid target SOC %q", payload)
		} else {
			settings.TargetSOC = value
			err = s.provider.UpdateStormChargeSettings(settings)
		}
	case commandTopic(cfg, "storm_charge_max_current"):
		settings := s.provider.GetStormChargeStatus().Settings
		value, parseErr := strconv.ParseFloat(payload, 64)
		if parseErr != nil {
			err = fmt.Errorf("invalid maximum current %q", payload)
		} else {
			settings.MaxCurrentA = value
			err = s.provider.UpdateStormChargeSettings(settings)
		}
	case commandTopic(cfg, "storm_charge_timeout_minutes"):
		settings := s.provider.GetStormChargeStatus().Settings
		value, parseErr := strconv.ParseFloat(payload, 64)
		if parseErr != nil {
			err = fmt.Errorf("invalid timeout %q", payload)
		} else {
			settings.Timeout = config.Duration{Duration: time.Duration(value * float64(time.Minute))}
			err = s.provider.UpdateStormChargeSettings(settings)
		}
	default:
		return false
	}

	if err != nil {
		log.Printf("mqtt storm charge command failed topic=%s payload=%q err=%v", msg.Topic(), payload, err)
		return true
	}
	if publishErr := s.publishStormCharge(s.provider.GetConfig()); publishErr != nil {
		log.Printf("mqtt storm charge state publish failed: %v", publishErr)
	}
	return true
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
		if err := s.publishCommandState(cfg, reg, payload); err != nil {
			log.Printf("mqtt state publish after command failed id=%s err=%v", reg.ID, err)
			return
		}

		log.Printf("mqtt command applied id=%s payload=%q", reg.ID, payload)
	}
}

func (s *Service) publishCommandState(cfg config.Config, reg registers.Register, payload string) error {
	if reg.WriteOnly {
		return nil
	}

	encoded, err := reg.EncodeWrite(payload)
	if err == nil && reg.Count == 1 {
		decoded, decodeErr := reg.Decode([]uint16{encoded}, time.Now().UTC())
		if decodeErr == nil {
			topic := stateTopic(cfg, reg.ID)
			if err := s.publish(cfg, topic, decoded.Rendered, cfg.MQTT.Retain); err != nil {
				return err
			}
			s.setLastPublished(topic, decoded.Rendered)
			return nil
		}
	}

	return s.publishCurrentValue(cfg, reg.ID)
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
	s.unhealthySince = time.Time{}
	s.mu.Unlock()

	if client != nil && client.IsConnectionOpen() {
		token := client.Publish(availabilityTopic(cfg), 0, true, "offline")
		token.WaitTimeout(3 * time.Second)
		client.Disconnect(250)
	}
}

func (s *Service) recordError(context string, err error) {
	stalledFor := s.markUnhealthy(time.Now().UTC())
	s.state.SetServiceStatus("mqtt", "error", false, err.Error(), time.Time{})
	if stalledFor < mqttClientResetAfter {
		return
	}

	s.resetClient(fmt.Sprintf("%s failed for %s: %v", context, stalledFor.Round(time.Second), err), nil)
}

func (s *Service) markHealthy() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unhealthySince = time.Time{}
}

func (s *Service) markUnhealthy(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.unhealthySince.IsZero() {
		s.unhealthySince = now
		return 0
	}

	return now.Sub(s.unhealthySince)
}

func (s *Service) resetClient(reason string, expected paho.Client) {
	s.mu.Lock()
	client := s.client
	if expected != nil && client != expected {
		s.mu.Unlock()
		expected.Disconnect(250)
		return
	}

	s.client = nil
	s.key = ""
	s.discoveryKey = ""
	s.lastPublished = make(map[string]string)
	s.unhealthySince = time.Now().UTC()
	s.mu.Unlock()

	if client != nil {
		client.Disconnect(250)
	}
	log.Printf("mqtt client reset reason=%q", reason)
}

func mqttKey(cfg config.Config) string {
	inverterType, _ := cfg.Device.NormalizedInverterType()
	return fmt.Sprintf("%s|%s|%s|%s|%s|%t|%s",
		cfg.MQTT.Broker,
		cfg.MQTT.ClientID,
		cfg.MQTT.Username,
		cfg.MQTT.TopicPrefix,
		cfg.MQTT.DiscoveryPrefix,
		cfg.MQTT.Retain,
		inverterType,
	)
}

func catalogForConfig(cfg config.Config) []registers.Register {
	inverterType, err := cfg.Device.NormalizedInverterType()
	if err != nil {
		inverterType = registers.InverterTypeSinglePhase
	}
	return registers.CatalogForInverterType(inverterType)
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
		if reg.ButtonClass != "" {
			payload["device_class"] = reg.ButtonClass
		}
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
