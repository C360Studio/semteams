package agenticloop_test

import (
	"testing"

	"github.com/c360studio/semstreams/agentic"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

func TestContextManager_GCToolResults_AgeBasedEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Iteration 1: Add tool result
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result from iteration 1", ToolCallID: "call-1",
	})
	cm.AdvanceIteration() // Move to iteration 2

	// Iteration 2: Add tool result
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result from iteration 2", ToolCallID: "call-2",
	})
	cm.AdvanceIteration() // Move to iteration 3

	// Iteration 3: Add tool result
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result from iteration 3", ToolCallID: "call-3",
	})
	cm.AdvanceIteration() // Move to iteration 4

	// Iteration 4: Add tool result
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result from iteration 4", ToolCallID: "call-4",
	})

	// Verify all 4 results exist
	if cm.GetRegionTokens(agenticloop.RegionRecentHistory) == 0 {
		t.Fatal("Recent history region should have tokens before GC")
	}

	// Run GC at iteration 5 (age 4 iterations)
	evicted := cm.GCToolResults(5)

	// Should evict iteration 1 result (age 4 > maxAge 3)
	if evicted != 1 {
		t.Errorf("GCToolResults(5) evicted %d results, want 1", evicted)
	}

	// Run GC at iteration 6 (age 5 iterations)
	evicted = cm.GCToolResults(6)

	// Should evict iteration 2 result (age 4 > maxAge 3)
	if evicted != 1 {
		t.Errorf("GCToolResults(6) evicted %d results, want 1", evicted)
	}

	// Run GC at iteration 7
	evicted = cm.GCToolResults(7)

	// Should evict iteration 3 result
	if evicted != 1 {
		t.Errorf("GCToolResults(7) evicted %d results, want 1", evicted)
	}

	// Run GC at iteration 8
	evicted = cm.GCToolResults(8)

	// Should evict iteration 4 result
	if evicted != 1 {
		t.Errorf("GCToolResults(8) evicted %d results, want 1", evicted)
	}

	// All tool results should be evicted — only check via GetContext
	messages := cm.GetContext()
	for _, msg := range messages {
		if msg.Role == "tool" {
			t.Error("All tool results should be evicted after full GC")
		}
	}
}

func TestContextManager_GCToolResults_NoEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Iteration 1: Add tool results
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result A", ToolCallID: "call-1",
	})

	initialTokens := cm.GetRegionTokens(agenticloop.RegionRecentHistory)

	// Run GC at iteration 1 (age 0)
	evicted := cm.GCToolResults(1)

	if evicted != 0 {
		t.Errorf("GCToolResults(1) evicted %d results, want 0 (age 0)", evicted)
	}

	// Run GC at iteration 2 (age 1)
	evicted = cm.GCToolResults(2)

	if evicted != 0 {
		t.Errorf("GCToolResults(2) evicted %d results, want 0 (age 1)", evicted)
	}

	// Run GC at iteration 3 (age 2)
	evicted = cm.GCToolResults(3)

	if evicted != 0 {
		t.Errorf("GCToolResults(3) evicted %d results, want 0 (age 2)", evicted)
	}

	// Run GC at iteration 4 (age 3, at max age)
	evicted = cm.GCToolResults(4)

	if evicted != 0 {
		t.Errorf("GCToolResults(4) evicted %d results, want 0 (age 3, at max)", evicted)
	}

	// Tokens should be unchanged
	currentTokens := cm.GetRegionTokens(agenticloop.RegionRecentHistory)
	if currentTokens != initialTokens {
		t.Errorf("Region tokens changed from %d to %d, should be unchanged within maxAge", initialTokens, currentTokens)
	}
}

func TestContextManager_GCToolResults_MultipleIterations(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 2
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add tool results across iterations
	for i := 1; i <= 10; i++ {
		// Add result for this iteration
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:       "tool",
			Content:    "Result from iteration " + string(rune('0'+i)),
			ToolCallID: "call-" + string(rune('0'+i)),
		})

		// Run GC (also advances iteration for next loop)
		evicted := cm.GCToolResults(i)

		// For iterations 1-3, no eviction expected
		if i <= 3 {
			if evicted != 0 {
				t.Errorf("Iteration %d: GCToolResults() evicted %d, want 0", i, evicted)
			}
		}

		// For iteration 4+, should evict old results
		if i > 3 {
			if evicted == 0 {
				t.Errorf("Iteration %d: GCToolResults() evicted 0, want > 0", i)
			}
		}
	}

	// After iteration 10, only iterations 8, 9, 10 should remain (maxAge = 2)
	// Should have at most 3 results
	messages := cm.GetContext()
	toolResultCount := 0
	for _, msg := range messages {
		if msg.Role == "tool" {
			toolResultCount++
		}
	}

	if toolResultCount > 3 {
		t.Errorf("After iteration 10, found %d tool results, want <= 3", toolResultCount)
	}
}

