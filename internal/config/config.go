package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar value")
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(value.Value))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}

	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.String(), nil
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a JSON string: %w", err)
	}

	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}

	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

type Config struct {
	Device  DeviceConfig  `yaml:"device" json:"device"`
	Serial  SerialConfig  `yaml:"serial" json:"serial"`
	Polling PollingConfig `yaml:"polling" json:"polling"`
	MQTT    MQTTConfig    `yaml:"mqtt" json:"mqtt"`
	HTTP    HTTPConfig    `yaml:"http" json:"http"`
	Logging LoggingConfig `yaml:"logging" json:"logging"`
}

type DeviceConfig struct {
	Name    string `yaml:"name" json:"name"`
	SlaveID uint8  `yaml:"slave_id" json:"slaveId"`
}

type SerialConfig struct {
	Port     string   `yaml:"port" json:"port"`
	BaudRate int      `yaml:"baud_rate" json:"baudRate"`
	DataBits int      `yaml:"data_bits" json:"dataBits"`
	Parity   string   `yaml:"parity" json:"parity"`
	StopBits int      `yaml:"stop_bits" json:"stopBits"`
	Timeout  Duration `yaml:"timeout" json:"timeout"`
}

type PollingConfig struct {
	FastInterval   Duration `yaml:"fast_interval" json:"fastInterval"`
	SlowInterval   Duration `yaml:"slow_interval" json:"slowInterval"`
	ReconnectDelay Duration `yaml:"reconnect_delay" json:"reconnectDelay"`
}

type MQTTConfig struct {
	Broker          string `yaml:"broker" json:"broker"`
	Username        string `yaml:"username" json:"username"`
	Password        string `yaml:"password" json:"password"`
	ClientID        string `yaml:"client_id" json:"clientId"`
	TopicPrefix     string `yaml:"topic_prefix" json:"topicPrefix"`
	DiscoveryPrefix string `yaml:"discovery_prefix" json:"discoveryPrefix"`
	Retain          bool   `yaml:"retain" json:"retain"`
}

type HTTPConfig struct {
	Listen string `yaml:"listen" json:"listen"`
}

type LoggingConfig struct {
	Level string `yaml:"level" json:"level"`
}

func Default() Config {
	return Config{
		Device: DeviceConfig{
			Name:    "srne-main",
			SlaveID: 1,
		},
		Serial: SerialConfig{
			Port:     "/dev/ttyUSB0",
			BaudRate: 9600,
			DataBits: 8,
			Parity:   "N",
			StopBits: 1,
			Timeout:  Duration{Duration: 3 * time.Second},
		},
		Polling: PollingConfig{
			FastInterval:   Duration{Duration: 15 * time.Second},
			SlowInterval:   Duration{Duration: 60 * time.Second},
			ReconnectDelay: Duration{Duration: 5 * time.Second},
		},
		MQTT: MQTTConfig{
			Broker:          "tcp://127.0.0.1:1883",
			ClientID:        "srne-hive-nuc",
			TopicPrefix:     "srne/srne-main",
			DiscoveryPrefix: "homeassistant",
			Retain:          true,
		},
		HTTP: HTTPConfig{
			Listen: "127.0.0.1:8080",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func LoadOrCreate(path string) (Config, bool, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, false, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return Config{}, false, err
	}

	cfg = Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, false, err
	}

	return cfg, true, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}

	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Device.Name) == "" {
		return errors.New("device.name is required")
	}

	if c.Device.SlaveID == 0 {
		return errors.New("device.slave_id must be greater than 0")
	}

	if c.Serial.BaudRate <= 0 {
		return errors.New("serial.baud_rate must be greater than 0")
	}

	if c.Serial.DataBits < 5 || c.Serial.DataBits > 8 {
		return errors.New("serial.data_bits must be between 5 and 8")
	}

	if c.Serial.StopBits < 1 || c.Serial.StopBits > 2 {
		return errors.New("serial.stop_bits must be 1 or 2")
	}

	if parity := strings.ToUpper(strings.TrimSpace(c.Serial.Parity)); parity != "N" && parity != "E" && parity != "O" {
		return errors.New("serial.parity must be N, E, or O")
	}

	if c.Serial.Timeout.Duration <= 0 {
		return errors.New("serial.timeout must be greater than 0")
	}

	if c.Polling.FastInterval.Duration <= 0 {
		return errors.New("polling.fast_interval must be greater than 0")
	}

	if c.Polling.SlowInterval.Duration <= 0 {
		return errors.New("polling.slow_interval must be greater than 0")
	}

	if c.Polling.ReconnectDelay.Duration <= 0 {
		return errors.New("polling.reconnect_delay must be greater than 0")
	}

	if strings.TrimSpace(c.HTTP.Listen) == "" {
		return errors.New("http.listen is required")
	}

	if level := strings.ToLower(strings.TrimSpace(c.Logging.Level)); level == "" {
		return errors.New("logging.level is required")
	}

	return nil
}
