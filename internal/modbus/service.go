package modbus

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	gomodbus "github.com/goburrow/modbus"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/registers"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
)

type ConfigProvider interface {
	GetConfig() config.Config
}

type Service struct {
	provider ConfigProvider
	state    *state.Store

	mu      sync.Mutex
	key     string
	handler *gomodbus.RTUClientHandler
	client  gomodbus.Client
}

const unlockRegisterAddress = 0xE203

func NewService(provider ConfigProvider, runtimeState *state.Store) *Service {
	return &Service{
		provider: provider,
		state:    runtimeState,
	}
}

func (s *Service) Run(ctx context.Context) error {
	s.state.SetServiceStatus("modbus", "starting", false, "", time.Time{})
	s.pollScheduler(ctx)
	s.close()
	return nil
}

func (s *Service) WriteRegister(id string, value any) error {
	reg, ok := registers.FindByID(id)
	if !ok {
		return fmt.Errorf("unknown register %q", id)
	}
	if !reg.Writable {
		return fmt.Errorf("register %q is not writable", id)
	}

	cfg := s.provider.GetConfig()
	if strings.TrimSpace(cfg.Serial.Port) == "" {
		return fmt.Errorf("serial port is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	client, err := s.ensureClientLocked(cfg)
	if err != nil {
		s.state.SetServiceStatus("modbus", "error", false, err.Error(), time.Time{})
		return err
	}

	encoded, err := reg.EncodeWrite(value)
	if err != nil {
		return err
	}

	log.Printf("modbus write requested id=%s address=0x%04X raw=%d value=%v", reg.ID, reg.Address, encoded, value)
	s.unlockBeforeWriteLocked(client)

	if _, err := client.WriteSingleRegister(reg.Address, encoded); err != nil {
		log.Printf("modbus write failed id=%s address=0x%04X raw=%d err=%v", reg.ID, reg.Address, encoded, err)
		verified, verifyErr := s.verifyWriteLocked(cfg, reg, encoded)
		if !verified {
			if verifyErr != nil {
				log.Printf("modbus write verification failed id=%s address=0x%04X raw=%d err=%v", reg.ID, reg.Address, encoded, verifyErr)
				s.state.SetServiceStatus("modbus", "error", false, verifyErr.Error(), time.Time{})
			} else {
				s.state.SetServiceStatus("modbus", "error", false, err.Error(), time.Time{})
			}
			return fmt.Errorf("write 0x%04X: %w", reg.Address, err)
		}
		log.Printf("modbus write recovered by verification id=%s address=0x%04X raw=%d", reg.ID, reg.Address, encoded)
	}

	now := time.Now().UTC()
	optimistic, err := reg.Decode([]uint16{encoded}, now)
	if err != nil {
		return err
	}
	s.state.UpsertTelemetry([]registers.DecodedValue{optimistic})

	payload, err := s.readHoldingWithRetryLocked(client, cfg, reg.Address, reg.Count)
	if err == nil {
		words, decodeErr := registers.WordsFromBytes(payload, reg.Count)
		if decodeErr == nil {
			decoded, decodeErr := reg.Decode(words[:reg.Count], now)
			if decodeErr == nil {
				s.state.UpsertTelemetry([]registers.DecodedValue{decoded})
				log.Printf("modbus write confirmed by readback id=%s address=0x%04X rendered=%s", reg.ID, reg.Address, decoded.Rendered)
			}
		}
	}

	s.state.SetServiceStatus("modbus", "connected", true, "", now)
	return nil
}

func (s *Service) unlockBeforeWriteLocked(client gomodbus.Client) {
	if _, err := client.WriteSingleRegister(unlockRegisterAddress, 0); err != nil {
		if isAcknowledgeException(err) {
			log.Printf("modbus unlock acknowledged address=0x%04X", unlockRegisterAddress)
		} else {
			log.Printf("modbus unlock write failed address=0x%04X err=%v", unlockRegisterAddress, err)
		}
	}
	time.Sleep(100 * time.Millisecond)
}

func isAcknowledgeException(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "exception '5'")
}

func (s *Service) pollScheduler(ctx context.Context) {
	cfg := s.provider.GetConfig()
	now := time.Now()
	nextFast := now
	nextSlow := now.Add(initialSlowOffset(cfg))

	for {
		now = time.Now()
		ran := false

		if !nextFast.After(now) {
			s.pollGroup(registers.GroupFast)
			nextFast = time.Now().Add(s.provider.GetConfig().Polling.FastInterval.Duration)
			ran = true
		}

		now = time.Now()
		if !nextSlow.After(now) {
			s.pollGroup(registers.GroupSlow)
			nextSlow = time.Now().Add(s.provider.GetConfig().Polling.SlowInterval.Duration)
			ran = true
		}

		if ran {
			continue
		}

		waitFor := time.Until(nextFast)
		if slowWait := time.Until(nextSlow); slowWait < waitFor {
			waitFor = slowWait
		}
		if waitFor < 0 {
			waitFor = 0
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitFor):
		}
	}
}

func initialSlowOffset(cfg config.Config) time.Duration {
	offset := cfg.Polling.FastInterval.Duration / 2
	if offset <= 0 {
		return time.Second
	}
	return offset
}

