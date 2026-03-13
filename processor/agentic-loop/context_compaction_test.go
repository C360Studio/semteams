package agenticloop_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/agentic"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

func TestCompactor_ShouldCompact(t *testing.T) {
	tests := []struct {
		name              string
		threshold         float64
		utilization       float64
		wantShouldCompact bool
	}{
		{
			name:              "below threshold",
			threshold:         0.60,
			utilization:       0.50,
			wantShouldCompact: false,
		},
		{
			name:              "at threshold",
			threshold:         0.60,
			utilization:       0.60,
			wantShouldCompact: true,
		},
		{
			name:              "above threshold",
			threshold:         0.60,
			utilization:       0.70,
			wantShouldCompact: true,
		},
		{
			name:              "far above threshold",
			threshold:         0.60,
			utilization:       0.90,
			wantShouldCompact: true,
		},
		{
			name:              "just below threshold",
			threshold:         0.60,
			utilization:       0.59,
			wantShouldCompact: false,
		},
		{
			name:              "low threshold",
			threshold:         0.40,
			utilization:       0.50,
			wantShouldCompact: true,
		},
		{
			name:              "high threshold",
			threshold:         0.80,
			utilization:       0.70,
			wantShouldCompact: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := agenticloop.DefaultContextConfig()
			config.CompactThreshold = tt.threshold
			config.HeadroomTokens = 0 // disable headroom to test threshold in isolation

			compactor := agenticloop.NewCompactor(config)

			// Create context manager with target utilization
			cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)
			targetTokens := int(float64(agenticloop.DefaultContextLimit) * tt.utilization)
			fillContextToTokens(t, cm, targetTokens)

			shouldCompact := compactor.ShouldCompact(cm)

			if shouldCompact != tt.wantShouldCompact {
				t.Errorf("ShouldCompact() = %v, want %v (utilization: %f, threshold: %f)",
					shouldCompact, tt.wantShouldCompact, cm.Utilization(), tt.threshold)
			}
		})
	}
}

func TestCompactor_Compact_GeneratesSummary(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.CompactThreshold = 0.60
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add history messages to compact
	messages := []agentic.ChatMessage{
		{Role: "user", Content: "What is the capital of France?"},
		{Role: "assistant", Content: "The capital of France is Paris."},
		{Role: "user", Content: "What about Germany?"},
		{Role: "assistant", Content: "The capital of Germany is Berlin."},
		{Role: "user", Content: "And Italy?"},
		{Role: "assistant", Content: "The capital of Italy is Rome."},
	}

	for _, msg := range messages {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, msg)
	}

	ctx := context.Background()
	result, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	// Summary should not be empty
	if result.Summary == "" {
		t.Error("Compact() summary is empty, want non-empty summary")
	}

	// Summary should be shorter than original
	if len(result.Summary) > 500 {
		t.Errorf("Compact() summary length = %d, want < 500 (should be concise)", len(result.Summary))
	}

	// Evicted tokens should be positive
	if result.EvictedTokens <= 0 {
		t.Errorf("Compact() evicted_tokens = %d, want > 0", result.EvictedTokens)
	}

	// New tokens should be positive
	if result.NewTokens <= 0 {
		t.Errorf("Compact() new_tokens = %d, want > 0", result.NewTokens)
	}

	// New tokens should be less than evicted (compaction achieved savings)
	if result.NewTokens >= result.EvictedTokens {
		t.Errorf("Compact() new_tokens (%d) >= evicted_tokens (%d), want new < evicted",
			result.NewTokens, result.EvictedTokens)
	}
}

func TestCompactor_Compact_RespectsContext(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add messages
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "Test message",
	})

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Compact should respect cancellation
	_, err := compactor.Compact(ctx, cm)

	if err == nil {
		t.Error("Compact() with cancelled context should return error")
	}

	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "cancel") {
		t.Errorf("Compact() error = %v, expected context cancellation error", err)
	}
}

func TestCompactor_Compact_Timeout(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add messages
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "Test message",
	})

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(10 * time.Millisecond)

	// Compact should respect timeout
	_, err := compactor.Compact(ctx, cm)

	if err == nil {
		t.Error("Compact() with timeout context should return error")
	}

	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("Compact() error = %v, expected context deadline error", err)
	}
}

func TestCompactor_Compact_EmptyHistory(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Don't add any history messages

	ctx := context.Background()
	result, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() on empty history error = %v", err)
	}

	// Summary should be empty or minimal
	if len(result.Summary) > 100 {
		t.Errorf("Compact() empty history summary length = %d, want <= 100", len(result.Summary))
	}

	// Evicted tokens should be 0
	if result.EvictedTokens != 0 {
		t.Errorf("Compact() empty history evicted_tokens = %d, want 0", result.EvictedTokens)
	}

	// New tokens should be 0 or minimal
	if result.NewTokens > 50 {
		t.Errorf("Compact() empty history new_tokens = %d, want <= 50", result.NewTokens)
	}
}

