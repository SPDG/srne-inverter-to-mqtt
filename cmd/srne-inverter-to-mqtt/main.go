package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/tomasz/srne-inverter-to-mqtt/internal/app"
	"github.com/tomasz/srne-inverter-to-mqtt/internal/buildinfo"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "Path to the YAML configuration file")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	build := buildinfo.Info{
		Version:   buildinfo.Version,
		Commit:    buildinfo.Commit,
		BuildDate: buildinfo.BuildDate,
	}

	application, err := app.New(*configPath, build)
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}

	if err := application.Run(ctx); err != nil {
		log.Fatalf("application stopped with error: %v", err)
	}
}
