package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/c360/semstreams/health"
	"github.com/c360/semstreams/metric"
	"github.com/c360/semstreams/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dynamicMockService implements the Service interface for dynamic lifecycle testing
type dynamicMockService struct {
	name       string
	started    bool
	stopped    bool
	healthy    bool
	startCount int
	stopCount  int
	mu         sync.RWMutex
}

func newDynamicMockService(name string) *dynamicMockService {
	return &dynamicMockService{
		name:    name,
		healthy: true,
	}
}

func (m *dynamicMockService) Start(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("service %s already started", m.name)
	}
	m.started = true
	m.stopped = false
	m.startCount++
	return nil
}

func (m *dynamicMockService) Stop(_ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return fmt.Errorf("service %s not started", m.name)
	}
	m.started = false
	m.stopped = true
	m.stopCount++
	return nil
}

func (m *dynamicMockService) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthy && m.started
}

func (m *dynamicMockService) Status() service.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.started {
		return service.StatusRunning
	}
	return service.StatusStopped
}

func (m *dynamicMockService) GetStatus() service.Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return service.Info{
		Name:   m.name,
		Status: m.Status(),
	}
}

func (m *dynamicMockService) Name() string {
	return m.name
}

func (m *dynamicMockService) RegisterMetrics(_ metric.MetricsRegistrar) error {
	return nil
}

func (m *dynamicMockService) Health() health.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.healthy {
		return health.NewUnhealthy(m.name, "Mock service is unhealthy")
	}

	if !m.started {
		return health.NewUnhealthy(m.name, "Mock service not started")
	}

	return health.NewHealthy(m.name, "Mock service operating normally")
}

func (m *dynamicMockService) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

func (m *dynamicMockService) IsStopped() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopped
}

func (m *dynamicMockService) GetStartCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startCount
}

func (m *dynamicMockService) GetStopCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopCount
}

// createTestDependencies creates Dependencies for dynamic testing
func createTestDependencies() *service.Dependencies {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	metricsRegistry := metric.NewMetricsRegistry()
	return &service.Dependencies{
		Logger:          logger,
		MetricsRegistry: metricsRegistry,
	}
}

// TestServiceManagerDynamicLifecycle tests dynamic start/stop of individual services
func TestServiceManagerDynamicLifecycle(t *testing.T) {
	t.Run("StartService creates and starts a new service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register a test constructor
		err := registry.Register("dynamic-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("dynamic-service"), nil
		})
		require.NoError(t, err)

		// Start the service dynamically
		ctx := context.Background()
		err = manager.StartService(ctx, "dynamic-service", json.RawMessage(`{}`), createTestDependencies())
		require.NoError(t, err)

		// Verify service exists and is running
		svc, exists := manager.GetService("dynamic-service")
		assert.True(t, exists, "Service should exist after StartService")
		require.NotNil(t, svc)
		assert.Equal(t, service.StatusRunning, svc.Status(), "Service should be running")
		assert.True(t, svc.IsHealthy(), "Service should be healthy")
	})

	t.Run("StopService stops and removes a service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register and start a test service
		err := registry.Register("removable-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("removable-service"), nil
		})
		require.NoError(t, err)

		ctx := context.Background()
		err = manager.StartService(ctx, "removable-service", json.RawMessage(`{}`), createTestDependencies())
		require.NoError(t, err)

		// Verify service is running
		svc, exists := manager.GetService("removable-service")
		require.True(t, exists)
		require.NotNil(t, svc)

		// Stop the service
		err = manager.StopService("removable-service", 5*time.Second)
		require.NoError(t, err)

		// Verify service is removed from manager
		_, exists = manager.GetService("removable-service")
		assert.False(t, exists, "Service should be removed after StopService")
	})

	t.Run("StartService is idempotent for already running service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		var createCount int
		err := registry.Register("idempotent-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			createCount++
			return newDynamicMockService("idempotent-service"), nil
		})
		require.NoError(t, err)

		ctx := context.Background()
		deps := createTestDependencies()

		// Start service first time
		err = manager.StartService(ctx, "idempotent-service", json.RawMessage(`{}`), deps)
		require.NoError(t, err)
		assert.Equal(t, 1, createCount, "Service should be created once")

		// Start service second time - should be idempotent
		err = manager.StartService(ctx, "idempotent-service", json.RawMessage(`{}`), deps)
		require.NoError(t, err)
		assert.Equal(t, 1, createCount, "Service should not be created again")

		// Verify service is still running
		svc, exists := manager.GetService("idempotent-service")
		assert.True(t, exists)
		assert.Equal(t, service.StatusRunning, svc.Status())
	})

	t.Run("StopService is idempotent for non-existent service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Stop a service that doesn't exist - should not error
		err := manager.StopService("non-existent-service", 5*time.Second)
		assert.NoError(t, err, "Stopping non-existent service should not error")
	})

	t.Run("StopService cannot stop mandatory services", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register the mandatory component-manager service
		err := registry.Register("component-manager", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("component-manager"), nil
		})
		require.NoError(t, err)

		// Start the mandatory service
		ctx := context.Background()
		err = manager.StartService(ctx, "component-manager", json.RawMessage(`{}`), createTestDependencies())
		require.NoError(t, err)

		// Attempt to stop mandatory service - should error
		err = manager.StopService("component-manager", 5*time.Second)
		assert.Error(t, err, "Should not be able to stop mandatory service")
		assert.Contains(t, err.Error(), "cannot stop mandatory service")

		// Verify service is still running
		svc, exists := manager.GetService("component-manager")
		assert.True(t, exists, "Mandatory service should still exist")
		assert.Equal(t, service.StatusRunning, svc.Status(), "Mandatory service should still be running")
	})

	t.Run("StartService fails gracefully for unknown constructor", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		ctx := context.Background()
		err := manager.StartService(ctx, "unknown-service", json.RawMessage(`{}`), createTestDependencies())
		assert.Error(t, err, "Should error for unknown service constructor")
		assert.Contains(t, err.Error(), "unknown-service")
	})

	t.Run("StartService removes service on start failure", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register a service that fails to start
		err := registry.Register("failing-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return &failingStartService{name: "failing-service"}, nil
		})
		require.NoError(t, err)

		ctx := context.Background()
		err = manager.StartService(ctx, "failing-service", json.RawMessage(`{}`), createTestDependencies())
		assert.Error(t, err, "Should error when service fails to start")

		// Verify service is not in manager
		_, exists := manager.GetService("failing-service")
		assert.False(t, exists, "Failed service should be removed from manager")
	})
}