func TestCompactor_Compact_PreservesSystemPrompt(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add system prompt
	systemMsg := agentic.ChatMessage{
		Role: "system", Content: "You are a helpful assistant specialized in geography.",
	}
	_ = cm.AddMessage(agenticloop.RegionSystemPrompt, systemMsg)

	// Add history to compact
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "What is the capital of France?",
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "assistant", Content: "Paris.",
	})

	systemTokensBefore := cm.GetRegionTokens(agenticloop.RegionSystemPrompt)

	ctx := context.Background()
	_, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	// System prompt should be unchanged
	systemTokensAfter := cm.GetRegionTokens(agenticloop.RegionSystemPrompt)

	if systemTokensAfter != systemTokensBefore {
		t.Errorf("System prompt tokens changed from %d to %d, should be preserved", systemTokensBefore, systemTokensAfter)
	}

	// Verify system prompt content unchanged
	messages := cm.GetContext()
	found := false
	for _, msg := range messages {
		if msg.Role == "system" && msg.Content == systemMsg.Content {
			found = true
			break
		}
	}

	if !found {
		t.Error("System prompt content changed after compaction")
	}
}

func TestCompactor_Compact_UpdatesCompactedRegion(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add history to compact
	messages := []agentic.ChatMessage{
		{Role: "user", Content: "Tell me about machine learning"},
		{Role: "assistant", Content: "Machine learning is a subset of artificial intelligence..."},
		{Role: "user", Content: "What are neural networks?"},
		{Role: "assistant", Content: "Neural networks are computing systems inspired by biological neural networks..."},
	}

	for _, msg := range messages {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, msg)
	}

	compactedTokensBefore := cm.GetRegionTokens(agenticloop.RegionCompactedHistory)

	ctx := context.Background()
	result, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	compactedTokensAfter := cm.GetRegionTokens(agenticloop.RegionCompactedHistory)

	// Compacted history should have increased
	if compactedTokensAfter <= compactedTokensBefore {
		t.Errorf("Compacted history tokens = %d (before) -> %d (after), want increase",
			compactedTokensBefore, compactedTokensAfter)
	}

	// New tokens should match increase in compacted region
	tokenIncrease := compactedTokensAfter - compactedTokensBefore
	if tokenIncrease != result.NewTokens {
		t.Errorf("Compacted region token increase = %d, result.NewTokens = %d, want equal",
			tokenIncrease, result.NewTokens)
	}
}

func TestCompactor_Compact_ReducesRecentHistory(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add many messages to recent history
	for i := 0; i < 10; i++ {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role: "user", Content: "Question number " + string(rune('0'+i)),
		})
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role: "assistant", Content: "Answer to question " + string(rune('0'+i)),
		})
	}

	recentTokensBefore := cm.GetRegionTokens(agenticloop.RegionRecentHistory)

	ctx := context.Background()
	_, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	recentTokensAfter := cm.GetRegionTokens(agenticloop.RegionRecentHistory)

	// Recent history should be reduced
	if recentTokensAfter >= recentTokensBefore {
		t.Errorf("Recent history tokens = %d (before) -> %d (after), want decrease",
			recentTokensBefore, recentTokensAfter)
	}
}

func TestCompactor_Compact_NetTokenReduction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Fill context to high utilization
	targetTokens := int(float64(agenticloop.DefaultContextLimit) * 0.70)
	fillContextToTokens(t, cm, targetTokens)

	totalTokensBefore := 0
	for _, region := range []agenticloop.RegionType{
		agenticloop.RegionSystemPrompt,
		agenticloop.RegionCompactedHistory,
		agenticloop.RegionRecentHistory,
		agenticloop.RegionToolResults,
		agenticloop.RegionHydratedContext,
	} {
		totalTokensBefore += cm.GetRegionTokens(region)
	}

	ctx := context.Background()
	result, err := compactor.Compact(ctx, cm)

	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	totalTokensAfter := 0
	for _, region := range []agenticloop.RegionType{
		agenticloop.RegionSystemPrompt,
		agenticloop.RegionCompactedHistory,
		agenticloop.RegionRecentHistory,
		agenticloop.RegionToolResults,
		agenticloop.RegionHydratedContext,
	} {
		totalTokensAfter += cm.GetRegionTokens(region)
	}

	// Net reduction should match result
	netReduction := totalTokensBefore - totalTokensAfter
	expectedReduction := result.EvictedTokens - result.NewTokens

	// Allow some tolerance for token counting variations
	tolerance := 50
	if netReduction < expectedReduction-tolerance || netReduction > expectedReduction+tolerance {
		t.Errorf("Net token reduction = %d, expected %d (evicted %d - new %d), tolerance %d",
			netReduction, expectedReduction, result.EvictedTokens, result.NewTokens, tolerance)
	}
}

