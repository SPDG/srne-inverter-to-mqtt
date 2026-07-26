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
		a.assets,
	)

	server := &http.Server{
		Addr:              cfg.HTTP.Listen,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	modbusService := modbussvc.NewService(a, a.runtimeState)
	mqttService := mqttsvc.NewService(a, a.runtimeState, a.build)
	a.modbus = modbusService

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		return modbusService.Run(groupCtx)
	})

	group.Go(func() error {
		return mqttService.Run(groupCtx)
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
	return a.modbus.WriteRegister(id, value)
}