func (s *Service) pollGroup(group registers.PollGroup) {
	cfg := s.provider.GetConfig()
	now := time.Now().UTC()

	if strings.TrimSpace(cfg.Serial.Port) == "" {
		s.state.SetServiceStatus("modbus", "disabled", false, "serial port is empty", time.Time{})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeLocked()
	defer s.closeLocked()

	plan := s.readPlanForGroup(group)
	values := make([]registers.DecodedValue, 0, len(registers.ByGroup(group)))
	errors := make([]string, 0)

	for idx, readRange := range plan {
		client, err := s.ensureClientLocked(cfg)
		if err != nil {
			errors = append(errors, fmt.Sprintf("0x%04X/%d: %v", readRange.Start, readRange.Count, err))
			if idx < len(plan)-1 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		payload, err := s.readHoldingWithRetryLocked(client, cfg, readRange.Start, readRange.Count)
		s.closeLocked()
		if err != nil {
			errors = append(errors, fmt.Sprintf("0x%04X/%d: %v", readRange.Start, readRange.Count, err))
			if idx < len(plan)-1 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		words, err := registers.WordsFromBytes(payload, readRange.Count)
		if err != nil {
			errors = append(errors, fmt.Sprintf("0x%04X/%d: %v", readRange.Start, readRange.Count, err))
			if idx < len(plan)-1 {
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		for _, reg := range readRange.Registers {
			offset := int(reg.Address - readRange.Start)
			end := offset + int(reg.Count)
			if offset < 0 || end > len(words) {
				errors = append(errors, fmt.Sprintf("0x%04X: range decode overflow", reg.Address))
				continue
			}

			decoded, err := reg.Decode(words[offset:end], now)
			if err != nil {
				errors = append(errors, fmt.Sprintf("0x%04X: %v", reg.Address, err))
				continue
			}

			values = append(values, decoded)
		}

		if idx < len(plan)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	if len(values) > 0 {
		s.state.UpsertTelemetry(values)
		if len(errors) > 0 {
			s.state.SetServiceStatus("modbus", "degraded", true, strings.Join(errors, " | "), now)
			return
		}
		s.state.SetServiceStatus("modbus", "connected", true, "", now)
		return
	}

	if len(errors) == 0 {
		errors = append(errors, "no registers configured")
	}
	s.state.SetServiceStatus("modbus", "error", false, strings.Join(errors, " | "), time.Time{})
}

func (s *Service) readPlanForGroup(group registers.PollGroup) []registers.ReadRange {
	if group == registers.GroupFast {
		return registers.BuildCriticalFastReadPlan()
	}
	return registers.BuildReadPlan(group)
}

func (s *Service) readHoldingWithRetryLocked(client gomodbus.Client, cfg config.Config, address, count uint16) ([]byte, error) {
	payload, err := client.ReadHoldingRegisters(address, count)
	if err == nil {
		return payload, nil
	}

	time.Sleep(100 * time.Millisecond)
	s.closeLocked()

	reconnectedClient, reconnectErr := s.ensureClientLocked(cfg)
	if reconnectErr != nil {
		return nil, fmt.Errorf("%v; reconnect failed: %w", err, reconnectErr)
	}

	payload, retryErr := reconnectedClient.ReadHoldingRegisters(address, count)
	if retryErr != nil {
		return nil, retryErr
	}

	return payload, nil
}

func (s *Service) verifyWriteLocked(cfg config.Config, reg registers.Register, expected uint16) (bool, error) {
	time.Sleep(150 * time.Millisecond)
	s.closeLocked()

	client, err := s.ensureClientLocked(cfg)
	if err != nil {
		return false, err
	}

	payload, err := s.readHoldingWithRetryLocked(client, cfg, reg.Address, reg.Count)
	if err != nil {
		return false, err
	}

	words, err := registers.WordsFromBytes(payload, reg.Count)
	if err != nil {
		return false, err
	}

	if len(words) == 0 {
		return false, fmt.Errorf("empty verification payload")
	}

	return words[0] == expected, nil
}

func (s *Service) ensureClientLocked(cfg config.Config) (gomodbus.Client, error) {
	key := serialKey(cfg)
	if s.client != nil && s.key == key {
		return s.client, nil
	}

	s.closeLocked()

	handler := gomodbus.NewRTUClientHandler(cfg.Serial.Port)
	handler.BaudRate = cfg.Serial.BaudRate
	handler.DataBits = cfg.Serial.DataBits
	handler.Parity = strings.ToUpper(strings.TrimSpace(cfg.Serial.Parity))
	handler.StopBits = cfg.Serial.StopBits
	handler.Timeout = cfg.Serial.Timeout.Duration
	handler.SlaveId = cfg.Device.SlaveID

	if err := handler.Connect(); err != nil {
		return nil, fmt.Errorf("connect serial %s: %w", cfg.Serial.Port, err)
	}

	s.handler = handler
	s.client = gomodbus.NewClient(handler)
	s.key = key
	return s.client, nil
}

func (s *Service) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

func (s *Service) closeLocked() {
	if s.handler != nil {
		_ = s.handler.Close()
	}
	s.handler = nil
	s.client = nil
	s.key = ""
}

func serialKey(cfg config.Config) string {
	return fmt.Sprintf("%s|%d|%d|%d|%s|%d|%s",
		cfg.Serial.Port,
		cfg.Serial.BaudRate,
		cfg.Serial.DataBits,
		cfg.Serial.StopBits,
		strings.ToUpper(strings.TrimSpace(cfg.Serial.Parity)),
		cfg.Device.SlaveID,
		cfg.Serial.Timeout.Duration.String(),
	)
}
