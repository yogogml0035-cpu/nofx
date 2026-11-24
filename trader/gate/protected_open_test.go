package gate

// 随单保护开仓与保护核对的单元测试。httptest 服务按 Gate 线上响应格式返回数据
// （后端质量规范：适配层测试用真实响应格式；只读的真实接口检查放在任务 probe/ 目录）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/trader/types"
)

// gateMock 按 Gate 线上格式响应契约（DOGE_USDT：整数张；ETH_USDT：0.1 张步长），
// 并记录收到的下单请求体供断言。
type gateMock struct {
	mu          sync.Mutex
	orderBodies []map[string]interface{}
	positions   []map[string]interface{}       // GET /positions 的返回；nil 表示空仓
	priceOrders []map[string]interface{}       // GET /price_orders 的返回
	orderByID   map[string]map[string]interface{} // GET /orders/{id} 的返回
}

func newGateMock() *gateMock {
	return &gateMock{orderByID: map[string]map[string]interface{}{}}
}

// contractResponse 按 Gate Contract 模型返回 DOGE（步长 1）或 ETH（步长 0.1）的元数据
func contractResponse(contract string) map[string]interface{} {
	switch contract {
	case "ETH_USDT":
		return map[string]interface{}{
			"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
			"order_size_max": "10000000", "market_order_size_max": "8000000", "enable_decimal": true,
		}
	default: // DOGE_USDT
		return map[string]interface{}{
			"name": "DOGE_USDT", "quanto_multiplier": "10", "order_size_min": "1",
			"order_size_max": "1000000", "market_order_size_max": "0", "enable_decimal": false,
		}
	}
}

func filledOrderResponse(id int64, size, fillPrice string, tpslSL, tpslTP string) map[string]interface{} {
	resp := map[string]interface{}{
		"id": id, "size": size, "left": "0", "fill_price": fillPrice,
		"status": "finished", "finish_as": "filled", "contract": "DOGE_USDT",
	}
	if tpslSL != "" {
		resp["tpsl_sl_trigger_price"] = tpslSL
	}
	if tpslTP != "" {
		resp["tpsl_tp_trigger_price"] = tpslTP
	}
	return resp
}

// positionLevelTriggerOrder 仓位级条件单的线上返回格式
func positionLevelTriggerOrder(id int64, contract, side, kind string) map[string]interface{} {
	orderType := "close-long-position"
	rule := PriceRuleLE // 多头止损
	if side == "SHORT" {
		orderType = "close-short-position"
	}
	if kind == "TP" {
		rule = PriceRuleGE
		if side == "SHORT" {
			rule = PriceRuleLE
		}
	} else if side == "SHORT" {
		rule = PriceRuleGE
	}
	price := "0.48"
	if kind == "TP" {
		price = "0.65"
	}
	return map[string]interface{}{
		"id": id, "order_type": orderType, "status": "open",
		"initial":  map[string]interface{}{"contract": contract, "size": 0, "price": "0", "close": true, "tif": "ioc"},
		"trigger":  map[string]interface{}{"strategy_type": 0, "price_type": 0, "price": price, "rule": rule},
	}
}

