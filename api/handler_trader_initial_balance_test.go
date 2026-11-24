package api

// 初始资金在 API 层的单元测试（子任务 account-equity）：创建时取交易所账户的本金，
// 更新时拒绝负值，同步余额取本金。探测用的适配器换成不连交易所的桩。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/manager"
	"nofx/store"
	"nofx/trader"
	"nofx/trader/types"
)

const (
	apiTestUser     = "user-1"
	apiTestExchange = "exchange-1"
	apiTestAIModel  = "model-1"
	apiTestStrategy = "strategy-1"
	apiTestTraderID = "trader-1"
)

// apiBalanceStub 探测用的适配器桩：只实现 GetBalance
type apiBalanceStub struct {
	trader.Trader
	balance map[string]interface{}
	err     error
}

func (s *apiBalanceStub) GetBalance() (map[string]interface{}, error) {
	return s.balance, s.err
}

// stubExchangeProbe 把探测适配器换成桩，测试结束后恢复
func stubExchangeProbe(t *testing.T, stub *apiBalanceStub) {
	t.Helper()
	original := buildExchangeProbeTrader
	buildExchangeProbeTrader = func(*store.Exchange, string) (trader.Trader, error) {
		return stub, nil
	}
	t.Cleanup(func() { buildExchangeProbeTrader = original })
}

// newInitialBalanceAPITestEnv 建一个带临时 sqlite 库的 API 服务，库里有一个启用中的 Gate 交易所账户、
// 一个启用中的模型和一条配置不是合法 JSON 的策略。创建只要求策略存在；创建、更新之后后端会重新加载
// trader，加载在解析策略配置一步失败，不会构造真实的交易所适配器去读余额
func newInitialBalanceAPITestEnv(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "api.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	db := st.GormDB()
	require.NoError(t, db.Create(&store.Exchange{
		ID: apiTestExchange, UserID: apiTestUser, Name: "gate", Type: "cex", ExchangeType: "gate", Enabled: true,
		APIKey: "key", SecretKey: "secret",
	}).Error)
	require.NoError(t, db.Create(&store.AIModel{
		ID: apiTestAIModel, UserID: apiTestUser, Name: "model", Provider: "openai", Enabled: true, APIKey: "key",
		CustomModelName: "gpt",
	}).Error)
	require.NoError(t, db.Create(&store.Strategy{
		ID: apiTestStrategy, UserID: apiTestUser, Name: "strategy", Config: "not a json config",
	}).Error)
	return &Server{
		router:                    gin.New(),
		traderManager:             manager.NewTraderManager(),
		store:                     st,
		exchangeAccountStateCache: NewExchangeAccountStateCache(),
	}
}

func storedTrader(t *testing.T, s *Server) *store.Trader {
	t.Helper()
	record, err := s.store.Trader().GetByID(apiTestTraderID)
	require.NoError(t, err)
	require.NotNil(t, record)
	return record
}

func createStoredTrader(t *testing.T, s *Server, initialBalance float64) {
	t.Helper()
	require.NoError(t, s.store.Trader().Create(&store.Trader{
		ID: apiTestTraderID, UserID: apiTestUser, Name: "t", AIModelID: apiTestAIModel,
		ExchangeID: apiTestExchange, InitialBalance: initialBalance, ScanIntervalMinutes: 3,
	}))
}

// callHandler 以已登录用户的身份调用一个处理器
func callHandler(t *testing.T, handler gin.HandlerFunc, method, path, body string, params gin.Params) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("user_id", apiTestUser)
	handler(c)
	return w
}

