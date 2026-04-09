// Package rule - NATS KV Configuration Integration for Rules
package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// WatchCallback is invoked when a rule is created, updated, or deleted via KV.
// operation is "put" or "delete".
type WatchCallback func(ruleID string, rule Rule, operation string)

// ConfigManager manages rules through NATS KV configuration
type ConfigManager struct {
	processor  *Processor
	kvStore    *natsclient.KVStore
	configMgr  *config.Manager
	updateChan <-chan config.Update // Channel received from ConfigManager
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	mu         sync.RWMutex

	// Direct KV watcher for rules (config.Manager doesn't watch "rules.*")
	ruleWatcher jetstream.KeyWatcher

	// WatchRules callbacks
	watchCallbacks []WatchCallback
	watchMu        sync.RWMutex

	// Goroutine lifecycle tracking
	wg sync.WaitGroup
}

// NewConfigManager creates a new rule configuration manager
func NewConfigManager(processor *Processor, configMgr *config.Manager, logger *slog.Logger) *ConfigManager {
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ConfigManager{
		processor: processor,
		configMgr: configMgr,
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger.With("component", "rule-config-manager"),
	}
}

// Start begins watching for rule configuration updates.
// It sets up a direct KV watcher on "rules.>" in the config bucket since the
// central config.Manager does not watch rule keys (they are not part of the
// platform config schema). It also subscribes via OnChange for any future
// config.Manager integration.
func (rcm *ConfigManager) Start(ctx context.Context) error {
	// Subscribe to config.Manager's notification channel (used when rules are
	// written through config.Manager's own KV or via PushToKV).
	rcm.updateChan = rcm.configMgr.OnChange("rules.*")
	rcm.wg.Add(1)
	go func() {
		defer rcm.wg.Done()
		rcm.processConfigUpdates()
	}()

	// Set up direct KV watcher for rules written directly to the config bucket.
	kv := rcm.configMgr.GetKV()
	if kv != nil {
		watcher, err := kv.Watch(ctx, "rules.>", jetstream.UpdatesOnly())
		if err != nil {
			rcm.logger.Warn("Failed to create direct rule KV watcher, rules will only update via config manager",
				slog.Any("error", err))
		} else {
			rcm.ruleWatcher = watcher
			rcm.wg.Add(1)
			go func() {
				defer rcm.wg.Done()
				rcm.processRuleWatcher()
			}()
		}
	}

	rcm.logger.Info("Rule configuration manager started", "pattern", "rules.>")
	return nil
}

// Stop stops the configuration manager and waits for goroutines to exit.
func (rcm *ConfigManager) Stop() error {
	rcm.cancel()

	// Wait for goroutines to exit before stopping the watcher
	rcm.wg.Wait()

	// Stop our direct KV watcher if running
	if rcm.ruleWatcher != nil {
		_ = rcm.ruleWatcher.Stop()
	}

	// The channel from ConfigManager will be closed when ConfigManager stops
	// We don't close it here since we don't own it

	rcm.logger.Info("Rule configuration manager stopped")
	return nil
}

// processConfigUpdates handles configuration change notifications
func (rcm *ConfigManager) processConfigUpdates() {
	for {
		select {
		case <-rcm.ctx.Done():
			return
		case update, ok := <-rcm.updateChan:
			if !ok {
				return // channel closed
			}
			rcm.handleConfigUpdate(update)
		}
	}
}

// processRuleWatcher handles raw KV entries from the direct rule watcher.
// This is the primary path for detecting rule changes since config.Manager
// does not watch "rules.*" keys.
func (rcm *ConfigManager) processRuleWatcher() {
	for {
		select {
		case <-rcm.ctx.Done():
			return
		case entry := <-rcm.ruleWatcher.Updates():
			if entry == nil {
				continue
			}
			rcm.handleRuleEntry(entry)
		}
	}
}

