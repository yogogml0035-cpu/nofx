package trader

// 启动对账（F63）：停机跨过平仓后残留的 OPEN 记录不能「收养」之后出现的同合约同向仓位。

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
)

func newReconcileStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "reconcile.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func createOpenRecord(t *testing.T, st *store.Store, quantity, entryPrice float64) {
	t.Helper()
	require.NoError(t, st.Position().Create(&store.TraderPosition{
		TraderID: "test-trader", Symbol: "DOGEUSDT", Side: "LONG",
		Quantity: quantity, EntryPrice: entryPrice, EntryTime: time.Now().Add(-2 * 24 * time.Hour).UnixMilli(),
	}))
}

// 交易所上已经没有这个仓位：记录关闭，模型看不到它（也就不会触发兜底平仓）
func TestBuildTradingContextClosesStaleOpenRecord(t *testing.T) {
	st := newReconcileStore(t)
	createOpenRecord(t, st, 17, 0.05)

	at := newOwnershipTrader(&ownershipStub{}) // 交易所上没有任何仓位
	at.store = st
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	assert.Empty(t, ctx.Positions)

	open, err := st.Position().GetOpenPositions("test-trader")
	require.NoError(t, err)
	assert.Empty(t, open, "stale OPEN record must be closed at reconciliation")
}

// 仓位仍在、入场价一致、只有数量不同（错过了部分平仓成交）：把记录数量对齐到交易所
func TestReconcileAlignsQuantityWithExchange(t *testing.T) {
	st := newReconcileStore(t)
	createOpenRecord(t, st, 17, 0.05)

	at := newOwnershipTrader(&ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 9)}})
	at.store = st
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)

	open, err := st.Position().GetOpenPositions("test-trader")
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.InDelta(t, 9.0, open[0].Quantity, 1e-9, "record quantity must follow the exchange")
}

// 仓位在但入场价显著不符（被别人的加减仓摊平过）：视为不可归属，记录关闭，不管理也不平仓
func TestReconcileTreatsEntryMismatchAsForeign(t *testing.T) {
	st := newReconcileStore(t)
	createOpenRecord(t, st, 17, 0.05)

	foreign := ownPosition("DOGE_USDT", "long", 17)
	foreign["entryPrice"] = 0.06
	at := newOwnershipTrader(&ownershipStub{positions: []map[string]interface{}{foreign}})
	at.store = st
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	assert.Empty(t, ctx.Positions, "a foreign position must not be adopted by a stale record")

	open, err := st.Position().GetOpenPositions("test-trader")
	require.NoError(t, err)
	assert.Empty(t, open)
}

// 正常的本 trader 仓位：记录保留，数量一致时不做任何改动
func TestReconcileKeepsMatchingOwnPosition(t *testing.T) {
	st := newReconcileStore(t)
	createOpenRecord(t, st, 17, 0.05)

	at := newOwnershipTrader(&ownershipStub{positions: []map[string]interface{}{ownPosition("DOGE_USDT", "long", 17)}})
	at.store = st
	ctx, err := at.buildTradingContext()
	require.NoError(t, err)
	require.Len(t, ctx.Positions, 1)
	assert.True(t, at.isOwnPosition("DOGE_USDT", "long"))

	open, err := st.Position().GetOpenPositions("test-trader")
	require.NoError(t, err)
	require.Len(t, open, 1)
}
