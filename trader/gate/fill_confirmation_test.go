package gate

// 成交确认与提交前亏损上限复核的单元测试（父任务对抗审查视角 A）：
// IOC 市价单可能零成交或部分成交，平仓与开仓不能无条件当成已成交；
// 2% 亏损上限要按最坏成交价在提交前再算一次。

import (
	"net/http"
	"testing"

	"github.com/gate/gateapi-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderFilled(t *testing.T) {
	for _, tc := range []struct {
		name     string
		finishAs string
		left     string
		want     bool
	}{
		{"全部成交", "filled", "0", true},
		{"IOC 未成交部分被取消", "ioc", "17", false},
		{"finish_as 缺失且 left 为 0", "", "0", true},
		{"finish_as 缺失且有剩余", "", "3", false},
		{"finish_as 缺失且 left 缺失", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := orderFilled(orderWithFill(tc.finishAs, tc.left))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFilledContractsOf(t *testing.T) {
	for _, tc := range []struct {
		name    string
		left    string
		want    string
		wantErr bool
	}{
		{"全部成交", "0", "17", false},
		{"部分成交", "5", "12", false},
		{"零成交", "17", "", true},
		{"left 缺失", "", "17", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filledContractsOf(orderWithFill("filled", tc.left), "17")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	// 小数张合约保留原有小数位
	got, err := filledContractsOf(orderWithFill("ioc", "0.1"), "0.3")
	require.NoError(t, err)
	assert.Equal(t, "0.2", got)
}

func TestOpenProtectedRecordsPartiallyFilledContracts(t *testing.T) {
	mock := newGateMock()
	mock.orderResponses = []map[string]interface{}{{"finish_as": "ioc", "left": "5"}}
	gt := newLocalGateTrader(t, mock.handler)

	res, err := gt.OpenLongProtected(longRequest(95.5))
	require.NoError(t, err)
	// 计划 17 张、成交 12 张：按成交部分记账
	assert.Equal(t, "12", res.Contracts)
	assert.InDelta(t, 120, res.Quantity, 1e-9)
	assert.InDelta(t, 67.2, res.Notional, 1e-9)
}

func TestOpenProtectedRejectsZeroFill(t *testing.T) {
	mock := newGateMock()
	mock.orderResponses = []map[string]interface{}{{"finish_as": "ioc", "left": "17", "fill_price": "0"}}
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.OpenLongProtected(longRequest(95.5))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not positive")
}

// 决策校验时按收盘价算刚好不超上限的仓位，按最坏成交价（最新价 × 1.01）会超：
// 提交前必须等比收缩名义价值。
func TestOpenProtectedShrinksNotionalForWorstCaseLoss(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	req := longRequest(95.5)
	req.Principal, req.MaxLossPercent = 500, 2 // 上限 10 USDT
	res, err := gt.OpenLongProtected(req)
	require.NoError(t, err)
	// 最坏成交价 0.5656：17 张的亏损 17×10×(0.5656−0.48)=14.552 > 10 → 收缩到 11 张（亏损 9.416）
	assert.Equal(t, "11", res.Contracts)
	assert.Equal(t, "11", mock.lastOrderBody(t)["size"])
}

func TestOpenProtectedKeepsNotionalWhenWithinCap(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	req := longRequest(95.5)
	req.Principal, req.MaxLossPercent = 100000, 2 // 上限 2000 USDT
	res, err := gt.OpenLongProtected(req)
	require.NoError(t, err)
	assert.Equal(t, "17", res.Contracts)
}

func TestOpenProtectedRefusesWhenCapCannotBeMet(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	req := longRequest(11.2)                  // 2 张（11.2 USDT），可下单
	req.Principal, req.MaxLossPercent = 50, 2 // 上限 1 USDT：2 张的最坏亏损 1.712 已超，收缩后 1 张又低于最小下单金额
	_, err := gt.OpenLongProtected(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too far")
	mock.assertNoOrder(t)
}

func TestOpenMarketRejectsUnfilledOrder(t *testing.T) {
	mock := newGateMock()
	mock.orderResponses = []map[string]interface{}{{"finish_as": "ioc", "left": "17", "fill_price": "0"}}
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.OpenLong("DOGE_USDT", 170, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not fully filled")
}

func TestCloseAllRetriesWhenPositionRemains(t *testing.T) {
	mock := newGateMock()
	mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "17", "0.55")}
	mock.orderResponses = []map[string]interface{}{{"finish_as": "ioc", "left": "17", "fill_price": "0"}}
	// 第二次平仓成交后持仓消失
	mock.afterOrder = func(m *gateMock, body map[string]interface{}) {
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(m.orderBodies) >= 2 {
			m.positions = nil
		}
	}
	gt := newLocalGateTrader(t, mock.handler)

	order, err := gt.CloseLong("DOGE_USDT", 0)
	require.NoError(t, err)
	assert.Equal(t, "FILLED", order["status"])
	m := mock
	m.mu.Lock()
	count := len(m.orderBodies)
	m.mu.Unlock()
	assert.Equal(t, 2, count, "IOC 零成交后必须再提交一次")
}

func TestCloseAllReportsErrorWhenItCannotFill(t *testing.T) {
	mock := newGateMock()
	mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "17", "0.55")}
	mock.orderResponses = []map[string]interface{}{
		{"finish_as": "ioc", "left": "17", "fill_price": "0"},
		{"finish_as": "ioc", "left": "9", "fill_price": "0.55"},
	}
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.CloseLong("DOGE_USDT", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not fill after retry")
}

// 持仓已经不在（例如被保护单平掉）：IOC 返回未成交也不能报错
func TestCloseAllAcceptsAlreadyClosedPosition(t *testing.T) {
	mock := newGateMock()
	mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "17", "0.55")}
	mock.orderResponses = []map[string]interface{}{{"finish_as": "ioc", "left": "17", "fill_price": "0"}}
	mock.afterOrder = func(m *gateMock, body map[string]interface{}) {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.positions = nil
	}
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.CloseLong("DOGE_USDT", 0)
	require.NoError(t, err)
}

// 成交后持仓读数有延迟：核对要重试读持仓，不能读不到就通过
func TestVerifyOrderProtectionRetriesPositionReadback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		position  bool
		wantReads int
	}{
		{"position visible at once", true, 1},
		{"position never appears", false, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := newGateMock()
			if tc.position {
				mock.positions = []map[string]interface{}{livePosition("DOGE_USDT", "17", "0.55")}
			}
			gt := newLocalGateTrader(t, mock.handler)

			err := gt.VerifyOrderProtection("DOGE_USDT", "99123")
			// 持仓在而没有条件单 → 保护缺失（调用方必须立即平仓）；持仓始终读不到 → 视为已平
			if tc.position {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			reads := 0
			for _, request := range mock.requestsSnapshot(t) {
				if request == "GET /futures/usdt/positions" {
					reads++
				}
			}
			assert.Equal(t, tc.wantReads, reads)
		})
	}
}

