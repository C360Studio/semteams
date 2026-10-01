//go:build integration

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	"github.com/c360studio/semstreams/config"
	"github.com/c360studio/semstreams/natsclient"
	rulepkg "github.com/c360studio/semstreams/processor/rule"
	"github.com/c360studio/semstreams/types"
	"github.com/stretchr/testify/require"
)

func migrationIntegrationConfig(url string) *config.Config {
	return &config.Config{Version: "1.0.0", Platform: config.PlatformConfig{Org: "c360", ID: "migration-test", Type: "test"},
		NATS:     config.NATSConfig{URLs: []string{url}, JetStream: config.JetStreamConfig{Enabled: true}},
		Services: make(types.ServiceConfigs), Components: make(config.ComponentConfigs)}
}

// Product-local graph writers must share the framework's durable minted authority,
// rather than emitting a second unsuffixed family that no run can resolve.
func TestInfrastructureUsesMintedPlatformForRunAndLoopIDs(t *testing.T) {
	tc := natsclient.NewTestClient(t, natsclient.WithJetStream(), natsclient.WithKV())
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cfg := migrationIntegrationConfig(tc.URL)
	rules, err := rulepkg.NewConfigManager(quietLogger())
	require.NoError(t, err)
	_, platform, manager, err := setupRemainingInfrastructure(ctx, cfg, tc.Client, quietLogger(), rules)
	require.NoError(t, err)
	defer func() { require.NoError(t, manager.Stop(5*time.Second)) }()
	require.True(t, strings.HasPrefix(platform.Platform, cfg.Platform.ID+"-"), "effective platform must be minted from the declared stem")
	require.Equal(t, manager.GetConfig().Get().Platform.ID, platform.Platform)
	const loopID = "c4b7d8e0-17c1-49f4-a170-ef0dbeac7b2f"
	require.Equal(t, "c360."+platform.Platform+".agentic-loop.agent.execution."+loopID, agentic.LoopExecutionEntityID(platform.Org, platform.Platform, loopID))
	require.Equal(t, "c360."+platform.Platform+".chain.agent.execution."+loopID, agentic.ChainExecutionEntityID(platform.Org, platform.Platform, loopID))
}