func TestContextManager_GCToolResults_EmptyRegion(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Don't add any tool results

	// Run GC should not error
	evicted := cm.GCToolResults(5)

	if evicted != 0 {
		t.Errorf("GCToolResults() on empty region evicted %d, want 0", evicted)
	}
}

func TestContextManager_GCToolResults_MaxAgeOne(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 1
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add result at iteration 1
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result 1", ToolCallID: "call-1",
	})

	// GC at iteration 1 (age 0)
	evicted := cm.GCToolResults(1)
	if evicted != 0 {
		t.Errorf("GCToolResults(1) evicted %d, want 0", evicted)
	}

	// GC at iteration 2 (age 1)
	evicted = cm.GCToolResults(2)
	if evicted != 0 {
		t.Errorf("GCToolResults(2) evicted %d, want 0 (age equals maxAge)", evicted)
	}

	// GC at iteration 3 (age 2 > maxAge 1)
	evicted = cm.GCToolResults(3)
	if evicted != 1 {
		t.Errorf("GCToolResults(3) evicted %d, want 1", evicted)
	}
}

func TestContextManager_GCToolResults_PreservesNonToolMessages(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 2
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add non-tool messages to recent history
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "User message",
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "assistant", Content: "Assistant response",
	})

	// Add system prompt to verify other regions are untouched
	_ = cm.AddMessage(agenticloop.RegionSystemPrompt, agentic.ChatMessage{
		Role: "system", Content: "System prompt",
	})

	systemTokensBefore := cm.GetRegionTokens(agenticloop.RegionSystemPrompt)

	// Add old tool results interleaved
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Old result", ToolCallID: "call-1",
	})

	// Run GC at iteration 10 (should evict tool result but keep user/assistant)
	evicted := cm.GCToolResults(10)

	if evicted != 1 {
		t.Errorf("GCToolResults(10) evicted %d, want 1", evicted)
	}

	// System prompt should be unchanged
	systemTokensAfter := cm.GetRegionTokens(agenticloop.RegionSystemPrompt)
	if systemTokensAfter != systemTokensBefore {
		t.Errorf("System prompt tokens changed from %d to %d", systemTokensBefore, systemTokensAfter)
	}

	// Non-tool messages in recent history should survive
	messages := cm.GetContext()
	var hasUser, hasAssistant bool
	for _, msg := range messages {
		if msg.Role == "user" && msg.Content == "User message" {
			hasUser = true
		}
		if msg.Role == "assistant" && msg.Content == "Assistant response" {
			hasAssistant = true
		}
		if msg.Role == "tool" {
			t.Error("Tool result should have been evicted")
		}
	}
	if !hasUser {
		t.Error("User message should survive GC")
	}
	if !hasAssistant {
		t.Error("Assistant message should survive GC")
	}
}

func TestContextManager_GCToolResults_BatchEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add 10 tool results at iteration 1 (batch - all same iteration)
	for i := range 10 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:       "tool",
			Content:    "Result " + string(rune('0'+i)),
			ToolCallID: "call-" + string(rune('0'+i)),
		})
	}

	// Run GC at iteration 10 (all results are age 9, should evict all)
	evicted := cm.GCToolResults(10)

	if evicted != 10 {
		t.Errorf("GCToolResults(10) evicted %d, want 10", evicted)
	}

	// No tool results should remain
	messages := cm.GetContext()
	for _, msg := range messages {
		if msg.Role == "tool" {
			t.Error("All tool results should be evicted after batch eviction")
		}
	}
}

func TestContextManager_GCToolResults_ZeroIteration(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add tool result
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "tool", Content: "Result", ToolCallID: "call-1",
	})

	// GC at iteration 0 (should not evict)
	evicted := cm.GCToolResults(0)

	if evicted != 0 {
		t.Errorf("GCToolResults(0) evicted %d, want 0", evicted)
	}
}

func TestContextManager_GCToolResults_ReturnValue(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add 3 results at iteration 1
	for i := range 3 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:       "tool",
			Content:    "Result " + string(rune('0'+i)),
			ToolCallID: "call-" + string(rune('0'+i)),
		})
	}

	// Advance to iteration 2
	cm.AdvanceIteration()

	// Add 2 results at iteration 2
	for i := range 2 {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:       "tool",
			Content:    "Result new " + string(rune('0'+i)),
			ToolCallID: "call-new-" + string(rune('0'+i)),
		})
	}

	// GC at iteration 5:
	// - iteration 1 results: age = 5-1 = 4 > maxAge 3, evict
	// - iteration 2 results: age = 5-2 = 3 <= maxAge 3, keep
	evicted := cm.GCToolResults(5)

	// Should evict exactly 3 results from iteration 1
	if evicted != 3 {
		t.Errorf("GCToolResults(5) evicted %d, want 3", evicted)
	}

	// GC at iteration 6:
	// - iteration 2 results: age = 6-2 = 4 > maxAge 3, evict
	evicted = cm.GCToolResults(6)

	// Should evict exactly 2 results from iteration 2
	if evicted != 2 {
		t.Errorf("GCToolResults(6) evicted %d, want 2", evicted)
	}
}