// handleRuleEntry processes a single KV entry from the rule watcher.
func (rcm *ConfigManager) handleRuleEntry(entry jetstream.KeyValueEntry) {
	key := entry.Key()

	// Extract rule ID from key (format: "rules.<ruleID>")
	parts := strings.SplitN(key, ".", 2)
	if len(parts) < 2 {
		rcm.logger.Warn("Invalid rule key format", "key", key)
		return
	}
	ruleID := parts[1]

	op := entry.Operation()

	if op == jetstream.KeyValueDelete || op == jetstream.KeyValuePurge {
		rcm.logger.Info("Rule deleted via KV", "rule_id", ruleID)
		// Remove from processor's runtime config
		changes := map[string]any{
			"rules": map[string]any{
				ruleID: nil, // nil signals deletion
			},
		}
		if err := rcm.processor.ApplyConfigUpdate(changes); err != nil {
			rcm.logger.Error("Failed to apply rule deletion", "rule_id", ruleID, "error", err)
		}
		rcm.notifyWatchCallbacks(ruleID, nil, "delete")
		return
	}

	rawValue := entry.Value()

	// Parse for validation
	var ruleDef Definition
	if err := json.Unmarshal(rawValue, &ruleDef); err != nil {
		rcm.logger.Error("Failed to unmarshal rule from KV", "key", key, "error", err)
		return
	}

	// Ensure ID is set
	if ruleDef.ID == "" {
		ruleDef.ID = ruleID
	}

	// Unmarshal raw bytes directly to map (avoids marshal/unmarshal round-trip)
	var ruleMap map[string]any
	if err := json.Unmarshal(rawValue, &ruleMap); err != nil {
		rcm.logger.Error("Failed to convert rule to map", "rule_id", ruleID, "error", err)
		return
	}
	// Ensure the map has the ID set consistently
	ruleMap["id"] = ruleID

	changes := map[string]any{
		"rules": map[string]any{
			ruleID: ruleMap,
		},
	}

	// Validate first
	if err := rcm.processor.ValidateConfigUpdate(changes); err != nil {
		rcm.logger.Error("Rule from KV failed validation", "rule_id", ruleID, "error", err)
		return
	}

	// Apply
	if err := rcm.processor.ApplyConfigUpdate(changes); err != nil {
		rcm.logger.Error("Failed to apply rule from KV", "rule_id", ruleID, "error", err)
		return
	}

	rcm.logger.Info("Applied rule from KV", "rule_id", ruleID, "name", ruleDef.Name)

	// Notify watch callbacks — look up the compiled Rule from the processor
	compiledRule := rcm.processor.GetCompiledRule(ruleID)
	rcm.notifyWatchCallbacks(ruleID, compiledRule, "put")
}

// notifyWatchCallbacks invokes all registered WatchRules callbacks.
func (rcm *ConfigManager) notifyWatchCallbacks(ruleID string, rule Rule, operation string) {
	rcm.watchMu.RLock()
	callbacks := rcm.watchCallbacks
	rcm.watchMu.RUnlock()

	for _, cb := range callbacks {
		cb(ruleID, rule, operation)
	}
}

// handleConfigUpdate processes a single configuration update
func (rcm *ConfigManager) handleConfigUpdate(update config.Update) {
	rcm.logger.Debug("Received configuration update", "path", update.Path)

	// Parse the path to determine the operation
	// Expected patterns:
	// - rules.battery_monitor_001 → single rule update
	// - rules.* → batch update (handled by iterating)

	parts := strings.Split(update.Path, ".")
	if len(parts) < 2 || parts[0] != "rules" {
		rcm.logger.Warn("Invalid rule configuration path", "path", update.Path)
		return
	}

	// Get all rule configurations from the updated config
	rulesConfig := rcm.extractRulesConfig(update.Config)
	if rulesConfig == nil {
		rcm.logger.Debug("No rules configuration in update")
		return
	}

	// Convert to change map for processor
	changes := map[string]any{
		"rules": rulesConfig,
	}

	// Validate changes directly with processor
	if err := rcm.processor.ValidateConfigUpdate(changes); err != nil {
		rcm.logger.Error("Rule configuration validation failed",
			"path", update.Path,
			"error", err)
		return
	}

	// Apply changes directly to processor
	if err := rcm.processor.ApplyConfigUpdate(changes); err != nil {
		rcm.logger.Error("Failed to apply rule configuration",
			"path", update.Path,
			"error", err)
		return
	}

	rcm.logger.Info("Applied rule configuration update",
		"path", update.Path,
		"rule_count", len(rulesConfig))
}

