package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/market"
)

// D7：/api/symbols 与 /api/klines 无参时默认数据源均为 Gate
func TestDefaultMarketDataSourceIsGate(t *testing.T) {
	if defaultExchange != "gate" {
		t.Fatalf("default exchange should be gate, got %q", defaultExchange)
	}
}

type klineCall struct {
	symbol   string
	interval string
	limit    int
}

func newKlinesTestServer(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := &Server{router: gin.New()}
	s.router.GET("/api/klines", s.handleKlines)
	return s
}

func overrideCryptoKlines(t *testing.T, fn func(symbol, interval string, limit int) ([]market.Kline, error)) {
	t.Helper()
	orig := cryptoKlines
	cryptoKlines = fn
	t.Cleanup(func() { cryptoKlines = orig })
}

func doKlinesRequest(t *testing.T, s *Server, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/klines"+query, nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestHandleKlinesCryptoUsesGateAndIgnoresExchange(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{name: "默认", query: "?symbol=DOGEUSDT&interval=5m&limit=900"},
		{name: "旧 exchange=binance 仍走 Gate", query: "?symbol=DOGEUSDT&interval=5m&limit=900&exchange=binance"},
		{name: "exchange=gate", query: "?symbol=DOGEUSDT&interval=5m&limit=900&exchange=gate"},
		{name: "未知 exchange 仍走 Gate", query: "?symbol=DOGEUSDT&interval=5m&limit=900&exchange=kucoin"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []klineCall
			overrideCryptoKlines(t, func(symbol, interval string, limit int) ([]market.Kline, error) {
				calls = append(calls, klineCall{symbol: symbol, interval: interval, limit: limit})
				return []market.Kline{{OpenTime: 1700000000000, Open: 0.05, High: 0.06, Low: 0.04, Close: 0.0543, Volume: 12345.6, CloseTime: 1700000300000}}, nil
			})
			s := newKlinesTestServer(t)
			rec := doKlinesRequest(t, s, c.query)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Len(t, calls, 1, "exactly one Gate fetch")
			assert.Equal(t, "DOGEUSDT", calls[0].symbol)
			assert.Equal(t, "5m", calls[0].interval)
			assert.Equal(t, 900, calls[0].limit)

			var body []market.Kline
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Len(t, body, 1)
			assert.Equal(t, 12345.6, body[0].Volume)
			assert.Equal(t, body[0].Volume, body[0].QuoteVolume, "QuoteVolume mirrors USDT turnover")
		})
	}
}

func TestHandleKlinesLimitBounds(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		wantLimit int
	}{
		{name: "零取默认", query: "?symbol=DOGEUSDT&interval=5m&limit=0", wantLimit: 1000},
		{name: "非法取默认", query: "?symbol=DOGEUSDT&interval=5m&limit=abc", wantLimit: 1000},
		{name: "超上限夹到 2000", query: "?symbol=DOGEUSDT&interval=5m&limit=5000", wantLimit: 2000},
		{name: "上限 2000 保留", query: "?symbol=DOGEUSDT&interval=5m&limit=2000", wantLimit: 2000},
		{name: "正常值保留", query: "?symbol=DOGEUSDT&interval=5m&limit=1500", wantLimit: 1500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got int
			overrideCryptoKlines(t, func(symbol, interval string, limit int) ([]market.Kline, error) {
				got = limit
				return []market.Kline{}, nil
			})
			s := newKlinesTestServer(t)
			rec := doKlinesRequest(t, s, c.query)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, c.wantLimit, got)
		})
	}
}

func TestHandleKlinesMissingSymbol(t *testing.T) {
	s := newKlinesTestServer(t)
	rec := doKlinesRequest(t, s, "?interval=5m")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleKlinesGateFailureReturnsInternalError(t *testing.T) {
	overrideCryptoKlines(t, func(symbol, interval string, limit int) ([]market.Kline, error) {
		return nil, errors.New("gate unreachable")
	})
	s := newKlinesTestServer(t)
	rec := doKlinesRequest(t, s, "?symbol=DOGEUSDT&interval=5m")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleKlinesRejectsNonRetainedInterval(t *testing.T) {
	// 用真实取数入口：非保留周期在发请求前就被拒绝（不需要网络）
	overrideCryptoKlines(t, market.GetGateKlines)
	s := newKlinesTestServer(t)
	for _, interval := range []string{"1s", "5s", "30s", "3m", "2h", "6h", "12h", "3d", "1M"} {
		rec := doKlinesRequest(t, s, "?symbol=DOGEUSDT&interval="+interval)
		assert.Equalf(t, http.StatusInternalServerError, rec.Code, "interval %s must be rejected", interval)
	}
}
