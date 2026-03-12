// Package main implements the entry point for the GitHub PR Workflow example.
// This binary extends the core semstreams with the pr-workflow-spawner component
// that automates issue-to-PR workflows using adversarial agents.
//
// Usage:
//
//	go run ./cmd/examples/github-pr-workflow -config configs/github-pr-workflow.json
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/c360studio/semstreams/component"
	"github.com/c360studio/semstreams/componentregistry"
	"github.com/c360studio/semstreams/config"
	githubprworkflow "github.com/c360studio/semstreams/examples/github-pr-workflow"
	"github.com/c360studio/semstreams/metric"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/service"
	"github.com/c360studio/semstreams/types"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := "configs/github-pr-workflow.json"
	if len(os.Args) > 2 && os.Args[1] == "-config" {
		configPath = os.Args[2]
	}

	cfg, err := loadAndValidateConfig(configPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	natsClient, err := connectNATS(ctx, cfg)
	if err != nil {
		return err
	}
	defer natsClient.Close(ctx)

	logger := slog.Default()

	streamsManager := config.NewStreamsManager(natsClient, logger)
	if err := streamsManager.EnsureStreams(ctx, cfg); err != nil {
		return fmt.Errorf("ensure streams: %w", err)
	}

	componentRegistry, err := createComponentRegistry()
	if err != nil {
		return err
	}

	configManager, err := config.NewConfigManager(cfg, natsClient, logger)
	if err != nil {
		return fmt.Errorf("create config manager: %w", err)
	}
	if err := configManager.Start(ctx); err != nil {
		return fmt.Errorf("start config manager: %w", err)
	}
	defer configManager.Stop(5 * time.Second)

	manager, svcDeps, err := setupServices(cfg, natsClient, logger, componentRegistry, configManager)
	if err != nil {
		return err
	}

	if err := createEnabledServices(cfg, manager, svcDeps); err != nil {
		return err
	}

	return runWithSignalHandling(ctx, manager)
}

func loadAndValidateConfig(path string) (*config.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func connectNATS(ctx context.Context, cfg *config.Config) (*natsclient.Client, error) {
	natsURLs := "nats://localhost:4222"
	if len(cfg.NATS.URLs) > 0 {
		natsURLs = strings.Join(cfg.NATS.URLs, ",")
	}
	natsClient, err := natsclient.NewClient(natsURLs)
	if err != nil {
		return nil, fmt.Errorf("create NATS client: %w", err)
	}
	if err := natsClient.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	connCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := natsClient.WaitForConnection(connCtx); err != nil {
		return nil, fmt.Errorf("NATS connection timeout: %w", err)
	}

	return natsClient, nil
}

func createComponentRegistry() (*component.Registry, error) {
	registry := component.NewRegistry()
	if err := componentregistry.Register(registry); err != nil {
		return nil, fmt.Errorf("register core components: %w", err)
	}
	if err := githubprworkflow.Register(registry); err != nil {
		return nil, fmt.Errorf("register pr-workflow component: %w", err)
	}
	return registry, nil
}

func setupServices(
	cfg *config.Config,
	natsClient *natsclient.Client,
	logger *slog.Logger,
	componentRegistry *component.Registry,
	configManager *config.Manager,
) (*service.Manager, *service.Dependencies, error) {
	platformID := cfg.Platform.InstanceID
	if platformID == "" {
		platformID = cfg.Platform.ID
	}

	metricsRegistry := metric.NewMetricsRegistry()

	serviceRegistry := service.NewServiceRegistry()
	if err := service.RegisterAll(serviceRegistry); err != nil {
		return nil, nil, fmt.Errorf("register services: %w", err)
	}

	manager := service.NewServiceManager(serviceRegistry)

	svcDeps := &service.Dependencies{
		NATSClient:      natsClient,
		MetricsRegistry: metricsRegistry,
		Logger:          logger,
		Platform: types.PlatformMeta{
			Org:      cfg.Platform.Org,
			Platform: platformID,
		},
		Manager:           configManager,
		ComponentRegistry: componentRegistry,
	}

	if err := manager.ConfigureFromServices(cfg.Services, svcDeps); err != nil {
		return nil, nil, fmt.Errorf("configure services: %w", err)
	}

	return manager, svcDeps, nil
}

func createEnabledServices(cfg *config.Config, manager *service.Manager, svcDeps *service.Dependencies) error {
	for name, svcConfig := range cfg.Services {
		if name == "service-manager" || !svcConfig.Enabled {
			continue
		}
		if !manager.HasConstructor(name) {
			continue
		}
		if _, err := manager.CreateService(name, svcConfig.Config, svcDeps); err != nil {
			return fmt.Errorf("create service %s: %w", name, err)
		}
	}
	return nil
}

func runWithSignalHandling(ctx context.Context, manager *service.Manager) error {
	signalCtx, signalCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()

	slog.Info("Starting GitHub PR Workflow example")
	if err := manager.StartAll(signalCtx); err != nil {
		return fmt.Errorf("start services: %w", err)
	}

	<-signalCtx.Done()
	slog.Info("Shutting down")
	return manager.StopAll(10 * time.Second)
}
