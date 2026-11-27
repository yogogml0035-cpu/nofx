package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeReasoningEffort(t *testing.T) {
	for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
		got, ok := NormalizeReasoningEffort("  " + level + "  ")
		require.True(t, ok, level)
		assert.Equal(t, level, got)
	}

	got, ok := NormalizeReasoningEffort("")
	require.True(t, ok)
	assert.Equal(t, DefaultReasoningEffort, got)

	if _, ok := NormalizeReasoningEffort("ultra"); ok {
		t.Fatal("unknown effort must be rejected")
	}
}

func TestAIModelReasoningEffortPersistsAndPreservesOnEmpty(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "ai-model.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	// Create with an explicit level.
	require.NoError(t, st.AIModel().UpdateWithName(
		"user-1", "deepseek", "DeepSeek", true, "sk-test", "", "deepseek-chat", ReasoningEffortHigh,
	))
	modelID := "user-1_deepseek"
	stored, err := st.AIModel().Get("user-1", modelID)
	require.NoError(t, err)
	assert.Equal(t, ReasoningEffortHigh, stored.ReasoningEffort)

	// A partial update without the field must not reset the user's choice.
	require.NoError(t, st.AIModel().UpdateWithName(
		"user-1", modelID, "", false, "", "", "", "",
	))
	stored, err = st.AIModel().Get("user-1", modelID)
	require.NoError(t, err)
	assert.Equal(t, ReasoningEffortHigh, stored.ReasoningEffort)
	assert.False(t, stored.Enabled)

	// An explicit level replaces it.
	require.NoError(t, st.AIModel().UpdateWithName(
		"user-1", modelID, "", true, "", "", "", ReasoningEffortMax,
	))
	stored, err = st.AIModel().Get("user-1", modelID)
	require.NoError(t, err)
	assert.Equal(t, ReasoningEffortMax, stored.ReasoningEffort)

	// Invalid values are rejected before touching the row.
	require.Error(t, st.AIModel().UpdateWithName(
		"user-1", modelID, "", true, "", "", "", "ultra",
	))
	stored, err = st.AIModel().Get("user-1", modelID)
	require.NoError(t, err)
	assert.Equal(t, ReasoningEffortMax, stored.ReasoningEffort)
}

func TestAIModelReasoningEffortNewRowDefaultsToLow(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "ai-model-default.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	require.NoError(t, st.AIModel().UpdateWithName(
		"user-1", "qwen", "Qwen", true, "sk-test", "", "qwen3-max", "",
	))
	stored, err := st.AIModel().Get("user-1", "user-1_qwen")
	require.NoError(t, err)
	assert.Equal(t, DefaultReasoningEffort, stored.ReasoningEffort)
}
