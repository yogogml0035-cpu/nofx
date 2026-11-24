package gate

import (
	"math"
	"testing"

	"github.com/gate/gateapi-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtectiveTriggerRule(t *testing.T) {
	// 方向 × 类型 → 触发规则矩阵（Gate 语义：1 = ≥ 且触发价须高于最新价，2 = ≤ 且须低于最新价）
	cases := []struct {
		side    string
		kind    ProtectiveKind
		rule    int32
		wantErr bool
	}{
		{side: "LONG", kind: ProtectiveStopLoss, rule: PriceRuleLE},
		{side: "LONG", kind: ProtectiveTakeProfit, rule: PriceRuleGE},
		{side: "SHORT", kind: ProtectiveStopLoss, rule: PriceRuleGE},
		{side: "SHORT", kind: ProtectiveTakeProfit, rule: PriceRuleLE},
		{side: "long", kind: ProtectiveStopLoss, rule: PriceRuleLE},
		{side: " short ", kind: ProtectiveTakeProfit, rule: PriceRuleLE},
		{side: "BOTH", kind: ProtectiveStopLoss, wantErr: true},
		{side: "", kind: ProtectiveStopLoss, wantErr: true},
		{side: "LONG", kind: 0, wantErr: true},
		{side: "LONG", kind: ProtectiveTakeProfit + 1, wantErr: true},
	}
	for _, c := range cases {
		rule, err := protectiveTriggerRule(c.side, c.kind)
		if c.wantErr {
			assert.Error(t, err, "side=%q kind=%v", c.side, c.kind)
			continue
		}
		require.NoError(t, err, "side=%q kind=%v", c.side, c.kind)
		assert.Equal(t, c.rule, rule, "side=%q kind=%v", c.side, c.kind)
	}
}

func TestProtectiveOrderType(t *testing.T) {
	orderType, err := protectiveOrderType("LONG")
	require.NoError(t, err)
	assert.Equal(t, "close-long-position", orderType)

	orderType, err = protectiveOrderType("SHORT")
	require.NoError(t, err)
	assert.Equal(t, "close-short-position", orderType)

	_, err = protectiveOrderType("both")
	assert.Error(t, err)
}

func TestBuildPositionProtectiveOrder(t *testing.T) {
	// 构造矩阵：方向（多/空）× 类型（止损/止盈）→ rule、order_type、initial 逐项断言
	cases := []struct {
		side      string
		kind      ProtectiveKind
		rule      int32
		orderType string
	}{
		{side: "LONG", kind: ProtectiveStopLoss, rule: PriceRuleLE, orderType: "close-long-position"},
		{side: "LONG", kind: ProtectiveTakeProfit, rule: PriceRuleGE, orderType: "close-long-position"},
		{side: "SHORT", kind: ProtectiveStopLoss, rule: PriceRuleGE, orderType: "close-short-position"},
		{side: "SHORT", kind: ProtectiveTakeProfit, rule: PriceRuleLE, orderType: "close-short-position"},
	}
	for _, c := range cases {
		order, err := BuildPositionProtectiveOrder("DOGE_USDT", c.side, c.kind, 0.1234)
		require.NoError(t, err, "side=%q kind=%v", c.side, c.kind)

		assert.Equal(t, c.rule, order.Trigger.Rule, "side=%q kind=%v", c.side, c.kind)
		assert.Equal(t, c.orderType, order.OrderType, "side=%q kind=%v", c.side, c.kind)
		assert.Equal(t, int32(0), order.Trigger.StrategyType, "价格触发")
		assert.Equal(t, int32(0), order.Trigger.PriceType, "最新成交价")
		assert.Equal(t, "0.1234", order.Trigger.Price)
		assert.Equal(t, int64(0), order.Initial.Size, "仓位级全平 size=0")
		assert.True(t, order.Initial.Close, "单向持仓全平必须 close=true")
		assert.Equal(t, "0", order.Initial.Price, "触发后市价出场")
		assert.Equal(t, "ioc", order.Initial.Tif, "市价单只支持 ioc")
		assert.Equal(t, "DOGE_USDT", order.Initial.Contract)
		assert.Empty(t, order.Initial.AutoSize, "单向持仓模式不需 auto_size")
		assert.Empty(t, order.Trigger.Expiration, "保护单不设超时")
	}

	// 价格按 float64 的最短十进制表示输出
	order, err := BuildPositionProtectiveOrder("ETH_USDT", "LONG", ProtectiveStopLoss, 2077.25)
	require.NoError(t, err)
	assert.Equal(t, "2077.25", order.Trigger.Price)

	// 拒绝矩阵：空合约、非法方向/类型、非正价格、NaN
	for _, kind := range []ProtectiveKind{ProtectiveStopLoss, ProtectiveTakeProfit} {
		_, err := BuildPositionProtectiveOrder("", "LONG", kind, 1)
		assert.Error(t, err, "empty contract kind=%v", kind)

		_, err = BuildPositionProtectiveOrder("DOGE_USDT", "BOTH", kind, 1)
		assert.Error(t, err, "bad side kind=%v", kind)

		for _, price := range []float64{0, -0.5, math.NaN(), math.Inf(1)} {
			_, err := BuildPositionProtectiveOrder("DOGE_USDT", "LONG", kind, price)
			assert.Error(t, err, "price=%v kind=%v", price, kind)
		}
	}
	_, err = BuildPositionProtectiveOrder("DOGE_USDT", "LONG", 0, 1)
	assert.Error(t, err, "kind 为零值")
}

