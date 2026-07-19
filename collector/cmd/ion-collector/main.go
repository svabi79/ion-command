package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ion-command/ion-command/collector/internal/api"
	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/pipeline"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/plugins/contexts/hfpropagation"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/geophysics"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/hamradio"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/ionosphere"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/orbital"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/spaceweather"
	"github.com/ion-command/ion-command/collector/internal/plugins/domains/weather"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/blitzortung"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/celestrak"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/kc2g"
	mocksource "github.com/ion-command/ion-command/collector/internal/plugins/sources/mock"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/pskreporter"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/swpc"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/usgs"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/wsjtx"
	"github.com/ion-command/ion-command/collector/internal/recording"
	"github.com/ion-command/ion-command/collector/internal/stream"
	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

func main() {
	configPath := flag.String("config", "configs/development.json", "path to collector JSON configuration")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(*configPath, logger); err != nil {
		logger.Error("collector stopped", "error", err)
		os.Exit(1)
	}
}

func run(configPath string, logger *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	registry := plugins.NewRegistry()
	for _, domain := range []plugins.Domain{hamradio.New(), weather.New(), spaceweather.New(), ionosphere.New(), geophysics.New(), orbital.New()} {
		if err := registry.RegisterDomain(domain); err != nil {
			return err
		}
	}
	if err := registry.RegisterContext(hfpropagation.New()); err != nil {
		return err
	}
	for _, sourceConfig := range cfg.Sources {
		if !sourceConfig.Enabled {
			continue
		}
		var source plugins.Source
		var err error
		switch {
		case strings.HasPrefix(sourceConfig.Type, "mock."):
			source, err = mocksource.New(sourceConfig)
		case sourceConfig.Type == "pskreporter.mqtt":
			source, err = pskreporter.NewMQTT(sourceConfig, logger)
		case sourceConfig.Type == "wsjtx.udp":
			source, err = wsjtx.New(sourceConfig, logger)
		case sourceConfig.Type == "spaceweather.swpc":
			source, err = swpc.New(sourceConfig, logger)
		case sourceConfig.Type == "ionosonde.kc2g":
			source, err = kc2g.New(sourceConfig, logger)
		case sourceConfig.Type == "lightning.blitzortung":
			source, err = blitzortung.New(sourceConfig, logger)
		case sourceConfig.Type == "earthquake.usgs":
			source, err = usgs.New(sourceConfig, logger)
		case sourceConfig.Type == "orbital.celestrak":
			source, err = celestrak.New(sourceConfig, logger)
		default:
			err = fmt.Errorf("unknown source type %q", sourceConfig.Type)
		}
		if err != nil {
			return fmt.Errorf("create source %s: %w", sourceConfig.ID, err)
		}
		if err := registry.RegisterSource(source); err != nil {
			return err
		}
	}
	stats := telemetry.New()
	hub := stream.NewHub(cfg.Pipeline.ClientQueueCapacity, stats)
	flush := time.Duration(cfg.Recording.FlushIntervalSeconds) * time.Second
	if flush <= 0 {
		flush = time.Second
	}
	recorder := recording.New(cfg.Recording.Directory, cfg.Recording.Enabled, flush).WithRetention(int64(cfg.Recording.MaxTotalGigabytes * 1024 * 1024 * 1024))
	pipe := pipeline.New(registry, cfg.Pipeline.QueueCapacity, cfg.Pipeline.WorkerCount, hub, recorder, stats, logger, cfg.Pipeline.RetainLatest)
	if err := pipe.Verify(); err != nil {
		return err
	}
	server := api.New(cfg, pipe, hub, stats, logger).HTTPServer()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pipelineErrors := make(chan error, 1)
	go func() { pipelineErrors <- pipe.Run(ctx) }()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("collector listening", "address", cfg.Server.ListenAddress, "version", api.BuildVersion)
		serverErrors <- server.ListenAndServe()
	}()
	pipelineFinished := false
	var pipelineResult error
	select {
	case err := <-serverErrors:
		cancel()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case err := <-pipelineErrors:
		pipelineFinished = true
		pipelineResult = err
		cancel()
		if err != nil {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if pipelineFinished {
		return pipelineResult
	}
	select {
	case err := <-pipelineErrors:
		return err
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}
