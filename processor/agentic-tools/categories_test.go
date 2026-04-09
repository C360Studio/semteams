package agentictools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetToolCategory_Known(t *testing.T) {
	assert.Equal(t, CategoryInspect, GetToolCategory("bash"))
	assert.Equal(t, CategoryKnowledge, GetToolCategory("graph_query"))
	assert.Equal(t, CategoryNetwork, GetToolCategory("web_search"))
	assert.Equal(t, CategoryOrchestration, GetToolCategory("spawn_agent"))
	assert.Equal(t, CategoryMeta, GetToolCategory("create_rule"))
	assert.Equal(t, CategoryCore, GetToolCategory("submit_work"))
}

func TestGetToolCategory_Unknown(t *testing.T) {
	assert.Equal(t, CategoryCore, GetToolCategory("unknown_tool"))
}

func TestRegisterToolCategory(t *testing.T) {
	RegisterToolCategory("custom_tool", CategoryNetwork)
	assert.Equal(t, CategoryNetwork, GetToolCategory("custom_tool"))
	// Cleanup
	delete(toolCategories, "custom_tool")
}

func TestReadOnlyCategories(t *testing.T) {
	cats := ReadOnlyCategories()
	assert.True(t, cats[CategoryCore])
	assert.True(t, cats[CategoryKnowledge])
	assert.True(t, cats[CategoryNetwork])
	assert.False(t, cats[CategoryInspect])
	assert.False(t, cats[CategoryOrchestration])
	assert.False(t, cats[CategoryMeta])
}
