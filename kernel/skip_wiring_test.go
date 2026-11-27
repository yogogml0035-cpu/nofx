package kernel

// 无行情且无仓位时 GetFullDecisionWithStrategy 不调用模型（接线测试，测试缝替换行情取数）。

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/mcp"
	"nofx/store"
)

type countingAIClient struct{ calls int }

func (c *countingAIClient) SetAPIKey(string, string, string) {}
func (c *countingAIClient) SetTimeout(time.Duration)         {}
func (c *countingAIClient) CallWithMessages(string, string) (string, error) {
	c.calls++
	return "<decision>[]</decision>", nil
}
func (c *countingAIClient) CallWithRequest(*mcp.Request) (string, error) {
	c.calls++
	return "<decision>[]</decision>", nil
}
func (c *countingAIClient) CallWithRequestStream(*mcp.Request, func(string)) (string, error) {
	c.calls++
	return "<decision>[]</decision>", nil
}
func (c *countingAIClient) CallWithRequestFull(*mcp.Request) (*mcp.LLMResponse, error) {
	c.calls++
	return &mcp.LLMResponse{
		Content:      "<decision>[]</decision>",
		FinishReason: "stop",
	}, nil
}

func TestFullDecisionSkipsModelWithoutDataOrPositions(t *testing.T) {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.StaticCoins = []string{"BTCUSDT"}
	engine := NewStrategyEngine(&cfg)

	previous := fetchMarketDataForDecision
	fetchMarketDataForDecision = func(*Context, *StrategyEngine) error { return nil } // 全部取数失败
	t.Cleanup(func() { fetchMarketDataForDecision = previous })

	client := &countingAIClient{}
	decision, err := GetFullDecisionWithStrategy(&Context{
		CandidateCoins: []CandidateCoin{{Symbol: "BTCUSDT", Sources: []string{"static"}}},
		Account:        AccountInfo{Principal: 1000},
	}, client, engine, "balanced")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.True(t, strings.Contains(decision.SkippedReason, "no market data"), "skip reason = %q", decision.SkippedReason)
	assert.Zero(t, client.calls, "the model must not be called")
	assert.Empty(t, decision.Decisions)
}

func TestFullDecisionStillCallsModelWithPositions(t *testing.T) {
	// 有仓位时即使没有候选币行情也要调用模型（平仓与改保护不依赖行情）
	cfg := store.GetDefaultStrategyConfig("en")
	engine := NewStrategyEngine(&cfg)
	previous := fetchMarketDataForDecision
	fetchMarketDataForDecision = func(*Context, *StrategyEngine) error { return nil }
	t.Cleanup(func() { fetchMarketDataForDecision = previous })

	client := &countingAIClient{}
	decision, err := GetFullDecisionWithStrategy(&Context{
		Positions: []PositionInfo{{Symbol: "BTCUSDT", Side: "long", EntryPrice: 100, MarkPrice: 100, Quantity: 1}},
		Account:   AccountInfo{Principal: 1000},
	}, client, engine, "balanced")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Empty(t, decision.SkippedReason)
	assert.Equal(t, 1, client.calls)
}
