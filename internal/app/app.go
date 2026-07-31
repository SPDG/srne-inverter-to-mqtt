package app

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/config"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/httpapi"
	modbussvc "github.com/tomasz/srne-inverter-to-mqtt/internal/modbus"
	mqttsvc "github.com/tomasz/srne-inverter-to-mqtt/internal/mqtt"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/state"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/stormcharge"
	"github.com/tomasz/srne-inverter-to-mqtt/web"
)

type App struct {
	build      buildinfo.Info
	configPath string
	startedAt  time.Time
	assets     fs.FS

	mu           sync.RWMutex
	cfg          config.Config
	runtimeState *state.Store
	modbus       *modbussvc.Service
	stormCharge  *stormcharge.Manager
	exit         func(int)
}

func New(configPath string, build buildinfo.Info) (*App, error) {
	assets, err := web.Assets()
	if err != nil {
		return nil, fmt.Errorf("load embedded web assets: %w", err)
	}

	return &App{
		build:        build,
		configPath:   configPath,
		startedAt:    time.Now(),
		assets:       assets,
		runtimeState: state.New(),
		exit:         os.Exit,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	cfg, created, err := config.LoadOrCreate(a.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()

	if created {
		log.Printf("created default config at %s", a.configPath)
	}

	a.runtimeState.SetServiceStatus("web", "running", true, "", time.Now().UTC())

	modbusService := modbussvc.NewService(a, a.runtimeState)
	stormChargeManager, err := stormcharge.New(
		stormcharge.RuntimePath(a.configPath),
		modbusService,
		a.runtimeState,
	)
	if err != nil {
		return fmt.Errorf("initialize storm charge: %w", err)
	}

	a.modbus = modbusService
	a.stormCharge = stormChargeManager

	mqttService := mqttsvc.NewService(a, a.runtimeState, a.build)

	handler := httpapi.NewHandler(
		a.build,
		httpapi.StatusSnapshot{
			StartedAt:   a.startedAt,
			ConfigPath:  a.configPath,
			ConfigReady: true,
		},
		a,
		a,
		a,
		a,
		a.assets,
	)

	server := &http.Server{
		Addr:              cfg.HTTP.Listen,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return modbusService.Run(groupCtx)
	})

	group.Go(func() error {
		return mqttService.Run(groupCtx)
	})

	group.Go(func() error {
		return stormChargeManager.Run(groupCtx)
	})

	group.Go(func() error {
		return a.watchdogLoop(groupCtx)
	})

	group.Go(func() error {
		log.Printf("web server listening on http://%s", cfg.HTTP.Listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	group.Go(func() error {
		<-groupCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	})

	return group.Wait()
}

func (a *App) watchdogLoop(ctx context.Context) error {
	for {
		interval := a.watchdogInterval()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}

		trigger, stalledFor, modbusState := a.shouldTriggerWatchdog(time.Now().UTC())
		if !trigger {
			continue
		}

		log.Printf(
			"watchdog exiting process due to modbus stall status=%s stalled_for=%s last_error=%q",
			modbusState.Status,
			stalledFor.Round(time.Second),
			modbusState.LastError,
		)
		a.exit(1)
		return fmt.Errorf("watchdog requested process restart")
	}
}

func (a *App) watchdogInterval() time.Duration {
	cfg := a.GetConfig()
	interval := cfg.Polling.FastInterval.Duration
	if interval <= 0 {
		return 15 * time.Second
	}
	if interval > 15*time.Second {
		return 15 * time.Second
	}
	return interval
}

func (a *App) shouldTriggerWatchdog(now time.Time) (bool, time.Duration, state.ServiceStatus) {
	cfg := a.GetConfig()
	snapshot := a.runtimeState.Snapshot()
	modbusState, ok := snapshot.Services["modbus"]
	if !ok {
		return false, 0, state.ServiceStatus{}
	}

	if modbusState.Connected || modbusState.Status == "connected" ||
		modbusState.Status == "starting" || modbusState.Status == "disabled" {
		return false, 0, modbusState
	}

	if modbusState.LastSuccess.IsZero() {
		return false, 0, modbusState
	}

	threshold := modbusWatchdogThreshold(cfg)
	stalledFor := now.Sub(modbusState.LastSuccess)
	if stalledFor < threshold {
		return false, stalledFor, modbusState
	}

	return true, stalledFor, modbusState
}

func modbusWatchdogThreshold(cfg config.Config) time.Duration {
	threshold := cfg.Polling.SlowInterval.Duration * 2
	if threshold < 90*time.Second {
		return 90 * time.Second
	}
	return threshold
}

func (a *App) GetConfig() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *App) UpdateConfig(cfg config.Config) error {
	if a.stormCharge != nil && a.stormCharge.IsActive() {
		return fmt.Errorf("service configuration cannot be changed while storm charge is active")
	}
	if err := config.Save(a.configPath, cfg); err != nil {
		return err
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return nil
}

func (a *App) GetStateSnapshot() state.Snapshot {
	return a.runtimeState.Snapshot()
}

func (a *App) WriteRegister(id string, value any) error {
	if a.modbus == nil {
		return fmt.Errorf("modbus service is not initialized")
	}
	if a.stormCharge != nil && a.stormCharge.IsActive() && stormcharge.IsManagedRegister(id) {
		return fmt.Errorf("register %q is managed by active storm charge; cancel storm charge first", id)
	}
	return a.modbus.WriteRegister(id, value)
}

func (a *App) GetStormChargeStatus() stormcharge.Status {
	settings := stormcharge.SettingsFromConfig(a.GetConfig().StormCharge)
	if a.stormCharge == nil {
		return stormcharge.Status{Phase: stormcharge.PhaseIdle, Settings: settings}
	}
	return a.stormCharge.Status(settings)
}

func (a *App) UpdateStormChargeSettings(settings stormcharge.Settings) error {
	if err := stormcharge.ValidateSettings(settings); err != nil {
		return err
	}
	if a.stormCharge != nil && a.stormCharge.IsActive() {
		return fmt.Errorf("storm charge settings cannot be changed while charging is active")
	}

	cfg := a.GetConfig()
	cfg.StormCharge = config.StormChargeConfig{
		TargetSOC:   settings.TargetSOC,
		MaxCurrentA: settings.MaxCurrentA,
		Timeout:     settings.Timeout,
	}
	return a.UpdateConfig(cfg)
}

func (a *App) StartStormCharge(settings stormcharge.Settings) error {
	if a.stormCharge == nil {
		return fmt.Errorf("storm charge manager is not initialized")
	}
	inverterType, err := a.GetConfig().Device.NormalizedInverterType()
	if err != nil {
		return err
	}
	if inverterType != "spi_h3p" {
		return fmt.Errorf("storm charge is currently supported only for the SPI H3P profile")
	}
	if a.stormCharge.IsActive() {
		return fmt.Errorf("storm charge is already active")
	}
	if err := a.UpdateStormChargeSettings(settings); err != nil {
		return err
	}
	return a.stormCharge.Start(settings)
}

func (a *App) CancelStormCharge() error {
	if a.stormCharge == nil {
		return fmt.Errorf("storm charge manager is not initialized")
	}
	return a.stormCharge.Cancel()
}
