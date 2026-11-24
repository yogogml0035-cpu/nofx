package api

// 初始资金在 API 层的单元测试（子任务 account-equity）：创建时取交易所账户的本金，
// 更新时拒绝负值、换绑交易所时重置旧的初始余额，同步余额取本金，加载失败的原因按错误类型归类。
// 探测用的适配器换成不连交易所的桩。

import (
	"encoding/json"
	"errors"
	"fmt"
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
	apiTestUser       = "user-1"
	apiTestExchange   = "exchange-1"
	apiTestExchange2  = "exchange-2"
	apiTestAIModel    = "model-1"
	apiTestStrategy   = "strategy-1"
	apiTestTraderID   = "trader-1"
	reasonUnreadable  = "系统暂时无法从交易所读取账户余额"
	reasonNotPositive = "交易所账户的本金不为正：请先入金，或把初始余额改成大于 0 的值"
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

// stubExchangeProbe 把探测适配器换成桩，测试结束后恢复。
// 这几个包的测试都不并行（没有 t.Parallel），替换包级变量不会互相干扰
func stubExchangeProbe(t *testing.T, stub *apiBalanceStub) {
	t.Helper()
	original := buildExchangeProbeTrader
	buildExchangeProbeTrader = func(*store.Exchange, string) (trader.Trader, error) {
		return stub, nil
	}
	t.Cleanup(func() { buildExchangeProbeTrader = original })
}

// newInitialBalanceAPITestEnv 建一个带临时 sqlite 库的 API 服务，库里有两个启用中的 Gate 交易所账户、
// 一个启用中的模型和一条配置不是合法 JSON 的策略。创建只要求策略存在；创建、更新之后后端会重新加载
// trader，加载在解析策略配置一步失败，不会构造真实的交易所适配器去读余额
func newInitialBalanceAPITestEnv(t *testing.T) *Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.New(filepath.Join(t.TempDir(), "api.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	db := st.GormDB()
	for _, id := range []string{apiTestExchange, apiTestExchange2} {
		require.NoError(t, db.Create(&store.Exchange{
			ID: id, UserID: apiTestUser, Name: "gate " + id, Type: "cex", ExchangeType: "gate", Enabled: true,
			APIKey: "key", SecretKey: "secret",
		}).Error)
	}
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

var traderPathParams = gin.Params{{Key: "id", Value: apiTestTraderID}}

func createBody() string {
	return `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange + `","strategy_id":"` + apiTestStrategy + `"}`
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

	w := callHandler(t, s.handleCreateTrader, http.MethodPost, "/api/traders", createBody(), nil)

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

func TestCreateTraderLeavesZeroWithoutValidPrincipal(t *testing.T) {
	// 读不到账户读数，或账户本金不为正：都不阻断创建，初始余额留 0，加载时再以当前本金重建
	tests := []struct {
		name string
		stub *apiBalanceStub
	}{
		{name: "exchange read fails", stub: &apiBalanceStub{err: assert.AnError}},
		{name: "balance is unreadable", stub: &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyAvailable: 100.0,
		}}},
		{name: "principal is zero while equity is positive", stub: &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: 0.0, types.BalanceKeyUnrealizedPnL: 40.0, types.BalanceKeyAvailable: 10.0, "totalEquity": 40.0,
		}}},
		{name: "principal is negative", stub: &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: -5.0, types.BalanceKeyUnrealizedPnL: 0.0, types.BalanceKeyAvailable: 0.0,
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			stubExchangeProbe(t, tt.stub)

			w := callHandler(t, s.handleCreateTrader, http.MethodPost, "/api/traders", createBody(), nil)

			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			var created struct {
				TraderID string `json:"trader_id"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
			record, err := s.store.Trader().GetByID(created.TraderID)
			require.NoError(t, err)
			assert.Zero(t, record.InitialBalance, "no valid principal: nothing is recorded as the initial balance")
		})
	}
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
		{name: "same exchange and the stored value echoed back: kept", body: `"initial_balance":200`, initial: 200, wantStatus: http.StatusOK, wantValue: 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, tt.initial)

			body := `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange + `",` + tt.body + `}`
			w := callHandler(t, s.handleUpdateTrader, http.MethodPut, "/api/traders/"+apiTestTraderID, body, traderPathParams)

			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tt.wantValue, storedTrader(t, s).InitialBalance)
		})
	}
}

func TestUpdateTraderRebindingExchangeResetsInitialBalance(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantValue float64
	}{
		// 旧值属于原来那个交易所账户：换绑后重置为 0，加载时以新账户的本金重建
		{name: "field omitted: reset", body: ``, wantValue: 0},
		{name: "stored value echoed back (the edit form does this when untouched): reset",
			body: `"initial_balance":200,`, wantValue: 0},
		{name: "a new value given explicitly: used as is",
			body: `"initial_balance":350,`, wantValue: 350},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, 200)

			body := `{"name":"t","ai_model_id":"` + apiTestAIModel + `","exchange_id":"` + apiTestExchange2 + `",` + tt.body + `"scan_interval_minutes":3}`
			w := callHandler(t, s.handleUpdateTrader, http.MethodPut, "/api/traders/"+apiTestTraderID, body, traderPathParams)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			record := storedTrader(t, s)
			assert.Equal(t, apiTestExchange2, record.ExchangeID, "the trader is rebound to the new exchange")
			assert.Equal(t, tt.wantValue, record.InitialBalance)
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
		traderPathParams)

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
		traderPathParams)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), "the response must be valid JSON (no Infinity)")
	assert.Equal(t, 0.0, response["change_percent"])
	assert.Equal(t, 107.49769421, storedTrader(t, s).InitialBalance)
}

