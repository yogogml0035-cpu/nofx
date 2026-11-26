package gate

// 仓位归属在同步层的单元测试（计划文档 D40/F53）：同一个账户上有多个下单入口，
// 同步只把带 AI trader 专属备注的开仓成交记到 trader 名下；平仓成交只作用于它已有的持仓。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
)

// myTrade 成交记录的线上格式（my_trades）
func myTrade(id int64, contract, text, size, closeSize string) map[string]interface{} {
	return map[string]interface{}{
		"id": id, "create_time": time.Now().Unix(), "contract": contract, "order_id": fmt.Sprintf("9%d", id),
		"size": size, "close_size": closeSize, "price": "0.05", "role": "taker", "text": text, "fee": "-0.001",
	}
}

func syncHarness(t *testing.T, trades ...map[string]interface{}) (*GateTrader, *store.Store, *gateMock) {
	t.Helper()
	list := trades
	return syncHarnessWithTrades(t, &list)
}

// syncHarnessWithTrades 允许测试在两次同步之间替换成交列表（模拟迟到成交）。
func syncHarnessWithTrades(t *testing.T, trades *[]map[string]interface{}) (*GateTrader, *store.Store, *gateMock) {
	t.Helper()
	mock := newGateMock()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/futures/usdt/my_trades" {
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(*trades))
			return
		}
		mock.handler(w, r)
	}))
	t.Cleanup(server.Close)
	gt := NewGateTrader("test_key", "test_secret")
	gt.client.GetConfig().BasePath = server.URL + "/api/v4"
	st, err := store.New(filepath.Join(t.TempDir(), "sync.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return gt, st, mock
}

func openPositionCount(t *testing.T, st *store.Store, traderID, symbol, side string) int {
	t.Helper()
	pos, err := st.Position().GetOpenPositionBySymbol(traderID, symbol, side)
	require.NoError(t, err)
	if pos == nil {
		return 0
	}
	return 1
}

func TestSyncAttributesOnlyAITraderOpens(t *testing.T) {
	gt, st, _ := syncHarness(t,
		myTrade(1, "DOGE_USDT", "t-nofx-ai", "17", "0"),             // 本 trader 的随单保护开仓
		myTrade(2, "ETH_USDT", "t-nofx", "0.1", "0"),                // 助手手动下单
		myTrade(3, "SOL_USDT", "app", "1", "0"),                     // 用户在 App 里手动开仓
		myTrade(4, "XRP_USDT", "ao-2125517580666933248", "10", "0"), // 其他工具的自动跟踪单
	)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	assert.Equal(t, 1, openPositionCount(t, st, "trader-1", "DOGEUSDT", "LONG"), "AI trader 的开仓建立持仓记录")
	for _, symbol := range []string{"ETHUSDT", "SOLUSDT", "XRPUSDT"} {
		assert.Equal(t, 0, openPositionCount(t, st, "trader-1", symbol, "LONG"),
			"%s 不是本 trader 开出的仓位，不能记到它名下", symbol)
		orders, err := st.Order().GetTraderOrders("trader-1", 50)
		require.NoError(t, err)
		for _, order := range orders {
			assert.NotEqual(t, symbol, order.Symbol, "别的入口的成交不进本 trader 的成交历史")
		}
	}
}

func TestSyncAppliesOwnClosesAndSkipsForeignOnes(t *testing.T) {
	gt, st, _ := syncHarness(t,
		myTrade(1, "DOGE_USDT", "t-nofx-ai", "17", "0"),       // 开仓（本 trader）
		myTrade(2, "ETH_USDT", "app", "0.1", "0"),             // 用户手动开仓
		myTrade(3, "ETH_USDT", "app", "-0.1", "-0.1"),         // 用户手动平掉自己的仓位
		myTrade(4, "DOGE_USDT", "t-nofx-close", "-17", "-17"), // 本 trader 全平
	)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	assert.Equal(t, 0, openPositionCount(t, st, "trader-1", "DOGEUSDT", "LONG"), "自己的平仓成交关掉持仓记录")
	assert.Equal(t, 0, openPositionCount(t, st, "trader-1", "ETHUSDT", "LONG"))
	orders, err := st.Order().GetTraderOrders("trader-1", 50)
	require.NoError(t, err)
	assert.Len(t, orders, 2, "只记本 trader 的开仓与平仓两笔成交")
}

func TestSyncRollsBackOpeningTradeWithoutPositionRecord(t *testing.T) {
	broken := myTrade(1, "DOGE_USDT", "t-nofx-ai", "17", "0")
	broken["price"] = "0" // 成交价无效：持仓记录不会写
	gt, st, _ := syncHarness(t, broken)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	order, err := st.Order().GetOrderByExchangeID("exchange-1", "1")
	require.NoError(t, err)
	assert.Nil(t, order, "订单行必须回滚，下一次同步才能从头重放这笔开仓成交")
}

// 部分平仓后记录仍是 OPEN：重复同步不能把同一条平仓成交再应用一次
// （否则数量/手续费/盈亏被反复扣减，记录最终被误关，剩余仓位脱离归属）。
func TestSyncAppliesEachFillOnlyOnce(t *testing.T) {
	gt, st, _ := syncHarness(t,
		myTrade(1, "DOGE_USDT", "t-nofx-ai", "17", "0"),
		myTrade(2, "DOGE_USDT", "t-nofx-close", "-8", "-8"),
	)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "DOGEUSDT", "LONG")
	require.NoError(t, err)
	require.NotNil(t, pos)
	firstQty, firstFee, firstPnL := pos.Quantity, pos.Fee, pos.RealizedPnL
	assert.Greater(t, firstQty, 0.0)
	assert.Less(t, firstQty, pos.EntryQuantity, "部分平仓后剩余数量应小于开仓数量")

	// 第二次同步读到同一批成交：订单行已存在，必须整笔跳过。
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))
	pos, err = st.Position().GetOpenPositionBySymbol("trader-1", "DOGEUSDT", "LONG")
	require.NoError(t, err)
	require.NotNil(t, pos, "分批平仓后记录仍应是 OPEN")
	assert.InDelta(t, firstQty, pos.Quantity, 0.0001, "数量不能被重复扣减")
	assert.InDelta(t, firstFee, pos.Fee, 0.0000001, "手续费不能重复累加")
	assert.InDelta(t, firstPnL, pos.RealizedPnL, 0.0000001, "盈亏不能重复累加")
}