func TestApplyAttachedTPSL(t *testing.T) {
	t.Run("多头止损低于止盈", func(t *testing.T) {
		order := &gateapi.FuturesOrder{Contract: "DOGE_USDT", Size: "17"}
		require.NoError(t, ApplyAttachedTPSL(order, "LONG", 0.14, 0.18))
		assert.Equal(t, "0.14", order.TpslSlTriggerPrice)
		assert.Equal(t, "0.18", order.TpslTpTriggerPrice)
	})

	t.Run("空头止盈低于止损", func(t *testing.T) {
		order := &gateapi.FuturesOrder{Contract: "DOGE_USDT", Size: "-17"}
		require.NoError(t, ApplyAttachedTPSL(order, "SHORT", 0.18, 0.14))
		assert.Equal(t, "0.18", order.TpslSlTriggerPrice)
		assert.Equal(t, "0.14", order.TpslTpTriggerPrice)
	})

	t.Run("方向大小写与空白", func(t *testing.T) {
		order := &gateapi.FuturesOrder{Contract: "DOGE_USDT"}
		require.NoError(t, ApplyAttachedTPSL(order, " long ", 0.14, 0.18))
		assert.Equal(t, "0.14", order.TpslSlTriggerPrice)
	})

	t.Run("拒绝矩阵", func(t *testing.T) {
		err := ApplyAttachedTPSL(nil, "LONG", 1, 2)
		assert.Error(t, err, "nil order")

		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{}, "LONG", 1, 2)
		assert.Error(t, err, "empty contract")

		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "BOTH", 1, 2)
		assert.Error(t, err, "bad side")

		for _, price := range []float64{0, -1, math.NaN()} {
			err := ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "LONG", price, 2)
			assert.Error(t, err, "stop loss %v", price)
			err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "LONG", 1, price)
			assert.Error(t, err, "take profit %v", price)
		}

		// 多头止损不低于止盈、空头止盈不低于止损都拒绝：两价必须分居入场价两侧
		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "LONG", 0.18, 0.18)
		assert.Error(t, err, "long sl == tp")
		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "LONG", 0.20, 0.18)
		assert.Error(t, err, "long sl > tp")
		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "SHORT", 0.14, 0.14)
		assert.Error(t, err, "short tp == sl")
		err = ApplyAttachedTPSL(&gateapi.FuturesOrder{Contract: "DOGE_USDT"}, "SHORT", 0.12, 0.14)
		assert.Error(t, err, "short tp > sl")
	})
}

func TestClassifyTriggerOrder(t *testing.T) {
	// 判定矩阵：方向 × 规则 → 止损/止盈（构造矩阵的倒查）
	cases := []struct {
		orderType string
		rule      int32
		side      string
		kind      ProtectiveKind
		known     bool
	}{
		{orderType: "close-long-position", rule: PriceRuleLE, side: "LONG", kind: ProtectiveStopLoss, known: true},
		{orderType: "close-long-position", rule: PriceRuleGE, side: "LONG", kind: ProtectiveTakeProfit, known: true},
		{orderType: "close-short-position", rule: PriceRuleGE, side: "SHORT", kind: ProtectiveStopLoss, known: true},
		{orderType: "close-short-position", rule: PriceRuleLE, side: "SHORT", kind: ProtectiveTakeProfit, known: true},
		{orderType: "plan-close-long-position", rule: PriceRuleLE, side: "LONG", kind: ProtectiveStopLoss, known: true},
		{orderType: "plan-close-short-position", rule: PriceRuleGE, side: "SHORT", kind: ProtectiveStopLoss, known: true},
		{orderType: "close-long-order", rule: PriceRuleLE, side: "LONG", kind: ProtectiveStopLoss, known: true},
		{orderType: "close-short-order", rule: PriceRuleLE, side: "SHORT", kind: ProtectiveTakeProfit, known: true},
		{orderType: "", rule: PriceRuleLE, side: "", kind: 0, known: false},
		{orderType: "something-else", rule: PriceRuleGE, side: "", kind: 0, known: false},
		{orderType: "close-long-position", rule: 5, side: "LONG", kind: 0, known: false},
	}
	for _, c := range cases {
		side, kind, known := classifyTriggerOrder(c.orderType, c.rule)
		assert.Equal(t, c.side, side, "orderType=%q rule=%d", c.orderType, c.rule)
		assert.Equal(t, c.known, known, "orderType=%q rule=%d", c.orderType, c.rule)
		if known {
			assert.Equal(t, c.kind, kind, "orderType=%q rule=%d", c.orderType, c.rule)
		}
	}
}

