package gate

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLocalGateTrader 返回指向本地测试服务的 trader；服务按 Gate 的线上格式返回数据
func newLocalGateTrader(t *testing.T, handler http.HandlerFunc) *GateTrader {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	gt := NewGateTrader("test_key", "test_secret")
	gt.client.GetConfig().BasePath = server.URL + "/api/v4"
	return gt
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestParseRequiredSize(t *testing.T) {
	cases := []struct {
		raw     string
		want    float64
		wantErr bool
	}{
		{raw: "7", want: 7},
		{raw: "-7", want: -7},
		{raw: "0.5", want: 0.5},
		{raw: "", wantErr: true},
		{raw: "abc", wantErr: true},
	}
	for _, c := range cases {
		got, err := parseRequiredSize("size", c.raw)
		if c.wantErr {
			assert.Error(t, err, "raw=%q", c.raw)
			continue
		}
		require.NoError(t, err, "raw=%q", c.raw)
		assert.Equal(t, c.want, got, "raw=%q", c.raw)
	}
}

func TestParseOptionalSize(t *testing.T) {
	cases := []struct {
		raw     string
		want    float64
		wantErr bool
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: "1.25", want: 1.25},
		{raw: "-2", want: -2},
		{raw: "abc", wantErr: true},
	}
	for _, c := range cases {
		got, err := parseOptionalSize("left", c.raw)
		if c.wantErr {
			assert.Error(t, err, "raw=%q", c.raw)
			continue
		}
		require.NoError(t, err, "raw=%q", c.raw)
		assert.Equal(t, c.want, got, "raw=%q", c.raw)
	}
}

// SDK 默认带 X-Gate-Size-Decimal: 1，张数以字符串返回并保留小数部分
func TestGateTrader_GetPositionsParsesDecimalSizes(t *testing.T) {
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/futures/usdt/positions":
			writeJSON(w, []map[string]interface{}{
				{"contract": "SOL_USDT", "size": "0.5", "entry_price": "200", "mark_price": "210", "unrealised_pnl": "5", "liq_price": "150", "leverage": "0", "lever": "3"},
				{"contract": "BTC_USDT", "size": "-7", "entry_price": "90000", "mark_price": "89000", "unrealised_pnl": "0.7", "liq_price": "99000", "leverage": "5", "lever": "5"},
				{"contract": "ETH_USDT", "size": "0", "entry_price": "0", "mark_price": "0", "unrealised_pnl": "0", "liq_price": "0", "leverage": "3", "lever": "3"},
			})
		case "/api/v4/futures/usdt/contracts/SOL_USDT":
			writeJSON(w, map[string]interface{}{
				"name": "SOL_USDT", "quanto_multiplier": "1", "order_size_min": "0.1",
				"order_size_max": "200000", "market_order_size_max": "300000", "enable_decimal": true,
			})
		case "/api/v4/futures/usdt/contracts/BTC_USDT":
			writeJSON(w, map[string]interface{}{
				"name": "BTC_USDT", "quanto_multiplier": "0.0001", "order_size_min": "1",
				"order_size_max": "1000000", "market_order_size_max": "0", "enable_decimal": false,
			})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	})

	positions, err := gt.GetPositions()
	require.NoError(t, err)
	require.Len(t, positions, 2)

	assert.Equal(t, "SOL_USDT", positions[0]["symbol"])
	assert.Equal(t, "long", positions[0]["side"])
	assert.InDelta(t, 0.5, positions[0]["positionAmt"], 1e-9)

	assert.Equal(t, "BTC_USDT", positions[1]["symbol"])
	assert.Equal(t, "short", positions[1]["side"])
	assert.InDelta(t, 0.0007, positions[1]["positionAmt"], 1e-12)
}

