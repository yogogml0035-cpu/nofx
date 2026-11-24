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
	mu               sync.Mutex
	orderBodies      []map[string]interface{}
	positions        []map[string]interface{}          // GET /positions 的返回；nil 表示空仓
	priceOrders      []map[string]interface{}          // GET /price_orders 的返回
	orderByID        map[string]map[string]interface{} // GET /orders/{id} 的返回
	contractOverride map[string]map[string]interface{} // 合约名 → 自定义元数据（特殊场景用）
	tickerOverride   map[string]string                 // 合约名 → 自定义最新价

	// 仓位设置（合约 → 持仓条目，线上格式）。set_leverage 更新条目并返回它；
	// 没有条目的合约 GET positions/{contract} 返回 POSITION_NOT_FOUND
	settings map[string]map[string]interface{}
	// setLeverageFail 非 nil 时 set_leverage 返回这个错误（模拟交易所拒绝）
	setLeverageFail *gateAPIFailure
	// setLeverageRespondsWith 非 nil 时 set_leverage 的返回值由它给出且不写入条目（模拟被截断的读回）
	setLeverageRespondsWith func(mode, leverage string) map[string]interface{}
	// requests 收到的请求（"方法 路径?查询串"），用于断言顺序
	requests []string
}

// gateAPIFailure 模拟交易所的错误响应（非 2xx + label）
type gateAPIFailure struct {
	status  int
	label   string
	message string
}

func newGateMock() *gateMock {
	return &gateMock{orderByID: map[string]map[string]interface{}{}, settings: map[string]map[string]interface{}{}}
}

// positionEntry 仓位设置条目的线上格式：size=0 的合约也有条目；
// 全仓时 leverage 为 "0"、杠杆在 cross_leverage_limit；lever 两种模式都带
func positionEntry(contract, mode, leverage string) map[string]interface{} {
	return nextEntry(contract, nil, mode, leverage)
}

// nextEntry 模拟 set_leverage 之后的条目（W4 实测的形态）：
// 逐仓时 leverage 与 lever 是新杠杆，cross_leverage_limit 残留上一次全仓的值
func nextEntry(contract string, prev map[string]interface{}, mode, leverage string) map[string]interface{} {
	entry := map[string]interface{}{
		"contract": contract, "size": "0", "mode": "single",
		"pos_margin_mode": mode, "lever": leverage,
		"leverage": "0", "cross_leverage_limit": "0",
	}
	if mode == "cross" {
		entry["cross_leverage_limit"] = leverage
	} else {
		entry["leverage"] = leverage
		if prev != nil {
			entry["cross_leverage_limit"] = prev["cross_leverage_limit"]
		}
	}
	return entry
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
	m.mu.Lock()
	request := r.Method + " " + path
	if r.URL.RawQuery != "" {
		request += "?" + r.URL.RawQuery
	}
	m.requests = append(m.requests, request)
	m.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/futures/usdt/contracts/"):
		contract := strings.TrimPrefix(path, "/futures/usdt/contracts/")
		if override, ok := m.contractOverride[contract]; ok {
			writeObj(override)
		} else {
			writeObj(contractResponse(contract))
		}
	case r.Method == http.MethodGet && path == "/futures/usdt/tickers":
		contract := r.URL.Query().Get("contract")
		if last, ok := m.tickerOverride[contract]; ok {
			writeObj([]map[string]interface{}{{"contract": contract, "last": last}})
		} else {
			writeObj([]map[string]interface{}{{"contract": "DOGE_USDT", "last": "0.56"}})
		}
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
	// 仓位设置条目：GET positions/{contract}（没有条目 → POSITION_NOT_FOUND）
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/futures/usdt/positions/"):
		contract := strings.TrimPrefix(path, "/futures/usdt/positions/")
		m.mu.Lock()
		entry := m.settings[contract]
		m.mu.Unlock()
		if entry == nil {
			// 线上实测：没有条目的合约返回 HTTP 400 + application/json + POSITION_NOT_FOUND
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"label":"POSITION_NOT_FOUND","message":"position not found"}`)
			return
		}
		writeObj(entry)
	// 设置仓位保证金模式与杠杆：POST positions/{contract}/set_leverage（leverage + margin_mode）
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/set_leverage"):
		contract := strings.TrimSuffix(strings.TrimPrefix(path, "/futures/usdt/positions/"), "/set_leverage")
		query := r.URL.Query()
		m.mu.Lock()
		if m.setLeverageFail != nil {
			failure := *m.setLeverageFail
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(failure.status)
			fmt.Fprintf(w, `{"label":%q,"message":%q}`, failure.label, failure.message)
			return
		}
		entry := nextEntry(contract, m.settings[contract], query.Get("margin_mode"), query.Get("leverage"))
		if m.setLeverageRespondsWith != nil {
			entry = m.setLeverageRespondsWith(query.Get("margin_mode"), query.Get("leverage"))
		} else {
			m.settings[contract] = entry
		}
		m.mu.Unlock()
		writeObj(entry)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"label":"NOT_FOUND","message":"%s %s"}`, r.Method, path)
	}
}

// requestsSnapshot 返回到目前为止收到的请求
func (m *gateMock) requestsSnapshot(t *testing.T) []string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.requests...)
}

// settingSnapshot 返回合约当前的仓位设置条目
func (m *gateMock) settingSnapshot(t *testing.T, contract string) map[string]interface{} {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.settings[contract]
	if entry == nil {
		return nil
	}
	copied := map[string]interface{}{}
	for key, value := range entry {
		copied[key] = value
	}
	return copied
}

