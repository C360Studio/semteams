package agenticloop_test

import (
	"testing"

	"github.com/c360studio/semstreams/agentic"
	agenticloop "github.com/c360studio/semstreams/processor/agentic-loop"
)

// toolPair adds an assistant message with tool calls and matching tool result messages.
func toolPair(t *testing.T, cm *agenticloop.ContextManager, callIDs ...string) {
	t.Helper()
	calls := make([]agentic.ToolCall, len(callIDs))
	for i, id := range callIDs {
		calls[i] = agentic.ToolCall{ID: id, Name: "test_tool"}
	}
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:      "assistant",
		Content:   "Calling tools",
		ToolCalls: calls,
	})
	for _, id := range callIDs {
		_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
			Role:       "tool",
			Content:    "Result for " + id,
			ToolCallID: id,
		})
	}
}

func TestContextManager_GCToolResults_AgeBasedEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Iteration 1: Add tool pair
	toolPair(t, cm, "call-1")
	cm.AdvanceIteration() // Move to iteration 2

	// Iteration 2: Add tool pair
	toolPair(t, cm, "call-2")
	cm.AdvanceIteration() // Move to iteration 3

	// Iteration 3: Add tool pair
	toolPair(t, cm, "call-3")
	cm.AdvanceIteration() // Move to iteration 4

	// Iteration 4: Add tool pair
	toolPair(t, cm, "call-4")

	// Verify messages exist
	if cm.GetRegionTokens(agenticloop.RegionRecentHistory) == 0 {
		t.Fatal("Recent history region should have tokens before GC")
	}

	// Run GC at iteration 5 (age 4 iterations)
	// Should evict iteration 1 tool result (age 4 > maxAge 3)
	// AND its assistant message (repair pass)
	evicted := cm.GCToolResults(5)
	if evicted != 2 { // 1 tool result + 1 assistant
		t.Errorf("GCToolResults(5) evicted %d messages, want 2", evicted)
	}

	// Run GC at iteration 6
	evicted = cm.GCToolResults(6)
	if evicted != 2 {
		t.Errorf("GCToolResults(6) evicted %d messages, want 2", evicted)
	}

	// Run GC at iteration 7
	evicted = cm.GCToolResults(7)
	if evicted != 2 {
		t.Errorf("GCToolResults(7) evicted %d messages, want 2", evicted)
	}

	// Run GC at iteration 8
	evicted = cm.GCToolResults(8)
	if evicted != 2 {
		t.Errorf("GCToolResults(8) evicted %d messages, want 2", evicted)
	}

	// All messages should be evicted
	messages := cm.GetContext()
	for _, msg := range messages {
		if msg.Role == "tool" || (msg.Role == "assistant" && len(msg.ToolCalls) > 0) {
			t.Errorf("Unexpected %s message remaining after full GC", msg.Role)
		}
	}
}

func TestContextManager_GCToolResults_NoEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Iteration 1: Add tool pair
	toolPair(t, cm, "call-1")

	initialTokens := cm.GetRegionTokens(agenticloop.RegionRecentHistory)

	// GC at iterations 1-4 (ages 0-3, all within maxAge)
	for i := 1; i <= 4; i++ {
		evicted := cm.GCToolResults(i)
		if evicted != 0 {
			t.Errorf("GCToolResults(%d) evicted %d results, want 0", i, evicted)
		}
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

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Add tool pairs across iterations
	for i := 1; i <= 10; i++ {
		callID := "call-" + string(rune('0'+i))
		toolPair(t, cm, callID)

		// Run GC (also advances iteration for next loop)
		evicted := cm.GCToolResults(i)

		// For iterations 1-3, no eviction expected
		if i <= 3 && evicted != 0 {
			t.Errorf("Iteration %d: GCToolResults() evicted %d, want 0", i, evicted)
		}

		// For iteration 4+, should evict old pairs
		if i > 3 && evicted == 0 {
			t.Errorf("Iteration %d: GCToolResults() evicted 0, want > 0", i)
		}
	}

	// After iteration 10, only recent iterations should remain
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

	evicted := cm.GCToolResults(5)
	if evicted != 0 {
		t.Errorf("GCToolResults() on empty region evicted %d, want 0", evicted)
	}
}