func TestCreateTraderRecordsAccountPrincipal(t *testing.T) {
	// 桩同时报告了权益 205.31：初始资金取本金 200，不取权益
	s := newInitialBalanceAPITestEnv(t)
	stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal:     200.0,
		types.BalanceKeyUnrealizedPnL: 5.31,
		types.BalanceKeyAvailable:     150.0,
		"totalEquity":                 205.31,
	}})

	body := `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange + `","strategy_id":"` + apiTestStrategy + `"}`
	w := callHandler(t, s.handleCreateTrader, http.MethodPost, "/api/traders", body, nil)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		TraderID string `json:"trader_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.TraderID)

	record, err := s.store.Trader().GetByID(created.TraderID)
	require.NoError(t, err)
	assert.Equal(t, 200.0, record.InitialBalance, "the initial balance is the account principal at creation")
}

func TestCreateTraderLeavesZeroWhenPrincipalUnreadable(t *testing.T) {
	// 读取失败不阻断创建：初始资金留 0，由加载时以当前本金重建
	s := newInitialBalanceAPITestEnv(t)
	stubExchangeProbe(t, &apiBalanceStub{err: assert.AnError})

	body := `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange + `","strategy_id":"` + apiTestStrategy + `"}`
	w := callHandler(t, s.handleCreateTrader, http.MethodPost, "/api/traders", body, nil)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		TraderID string `json:"trader_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	record, err := s.store.Trader().GetByID(created.TraderID)
	require.NoError(t, err)
	assert.Zero(t, record.InitialBalance)
}

func TestCreateTraderRequestHasNoInitialBalanceField(t *testing.T) {
	// 创建时一律取本金：请求体里的 initial_balance 不再是入参
	var req CreateTraderRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"t","ai_model_id":"m","exchange_id":"e","initial_balance":999}`), &req))
	encoded, err := json.Marshal(req)
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), "initial_balance")
}

func TestUpdateTraderInitialBalance(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		initial    float64 // 更新前库里的值
		wantStatus int
		wantValue  float64
	}{
		{name: "positive value is saved", body: `"initial_balance":500.5`, initial: 200, wantStatus: http.StatusOK, wantValue: 500.5},
		{name: "zero keeps the current value", body: `"initial_balance":0`, initial: 200, wantStatus: http.StatusOK, wantValue: 200},
		{name: "field omitted keeps the current value", body: `"scan_interval_minutes":5`, initial: 200, wantStatus: http.StatusOK, wantValue: 200},
		{name: "negative value is rejected", body: `"initial_balance":-5`, initial: 200, wantStatus: http.StatusBadRequest, wantValue: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, tt.initial)

			body := `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange + `",` + tt.body + `}`
			w := callHandler(t, s.handleUpdateTrader, http.MethodPut, "/api/traders/"+apiTestTraderID, body,
				gin.Params{{Key: "id", Value: apiTestTraderID}})

			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tt.wantValue, storedTrader(t, s).InitialBalance)
		})
	}
}

func TestSyncBalanceRecordsAccountPrincipal(t *testing.T) {
	s := newInitialBalanceAPITestEnv(t)
	createStoredTrader(t, s, 100)
	stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal:     300.0,
		types.BalanceKeyUnrealizedPnL: 12.0,
		types.BalanceKeyAvailable:     280.0,
		"totalEquity":                 312.0,
	}})

	w := callHandler(t, s.handleSyncBalance, http.MethodPost, "/api/traders/"+apiTestTraderID+"/sync-balance", "",
		gin.Params{{Key: "id", Value: apiTestTraderID}})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 300.0, storedTrader(t, s).InitialBalance, "sync takes the principal, not the equity")
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 100.0, response["old_balance"])
	assert.Equal(t, 300.0, response["new_balance"])
	assert.Equal(t, "increase", response["change_type"])
	assert.InDelta(t, 200.0, response["change_percent"], 1e-9)
}