// failingStartService is a service that always fails to start
type failingStartService struct {
	name string
}

func (f *failingStartService) Name() string { return f.name }
func (f *failingStartService) Start(_ context.Context) error {
	return fmt.Errorf("simulated start failure")
}
func (f *failingStartService) Stop(_ time.Duration) error { return nil }
func (f *failingStartService) Status() service.Status     { return service.StatusStopped }
func (f *failingStartService) IsHealthy() bool            { return false }
func (f *failingStartService) GetStatus() service.Info {
	return service.Info{Name: f.name, Status: service.StatusStopped}
}
func (f *failingStartService) RegisterMetrics(_ metric.MetricsRegistrar) error { return nil }
func (f *failingStartService) Health() health.Status {
	return health.NewUnhealthy(f.name, "Failed to start")
}

// TestServiceManagerConfigDynamicUpdates tests config-driven dynamic start/stop
func TestServiceManagerConfigDynamicUpdates(t *testing.T) {
	t.Run("new service added in config starts the service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register test constructor
		var createdService *dynamicMockService
		err := registry.Register("new-config-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			createdService = newDynamicMockService("new-config-service")
			return createdService, nil
		})
		require.NoError(t, err)

		// Simulate config change: empty -> new service enabled
		oldConfigs := make(map[string]serviceConfig)
		newConfigs := map[string]serviceConfig{
			"new-config-service": {
				Enabled: true,
				Config:  json.RawMessage(`{}`),
			},
		}

		// Process the config change (this tests processServiceConfigChanges behavior)
		// Since processServiceConfigChanges is not exported, we simulate its behavior
		// by calling StartService for new services
		ctx := context.Background()
		for serviceName, cfg := range newConfigs {
			if _, existed := oldConfigs[serviceName]; !existed && cfg.Enabled {
				err := manager.StartService(ctx, serviceName, cfg.Config, createTestDependencies())
				require.NoError(t, err)
			}
		}

		// Verify service was started
		svc, exists := manager.GetService("new-config-service")
		assert.True(t, exists, "New service should be created")
		assert.Equal(t, service.StatusRunning, svc.Status(), "New service should be running")
	})

	t.Run("service removed from config stops the service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register and start a service
		err := registry.Register("removed-config-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("removed-config-service"), nil
		})
		require.NoError(t, err)

		ctx := context.Background()
		err = manager.StartService(ctx, "removed-config-service", json.RawMessage(`{}`), createTestDependencies())
		require.NoError(t, err)

		// Simulate config change: service exists -> service removed
		oldConfigs := map[string]serviceConfig{
			"removed-config-service": {
				Enabled: true,
				Config:  json.RawMessage(`{}`),
			},
		}
		newConfigs := make(map[string]serviceConfig)

		// Process the config change - stop removed services
		for serviceName := range oldConfigs {
			if _, exists := newConfigs[serviceName]; !exists {
				err := manager.StopService(serviceName, 5*time.Second)
				assert.NoError(t, err)
			}
		}

		// Verify service was removed
		_, exists := manager.GetService("removed-config-service")
		assert.False(t, exists, "Removed service should not exist")
	})

	t.Run("service disabled in config stops the service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register and start a service
		err := registry.Register("disabled-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("disabled-service"), nil
		})
		require.NoError(t, err)

		ctx := context.Background()
		err = manager.StartService(ctx, "disabled-service", json.RawMessage(`{}`), createTestDependencies())
		require.NoError(t, err)

		// Simulate config change: enabled -> disabled
		oldConfigs := map[string]serviceConfig{
			"disabled-service": {
				Enabled: true,
				Config:  json.RawMessage(`{}`),
			},
		}
		newConfigs := map[string]serviceConfig{
			"disabled-service": {
				Enabled: false,
				Config:  json.RawMessage(`{}`),
			},
		}

		// Process the config change - stop disabled services
		for serviceName, newCfg := range newConfigs {
			if oldCfg, existed := oldConfigs[serviceName]; existed {
				if oldCfg.Enabled && !newCfg.Enabled {
					err := manager.StopService(serviceName, 5*time.Second)
					assert.NoError(t, err)
				}
			}
		}

		// Verify service was stopped
		_, exists := manager.GetService("disabled-service")
		assert.False(t, exists, "Disabled service should be removed")
	})

	t.Run("service enabled in config starts the service", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register test constructor
		err := registry.Register("enabled-service", func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
			return newDynamicMockService("enabled-service"), nil
		})
		require.NoError(t, err)

		// Simulate config change: disabled -> enabled
		oldConfigs := map[string]serviceConfig{
			"enabled-service": {
				Enabled: false,
				Config:  json.RawMessage(`{}`),
			},
		}
		newConfigs := map[string]serviceConfig{
			"enabled-service": {
				Enabled: true,
				Config:  json.RawMessage(`{}`),
			},
		}

		// Process the config change - start newly enabled services
		ctx := context.Background()
		for serviceName, newCfg := range newConfigs {
			if oldCfg, existed := oldConfigs[serviceName]; existed {
				if !oldCfg.Enabled && newCfg.Enabled {
					err := manager.StartService(ctx, serviceName, newCfg.Config, createTestDependencies())
					require.NoError(t, err)
				}
			}
		}

		// Verify service was started
		svc, exists := manager.GetService("enabled-service")
		assert.True(t, exists, "Enabled service should exist")
		assert.Equal(t, service.StatusRunning, svc.Status(), "Enabled service should be running")
	})

	t.Run("concurrent config updates are handled safely", func(t *testing.T) {
		registry := service.NewServiceRegistry()
		manager := service.NewServiceManager(registry)

		// Register multiple services
		const numServices = 10
		for i := 0; i < numServices; i++ {
			serviceName := fmt.Sprintf("concurrent-service-%d", i)
			err := registry.Register(serviceName, func(_ json.RawMessage, _ *service.Dependencies) (service.Service, error) {
				return newDynamicMockService(serviceName), nil
			})
			require.NoError(t, err)
		}

		// Start and stop services concurrently
		var wg sync.WaitGroup
		ctx := context.Background()
		deps := createTestDependencies()

		// Start all services concurrently
		for i := 0; i < numServices; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				serviceName := fmt.Sprintf("concurrent-service-%d", idx)
				_ = manager.StartService(ctx, serviceName, json.RawMessage(`{}`), deps)
			}(i)
		}
		wg.Wait()

		// Verify all services are running
		for i := 0; i < numServices; i++ {
			serviceName := fmt.Sprintf("concurrent-service-%d", i)
			svc, exists := manager.GetService(serviceName)
			assert.True(t, exists, "Service %s should exist", serviceName)
			if exists {
				assert.Equal(t, service.StatusRunning, svc.Status(), "Service %s should be running", serviceName)
			}
		}

		// Stop half the services concurrently
		for i := 0; i < numServices/2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				serviceName := fmt.Sprintf("concurrent-service-%d", idx)
				_ = manager.StopService(serviceName, 5*time.Second)
			}(i)
		}
		wg.Wait()

		// Verify stopped services are removed, others still running
		for i := 0; i < numServices; i++ {
			serviceName := fmt.Sprintf("concurrent-service-%d", i)
			_, exists := manager.GetService(serviceName)
			if i < numServices/2 {
				assert.False(t, exists, "Service %s should be stopped", serviceName)
			} else {
				assert.True(t, exists, "Service %s should still exist", serviceName)
			}
		}
	})
}

// serviceConfig mirrors types.ServiceConfig for test purposes
type serviceConfig struct {
	Enabled bool
	Config  json.RawMessage
}
