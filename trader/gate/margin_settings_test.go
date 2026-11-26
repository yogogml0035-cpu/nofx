package gate

// 仓位设置（保证金模式与杠杆）的单元测试：一次性生效的请求与读回、失败闭合、两个旧方法的保持语义、
// 持仓条目的解析。复用 protected_open_test.go 的 gateMock（按 Gate 线上格式响应）：
// set_leverage 更新条目并返回它，没有条目的合约 GET positions/{contract} 返回 POSITION_NOT_FOUND。

import (
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gate/gateapi-go/v7"
)

func TestApplyMarginSettingsRequestAndReadBack(t *testing.T) {
	cases := []struct {
		name  string
		cross bool
		mode  string
	}{
		{name: "全仓", cross: true, mode: "cross"},
		{name: "逐仓", cross: false, mode: "isolated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			// 该合约已有的另一组设置（线上读数：DOGE_USDT 全仓 75x）
			mock.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "75")

			require.NoError(t, gt.ApplyMarginSettings("DOGEUSDT", c.cross, 10))

			// 走文档的新接口，参数只有 leverage 与 margin_mode：不带 dual_side（账户是单向持仓模式）；
			// 返回值已确认，不再额外读回
			assert.Equal(t, []string{
				"POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=10&margin_mode=" + c.mode,
			}, mock.requestsSnapshot(t))
			entry := mock.settingSnapshot(t, "DOGE_USDT")
			require.NotNil(t, entry)
			assert.Equal(t, c.mode, entry["pos_margin_mode"])
			assert.Equal(t, "10", entry["lever"])
		})
	}
}

func TestApplyMarginSettingsRejectsNonPositiveLeverage(t *testing.T) {
	for _, leverage := range []int{0, -3} {
		mock := newGateMock()
		gt := newLocalGateTrader(t, mock.handler)

		err := gt.ApplyMarginSettings("DOGE_USDT", true, leverage)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "leverage must be at least 1")
		assert.Empty(t, mock.requestsSnapshot(t), "杠杆非法时不发任何请求")
	}
}

func TestApplyMarginSettingsFailsClosed(t *testing.T) {
	cases := []struct {
		name     string
		arrange  func(m *gateMock)
		wantErrs []string
	}{
		{
			name: "交易所拒绝（超过合约杠杆上限，线上实测的 label）",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "75")
				m.setLeverageFail = &gateAPIFailure{status: 400, label: "LEVERAGE_EXCEEDED", message: "limit [1, 75]"}
			},
			wantErrs: []string{"LEVERAGE_EXCEEDED"},
		},
		{
			name: "接口返回 200 但返回值与条目都不是目标设置（被截断）",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "3")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return positionEntry("DOGE_USDT", mode, "3")
				}
			},
			wantErrs: []string{"position shows"},
		},
		{
			name: "返回值与条目都读不出来",
			arrange: func(m *gateMock) {
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return map[string]interface{}{
						"contract": "DOGE_USDT", "pos_margin_mode": mode,
						"leverage": "0", "cross_leverage_limit": "0", "lever": "0",
					}
				}
			},
			wantErrs: []string{"cross_leverage_limit \"0\" is not a positive integer", "reading back failed"},
		},
		{
			name: "模式不对：要全仓，交易所返回逐仓",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "isolated", "10")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return positionEntry("DOGE_USDT", "isolated", leverage)
				}
			},
			wantErrs: []string{"cross 10x", "isolated 10x"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			c.arrange(mock)

			err := gt.ApplyMarginSettings("DOGE_USDT", true, 10)

			require.Error(t, err)
			for _, want := range c.wantErrs {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestApplyMarginSettingsFallsBackToPositionEntry(t *testing.T) {
	// 接口报错、返回值读不出或与目标不一致时读回持仓条目：条目等于目标设置才算成功，否则报错。
	// W4 实测重复设置同一组值时交易所直接返回 200，不走这条回退；这里模拟的是请求超时后重试、
	// 或交易所以错误表示「没有变化」之类的情形
	cases := []struct {
		name    string
		arrange func(m *gateMock)
		wantErr bool
	}{
		{
			name: "接口报错，条目已是目标设置 → 成功",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "10")
				m.setLeverageFail = &gateAPIFailure{status: 400, label: "NO_CHANGE", message: "no change"}
			},
		},
		{
			name: "接口返回 200 但返回值读不出，条目等于目标设置 → 成功",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "10")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return map[string]interface{}{"contract": "DOGE_USDT", "pos_margin_mode": mode}
				}
			},
		},
		{
			name: "接口返回 200 但返回值与目标不一致，条目等于目标设置 → 成功",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "10")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return positionEntry("DOGE_USDT", mode, "3")
				}
			},
		},
		{
			name: "接口返回 200 但返回值读不出，条目也不等于目标设置 → 报错",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "3")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return map[string]interface{}{"contract": "DOGE_USDT", "pos_margin_mode": mode}
				}
			},
			wantErr: true,
		},
		{
			name: "接口返回 200 但返回值与目标不一致，条目也不等于目标设置 → 报错",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "3")
				m.setLeverageRespondsWith = func(mode, leverage string) map[string]interface{} {
					return positionEntry("DOGE_USDT", mode, "3")
				}
			},
			wantErr: true,
		},
		{
			name: "接口报错，条目也不等于目标设置 → 报错",
			arrange: func(m *gateMock) {
				m.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "10")
				m.setLeverageFail = &gateAPIFailure{status: 400, label: "NO_CHANGE", message: "no change"}
			},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			c.arrange(mock)
			leverage := 10
			if c.wantErr {
				leverage = 5 // 目标 5x：条目（全仓 10x 或 3x）与返回值都对不上
			}

			err := gt.ApplyMarginSettings("DOGE_USDT", true, leverage)

			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "position shows")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, []string{
				"POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=" + strconv.Itoa(leverage) + "&margin_mode=cross",
				"GET /futures/usdt/positions/DOGE_USDT",
			}, mock.requestsSnapshot(t), "接口没有确认目标设置时必须读回持仓条目")
		})
	}
}

