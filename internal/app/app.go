package app

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
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
