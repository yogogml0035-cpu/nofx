package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BTC 合约 2 张 = 0.0002 BTC：平掉一半后剩余 0.0001。数量容差必须随持仓规模缩放，
// 不能用固定 0.0001，否则第一笔分批平仓就会把记录误关（剩余仓位脱离归属）。
func TestPartialCloseKeepsSmallPositionOpen(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "position-builder.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	pb := NewPositionBuilder(st.Position())
	now := time.Now().UTC().UnixMilli()
	require.NoError(t, pb.ProcessTrade(
		"trader-1", "exchange-1", "gate", "BTCUSDT", "LONG", "open_long",
		0.0002, 60000, 0.10, 0, now, "trade-open"))
	require.NoError(t, pb.ProcessTrade(
		"trader-1", "exchange-1", "gate", "BTCUSDT", "LONG", "close_long",
		0.0001, 60100, 0.05, 0, now+1000, "trade-partial"))

	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	require.NoError(t, err)
	require.NotNil(t, pos, "平掉一半后记录必须仍为 OPEN")
	assert.InDelta(t, 0.0001, pos.Quantity, 1e-9, "剩余数量应为一半")

	require.NoError(t, pb.ProcessTrade(
		"trader-1", "exchange-1", "gate", "BTCUSDT", "LONG", "close_long",
		0.0001, 60200, 0.05, 0, now+2000, "trade-final"))
	pos, err = st.Position().GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	require.NoError(t, err)
	assert.Nil(t, pos, "全部平完后记录关闭")

	closed, err := st.Position().GetClosedPositions("trader-1", 10)
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.InDelta(t, 0.0002, closed[0].Quantity, 1e-9)
}

// 迟到成交回填同样不能用固定 0.0001：0.0002 BTC 的记录回填一半后必须保持 RECONCILED。
func TestBackfillReconciledCloseKeepsSmallRemainder(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "backfill.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	pb := NewPositionBuilder(st.Position())
	now := time.Now().UTC().UnixMilli()
	require.NoError(t, pb.ProcessTrade(
		"trader-1", "exchange-1", "gate", "BTCUSDT", "LONG", "open_long",
		0.0002, 60000, 0.10, 0, now, "trade-open"))
	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	require.NoError(t, err)
	require.NotNil(t, pos)
	require.NoError(t, st.Position().ReconcileClose(pos.ID, ReconciledPositionGone))

	applied, err := st.Position().BackfillReconciledClose(pos.ID, 0.0001, 60100, 0, 0.05, now+1000, "trade-late-1")
	require.NoError(t, err)
	require.True(t, applied)
	pos, err = st.Position().GetBackfillableReconciledPosition("trader-1", "BTCUSDT", "LONG", now-1000)
	require.NoError(t, err)
	require.NotNil(t, pos, "回填一半后仍应可继续回填")
	assert.InDelta(t, 0.0001, pos.Quantity, 1e-9)

	applied, err = st.Position().BackfillReconciledClose(pos.ID, 0.0001, 60200, 0, 0.05, now+2000, "trade-late-2")
	require.NoError(t, err)
	require.True(t, applied)
	openPos, err := st.Position().GetOpenPositionBySymbol("trader-1", "BTCUSDT", "LONG")
	require.NoError(t, err)
	assert.Nil(t, openPos)
	closed, err := st.Position().GetClosedPositions("trader-1", 10)
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.Equal(t, PositionStatusClosed, closed[0].Status)
}