// extractRulesConfig extracts rule configurations from the full config
func (rcm *ConfigManager) extractRulesConfig(cfg *config.SafeConfig) map[string]any {
	if cfg == nil {
		return nil
	}

	// Get the config - returns *config.Config
	_ = cfg.Get()

	// Check if rules exist in Components map
	// Rules would be stored as components in the config
	// This might need adjustment based on how rules are stored
	// For now, return empty map - will be populated via KV updates
	return make(map[string]any)
}

// SaveRule saves a rule configuration to NATS KV
func (rcm *ConfigManager) SaveRule(ctx context.Context, ruleID string, ruleDef Definition) error {
	key := fmt.Sprintf("rules.%s", ruleID)

	// Convert to JSON
	data, err := json.Marshal(ruleDef)
	if err != nil {
		return errs.WrapInvalid(err, "ConfigManager", "SaveRule", "marshal rule definition")
	}

	// Use KVStore for safe CAS operations if available
	if rcm.kvStore != nil {
		_, err = rcm.kvStore.Put(ctx, key, data)
		return err
	}

	// Fallback to ConfigManager's internal KV
	return rcm.saveViaConfigManager(ctx, key, ruleDef)
}

// saveViaConfigManager saves through the ConfigManager's KV bucket.
// This is the fallback path when the rule ConfigManager's own KVStore
// has not been initialized via InitializeKVStore.
func (rcm *ConfigManager) saveViaConfigManager(ctx context.Context, key string, ruleDef Definition) error {
	kv := rcm.configMgr.GetKV()
	if kv == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "ConfigManager", "saveViaConfigManager", "config manager KV bucket not available")
	}

	data, err := json.Marshal(ruleDef)
	if err != nil {
		return errs.WrapInvalid(err, "ConfigManager", "saveViaConfigManager", "marshal rule definition")
	}

	if _, err := kv.Put(ctx, key, data); err != nil {
		return errs.WrapTransient(err, "ConfigManager", "saveViaConfigManager", "put rule to KV")
	}

	return nil
}

// DeleteRule removes a rule configuration from NATS KV
func (rcm *ConfigManager) DeleteRule(ctx context.Context, ruleID string) error {
	key := fmt.Sprintf("rules.%s", ruleID)

	if rcm.kvStore != nil {
		return rcm.kvStore.Delete(ctx, key)
	}

	return rcm.deleteViaConfigManager(ctx, key)
}

// deleteViaConfigManager deletes through the ConfigManager's KV bucket.
func (rcm *ConfigManager) deleteViaConfigManager(ctx context.Context, key string) error {
	kv := rcm.configMgr.GetKV()
	if kv == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "ConfigManager", "deleteViaConfigManager", "config manager KV bucket not available")
	}

	if err := kv.Delete(ctx, key); err != nil {
		if err == jetstream.ErrKeyNotFound {
			return errs.WrapInvalid(errs.ErrKeyNotFound, "ConfigManager", "deleteViaConfigManager", fmt.Sprintf("rule not found: %s", key))
		}
		return errs.WrapTransient(err, "ConfigManager", "deleteViaConfigManager", "delete rule from KV")
	}

	return nil
}