func TestGateTrader_GetPositionsRejectsMalformedSize(t *testing.T) {
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []map[string]interface{}{
			{"contract": "BTC_USDT", "size": "abc", "entry_price": "90000", "mark_price": "89000", "unrealised_pnl": "0", "liq_price": "0", "leverage": "5"},
		})
	})

	_, err := gt.GetPositions()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid position size "abc"`)
}

func TestGateTrader_GetOrderStatusParsesDecimalSizes(t *testing.T) {
	cases := []struct {
		name       string
		orderID    string
		size, left string
		wantSide   string
		wantQty    float64
	}{
		{name: "卖单部分成交", orderID: "123", size: "-2.5", left: "0.5", wantSide: "SELL", wantQty: 0.02},
		{name: "买单部分成交", orderID: "124", size: "4", left: "1.5", wantSide: "BUY", wantQty: 0.025},
		{name: "left 缺省", orderID: "125", size: "-2", left: "", wantSide: "SELL", wantQty: 0.02},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orderID, err := strconv.Atoi(c.orderID)
			require.NoError(t, err)
			gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v4/futures/usdt/orders/" + c.orderID:
					writeJSON(w, map[string]interface{}{
						"id": orderID, "contract": "ETH_USDT", "size": c.size, "left": c.left,
						"status": "finished", "finish_as": "filled", "fill_price": "3000",
						"tkfr": "0.0005", "mkfr": "0", "create_time": 1700000000, "finish_time": 1700000010,
					})
				case "/api/v4/futures/usdt/contracts/ETH_USDT":
					writeJSON(w, map[string]interface{}{
						"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
						"order_size_max": "10000000", "market_order_size_max": "8000000", "enable_decimal": true,
					})
				default:
					http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
				}
			})

			status, err := gt.GetOrderStatus("ETHUSDT", c.orderID)
			require.NoError(t, err)
			assert.Equal(t, c.wantSide, status["side"])
			assert.Equal(t, "FILLED", status["status"])
			// (|size| − left) 张 × 0.01 ETH
			assert.InDelta(t, c.wantQty, status["executedQty"], 1e-12)
			assert.InDelta(t, 3000, status["avgPrice"], 1e-9)
		})
	}
}

func TestGateTrader_GetTradesDerivesActionFromStringSizes(t *testing.T) {
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/futures/usdt/my_trades":
			writeJSON(w, []map[string]interface{}{
				{"id": 1, "create_time": time.Now().Unix(), "contract": "ETH_USDT", "order_id": "11", "size": "-2.5", "price": "3000", "fee": "-0.01"},
				{"id": 2, "create_time": time.Now().Unix(), "contract": "ETH_USDT", "order_id": "12", "size": "1.25", "close_size": "1.25", "price": "3100", "fee": "-0.01"},
			})
		case "/api/v4/futures/usdt/contracts/ETH_USDT":
			writeJSON(w, map[string]interface{}{
				"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
				"order_size_max": "10000000", "market_order_size_max": "8000000", "enable_decimal": true,
			})
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	})

	trades, err := gt.GetTrades(time.Now().Add(-time.Hour), 100)
	require.NoError(t, err)
	require.Len(t, trades, 2)

	assert.Equal(t, "SELL", trades[0].Side)
	assert.Equal(t, "open_short", trades[0].OrderAction)
	assert.InDelta(t, 0.025, trades[0].FillQty, 1e-12)

	assert.Equal(t, "BUY", trades[1].Side)
	assert.Equal(t, "close_short", trades[1].OrderAction)
	assert.InDelta(t, 0.0125, trades[1].FillQty, 1e-12)
}

func TestGateTrader_OpenLongSendsContractCountAsString(t *testing.T) {
	// 开仓是 IOC 市价单：不预撤挂单与条件单（避免误撤已有仓位的保护单），
	// 直接按精确换算后的张数提交
	type captured struct {
		orderBody   map[string]interface{}
		sizeDecimal string
	}
	capturedCh := make(chan captured, 4)
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/futures/usdt/positions/DOGE_USDT/leverage":
			writeJSON(w, map[string]interface{}{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/futures/usdt/contracts/DOGE_USDT":
			writeJSON(w, map[string]interface{}{
				"name": "DOGE_USDT", "quanto_multiplier": "10", "order_size_min": "1",
				"order_size_max": "1000000", "market_order_size_max": "0", "enable_decimal": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/futures/usdt/orders":
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var body map[string]interface{}
			require.NoError(t, json.Unmarshal(raw, &body))
			capturedCh <- captured{orderBody: body, sizeDecimal: r.Header.Get("X-Gate-Size-Decimal")}
			writeJSON(w, map[string]interface{}{
				"id": 123456, "contract": "DOGE_USDT", "size": "3", "status": "finished", "finish_as": "filled", "fill_price": "0.2",
			})
		default:
			http.Error(w, "unexpected request "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})

	// 30 DOGE ÷ 10 = 3 张
	result, err := gt.OpenLong("DOGEUSDT", 30, 5)
	require.NoError(t, err)

	placed := <-capturedCh

	assert.Equal(t, "DOGE_USDT", placed.orderBody["contract"])
	assert.Equal(t, "3", placed.orderBody["size"])
	assert.Equal(t, "0", placed.orderBody["price"])
	assert.Equal(t, "ioc", placed.orderBody["tif"])
	assert.Equal(t, "1", placed.sizeDecimal)
	assert.Equal(t, 0.2, result["fillPrice"])
}