func TestSyncBalanceWithInvalidStoredValue(t *testing.T) {
	// 存量 trader 的初始资金是 0：变化百分比没有意义，记 0，而不是除零得到无穷大
	s := newInitialBalanceAPITestEnv(t)
	createStoredTrader(t, s, 0)
	stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal: 107.49769421, types.BalanceKeyUnrealizedPnL: 0.0, types.BalanceKeyAvailable: 107.49769,
	}})

	w := callHandler(t, s.handleSyncBalance, http.MethodPost, "/api/traders/"+apiTestTraderID+"/sync-balance", "",
		gin.Params{{Key: "id", Value: apiTestTraderID}})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), "the response must be valid JSON (no Infinity)")
	assert.Equal(t, 0.0, response["change_percent"])
	assert.Equal(t, 107.49769421, storedTrader(t, s).InitialBalance)
}

func TestSyncBalanceFailsWhenPrincipalUnreadable(t *testing.T) {
	s := newInitialBalanceAPITestEnv(t)
	createStoredTrader(t, s, 100)
	stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{types.BalanceKeyAvailable: 50.0}})

	w := callHandler(t, s.handleSyncBalance, http.MethodPost, "/api/traders/"+apiTestTraderID+"/sync-balance", "",
		gin.Params{{Key: "id", Value: apiTestTraderID}})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, 100.0, storedTrader(t, s).InitialBalance, "a failed sync must not touch the stored value")
}

func TestProbeExchangeAccountStateUsesParsedBalance(t *testing.T) {
	exchangeCfg := &store.Exchange{
		ID: apiTestExchange, UserID: apiTestUser, Enabled: true, ExchangeType: "gate",
		APIKey: "key", SecretKey: "secret",
	}

	t.Run("ok: equity and available come from the parsed balance", func(t *testing.T) {
		stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: 107.49769421, types.BalanceKeyUnrealizedPnL: -0.31, types.BalanceKeyAvailable: 60.25,
		}})

		state := probeExchangeAccountState(exchangeCfg, apiTestUser)

		assert.Equal(t, exchangeAccountStatusOK, state.Status)
		assert.InDelta(t, 107.18769421, state.TotalEquity, 1e-9)
		assert.InDelta(t, 60.25, state.AvailableBalance, 1e-9)
		assert.Equal(t, "107.1877 USDT", state.DisplayBalance)
	})

	t.Run("request failure is classified by the exchange error", func(t *testing.T) {
		stubExchangeProbe(t, &apiBalanceStub{err: assert.AnError})

		state := probeExchangeAccountState(exchangeCfg, apiTestUser)

		assert.Equal(t, exchangeAccountStatusUnavailable, state.Status)
		assert.Equal(t, "EXCHANGE_UNAVAILABLE", state.ErrorCode)
		assert.Empty(t, state.TotalEquity)
	})

	t.Run("unreadable balance", func(t *testing.T) {
		stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{types.BalanceKeyAvailable: 60.25}})

		state := probeExchangeAccountState(exchangeCfg, apiTestUser)

		assert.Equal(t, exchangeAccountStatusUnavailable, state.Status)
		assert.Equal(t, "BALANCE_NOT_FOUND", state.ErrorCode)
		assert.Contains(t, state.ErrorMessage, "unreadable")
	})
}

func TestClassifyTraderSetupReasonForInitialBalance(t *testing.T) {
	tests := []struct {
		reason   string
		wantKey  string
		contains string
	}{
		{reason: "[t] initial balance 0.00 is not valid and the account principal could not be read: failed to get account balance: request timed out",
			wantKey: "trader.reason.exchange_balance_unavailable", contains: "账户余额"},
		{reason: "[t] initial balance 0.00 is not valid and the account principal could not be read: unreadable account balance: balance field \"totalWalletBalance\" is missing",
			wantKey: "trader.reason.exchange_balance_unavailable", contains: "账户余额"},
		{reason: "[t] initial balance 0.00 is not valid and the account principal is not positive (0.00); deposit funds or set an initial balance above 0",
			wantKey: "trader.reason.exchange_principal_not_positive", contains: "本金不为正"},
	}
	for _, tt := range tests {
		key, message := classifyTraderSetupReason(tt.reason)
		assert.Equal(t, tt.wantKey, key, tt.reason)
		assert.Contains(t, message, tt.contains, tt.reason)
	}
}