func TestApplyMarginSettingsWrapsExchangeError(t *testing.T) {
	// 交易所的错误保留在错误链里：调用方可用 errors.As 取出 GateAPIError 与 label
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "75")
	mock.setLeverageFail = &gateAPIFailure{status: 400, label: "LEVERAGE_EXCEEDED", message: "limit [1, 75]"}

	err := gt.ApplyMarginSettings("DOGE_USDT", true, 100)

	require.Error(t, err)
	var gateErr gateapi.GateAPIError
	require.True(t, errors.As(err, &gateErr), "交易所错误应可从错误链取出: %v", err)
	assert.Equal(t, "LEVERAGE_EXCEEDED", gateErr.Label)
}

func TestPositionMarginSettingsDistinguishesLabels(t *testing.T) {
	// 只有 POSITION_NOT_FOUND 代表「没有仓位设置条目」；其他 label 是读失败的错误，
	// 不得被当成没有条目——否则 SetLeverage 会在读失败时把合约设成逐仓
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.readSettingsFail = &gateAPIFailure{status: 400, label: "CONTRACT_NOT_FOUND", message: "contract not found"}

	_, err := gt.PositionMarginSettings("DOGE_USDT")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNoPositionSettings), "CONTRACT_NOT_FOUND 不是「没有条目」")
	assert.Contains(t, err.Error(), "CONTRACT_NOT_FOUND")
	// 读条目失败的错误链里带着交易所错误，调用方可以按 label 处理
	var gateErr gateapi.GateAPIError
	require.True(t, errors.As(err, &gateErr), "读条目失败的错误链里应带着交易所错误: %v", err)
	assert.Equal(t, "CONTRACT_NOT_FOUND", gateErr.Label)

	err = gt.SetLeverage("DOGE_USDT", 10)
	require.Error(t, err, "读条目失败时不得回退成逐仓去设置")
	assert.Contains(t, err.Error(), "CONTRACT_NOT_FOUND")

	err = gt.SetMarginMode("DOGE_USDT", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONTRACT_NOT_FOUND")

	for _, request := range mock.requestsSnapshot(t) {
		assert.NotContains(t, request, "/set_leverage", "读失败时不发设置请求")
	}
}