func TestContextManager_GCToolResults_MaxAgeOne(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 1
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Add tool pair at iteration 1
	toolPair(t, cm, "call-1")

	// GC at iteration 1 (age 0)
	evicted := cm.GCToolResults(1)
	if evicted != 0 {
		t.Errorf("GCToolResults(1) evicted %d, want 0", evicted)
	}

	// GC at iteration 2 (age 1, equals maxAge)
	evicted = cm.GCToolResults(2)
	if evicted != 0 {
		t.Errorf("GCToolResults(2) evicted %d, want 0 (age equals maxAge)", evicted)
	}

	// GC at iteration 3 (age 2 > maxAge 1)
	evicted = cm.GCToolResults(3)
	if evicted != 2 { // 1 tool result + 1 assistant
		t.Errorf("GCToolResults(3) evicted %d, want 2", evicted)
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

	// Add old tool pair interleaved
	toolPair(t, cm, "call-1")

	// Run GC at iteration 10 (should evict tool pair but keep user/assistant)
	evicted := cm.GCToolResults(10)

	if evicted != 2 { // 1 tool result + 1 assistant with tool calls
		t.Errorf("GCToolResults(10) evicted %d, want 2", evicted)
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
		if msg.Role == "assistant" && msg.Content == "Assistant response" && len(msg.ToolCalls) == 0 {
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
		t.Error("Assistant message (without tool calls) should survive GC")
	}
}

func TestContextManager_GCToolResults_BatchEviction(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Add assistant with 10 tool calls (one group)
	callIDs := make([]string, 10)
	for i := range 10 {
		callIDs[i] = "call-" + string(rune('0'+i))
	}
	toolPair(t, cm, callIDs...)

	// Run GC at iteration 10 (all results are age 9, should evict all)
	evicted := cm.GCToolResults(10)

	// 10 tool results + 1 assistant = 11
	if evicted != 11 {
		t.Errorf("GCToolResults(10) evicted %d, want 11", evicted)
	}

	// No messages should remain
	messages := cm.GetContext()
	for _, msg := range messages {
		if msg.Role == "tool" || (msg.Role == "assistant" && len(msg.ToolCalls) > 0) {
			t.Error("All tool pair messages should be evicted after batch eviction")
		}
	}
}

func TestContextManager_GCToolResults_ZeroIteration(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	toolPair(t, cm, "call-1")

	evicted := cm.GCToolResults(0)
	if evicted != 0 {
		t.Errorf("GCToolResults(0) evicted %d, want 0", evicted)
	}
}

func TestContextManager_GCToolResults_ReturnValue(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 3
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Add 3 tool pairs at iteration 1
	toolPair(t, cm, "call-1a", "call-1b", "call-1c")

	// Advance to iteration 2
	cm.AdvanceIteration()

	// Add 2 tool pairs at iteration 2
	toolPair(t, cm, "call-2a", "call-2b")

	// GC at iteration 5:
	// - iteration 1 results: age = 5-1 = 4 > maxAge 3, evict
	// - iteration 2 results: age = 5-2 = 3 <= maxAge 3, keep
	evicted := cm.GCToolResults(5)

	// Should evict 3 tool results + 1 assistant = 4
	if evicted != 4 {
		t.Errorf("GCToolResults(5) evicted %d, want 4", evicted)
	}

	// GC at iteration 6:
	// - iteration 2 results: age = 6-2 = 4 > maxAge 3, evict
	evicted = cm.GCToolResults(6)

	// Should evict 2 tool results + 1 assistant = 3
	if evicted != 3 {
		t.Errorf("GCToolResults(6) evicted %d, want 3", evicted)
	}
}

// --- Tool pair integrity tests ---

func TestContextManager_GCToolResults_EvictsEntireGroup(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 2
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Add assistant with 3 tool calls at iteration 1
	toolPair(t, cm, "call-a", "call-b", "call-c")

	// GC at iteration 4 (age 3 > maxAge 2 for all results)
	evicted := cm.GCToolResults(4)

	// All 4 messages (1 assistant + 3 tool results) should be evicted
	if evicted != 4 {
		t.Errorf("GCToolResults(4) evicted %d, want 4 (entire group)", evicted)
	}

	messages := cm.GetContext()
	for _, msg := range messages {
		if msg.Role == "tool" || (msg.Role == "assistant" && len(msg.ToolCalls) > 0) {
			t.Errorf("Expected entire tool group to be evicted, found %s message", msg.Role)
		}
	}
}

func TestContextManager_GCToolResults_PreservesCompleteGroups(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 5
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add tool pair at iteration 1
	toolPair(t, cm, "call-x", "call-y")

	// GC at iteration 3 (age 2, within maxAge 5)
	evicted := cm.GCToolResults(3)
	if evicted != 0 {
		t.Errorf("GCToolResults(3) evicted %d, want 0 (group still young)", evicted)
	}

	// Verify complete group is intact
	messages := cm.GetContext()
	var assistantCount, toolCount int
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			assistantCount++
		}
		if msg.Role == "tool" {
			toolCount++
		}
	}
	if assistantCount != 1 {
		t.Errorf("Expected 1 assistant message, got %d", assistantCount)
	}
	if toolCount != 2 {
		t.Errorf("Expected 2 tool results, got %d", toolCount)
	}
}