// GetRule retrieves a rule configuration from NATS KV
func (rcm *ConfigManager) GetRule(ctx context.Context, ruleID string) (*Definition, error) {
	key := fmt.Sprintf("rules.%s", ruleID)

	if rcm.kvStore != nil {
		entry, err := rcm.kvStore.Get(ctx, key)
		if err != nil {
			if err == jetstream.ErrKeyNotFound {
				return nil, errs.WrapInvalid(errs.ErrKeyNotFound, "ConfigManager", "GetRule", fmt.Sprintf("rule not found: %s", ruleID))
			}
			return nil, errs.WrapTransient(err, "ConfigManager", "GetRule", "get rule from KV")
		}

		var ruleDef Definition
		if err := json.Unmarshal(entry.Value, &ruleDef); err != nil {
			return nil, errs.WrapInvalid(err, "ConfigManager", "GetRule", "unmarshal rule definition")
		}

		return &ruleDef, nil
	}

	return rcm.getRuleViaConfigManager(ctx, ruleID)
}

// getRuleViaConfigManager retrieves through the ConfigManager
func (rcm *ConfigManager) getRuleViaConfigManager(_ context.Context, ruleID string) (*Definition, error) {
	// Get current config from processor
	currentConfig := rcm.processor.GetRuntimeConfig()

	if rulesMap, ok := currentConfig["rules"].(map[string]any); ok {
		if ruleConfig, ok := rulesMap[ruleID].(map[string]any); ok {
			// Convert map to Definition
			def := Definition{
				ID:      ruleID,
				Type:    getStringWithDefault(ruleConfig, "type", ""),
				Name:    getStringWithDefault(ruleConfig, "name", ruleID),
				Enabled: getBoolWithDefault(ruleConfig, "enabled", true),
			}
			return &def, nil
		}
	}

	return nil, errs.WrapInvalid(errs.ErrKeyNotFound, "ConfigManager", "getRuleViaConfigManager", fmt.Sprintf("rule not found: %s", ruleID))
}

// ListRules returns all rule configurations
func (rcm *ConfigManager) ListRules(_ context.Context) (map[string]Definition, error) {
	rules := make(map[string]Definition)

	// Get current config from processor
	currentConfig := rcm.processor.GetRuntimeConfig()

	if rulesMap, ok := currentConfig["rules"].(map[string]any); ok {
		for ruleID, ruleConfig := range rulesMap {
			if configMap, ok := ruleConfig.(map[string]any); ok {
				rules[ruleID] = Definition{
					ID:      ruleID,
					Type:    getStringWithDefault(configMap, "type", ""),
					Name:    getStringWithDefault(configMap, "name", ruleID),
					Enabled: getBoolWithDefault(configMap, "enabled", true),
				}
			}
		}
	}

	return rules, nil
}

// WatchRules registers a callback that is invoked whenever a rule is created,
// updated, or deleted via KV. The callback receives the rule ID, the compiled
// Rule (nil on delete), and the operation ("put" or "delete").
// Multiple callbacks can be registered; they are invoked in registration order.
func (rcm *ConfigManager) WatchRules(_ context.Context, callback func(ruleID string, rule Rule, operation string)) error {
	if callback == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "ConfigManager", "WatchRules", "callback cannot be nil")
	}

	rcm.watchMu.Lock()
	rcm.watchCallbacks = append(rcm.watchCallbacks, callback)
	count := len(rcm.watchCallbacks)
	rcm.watchMu.Unlock()

	rcm.logger.Debug("Registered rule watch callback", "total_callbacks", count)
	return nil
}

// InitializeKVStore initializes the KVStore for direct KV operations
func (rcm *ConfigManager) InitializeKVStore(natsClient *natsclient.Client) error {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	if natsClient == nil {
		return errs.WrapInvalid(errs.ErrMissingConfig, "ConfigManager", "InitializeKVStore", "NATS client is required")
	}

	// Get or create the config KV bucket
	kv, err := natsClient.CreateKeyValueBucket(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "semstreams_config",
		Description: "SemStreams runtime configuration",
		History:     5,
	})
	if err != nil {
		return errs.WrapTransient(err, "ConfigManager", "InitializeKVStore", "create/get KV bucket")
	}

	// Create KVStore for the config bucket
	rcm.kvStore = natsClient.NewKVStore(kv)

	rcm.logger.Info("Initialized KVStore for rule configuration")
	return nil
}