func TestPositionMarginSettingsHTTP(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "75")

	got, err := gt.PositionMarginSettings("DOGEUSDT")
	require.NoError(t, err)
	assert.Equal(t, MarginSettings{Cross: true, Leverage: 75}, got)
	assert.Equal(t, []string{"GET /futures/usdt/positions/DOGE_USDT"}, mock.requestsSnapshot(t))

	// 从未设置过的合约没有条目：POSITION_NOT_FOUND → ErrNoPositionSettings
	_, err = gt.PositionMarginSettings("ETH_USDT")
	require.ErrorIs(t, err, ErrNoPositionSettings)
	assert.Contains(t, err.Error(), "POSITION_NOT_FOUND")
}

func TestSetLeverageKeepsMarginMode(t *testing.T) {
	cases := []struct {
		name    string
		initial map[string]interface{} // nil：没有条目
		mode    string
	}{
		{name: "条目是全仓：保持全仓", initial: positionEntry("DOGE_USDT", "cross", "75"), mode: "cross"},
		{name: "条目是逐仓：保持逐仓", initial: positionEntry("DOGE_USDT", "isolated", "3"), mode: "isolated"},
		{name: "没有条目：按逐仓", initial: nil, mode: "isolated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := newGateMock()
			gt := newLocalGateTrader(t, mock.handler)
			if c.initial != nil {
				mock.settings["DOGE_USDT"] = c.initial
			}

			require.NoError(t, gt.SetLeverage("DOGE_USDT", 10))

			assert.Equal(t, []string{
				"GET /futures/usdt/positions/DOGE_USDT",
				"POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=10&margin_mode=" + c.mode,
			}, mock.requestsSnapshot(t))
			entry := mock.settingSnapshot(t, "DOGE_USDT")
			require.NotNil(t, entry)
			assert.Equal(t, c.mode, entry["pos_margin_mode"])
			assert.Equal(t, "10", entry["lever"])
		})
	}
}

func TestSetMarginModeKeepsLeverage(t *testing.T) {
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.settings["DOGE_USDT"] = positionEntry("DOGE_USDT", "cross", "75")

	// 只改模式：杠杆保持 75
	require.NoError(t, gt.SetMarginMode("DOGE_USDT", false))
	entry := mock.settingSnapshot(t, "DOGE_USDT")
	require.NotNil(t, entry)
	assert.Equal(t, "isolated", entry["pos_margin_mode"])
	assert.Equal(t, "75", entry["lever"])

	// 没有条目就没有可保持的杠杆：报错，且不发设置请求
	mock.requests = nil
	err := gt.SetMarginMode("ETH_USDT", true)
	require.ErrorIs(t, err, ErrNoPositionSettings)
	assert.Contains(t, err.Error(), "together with a leverage")
	assert.Nil(t, mock.settingSnapshot(t, "ETH_USDT"), "不得凭一个猜测的杠杆去设置")
	for _, request := range mock.requestsSnapshot(t) {
		assert.NotContains(t, request, "/set_leverage")
	}
}