// assertNoOrder 断言没有提交任何订单
func (m *gateMock) assertNoOrder(t *testing.T) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Empty(t, m.orderBodies, "不得提交订单")
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
			name:    "滑点比率非正",
			req: func() types.ProtectedOpenRequest {
				r := longRequest(95.5)
				r.SlipRatio = -0.01
				return r
			}(),
			wantErr: "slip ratio must be in (0, 0.5]",
		},
		{
			name:    "杠杆非正",
			req:     types.ProtectedOpenRequest{Symbol: "DOGE_USDT", NotionalUSD: 95.5, StopLoss: 0.48, TakeProfit: 0.65},
			wantErr: "leverage must be at least 1",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			_, err := gt.OpenLongProtected(c.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.wantErr)
			mock.assertNoOrder(t)
			for _, request := range mock.requestsSnapshot(t) {
				assert.NotContains(t, request, "/set_leverage", "被拒绝的请求不得改动交易所的仓位设置")
			}
		})
	}
}

func TestOpenProtectedAppliesMarginSettingsBeforeOrder(t *testing.T) {
	// 保证金模式与杠杆按 trader 的设置在下单前一并生效（D38）；接口不带 dual_side（单向持仓模式）
	cases := []struct {
		name   string
		cross  bool
		system string
	}{
		{name: "全仓", cross: true, system: "cross"},
		{name: "逐仓", cross: false, system: "isolated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)

			req := longRequest(95.5)
			req.IsCrossMargin = c.cross
			_, err := gt.OpenLongProtected(req)
			require.NoError(t, err)

			requests := mock.requestsSnapshot(t)
			setting := "POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=10&margin_mode=" + c.system
			settingAt, orderAt := -1, -1
			for i, request := range requests {
				switch request {
				case setting:
					settingAt = i
				case "POST /futures/usdt/orders":
					orderAt = i
				}
			}
			require.NotEqual(t, -1, settingAt, "下单前必须先生效仓位设置: %v", requests)
			require.NotEqual(t, -1, orderAt)
			assert.Less(t, settingAt, orderAt, "仓位设置必须先于下单请求")
			entry := mock.settingSnapshot(t, "DOGE_USDT")
			require.NotNil(t, entry)
			assert.Equal(t, c.system, entry["pos_margin_mode"])
			assert.Equal(t, "10", entry["lever"])
		})
	}
}

func TestOpenProtectedRefusedWhenMarginSettingsFail(t *testing.T) {
	// 交易所拒绝设置（如 LEVERAGE_TOO_HIGH）：不开仓，不提交订单
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.setLeverageFail = &gateAPIFailure{status: 400, label: "LEVERAGE_TOO_HIGH", message: "leverage too high"}
	mock.mu.Unlock()

	_, err := gt.OpenLongProtected(longRequest(95.5))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to open")
	assert.Contains(t, err.Error(), "LEVERAGE_TOO_HIGH")
	mock.assertNoOrder(t)
	assert.Nil(t, mock.settingSnapshot(t, "DOGE_USDT"), "被拒绝的设置不得改动条目")
}

func TestOpenProtectedRefusedWhenSettingsReadBackMismatch(t *testing.T) {
	// 接口返回 200 但读回的不是目标设置（如被截断），持仓条目也对不上：不开仓
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	mock.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "isolated", "3")
	mock.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
		return positionEntry("DOGE_USDT", mode, "3")
	}
	mock.mu.Unlock()

	_, err := gt.OpenLongProtected(longRequest(95.5))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to open")
	assert.Contains(t, err.Error(), "position shows")
	mock.assertNoOrder(t)
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
		Leverage:    10,
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

func TestOpenProtectedAboveMarketMax(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	// 市价单最大张数 20（order_size_max 更大，取较小者）：17 张可过，40 张被拒（F28）
	mock.contractOverride = map[string]map[string]interface{}{
		"DOGE_USDT": {
			"name": "DOGE_USDT", "quanto_multiplier": "10", "order_size_min": "1",
			"order_size_max": "1000000", "market_order_size_max": "20", "enable_decimal": false,
		},
	}
	mock.mu.Unlock()

	_, err := gt.OpenLongProtected(longRequest(95.5)) // 17 张 ≤ 20
	require.NoError(t, err)

	_, err = gt.OpenLongProtected(longRequest(224)) // 40 张 > 20
	require.Error(t, err)
	assert.Contains(t, err.Error(), "超过市价单最大张数")

	mock.mu.Lock()
	require.Len(t, mock.orderBodies, 1, "被拒的请求不得提交订单")
	mock.mu.Unlock()
}

func TestVerifyOrderProtectionIgnoresOtherSide(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.mu.Lock()
	// 本次开空（委托 size 为负）：列表里遗留的多头止损止盈对不算保护
	mock.orderByID["99123"] = func() map[string]interface{} {
		resp := filledOrderResponse(99123, "-17", "0.5600", "", "")
		return resp
	}()
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "LONG", "SL"),
		positionLevelTriggerOrder(552, "DOGE_USDT", "LONG", "TP"),
	}
	mock.mu.Unlock()

	err := gt.VerifyOrderProtection("DOGE_USDT", "99123")
	require.Error(t, err, "反向遗留的止损止盈对不得让空头仓位被误判为受保护")
	assert.Contains(t, err.Error(), "protection missing")

	// 换成本侧（空头）的止损止盈对 → 受保护
	mock.mu.Lock()
	mock.priceOrders = []map[string]interface{}{
		positionLevelTriggerOrder(551, "DOGE_USDT", "SHORT", "SL"),
		positionLevelTriggerOrder(552, "DOGE_USDT", "SHORT", "TP"),
	}
	mock.mu.Unlock()
	assert.NoError(t, gt.VerifyOrderProtection("DOGE_USDT", "99123"))
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