// 对账先于同步把记录转为 RECONCILED 时，迟到的平仓成交要回填出场价与盈亏，
// 并把记录转入 CLOSED（不能因为当时没有 OPEN 记录而永久丢弃这笔成交）。
func TestSyncBackfillsLateCloseIntoReconciledRecord(t *testing.T) {
	trades := []map[string]interface{}{
		myTrade(1, "DOGE_USDT", "t-nofx-ai", "17", "0"),
	}
	gt, st, _ := syncHarnessWithTrades(t, &trades)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "DOGEUSDT", "LONG")
	require.NoError(t, err)
	require.NotNil(t, pos)
	require.NoError(t, st.Position().ReconcileClose(pos.ID, store.ReconciledPositionGone))
	assert.Equal(t, 0, openPositionCount(t, st, "trader-1", "DOGEUSDT", "LONG"))

	// RECONCILED 不是成交记录：不能出现在最近成交/统计里（否则会造出 0 出场价的假交易）。
	recent, err := st.Position().GetRecentTrades("trader-1", 10)
	require.NoError(t, err)
	assert.Empty(t, recent, "对账中的记录不进最近成交")
	stats, err := st.Position().GetFullStats("trader-1")
	require.NoError(t, err)
	assert.Equal(t, 0, stats.TotalTrades, "对账中的记录不进统计")

	late := myTrade(2, "DOGE_USDT", "t-nofx-close", "-17", "-17")
	late["price"] = "0.06"
	trades = append(trades, late)
	require.NoError(t, gt.SyncOrdersFromGate("trader-1", "exchange-1", "gate", st))

	closed, err := st.Position().GetClosedPositions("trader-1", 10)
	require.NoError(t, err)
	require.Len(t, closed, 1, "迟到的平仓成交应把记录补成 CLOSED 并进入历史")
	assert.InDelta(t, 0.06, closed[0].ExitPrice, 0.0000001)
	expectedPnL := (0.06 - 0.05) * closed[0].EntryQuantity
	assert.InDelta(t, expectedPnL, closed[0].RealizedPnL, 0.0001, "按入场 0.05 出场 0.06 与持仓数量计算")

	orders, err := st.Order().GetTraderOrders("trader-1", 50)
	require.NoError(t, err)
	assert.Len(t, orders, 2, "迟到的平仓成交也要有自己的订单与成交记录")
}