func TestMarginSettingsFromPosition(t *testing.T) {
	// 杠杆取 lever 与同模式的旧字段（全仓 cross_leverage_limit、逐仓 leverage）：
	// 只有一个能读时用它，两个都能读时必须相等
	cases := []struct {
		name     string
		position gateapi.Position
		want     MarginSettings
		wantErr  bool
	}{
		{
			name:     "全仓条目（线上读数：DOGE_USDT 全仓 75x）",
			position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "75", Lever: "75"},
			want:     MarginSettings{Cross: true, Leverage: 75},
		},
		{
			name:     "逐仓条目",
			position: gateapi.Position{PosMarginMode: "isolated", Leverage: "5", CrossLeverageLimit: "0", Lever: "5"},
			want:     MarginSettings{Leverage: 5},
		},
		{
			// W4 实测：切到逐仓后 cross_leverage_limit 残留上一次全仓的值
			name:     "逐仓条目带着残留的旧全仓杠杆",
			position: gateapi.Position{Mode: "single", PosMarginMode: "isolated", Leverage: "3", CrossLeverageLimit: "5", Lever: "3"},
			want:     MarginSettings{Leverage: 3},
		},
		{
			// 文档 Position 响应示例的形态：mode isolated、leverage "0"、cross_leverage_limit "0"、lever "30"。
			// 逐仓只看 leverage（"0" 读不出杠杆），所以取 lever；残留的 cross_leverage_limit 不参与
			name:     "同模式的旧字段读不出杠杆时取 lever（文档示例的形态）",
			position: gateapi.Position{PosMarginMode: "isolated", Leverage: "0", CrossLeverageLimit: "0", Lever: "30"},
			want:     MarginSettings{Leverage: 30},
		},
		{
			name:     "同模式的旧字段缺失时取 lever",
			position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", Lever: "10"},
			want:     MarginSettings{Cross: true, Leverage: 10},
		},
		{
			// 两个来源都能读却不一致：自相矛盾的读数，不替交易所挑一个（父任务 design.md 约定 3）
			name:     "lever 与同模式的旧字段矛盾（全仓）",
			position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "75", Lever: "10"},
			wantErr:  true,
		},
		{
			name:     "lever 与同模式的旧字段矛盾（逐仓）",
			position: gateapi.Position{PosMarginMode: "isolated", Leverage: "5", Lever: "3"},
			wantErr:  true,
		},
		{
			name:     "没有 lever 的旧响应：全仓取 cross_leverage_limit",
			position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "10"},
			want:     MarginSettings{Cross: true, Leverage: 10},
		},
		{
			name:     "没有 lever 的旧响应：逐仓取 leverage",
			position: gateapi.Position{PosMarginMode: "isolated", Leverage: "5"},
			want:     MarginSettings{Leverage: 5},
		},
		{
			name:     "lever 不是正整数时按模式取旧字段",
			position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "10", Lever: "0"},
			want:     MarginSettings{Cross: true, Leverage: 10},
		},
		{name: "模式缺失", position: gateapi.Position{Lever: "10"}, wantErr: true},
		{name: "模式取值未知", position: gateapi.Position{PosMarginMode: "hedge", Lever: "10"}, wantErr: true},
		{name: "模式大小写不符", position: gateapi.Position{PosMarginMode: "CROSS", Lever: "10"}, wantErr: true},
		{
			// 设置接口不带 dual_side：双向持仓下的设置不属于任何一个方向
			name:     "双向持仓模式",
			position: gateapi.Position{Mode: "dual_long", PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "10", Lever: "10"},
			wantErr:  true,
		},
		{name: "杠杆为 0", position: gateapi.Position{PosMarginMode: "cross", Leverage: "0", CrossLeverageLimit: "0", Lever: "0"}, wantErr: true},
		{name: "杠杆缺失", position: gateapi.Position{PosMarginMode: "isolated"}, wantErr: true},
		{name: "杠杆非整数", position: gateapi.Position{PosMarginMode: "isolated", Leverage: "5.5", Lever: "5.5"}, wantErr: true},
		{name: "杠杆不是十进制", position: gateapi.Position{PosMarginMode: "isolated", Leverage: "abc", Lever: "1e2"}, wantErr: true},
		{name: "杠杆为负", position: gateapi.Position{PosMarginMode: "isolated", Leverage: "-5", Lever: "-5"}, wantErr: true},
		{name: "杠杆超出 int 范围", position: gateapi.Position{PosMarginMode: "isolated", Leverage: "99999999999999999999"}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := marginSettingsFromPosition(&c.position)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestModeSwitchKeepsLeverage(t *testing.T) {
	// W4 实测的条目形态：切到逐仓后 cross_leverage_limit 残留上一次全仓的值，读回仍以当前杠杆为准；
	// SetMarginMode 切回全仓时先读回当前杠杆、再带着它设置（适配器保持杠杆；交易所自身是否保留没有测）
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)

	require.NoError(t, gt.ApplyMarginSettings("DOGE_USDT", true, 5))
	require.NoError(t, gt.ApplyMarginSettings("DOGE_USDT", false, 3))

	entry := mock.settingSnapshot(t, "DOGE_USDT")
	assert.Equal(t, "5", entry["cross_leverage_limit"], "逐仓条目残留旧的全仓杠杆")
	assert.Equal(t, "3", entry["leverage"])
	assert.Equal(t, "3", entry["lever"])
	got, err := gt.PositionMarginSettings("DOGE_USDT")
	require.NoError(t, err)
	assert.Equal(t, MarginSettings{Leverage: 3}, got)

	// 逐仓 3x → 全仓 3x：读条目，再带 leverage=3 设成全仓
	mock.requests = nil
	require.NoError(t, gt.SetMarginMode("DOGE_USDT", true))
	assert.Equal(t, []string{
		"GET /futures/usdt/positions/DOGE_USDT",
		"POST /futures/usdt/positions/DOGE_USDT/set_leverage?leverage=3&margin_mode=cross",
	}, mock.requestsSnapshot(t))
	entry = mock.settingSnapshot(t, "DOGE_USDT")
	assert.Equal(t, "cross", entry["pos_margin_mode"])
	assert.Equal(t, "0", entry["leverage"])
	assert.Equal(t, "3", entry["cross_leverage_limit"])
	assert.Equal(t, "3", entry["lever"])
}

