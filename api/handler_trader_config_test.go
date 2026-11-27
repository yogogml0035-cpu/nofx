package api

// 编辑弹窗的初值来自 GET /traders/:id/config。前端类型把 show_in_competition 声明为必填，
// 这里的测试锁住该契约：漏字段会让弹窗把“显示”误判成“隐藏”，并且保存时把 undefined 丢掉。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"nofx/store"
	"nofx/trader/types"
)

func TestCreateTraderPersistsExplicitFalseBooleans(t *testing.T) {
	s := newInitialBalanceAPITestEnv(t)
	stubExchangeProbe(t, &apiBalanceStub{balance: map[string]interface{}{
		types.BalanceKeyPrincipal: 100.0,
	}})

	body := fmt.Sprintf(
		`{"name":"hidden isolated","ai_model_id":%q,"exchange_id":%q,"strategy_id":%q,"show_in_competition":false,"is_cross_margin":false}`,
		apiTestAIModel, apiTestExchange, apiTestStrategy,
	)
	w := callHandler(t, s.handleCreateTrader, http.MethodPost, "/api/traders", body, nil)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created struct {
		TraderID string `json:"trader_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.TraderID)

	record, err := s.store.Trader().GetByID(created.TraderID)
	require.NoError(t, err)
	assert.False(t, record.ShowInCompetition, "the create path must keep an explicit hide choice")
	assert.False(t, record.IsCrossMargin, "the create path must keep an explicit isolated choice")
}

func TestUpdateTraderClampsScanIntervalToManualRange(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		initial   int
		want      int
	}{
		{name: "below minimum is raised", requested: 1, initial: 3, want: 3},
		{name: "above maximum is lowered", requested: 90, initial: 30, want: 60},
		{name: "zero keeps the stored value", requested: 0, initial: 30, want: 30},
		{name: "in range is kept", requested: 5, initial: 3, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, 100)
			require.NoError(t, s.store.GormDB().Model(&store.Trader{}).
				Where("id = ? AND user_id = ?", apiTestTraderID, apiTestUser).
				Update("scan_interval_minutes", tt.initial).Error)

			body := fmt.Sprintf(
				`{"name":"t","ai_model_id":%q,"exchange_id":%q,"scan_interval_minutes":%d}`,
				apiTestAIModel, apiTestExchange, tt.requested,
			)
			w := callHandler(t, s.handleUpdateTrader, http.MethodPut,
				"/api/traders/"+apiTestTraderID, body, traderPathParams)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, tt.want, storedTrader(t, s).ScanIntervalMinutes)
		})
	}
}

func TestToggleCompetitionRequiresExplicitValue(t *testing.T) {
	s := newInitialBalanceAPITestEnv(t)
	createStoredTrader(t, s, 100)
	require.NoError(t, s.store.GormDB().Model(&store.Trader{}).
		Where("id = ? AND user_id = ?", apiTestTraderID, apiTestUser).
		Update("show_in_competition", true).Error)

	// 空 body 不能被当成 false，否则会静默隐藏交易员
	w := callHandler(t, s.handleToggleCompetition, http.MethodPut,
		"/api/traders/"+apiTestTraderID+"/competition", `{}`, traderPathParams)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.True(t, storedTrader(t, s).ShowInCompetition)

	w = callHandler(t, s.handleToggleCompetition, http.MethodPut,
		"/api/traders/"+apiTestTraderID+"/competition",
		`{"show_in_competition":false}`, traderPathParams)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, storedTrader(t, s).ShowInCompetition)
}

func TestGetTraderConfigReturnsCompetitionVisibility(t *testing.T) {
	for _, visible := range []bool{true, false} {
		t.Run(map[bool]string{true: "shown", false: "hidden"}[visible], func(t *testing.T) {
			s := newInitialBalanceAPITestEnv(t)
			createStoredTrader(t, s, 100)
			require.NoError(t, s.store.GormDB().Model(&store.Trader{}).
				Where("id = ? AND user_id = ?", apiTestTraderID, apiTestUser).
				Update("show_in_competition", visible).Error)

			w := callHandler(t, s.handleGetTraderConfig, http.MethodGet,
				"/api/traders/"+apiTestTraderID+"/config", "", traderPathParams)
			require.Equal(t, http.StatusOK, w.Code)

			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			value, ok := body["show_in_competition"]
			require.True(t, ok, "GET trader config must include show_in_competition")
			assert.Equal(t, visible, value)
		})
	}
}