func TestContextManager_GCToolResults_MixedAgeGroups(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 2
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// User message ensures repair safety check finds conversation content
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "user", Content: "test prompt",
	})

	// Old group at iteration 1
	toolPair(t, cm, "old-a", "old-b")
	cm.AdvanceIteration() // iteration 2

	// Young group at iteration 2
	toolPair(t, cm, "new-a")

	// GC at iteration 4: old group age=3 > maxAge=2, new group age=2 <= maxAge=2
	evicted := cm.GCToolResults(4)

	// Old group: 1 assistant + 2 tool results = 3
	if evicted != 3 {
		t.Errorf("GCToolResults(4) evicted %d, want 3 (old group only)", evicted)
	}

	// New group should be intact
	messages := cm.GetContext()
	var foundAssistant, foundTool bool
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			foundAssistant = true
		}
		if msg.Role == "tool" && msg.ToolCallID == "new-a" {
			foundTool = true
		}
		if msg.Role == "tool" && (msg.ToolCallID == "old-a" || msg.ToolCallID == "old-b") {
			t.Error("Old tool result should have been evicted")
		}
	}
	if !foundAssistant {
		t.Error("New assistant message should survive")
	}
	if !foundTool {
		t.Error("New tool result should survive")
	}
}

func TestContextManager_SliceForBudget_PreservesToolPairs(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	cm := agenticloop.NewContextManager("loop-1", "gpt-4o", config)

	// Add a tool pair to recent history
	toolPair(t, cm, "budget-call-1")

	// Add a large filler message to push over budget
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:    "user",
		Content: string(make([]byte, 1000)),
	})

	// Slice to a small budget that forces eviction
	err := cm.SliceForBudget(100, agenticloop.ContextSlice{})
	if err != nil {
		t.Fatalf("SliceForBudget failed: %v", err)
	}

	// After slicing, verify no orphaned tool messages remain
	messages := cm.GetContext()
	assistantCallIDs := make(map[string]bool)
	toolResultIDs := make(map[string]bool)

	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			assistantCallIDs[tc.ID] = true
		}
		if msg.Role == "tool" && msg.ToolCallID != "" {
			toolResultIDs[msg.ToolCallID] = true
		}
	}

	// Every tool result must have a matching assistant
	for id := range toolResultIDs {
		if !assistantCallIDs[id] {
			t.Errorf("Orphaned tool result %s: no matching assistant message", id)
		}
	}

	// Every assistant tool call must have a matching result
	for id := range assistantCallIDs {
		if !toolResultIDs[id] {
			t.Errorf("Orphaned assistant tool call %s: no matching tool result", id)
		}
	}
}

// TestContextManager_RepairPreservesUserMessage verifies that repairToolPairsLocked
// never removes non-tool, non-assistant messages (e.g., the user prompt). Even if
// all tool pairs are orphaned and removed, the user message must survive so the
// next model request has non-empty contents.
func TestContextManager_RepairPreservesUserMessage(t *testing.T) {
	config := agenticloop.DefaultContextConfig()
	config.ToolResultMaxAge = 1 // Aggressive: evict after 1 iteration
	cm := agenticloop.NewContextManager("loop-repair-user", "gpt-4o", config)

	// Add user message (this must survive all GC/repair)
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:    "user",
		Content: "Fix the bug",
	})

	// Iteration 1: Add a tool pair
	toolPair(t, cm, "call-orphan-1")
	cm.AdvanceIteration() // → iteration 2

	// Iteration 2: Add another tool pair
	toolPair(t, cm, "call-orphan-2")
	cm.AdvanceIteration() // → iteration 3

	// Iteration 3: no new tool pairs, just advance
	cm.AdvanceIteration() // → iteration 4

	// GC at iteration 4: tool results from iteration 1 have age=3, maxAge=1 → evicted.
	// Repair removes their orphaned assistant messages.
	evicted := cm.GCToolResults(4)
	if evicted == 0 {
		t.Fatal("Expected at least some messages to be evicted by GC/repair")
	}

	// The user message MUST survive
	messages := cm.GetContext()
	if len(messages) == 0 {
		t.Fatal("GetContext() returned empty — user message was lost during GC/repair")
	}

	foundUser := false
	for _, m := range messages {
		if m.Role == "user" && m.Content == "Fix the bug" {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Errorf("User message not found in context after GC/repair. Messages: %d", len(messages))
	}
}