func (m *gateMock) handler(w http.ResponseWriter, r *http.Request) {
	// BasePath 带 /api/v4 前缀（与线上一致），按接口路径路由
	path := strings.TrimPrefix(r.URL.Path, "/api/v4")
	writeObj := func(v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/futures/usdt/contracts/"):
		writeObj(contractResponse(strings.TrimPrefix(path, "/futures/usdt/contracts/")))
	case r.Method == http.MethodGet && path == "/futures/usdt/tickers":
		writeObj([]map[string]interface{}{{"contract": "DOGE_USDT", "last": "0.56"}})
	case r.Method == http.MethodGet && path == "/futures/usdt/positions":
		if m.positions == nil {
			writeObj([]map[string]interface{}{})
		} else {
			writeObj(m.positions)
		}
	case r.Method == http.MethodPost && path == "/futures/usdt/orders":
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.orderBodies = append(m.orderBodies, body)
		m.mu.Unlock()
		writeObj(filledOrderResponse(99123, "17", "0.5600", "", ""))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/futures/usdt/orders/"):
		id := strings.TrimPrefix(path, "/futures/usdt/orders/")
		if order, ok := m.orderByID[id]; ok {
			writeObj(order)
		} else {
			writeObj(filledOrderResponse(99123, "17", "0.5600", "", ""))
		}
	case r.Method == http.MethodGet && path == "/futures/usdt/price_orders":
		if m.priceOrders == nil {
			writeObj([]map[string]interface{}{})
		} else {
			writeObj(m.priceOrders)
		}
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/leverage"):
		writeObj(map[string]interface{}{"leverage": "10"})
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"label":"NOT_FOUND","message":"%s %s"}`, r.Method, path)
	}
}

func (m *gateMock) lastOrderBody(t *testing.T) map[string]interface{} {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	require.NotEmpty(t, m.orderBodies, "no order submitted")
	return m.orderBodies[len(m.orderBodies)-1]
}

// longRequest 一组合法的多头随单保护请求（DOGE：1 张 = 10 币，0.56 USDT/张）
func longRequest(notional float64) types.ProtectedOpenRequest {
	return types.ProtectedOpenRequest{
		Symbol:      "DOGE_USDT",
		NotionalUSD: notional,
		Leverage:    10,
		StopLoss:    0.48,
		TakeProfit:  0.65,
		MinNotional: 10,
		SlipRatio:   0.01,
	}
}

func TestOpenLongProtectedPayload(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	// 95.5 USDT ÷ 5.6 USDT/张（10×0.56）= 17 张，实际名义 17×10×0.56 = 95.2
	res, err := gt.OpenLongProtected(longRequest(95.5))
	require.NoError(t, err)

	body := mock.lastOrderBody(t)
	assert.Equal(t, "DOGE_USDT", body["contract"], "下单走 Gate 合约名")
	assert.Equal(t, "17", body["size"], "张数为十进制字符串，多头为正")
	assert.Equal(t, "0", body["price"], "市价单")
	assert.Equal(t, "ioc", body["tif"], "市价单只支持 ioc")
	assert.Equal(t, "0.010000", body["market_order_slip_ratio"], "滑点上限 1%（D8）")
	assert.Equal(t, "0.48", body["tpsl_sl_trigger_price"], "随单止损触发价")
	assert.Equal(t, "0.65", body["tpsl_tp_trigger_price"], "随单止盈触发价")
	assert.Equal(t, "t-nofx", body["text"])
	_, hasClose := body["close"]
	assert.False(t, hasClose, "开仓单不设 close")
	_, hasReduceOnly := body["reduce_only"]
	assert.False(t, hasReduceOnly, "开仓单不设 reduce_only")

	assert.Equal(t, "99123", res.OrderID)
	assert.Equal(t, "17", res.Contracts)
	assert.InDelta(t, 0.56, res.FillPrice, 1e-9)
	assert.InDelta(t, 170, res.Quantity, 1e-9, "币数量 = 17 张 × 10 币/张")
	assert.InDelta(t, 95.2, res.Notional, 1e-9, "实际名义价值按成交价计")
}

func TestOpenShortProtectedNegativeSize(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	// 空头：止损在上、止盈在下
	req := longRequest(95.5)
	req.StopLoss = 0.65
	req.TakeProfit = 0.48

	res, err := gt.OpenShortProtected(req)
	require.NoError(t, err)
	body := mock.lastOrderBody(t)
	assert.Equal(t, "-17", body["size"], "空头张数为负")
	assert.Equal(t, "17", res.Contracts, "返回的张数不带符号")
	assert.Equal(t, "0.65", body["tpsl_sl_trigger_price"])
	assert.Equal(t, "0.48", body["tpsl_tp_trigger_price"])
}

func TestOpenProtectedRejects(t *testing.T) {
	cases := []struct {
		name    string
		req     types.ProtectedOpenRequest
		wantErr string
	}{
		{
			name:    "非正名义价值",
			req:     types.ProtectedOpenRequest{Symbol: "DOGE_USDT", NotionalUSD: 0, StopLoss: 0.48, TakeProfit: 0.65},
			wantErr: "notional must be positive",
		},
		{
			name: "不足一个最小步长",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(4)
				r.MinNotional = 0
				return r
			}(),
			wantErr: "不足一个最小步长",
		},
		{
			name:    "取整后低于最小下单金额",
			req:     longRequest(5.7), // 1 张 5.6 < 10
			wantErr: "取整后低于最小下单金额",
		},
		{
			name: "保护价不合法",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(95.5)
				r.StopLoss = 0
				return r
			}(),
			wantErr: "stop loss trigger price must be positive",
		},
		{
			name: "多头止损高于止盈",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(95.5)
				r.StopLoss = 0.7
				r.TakeProfit = 0.65
				return r
			}(),
			wantErr: "must be below take profit",
		},
		{
			name: "滑点比率超上限",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(95.5)
				r.SlipRatio = 0.6
				return r
			}(),
			wantErr: "slip ratio must be in (0, 0.5]",
		},
		{
			name: "滑点比率非正",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(95.5)
				r.SlipRatio = -0.01
				return r
			}(),
			wantErr: "slip ratio must be in (0, 0.5]",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			_, err := gt.OpenLongProtected(c.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
			mock.mu.Lock()
			assert.Empty(t, mock.orderBodies, "被拒绝的请求不得提交订单")
			mock.mu.Unlock()
		})
	}
}

func TestOpenProtectedETHFractionalStep(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	// 0.1 张步长：价格 0.56 是 DOGE ticker，ETH 的换算只用于张数 —— 用同价构造 10 张
	mock.orderBodies = nil
	mock.mu.Unlock()

	req := types.ProtectedOpenRequest{
		Symbol:      "ETH_USDT",
		NotionalUSD: 10.02, // 一张 0.0056 USDT（0.01×0.56）：10.02 → 1789.2 张，落在 0.1 步长
		StopLoss:    0.48,
		TakeProfit:  0.65,
		MinNotional: 10,
	}
	res, err := gt.OpenLongProtected(req)
	require.NoError(t, err)
	body := mock.lastOrderBody(t)
	assert.NotEqual(t, "17", body["size"])
	// 小数张合约：张数是十进制字符串且落在 0.1 步长上
	assert.Regexp(t, `^-?\d+\.\d$`, body["size"], "ETH 张数保留 0.1 步长的小数位")
	assert.NotEmpty(t, res.Contracts)
}

func TestVerifyOrderProtectionViaOrderTpsl(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.orderByID["99123"] = filledOrderResponse(99123, "17", "0.5600", "0.48", "0.65")
	mock.mu.Unlock()

	assert.NoError(t, gt.VerifyOrderProtection("DOGE_USDT", "99123"),
		"委托回读带随单止损止盈即受保护")
}

func TestVerifyOrderProtectionViaTriggerOrders(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.orderByID["99123"] = filledOrderResponse(99123, "17", "0.5600", "", "")
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"),
		positionLevelTriggerOrder(552, "DOGE_USDT", "LONG", "TP"),
	}
	mock.mu.Unlock()

	assert.NoError(t, gt.VerifyOrderProtection("DOGE_USDT", "99123"),
		"委托无随单 tpsl 时按条件单列表核对：止损与止盈齐全")
}

func TestVerifyOrderProtectionMissing(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"), // 只有止损
	}
	mock.mu.Unlock()

	err := gt.VerifyOrderProtection("DOGE_USDT", "99123")
	require.Error(t, err, "缺止盈即保护缺失")
	assert.Contains(t, err.Error(), "protection missing")
	assert.Contains(t, err.Error(), "take profit present=false")
}

func TestVerifyOrderProtectionEmptyOrderID(t *testing.T) {
	gt := newLocalGateTrader(t, newGateMock().handler)
	assert.Error(t, gt.VerifyOrderProtection("DOGE_USDT", ""))
}