// orderWithFill 构造下单接口的返回（线上格式：数值字段为字符串）
func orderWithFill(finishAs, left string) gateapi.FuturesOrder {
	return gateapi.FuturesOrder{FinishAs: finishAs, Left: left}
}

// AI trader 的开仓带专属备注：同步层据此判定仓位归属（D40），助手/网格的普通市价单不带
func TestOpenProtectedCarriesAITraderText(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.OpenLongProtected(longRequest(95.5))
	require.NoError(t, err)
	assert.Equal(t, "t-nofx-ai", mock.lastOrderBody(t)["text"])

	_, err = gt.OpenLong("DOGE_USDT", 170, 10)
	require.NoError(t, err)
	assert.Equal(t, "t-nofx", mock.lastOrderBody(t)["text"], "the assistant/grid primitive keeps its own text")
}

// 请求报错时持仓缓存必须已清：调用方「读持仓→有仓位就平掉」的兜底才能读到最新值
func TestOpenProtectedClearsPositionCacheOnRequestError(t *testing.T) {
	mock := newGateMock()
	mock.positions = []map[string]interface{}{livePosition("ETH_USDT", "1", "1600")}
	mock.setLeverageFail = &gateAPIFailure{status: http.StatusInternalServerError, label: "INTERNAL", message: "boom"}
	gt := newLocalGateTrader(t, mock.handler)

	_, err := gt.OpenLongProtected(longRequest(95.5))
	require.Error(t, err)
	// 报错后交易所上多了一个仓位：不带缓存清理时 GetPositions 仍会返回开仓前的快照
	mock.mu.Lock()
	mock.positions = append(mock.positions, livePosition("DOGE_USDT", "6", "0.55"))
	mock.mu.Unlock()
	positions, err := gt.GetPositions()
	require.NoError(t, err)
	found := false
	for _, pos := range positions {
		if pos["symbol"] == "DOGE_USDT" {
			found = true
		}
	}
	assert.True(t, found, "positions must be re-read after the failed request")
}