func TestGCToolResults_EvictsMixedErrorGroup(t *testing.T) {
	// Regression test for the tool pair orphaning bug:
	// When GC evicts non-error results but preserves error results from the
	// same group, the group becomes incomplete — the assistant message has
	// tool_call IDs without matching tool results. This causes 400 errors
	// from provider APIs (OpenAI, Anthropic, Gemini) that require every
	// tool_call to have a corresponding tool result.
	//
	// Fix: repairToolPairsLocked evicts the ENTIRE group when any result
	// is missing, regardless of error status.
	cm := agenticloop.NewContextManager("test-loop", "test-model", agenticloop.ContextConfig{
		ToolResultMaxAge: 2,
	})

	// Iteration 1: assistant calls two tools, one succeeds, one errors
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "assistant",
		ToolCalls: []agentic.ToolCall{
			{ID: "ok-1", Name: "read_file"},
			{ID: "err-1", Name: "graph_search"},
		},
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:       "tool",
		ToolCallID: "ok-1",
		Content:    "file contents here",
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:       "tool",
		ToolCallID: "err-1",
		Content:    "Tool error: invalid request",
		IsError:    true,
	})

	// Advance well past ToolResultMaxAge
	evicted := cm.GCToolResults(10)

	// GC evicts ok-1 (non-error, age 9 > max 2), keeps err-1 (error exempt).
	// Repair sees the group is incomplete (ok-1 missing) and evicts the
	// entire group: assistant + err-1. This prevents orphaned tool_call IDs.
	messages := cm.GetContext()

	for _, m := range messages {
		if m.ToolCallID == "err-1" {
			t.Error("Error tool result should be evicted as part of incomplete group")
		}
		if m.ToolCallID == "ok-1" {
			t.Error("Success tool result should have been evicted by age")
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			t.Error("Assistant with tool_calls should be evicted as part of incomplete group")
		}
	}

	if evicted == 0 {
		t.Error("Expected messages to be evicted")
	}
}

func TestGCToolResults_PreservesCompleteErrorGroup(t *testing.T) {
	// When ALL results in a group are errors, none are evicted by GC
	// (IsError exempt from age-based eviction), so the group stays complete.
	// Repair sees all results present and does not touch the group.
	cm := agenticloop.NewContextManager("test-loop", "test-model", agenticloop.ContextConfig{
		ToolResultMaxAge: 2,
	})

	// Assistant calls two tools, both error
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role: "assistant",
		ToolCalls: []agentic.ToolCall{
			{ID: "err-a", Name: "read_file"},
			{ID: "err-b", Name: "graph_search"},
		},
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:       "tool",
		ToolCallID: "err-a",
		Content:    "Tool error: not found",
		IsError:    true,
	})
	_ = cm.AddMessage(agenticloop.RegionRecentHistory, agentic.ChatMessage{
		Role:       "tool",
		ToolCallID: "err-b",
		Content:    "Tool error: timeout",
		IsError:    true,
	})

	// Advance well past ToolResultMaxAge
	evicted := cm.GCToolResults(10)

	// Both error results survive GC → group stays complete → repair leaves it alone
	if evicted != 0 {
		t.Errorf("Complete error group should not be evicted, got evicted=%d", evicted)
	}

	messages := cm.GetContext()
	var foundErrA, foundErrB, foundAssistant bool
	for _, m := range messages {
		if m.ToolCallID == "err-a" {
			foundErrA = true
		}
		if m.ToolCallID == "err-b" {
			foundErrB = true
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			foundAssistant = true
		}
	}

	if !foundErrA || !foundErrB || !foundAssistant {
		t.Error("Complete error group should be preserved intact")
	}
}