func TestApplyMarginSettingsKeepsExchangeErrorWhenReadBackFails(t *testing.T) {
	// 接口被交易所拒绝、随后的读回也失败：错误链里是交易所的拒绝错误（不是读回失败的错误，
	// 也不得被误当成「没有条目」）
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.setLeverageFail = &gateAPIFailure{status: 400, label: "LEVERAGE_EXCEEDED", message: "limit [1, 75]"}
	mock.readSettingsFail = &gateAPIFailure{status: 400, label: "CONTRACT_NOT_FOUND", message: "contract not found"}

	err := gt.ApplyMarginSettings("DOGE_USDT", true, 100)

	require.Error(t, err)
	var gateErr gateapi.GateAPIError
	require.True(t, errors.As(err, &gateErr), "接口报错时错误链里应带着交易所的拒绝错误: %v", err)
	assert.Equal(t, "LEVERAGE_EXCEEDED", gateErr.Label)
	assert.Contains(t, err.Error(), "reading back failed")
	assert.Contains(t, err.Error(), "CONTRACT_NOT_FOUND", "读回失败的错误作为上下文保留")
	assert.False(t, errors.Is(err, ErrNoPositionSettings), "读回失败只是上下文，不表示合约没有条目")
}

func TestPositionMarginSettingsUnreadableEntry(t *testing.T) {
	// 条目读不出设置：报错，解析错误留在错误链里
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.settings["DOGE_USDT"] = map[string]interface{}{"contract": "DOGE_USDT", "pos_margin_mode": "hedge", "lever": "10"}

	_, err := gt.PositionMarginSettings("DOGE_USDT")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pos_margin_mode")
	assert.NotNil(t, errors.Unwrap(err), "解析错误应留在错误链里")
}

func TestMarginSettingsString(t *testing.T) {
	assert.Equal(t, "cross 10x", MarginSettings{Cross: true, Leverage: 10}.String())
	assert.Equal(t, "isolated 5x", MarginSettings{Leverage: 5}.String())
}

func TestOpenMarketLeverage(t *testing.T) {
	// 调用方给了杠杆：必须生效，失败即不开仓
	mock := newGateMock()
	gt := newLocalGateTrader(t, mock.handler)
	mock.setLeverageFail = &gateAPIFailure{status: 400, label: "LEVERAGE_EXCEEDED", message: "limit [1, 75]"}

	_, err := gt.OpenLong("DOGE_USDT", 170, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to open")
	assert.Contains(t, err.Error(), "LEVERAGE_EXCEEDED")
	mock.assertNoOrder(t)

	// leverage=0 表示不设置：不发设置请求，直接下单，沿用该合约在交易所的现有设置
	mock.requests = nil
	_, err = gt.OpenLong("DOGE_USDT", 170, 0)
	require.NoError(t, err)
	for _, request := range mock.requestsSnapshot(t) {
		assert.NotContains(t, request, "/set_leverage")
	}
}