func TestSyncBalanceFailsWithoutValidPrincipal(t *testing.T) {
	tests := []struct {
		name string
		stub *apiBalanceStub
	}{
		{name: "balance is unreadable", stub: &apiBalanceStub{balance: map[string]interface{}{types.BalanceKeyAvailable: 50.0}}},
		{name: "principal is not positive", stub: &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: 0.0, types.BalanceKeyUnrealizedPnL: 30.0, types.BalanceKeyAvailable: 10.0, "totalEquity": 30.0,
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, 100)
			stubExchangeProbe(t, tt.stub)

			w := callHandler(t, s.handleSyncBalance, http.MethodPost, "/api/traders/"+apiTestTraderID+"/sync-balance", "",
				traderPathParams)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Equal(t, 100.0, storedTrader(t, s).InitialBalance, "a failed sync must not touch the stored value")
		})
	}
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

// loadFailure 包出 trader 加载失败时的错误。原因本身由 types.ReadInitialBalance 产生
// （resolveInitialBalance 走的就是它），这里按线上的包装方式补上前缀：
// resolveInitialBalance 的「initial balance 0.00 is not valid: …」，再是 manager 与 NewAutoTrader 的包装
func loadFailure(t *testing.T, stub *apiBalanceStub) error {
	t.Helper()
	_, err := types.ReadInitialBalance(stub)
	require.Error(t, err)
	return fmt.Errorf("failed to create trader: [demo] initial balance %.2f is not valid: %w", 0.0, err)
}

func TestLoadFailureReasonSurvivesWrappingAndSanitizing(t *testing.T) {
	// 错误文本同时含「:」和「.」：按 SanitizeError 的文本规则会被整体抹掉，
	// 所以归类必须认错误类型，不能匹配文案
	exchangeErr := errors.New("label: INVALID_KEY, message: Invalid key provided")

	t.Run("account balance unreadable", func(t *testing.T) {
		wrapped := loadFailure(t, &apiBalanceStub{err: exchangeErr})
		require.Contains(t, wrapped.Error(), exchangeErr.Error())
		assert.Empty(t, SanitizeError(wrapped, ""), "the text of this error is wiped by sanitizing")

		key, message := classifyTraderSetupError(wrapped)
		assert.Equal(t, "trader.reason.exchange_balance_unavailable", key)
		assert.Equal(t, reasonUnreadable, message)
		assert.Contains(t, describeTraderCreationWarning("demo", wrapped), reasonUnreadable)
		assert.Contains(t, describeTraderStartError("demo", wrapped), reasonUnreadable)
		assert.Contains(t, describeTraderLoadError("demo", wrapped), reasonUnreadable)
		params := traderSetupReasonParams(wrapped, "配置校验未通过")
		assert.Equal(t, "trader.reason.exchange_balance_unavailable", params["reason_key"])
		assert.Equal(t, reasonUnreadable, params["reason"])
	})

	t.Run("account principal not positive", func(t *testing.T) {
		wrapped := loadFailure(t, &apiBalanceStub{balance: map[string]interface{}{
			types.BalanceKeyPrincipal: 0.0, types.BalanceKeyUnrealizedPnL: 0.0, types.BalanceKeyAvailable: 0.0,
		}})
		assert.Empty(t, SanitizeError(wrapped, ""))

		key, message := classifyTraderSetupError(wrapped)
		assert.Equal(t, "trader.reason.exchange_principal_not_positive", key)
		assert.Equal(t, reasonNotPositive, message)
		assert.Contains(t, describeTraderCreationWarning("demo", wrapped), reasonNotPositive)
		assert.Contains(t, describeTraderStartError("demo", wrapped), reasonNotPositive)
		params := traderSetupReasonParams(wrapped, "配置校验未通过")
		assert.Equal(t, "trader.reason.exchange_principal_not_positive", params["reason_key"])
		assert.Equal(t, reasonNotPositive, params["reason"])
	})

	t.Run("other errors keep the text-based classification", func(t *testing.T) {
		key, message := classifyTraderSetupError(errors.New("trader demo has no strategy configured"))
		assert.Equal(t, "trader.reason.strategy_missing", key)
		assert.Equal(t, "当前机器人缺少有效的交易策略配置", message)

		// 敏感文本仍然不外露：只有通用提示
		params := traderSetupReasonParams(errors.New("dial tcp 127.0.0.1:5432: connection refused"), "配置校验未通过")
		assert.Equal(t, "trader.reason.unknown", params["reason_key"])
		assert.Equal(t, "配置校验未通过", params["reason"])
		assert.Contains(t, describeTraderStartError("demo", errors.New("dial tcp 127.0.0.1:5432: connection refused")), "暂时还不能启动")
		assert.NotContains(t, describeTraderStartError("demo", errors.New("dial tcp 127.0.0.1:5432: connection refused")), "127.0.0.1")
	})
}