func TestParseTriggerOrder(t *testing.T) {
	order := gateapi.FuturesPriceTriggeredOrder{
		Id:       12345,
		IdString: "12345",
		OrderType: "close-long-position",
		Initial: gateapi.FuturesInitialOrder{
			Contract: "ETH_USDT",
			Size:     0,
			Amount:   "0.6",
			Price:    "0",
		},
		Trigger: gateapi.FuturesPriceTrigger{
			Price: "2077.25",
			Rule:  PriceRuleLE,
		},
		Status: "open",
	}

	view, err := parseTriggerOrder(order)
	require.NoError(t, err)
	assert.Equal(t, int64(12345), view.ID)
	assert.Equal(t, "12345", view.IDString)
	assert.Equal(t, "12345", view.OrderID())
	assert.Equal(t, "ETH_USDT", view.Contract)
	assert.Equal(t, "close-long-position", view.OrderType)
	assert.Equal(t, "LONG", view.Side)
	assert.True(t, view.KindKnown)
	assert.Equal(t, ProtectiveStopLoss, view.Kind, "多头 rule 2 = 止损")
	assert.Equal(t, PriceRuleLE, view.TriggerRule)
	assert.InDelta(t, 2077.25, view.TriggerPrice, 1e-12)
	assert.Equal(t, int64(0), view.InitialSize)
	assert.Equal(t, "0.6", view.InitialAmount, "小数张读数在 amount 字段")
	assert.Equal(t, "open", view.Status)

	// IDString 缺省时回退到数字 ID
	order.IdString = ""
	view, err = parseTriggerOrder(order)
	require.NoError(t, err)
	assert.Equal(t, "12345", view.OrderID())

	// 触发价解析失败即报错，不回落猜测值
	for _, bad := range []string{"", "abc", "1e5", "0.1.2"} {
		order.Trigger.Price = bad
		_, err := parseTriggerOrder(order)
		assert.Error(t, err, "trigger price %q", bad)
	}
}

func TestParseOrderID(t *testing.T) {
	id, num, err := parseOrderID("12345")
	require.NoError(t, err)
	assert.Equal(t, "12345", id)
	assert.Equal(t, int64(12345), num)

	// 非规范形式与非法值都拒绝：改单要求路径与请求体字面一致
	for _, bad := range []string{"", "0123", "+5", "abc", "0", "-5", "1.5"} {
		_, _, err := parseOrderID(bad)
		assert.Error(t, err, "order id %q", bad)
	}
}

func TestBuildAmendTriggerBody(t *testing.T) {
	id, body, err := buildAmendTriggerBody("12345", 0.1449)
	require.NoError(t, err)
	assert.Equal(t, "12345", id, "路径参数与请求体的订单 ID 字面一致")
	assert.Equal(t, int64(12345), body.OrderId)
	assert.Equal(t, "0.1449", body.TriggerPrice)
	assert.Empty(t, body.Amount, "只改触发价，不带张数")
	assert.Empty(t, body.Price, "不触碰委托价")
	assert.False(t, body.Close)

	_, _, err = buildAmendTriggerBody("0123", 1)
	assert.Error(t, err, "非规范订单 ID")

	_, _, err = buildAmendTriggerBody("12345", 0)
	assert.Error(t, err, "非正触发价")
}

func TestBuildAmendSizeBody(t *testing.T) {
	id, body, err := buildAmendSizeBody("12345", "0.6")
	require.NoError(t, err)
	assert.Equal(t, "12345", id)
	assert.Equal(t, int64(12345), body.OrderId)
	assert.Equal(t, "0.6", body.Amount, "小数张经 amount 提交（F32）")
	assert.Equal(t, int64(0), body.Size, "size 零值不随请求发送，避免两字段歧义")
	assert.Empty(t, body.TriggerPrice, "只改张数，不触碰触发价")

	_, body, err = buildAmendSizeBody("12345", "-0.6")
	require.NoError(t, err)
	assert.Equal(t, "-0.6", body.Amount, "平多仓为负")

	_, _, err = buildAmendSizeBody("12345", "0")
	assert.Error(t, err, "张数为零")
	_, _, err = buildAmendSizeBody("12345", "abc")
	assert.Error(t, err, "非法张数")
	_, _, err = buildAmendSizeBody("12345", "")
	assert.Error(t, err, "空张数")
}