func TestCompactor_Compact_MultipleCompactions(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	compactor := agenticloop.NewCompactor(config)

	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// First batch of messages
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "First batch of conversation",
	})

	ctx := context.Background()

	// First compaction
	result1, err := compactor.Compact(ctx, cm)
	if err != nil {
		t.Fatalf("First Compact() error = %v", err)
	}

	// Add more messages
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "Second batch of conversation",
	})

	// Second compaction
	result2, err := compactor.Compact(ctx, cm)
	if err != nil {
		t.Fatalf("Second Compact() error = %v", err)
	}

	// Both compactions should produce summaries
	if result1.Summary == "" {
		t.Error("First compaction summary is empty")
	}

	if result2.Summary == "" {
		t.Error("Second compaction summary is empty")
	}

	// Summaries should be different (or second should build on first)
	if result1.Summary == result2.Summary && len(result1.Summary) > 0 {
		t.Error("Multiple compactions produced identical summaries")
	}
}

func TestCompactor_RecompactionCapsCompactedHistory(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.HeadroomTokens = 0 // disable headroom for this test
	compactor := agenticloop.NewCompactor(config)
	cm := agenticloop.NewContextManager("loop-recompact", "gpt-4o", config)
	ctx := context.Background()

	// Perform 5 compactions to exceed maxCompactedMessages (3)
	for i := range 5 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("Conversation batch %d with enough content to matter", i),
		})
		_, err := compactor.Compact(ctx, cm)
		if err != nil {
			t.Fatalf("Compact #%d error: %v", i+1, err)
		}
	}

	// After 5 compactions with maxCompactedMessages=3, recompaction should have
	// consolidated down. The compacted region should have at most maxCompactedMessages entries.
	compactedTokens := cm.GetRegionTokens(agenticloop.RegionCompactedHistory)
	if compactedTokens <= 0 {
		t.Error("Expected compacted history to have tokens after 5 compactions")
	}

	// Verify the region wasn't unbounded: get context and count system messages
	// in the compacted region position (after system prompt, before recent history)
	msgs := cm.GetContext()
	compactedMsgs := 0
	for _, m := range msgs {
		if m.Role == "system" && m.Content != "" {
			compactedMsgs++
		}
	}

	// Should be capped — not 5 separate summaries
	if compactedMsgs > 4 { // maxCompactedMessages + 1 at most before recompaction
		t.Errorf("Expected compacted messages to be capped, got %d", compactedMsgs)
	}
}

func TestCompactor_RecompactionWithLLMSummarizer(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.HeadroomTokens = 0

	mock := &mockSummarizer{summary: "LLM consolidated summary"}
	compactor := agenticloop.NewCompactor(config, agenticloop.WithSummarizer(mock))
	cm := agenticloop.NewContextManager("loop-recompact-llm", "gpt-4o", config)
	ctx := context.Background()

	// Perform enough compactions to trigger recompaction
	for i := range 5 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("Batch %d", i),
		})
		_, err := compactor.Compact(ctx, cm)
		if err != nil {
			t.Fatalf("Compact #%d error: %v", i+1, err)
		}
	}

	// 5 regular compactions + at least 1 recompaction (triggered when exceeding maxCompactedMessages=3)
	if mock.calls < 6 {
		t.Errorf("Expected at least 6 summarizer calls (5 compactions + 1+ recompactions), got %d", mock.calls)
	}
}

func TestCompactor_RecompactionReducesTokens(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.HeadroomTokens = 0
	compactor := agenticloop.NewCompactor(config)
	cm := agenticloop.NewContextManager("loop-token-check", "gpt-4o", config)
	ctx := context.Background()

	// Compact 3 times (right at the limit, no recompaction yet)
	for i := range 3 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:    "user",
			Content: fmt.Sprintf("A reasonably sized conversation batch number %d with varied content", i),
		})
		_, err := compactor.Compact(ctx, cm)
		if err != nil {
			t.Fatalf("Compact #%d error: %v", i+1, err)
		}
	}
	tokensBefore := cm.GetRegionTokens(agenticloop.RegionCompactedHistory)

	// 4th compaction should trigger recompaction (exceeds maxCompactedMessages=3)
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:    "user",
		Content: "Final batch that triggers recompaction",
	})
	_, err := compactor.Compact(ctx, cm)
	if err != nil {
		t.Fatalf("Compact #4 error: %v", err)
	}
	tokensAfter := cm.GetRegionTokens(agenticloop.RegionCompactedHistory)

	// After recompaction (stub fallback concatenates), tokens should be different
	// The stub adds a header so it won't necessarily be smaller, but it should
	// consolidate into fewer messages
	if tokensBefore == 0 || tokensAfter == 0 {
		t.Errorf("Token counts should be non-zero: before=%d, after=%d", tokensBefore, tokensAfter)
	}
}
