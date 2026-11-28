package gate

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustSpec 构造未设置市价单上限的合约元数据
func mustSpec(t *testing.T, name, multiplier, sizeMin, sizeMax string, enableDecimal bool) ContractSpec {
	t.Helper()
	return mustSpecCapped(t, name, multiplier, sizeMin, sizeMax, "", enableDecimal)
}

func mustSpecCapped(t *testing.T, name, multiplier, sizeMin, sizeMax, marketMax string, enableDecimal bool) ContractSpec {
	t.Helper()
	spec, err := NewContractSpec(name, multiplier, sizeMin, sizeMax, marketMax, "0.01", enableDecimal)
	require.NoError(t, err)
	return spec
}

func TestNewContractSpec(t *testing.T) {
	cases := []struct {
		name          string
		multiplier    string
		sizeMin       string
		sizeMax       string
		marketMax     string
		tick          string
		enableDecimal bool
		wantMarketMax string // 构造成功时 MarketOrderSizeMax 的取值
		wantErr       string // 为空表示应构造成功
	}{
		{name: "整数张合约", multiplier: "0.0001", sizeMin: "1", sizeMax: "12000000", wantMarketMax: "0"},
		{name: "小数张合约", multiplier: "0.01", sizeMin: "0.1", sizeMax: "10000000", enableDecimal: true, wantMarketMax: "0"},
		{name: "小数张合约的整数步长", multiplier: "1", sizeMin: "1", sizeMax: "1000", enableDecimal: true, wantMarketMax: "0"},
		{name: "缺少小数张请求头时步长读成 0", multiplier: "0.01", sizeMin: "0", sizeMax: "10000000", enableDecimal: true, wantErr: "X-Gate-Size-Decimal: 1"},
		{name: "步长为空", multiplier: "0.01", sizeMin: "", sizeMax: "1000", enableDecimal: true, wantErr: `invalid order_size_min ""`},
		{name: "步长不是数字", multiplier: "0.01", sizeMin: "abc", sizeMax: "1000", wantErr: "invalid order_size_min"},
		{name: "步长是科学计数法", multiplier: "0.01", sizeMin: "1e-1", sizeMax: "1000", enableDecimal: true, wantErr: "invalid order_size_min"},
		{name: "步长为负", multiplier: "0.01", sizeMin: "-1", sizeMax: "1000", wantErr: "is not positive"},
		{name: "整数张合约的小数步长", multiplier: "0.0001", sizeMin: "0.1", sizeMax: "1000", wantErr: "supports whole contracts only"},
		{name: "每张币数量为空", multiplier: "", sizeMin: "1", sizeMax: "1000", wantErr: `invalid quanto_multiplier ""`},
		{name: "每张币数量为 0", multiplier: "0", sizeMin: "1", sizeMax: "1000", wantErr: `quanto_multiplier "0" is not positive`},
		{name: "每张币数量为负", multiplier: "-0.01", sizeMin: "1", sizeMax: "1000", wantErr: "is not positive"},
		{name: "最大张数为空", multiplier: "0.01", sizeMin: "1", sizeMax: "", wantErr: `invalid order_size_max ""`},
		{name: "最大张数小于步长", multiplier: "0.01", sizeMin: "1", sizeMax: "0.5", wantErr: "is below order_size_min"},
		{name: "市价单上限小于单笔上限", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "2", wantMarketMax: "2"},
		{name: "市价单上限为 0 时按单笔上限", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "0", wantMarketMax: "0"},
		{name: "市价单上限写作 0.00 时归一成 0", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "0.00", wantMarketMax: "0"},
		{name: "市价单上限大于单笔上限", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "5000", wantMarketMax: "5000"},
		{name: "市价单上限为负", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "-1", wantErr: "is negative"},
		{name: "市价单上限小于最小步长", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "0.5", wantErr: `market_order_size_max "0.5" is below order_size_min`},
		{name: "市价单上限不是数字", multiplier: "0.1", sizeMin: "1", sizeMax: "3", marketMax: "abc", wantErr: "invalid market_order_size_max"},
		{name: "价格步长为空", multiplier: "0.1", sizeMin: "1", sizeMax: "3", tick: " ", wantErr: "invalid order_price_round"},
		{name: "价格步长为零", multiplier: "0.1", sizeMin: "1", sizeMax: "3", tick: "0", wantErr: "is not positive"},
		{name: "价格步长为负", multiplier: "0.1", sizeMin: "1", sizeMax: "3", tick: "-0.01", wantErr: "is not positive"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tick := c.tick
			if tick == "" {
				tick = "0.01"
			}
			spec, err := NewContractSpec("TEST_USDT", c.multiplier, c.sizeMin, c.sizeMax, c.marketMax, tick, c.enableDecimal)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "TEST_USDT", spec.Name)
			assert.Equal(t, c.multiplier, spec.QuantoMultiplier)
			assert.Equal(t, c.sizeMin, spec.SizeStep)
			assert.Equal(t, c.sizeMax, spec.SizeMax)
			assert.Equal(t, c.wantMarketMax, spec.MarketOrderSizeMax)
		})
	}

	_, err := NewContractSpec("", "0.01", "1", "1000", "", "0.01", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contract name is empty")
}

func TestContractSpec_SizeForNotional(t *testing.T) {
	btc := mustSpec(t, "BTC_USDT", "0.0001", "1", "12000000", false)
	eth := mustSpec(t, "ETH_USDT", "0.01", "0.1", "10000000", true)
	// 价格 100 时一张 10U，便于构造最小张数与最大张数的边界；单笔最多 3 张
	ten := mustSpec(t, "TEN_USDT", "0.1", "1", "3", false)
	// 同上，但市价单最多 2 张
	capped := mustSpecCapped(t, "TEN_USDT", "0.1", "1", "3", "2", false)

	cases := []struct {
		name         string
		spec         ContractSpec
		price        float64
		notional     float64
		want         string
		wantNotional float64
		wantSkip     SkipReason
	}{
		{name: "整数张向下取整", spec: btc, price: 60240, notional: 20, want: "3", wantNotional: 18.072},
		{name: "小数张向下取整", spec: eth, price: 1693, notional: 20, want: "1.1", wantNotional: 18.623},
		{name: "取整前不足一个最小步长", spec: btc, price: 60240, notional: 5, wantSkip: SizeSkipBelowStep},
		{name: "恰好等于一个最小步长", spec: ten, price: 100, notional: 10, want: "1", wantNotional: 10},
		// 回归：本金 107.34 × 10% ≈ 10.73 USDT 曾被策略里 12 USDT 的金额下限拦下；
		// 下单量的下限只有合约最小张数，够一张就下单
		{name: "够一张就下单，没有金额下限", spec: btc, price: 60240, notional: 10.73, want: "1", wantNotional: 6.024},
		{name: "未设置市价单上限时按单笔最大张数", spec: ten, price: 100, notional: 100, wantSkip: SizeSkipAboveMax},
		{name: "恰好等于单笔最大张数", spec: ten, price: 100, notional: 30, want: "3", wantNotional: 30},
		{name: "超过市价单最大张数", spec: capped, price: 100, notional: 30, wantSkip: SizeSkipAboveMax},
		{name: "恰好等于市价单最大张数", spec: capped, price: 100, notional: 20, want: "2", wantNotional: 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			quote, err := c.spec.SizeForNotional(c.price, c.notional)
			require.NoError(t, err)
			assert.Equal(t, c.want, quote.Contracts)
			assert.InDelta(t, c.wantNotional, quote.Notional, 1e-9)
			assert.Equal(t, c.wantSkip, quote.Skip)
			assert.Equal(t, c.wantSkip == SizeOK, quote.Orderable())
		})
	}
}

func TestContractSpec_SizeForNotionalAvoidsFloatTruncation(t *testing.T) {
	// 价格 1、每张 0.1 币：一张 0.1U
	unit := mustSpec(t, "UNIT_USDT", "0.1", "1", "1000000", false)
	var price, notional = 1.0, 8.6

	// 浮点除法 8.6 ÷ 0.1 得 85.99999999999999，直接截断会少 1 张
	assert.EqualValues(t, 85, int64(notional/(price*0.1)))

	quote, err := unit.SizeForNotional(price, notional)
	require.NoError(t, err)
	require.True(t, quote.Orderable(), quote.Skip.String())
	assert.Equal(t, "86", quote.Contracts)
	assert.InDelta(t, 8.6, quote.Notional, 1e-12)
}

func TestContractSpec_SizeForNotionalRejectsInvalidInput(t *testing.T) {
	btc := mustSpec(t, "BTC_USDT", "0.0001", "1", "12000000", false)

	for _, price := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		_, err := btc.SizeForNotional(price, 20)
		assert.Error(t, err, "price=%v", price)
	}
	for _, notional := range []float64{0, -5, math.NaN()} {
		_, err := btc.SizeForNotional(60240, notional)
		assert.Error(t, err, "notional=%v", notional)
	}

	// 未经构造函数的零值不能参与换算
	_, err := ContractSpec{}.SizeForNotional(60240, 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

// sampleContract 一个有代表性的合约
type sampleContract struct {
	name          string
	multiplier    string
	sizeMin       string
	sizeMax       string
	enableDecimal bool
	price         float64 // 按表中每张的名义价值反推
	minPercent    float64 // 本金 107.5U 时能下单的最小百分比
}

// sizeCell 一档名义价值下的下单结果：可下单时是张数，否则是不下单的原因
type sizeCell struct {
	contracts string
	skip      SkipReason
}

// sampleContracts 七个有代表性的合约。元数据取自 2026-11-24 的真实读数：
// ETH、SOL 支持小数张，步长 0.1；其余为整数张
var sampleContracts = []sampleContract{
	{name: "BTC_USDT", multiplier: "0.0001", sizeMin: "1", sizeMax: "12000000", price: 60240, minPercent: 5.7},
	{name: "ETH_USDT", multiplier: "0.01", sizeMin: "0.1", sizeMax: "10000000", enableDecimal: true, price: 1693, minPercent: 1.6},
	{name: "SOL_USDT", multiplier: "1", sizeMin: "0.1", sizeMax: "300000", enableDecimal: true, price: 71.16, minPercent: 6.7},
	{name: "ZEC_USDT", multiplier: "0.01", sizeMin: "1", sizeMax: "900000", price: 756, minPercent: 7.1},
	{name: "HYPE_USDT", multiplier: "0.1", sizeMin: "1", sizeMax: "750000", price: 63.1, minPercent: 5.9},
	{name: "DOGE_USDT", multiplier: "10", sizeMin: "1", sizeMax: "12000000", price: 0.05588, minPercent: 0.6},
	{name: "XAU_USDT", multiplier: "0.0001", sizeMin: "1", sizeMax: "30000000", price: 3640, minPercent: 0.4},
}

// 下单量的下限只有合约的最小张数：名义价值够一个最小步长就下单，没有金额下限
func TestContractSpec_SizeForNotionalAcrossContracts(t *testing.T) {
	// 表头六列的名义价值：100U×5%、100U×10%、100U×20%、107.5U×10%、107.5U×20%、500U×10%
	notionals := []float64{5, 10, 20, 10.75, 21.5, 50}
	belowStep := sizeCell{skip: SizeSkipBelowStep}

	want := map[string][]sizeCell{
		"BTC_USDT":  {belowStep, {contracts: "1"}, {contracts: "3"}, {contracts: "1"}, {contracts: "3"}, {contracts: "8"}},
		"ETH_USDT":  {{contracts: "0.2"}, {contracts: "0.5"}, {contracts: "1.1"}, {contracts: "0.6"}, {contracts: "1.2"}, {contracts: "2.9"}},
		"SOL_USDT":  {belowStep, {contracts: "0.1"}, {contracts: "0.2"}, {contracts: "0.1"}, {contracts: "0.3"}, {contracts: "0.7"}},
		"ZEC_USDT":  {belowStep, {contracts: "1"}, {contracts: "2"}, {contracts: "1"}, {contracts: "2"}, {contracts: "6"}},
		"HYPE_USDT": {belowStep, {contracts: "1"}, {contracts: "3"}, {contracts: "1"}, {contracts: "3"}, {contracts: "7"}},
		"DOGE_USDT": {{contracts: "8"}, {contracts: "17"}, {contracts: "35"}, {contracts: "19"}, {contracts: "38"}, {contracts: "89"}},
		"XAU_USDT":  {{contracts: "13"}, {contracts: "27"}, {contracts: "54"}, {contracts: "29"}, {contracts: "59"}, {contracts: "137"}},
	}

	for _, c := range sampleContracts {
		t.Run(c.name, func(t *testing.T) {
			spec := mustSpec(t, c.name, c.multiplier, c.sizeMin, c.sizeMax, c.enableDecimal)
			for i, notional := range notionals {
				quote, err := spec.SizeForNotional(c.price, notional)
				require.NoError(t, err, "notional=%v", notional)
				got := sizeCell{contracts: quote.Contracts, skip: quote.Skip}
				assert.Equal(t, want[c.name][i], got, "notional=%v", notional)
			}
		})
	}
}

func TestContractSpec_MinOrderablePercent(t *testing.T) {
	const principal = 107.5

	for _, c := range sampleContracts {
		t.Run(c.name, func(t *testing.T) {
			spec := mustSpec(t, c.name, c.multiplier, c.sizeMin, c.sizeMax, c.enableDecimal)

			var first float64
			for tenths := 1; tenths <= 200; tenths++ {
				percent := float64(tenths) / 10
				// 名义价值 = 107.5 × 百分比，按十分位逐级放大
				quote, err := spec.SizeForNotional(c.price, principal*float64(tenths)/1000)
				require.NoError(t, err)
				if quote.Orderable() {
					first = percent
					break
				}
			}
			assert.Equal(t, c.minPercent, first)
		})
	}
}

func TestContractSpec_SizeForCloseRatio(t *testing.T) {
	btc := mustSpec(t, "BTC_USDT", "0.0001", "1", "12000000", false)
	eth := mustSpec(t, "ETH_USDT", "0.01", "0.1", "10000000", true)

	cases := []struct {
		name     string
		spec     ContractSpec
		position string
		percent  float64
		want     CloseQuote
	}{
		{name: "整数张分批", spec: btc, position: "3", percent: 50, want: CloseQuote{Contracts: "1", Remaining: "2"}},
		{name: "小数张分批", spec: eth, position: "0.6", percent: 50, want: CloseQuote{Contracts: "0.3", Remaining: "0.3"}},
		{name: "空头持仓按绝对值计算", spec: eth, position: "-0.6", percent: 50, want: CloseQuote{Contracts: "0.3", Remaining: "0.3"}},
		{name: "非整数比例向下取整", spec: eth, position: "0.6", percent: 33.3, want: CloseQuote{Contracts: "0.1", Remaining: "0.5"}},
		{name: "比例 100 为全平", spec: btc, position: "-3", percent: 100, want: CloseQuote{Contracts: "3", Remaining: "0", Full: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.spec.SizeForCloseRatio(c.position, c.percent)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}

	t.Run("换算后不足一个最小步长", func(t *testing.T) {
		_, err := eth.SizeForCloseRatio("0.2", 5)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrCloseBelowStep))
	})
	t.Run("单张持仓无法分批", func(t *testing.T) {
		_, err := btc.SizeForCloseRatio("1", 99)
		assert.True(t, errors.Is(err, ErrCloseBelowStep))
	})

	for _, percent := range []float64{0, -10, 100.1, 150, math.NaN()} {
		_, err := btc.SizeForCloseRatio("3", percent)
		assert.Error(t, err, "percent=%v", percent)
	}
	for _, position := range []string{"0", "", "abc", "0.65", "-0.05"} {
		_, err := eth.SizeForCloseRatio(position, 50)
		assert.Error(t, err, "position=%q", position)
	}
}

func TestContractSpec_SizeForCloseRatioLeavesAtLeastOneStep(t *testing.T) {
	eth := mustSpec(t, "ETH_USDT", "0.01", "0.1", "10000000", true)

	// 比例小于 100 时，只要能分批，剩余至少一个最小步长，也不会被判成全平
	for tenths := 2; tenths <= 30; tenths++ {
		position := strconv.FormatFloat(float64(tenths)/10, 'f', 1, 64)
		for percent := 1; percent < 100; percent++ {
			quote, err := eth.SizeForCloseRatio(position, float64(percent))
			if err != nil {
				require.True(t, errors.Is(err, ErrCloseBelowStep), "position=%s percent=%d err=%v", position, percent, err)
				continue
			}
			require.False(t, quote.Full, "position=%s percent=%d", position, percent)
			remaining, err := strconv.ParseFloat(quote.Remaining, 64)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, remaining, 0.1, "position=%s percent=%d", position, percent)
		}
	}
}

func TestContractSpec_ContractsFromQuantity(t *testing.T) {
	btc := mustSpec(t, "BTC_USDT", "0.0001", "1", "12000000", false)
	eth := mustSpec(t, "ETH_USDT", "0.01", "0.1", "10000000", true)

	t.Run("浮点除法会少算一张的边界", func(t *testing.T) {
		// 0.0003 ÷ 0.0001 的浮点除法得 2.9999999999999996，旧实现截断成 2 张
		var quantity = 0.0003
		assert.EqualValues(t, 2, int64(quantity/0.0001))

		contracts, err := btc.ContractsFromQuantity(quantity)
		require.NoError(t, err)
		assert.Equal(t, "3", contracts)
	})

	// 3 × 0.0001 的浮点乘法得 0.00030000000000000003，换算后是 3.0000000000000003 张。
	// 必须在运行时相乘：写成常量表达式会被编译器精确折叠，不产生误差
	var three = 3.0
	cases := []struct {
		name     string
		spec     ContractSpec
		quantity float64
		want     string
	}{
		{name: "整数张", spec: btc, quantity: 0.0007, want: "7"},
		{name: "小数张", spec: eth, quantity: 0.006, want: "0.6"},
		{name: "带浮点乘法噪声", spec: btc, quantity: three * 0.0001, want: "3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			contracts, err := c.spec.ContractsFromQuantity(c.quantity)
			require.NoError(t, err)
			assert.Equal(t, c.want, contracts)
		})
	}

	for _, quantity := range []float64{0, -0.0003, 0.00004, 0.00005, 0.00035, 0.0012345, math.NaN()} {
		_, err := btc.ContractsFromQuantity(quantity)
		assert.Error(t, err, "quantity=%v", quantity)
	}

	_, err := btc.ContractsFromQuantity(0.00004)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "below one size step")
	_, err = eth.ContractsFromQuantity(0.0065)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a multiple of size step 0.1")

	t.Run("超大持仓的浮点噪声超过绝对容差", func(t *testing.T) {
		// 10240003 × 0.0001 的浮点乘法得 1024.0003000000002，换算后是 10240003.000000002 张：
		// 离步长 2×10⁻⁹ 张，超过绝对容差 10⁻⁹，靠相对容差落回步长
		var contracts = 10240003.0
		quantity := contracts * 0.0001
		require.Equal(t, "1024.0003000000002", strconv.FormatFloat(quantity, 'f', -1, 64))

		got, err := btc.ContractsFromQuantity(quantity)
		require.NoError(t, err)
		assert.Equal(t, "10240003", got)

		// 容差放大后，真正不在步长上的数量仍然报错
		_, err = btc.ContractsFromQuantity(1024.00035)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a multiple of size step 1")
	})
}

func TestContractSpec_QuantityFromContracts(t *testing.T) {
	btc := mustSpec(t, "BTC_USDT", "0.0001", "1", "12000000", false)
	eth := mustSpec(t, "ETH_USDT", "0.01", "0.1", "10000000", true)

	quantity, err := btc.QuantityFromContracts("3")
	require.NoError(t, err)
	assert.InDelta(t, 0.0003, quantity, 1e-15)

	quantity, err = eth.QuantityFromContracts("-0.6")
	require.NoError(t, err)
	assert.InDelta(t, -0.006, quantity, 1e-15)

	for _, contracts := range []string{"", "abc", "0.65"} {
		_, err := eth.QuantityFromContracts(contracts)
		assert.Error(t, err, "contracts=%q", contracts)
	}
}

func TestGateTrader_GetContractSpec(t *testing.T) {
	var requests int32
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		// 最小步长依赖小数张请求头：不带时 ETH 的 order_size_min 返回 0
		if got := r.Header.Get("X-Gate-Size-Decimal"); got != "1" {
			http.Error(w, "missing X-Gate-Size-Decimal: "+got, http.StatusBadRequest)
			return
		}
		if r.URL.Path != "/api/v4/futures/usdt/contracts/ETH_USDT" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]interface{}{
			"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
			"order_size_max": "10000000", "market_order_size_max": "8000000", "enable_decimal": true,
			"order_price_round": "0.01",
		})
	})

	spec, err := gt.GetContractSpec("ETHUSDT")
	require.NoError(t, err)
	assert.Equal(t, "ETH_USDT", spec.Name)
	assert.Equal(t, "0.01", spec.QuantoMultiplier)
	assert.Equal(t, "0.1", spec.SizeStep)
	assert.Equal(t, "10000000", spec.SizeMax)
	assert.Equal(t, "8000000", spec.MarketOrderSizeMax)
	assert.EqualValues(t, 1, atomic.LoadInt32(&requests))

	// 有效期内复用缓存
	_, err = gt.GetContractSpec("ETH_USDT")
	require.NoError(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt32(&requests))

	// 过期后重新读取
	gt.contractSpecs.mu.Lock()
	entry := gt.contractSpecs.entries["ETH_USDT"]
	entry.at = time.Now().Add(-contractSpecCacheTTL - time.Second)
	gt.contractSpecs.entries["ETH_USDT"] = entry
	gt.contractSpecs.mu.Unlock()

	_, err = gt.GetContractSpec("ETHUSDT")
	require.NoError(t, err)
	assert.EqualValues(t, 2, atomic.LoadInt32(&requests))
}

func TestGateTrader_GetContractSpecDoesNotCacheFailures(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var requests int32
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		if fail.Load() {
			http.Error(w, `{"label":"INTERNAL"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]interface{}{
			"name": "BTC_USDT", "quanto_multiplier": "0.0001", "order_size_min": "1",
			"order_size_max": "12000000", "enable_decimal": false,
			"order_price_round": "0.1",
		})
	})

	_, err := gt.GetContractSpec("BTCUSDT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get contract info")

	// 读取失败不缓存：下一次重新请求并成功
	fail.Store(false)
	spec, err := gt.GetContractSpec("BTCUSDT")
	require.NoError(t, err)
	assert.Equal(t, "BTC_USDT", spec.Name)
	assert.Equal(t, "0", spec.MarketOrderSizeMax, "响应未带 market_order_size_max 时按未设置处理")
	assert.EqualValues(t, 2, atomic.LoadInt32(&requests))
}

func TestGateTrader_GetContractSpecRejectsInconsistentMetadata(t *testing.T) {
	var fractional atomic.Bool
	var requests int32
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		sizeMin := "1"
		if fractional.Load() {
			sizeMin = "0" // 支持小数张的合约在缺少请求头时的返回
		}
		writeJSON(w, map[string]interface{}{
			"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": sizeMin,
			"order_size_max": "10000000", "enable_decimal": fractional.Load(),
			"order_price_round": "0.01",
		})
	})

	fractional.Store(true)
	_, err := gt.GetContractSpec("ETHUSDT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "X-Gate-Size-Decimal: 1")
	assert.Contains(t, err.Error(), "failed to read contract info: contract ETH_USDT: order_size_min")

	// 自相矛盾的元数据不缓存：下一次重新请求
	fractional.Store(false)
	_, err = gt.GetContractSpec("ETHUSDT")
	require.NoError(t, err)
	assert.EqualValues(t, 2, atomic.LoadInt32(&requests))
}

func TestGateTrader_GetContractSpecConcurrent(t *testing.T) {
	var requests int32
	gt := newLocalGateTrader(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		writeJSON(w, map[string]interface{}{
			"name": "ETH_USDT", "quanto_multiplier": "0.01", "order_size_min": "0.1",
			"order_size_max": "10000000", "market_order_size_max": "8000000", "enable_decimal": true,
			"order_price_round": "0.01",
		})
	})

	const callers = 64
	results := make([]ContractSpec, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = gt.GetContractSpec("ETHUSDT")
		}(i)
	}
	wg.Wait()

	for i := range results {
		require.NoError(t, errs[i])
		assert.Equal(t, "ETH_USDT", results[i].Name)
		assert.Equal(t, "0.1", results[i].SizeStep)
	}
	// 冷缓存时并发的调用可能各读一次
	assert.GreaterOrEqual(t, atomic.LoadInt32(&requests), int32(1))

	// 缓存填好后不再请求；同一份元数据可以被多个 goroutine 同时拿来换算
	served := atomic.LoadInt32(&requests)
	quotes := make([]SizeQuote, callers)
	for i := range quotes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			quotes[i], errs[i] = results[0].SizeForNotional(1700, 20)
		}(i)
	}
	wg.Wait()
	for i := range quotes {
		require.NoError(t, errs[i])
		assert.Equal(t, "1.1", quotes[i].Contracts)
	}
	_, err := gt.GetContractSpec("ETHUSDT")
	require.NoError(t, err)
	assert.Equal(t, served, atomic.LoadInt32(&requests), "缓存填好后不再请求")
}

func TestContractSpec_AlignTriggerPrice(t *testing.T) {
	// DOGE：tick 0.00001（真实读数）；ETH：tick 0.01
	doge, err := NewContractSpec("DOGE_USDT", "10", "1", "1000000", "0", "0.00001", false)
	require.NoError(t, err)
	ethSpec, err := NewContractSpec("ETH_USDT", "0.01", "0.1", "10000000", "8000000", "0.01", true)
	require.NoError(t, err)

	cases := []struct {
		name    string
		spec    ContractSpec
		price   float64
		dir     PriceAlignDirection
		want    string
		wantErr string
	}{
		{name: "DOGE 向上对齐到 tick", spec: doge, price: 0.048915, dir: PriceAlignUp, want: "0.04892"},
		{name: "DOGE 向下对齐到 tick", spec: doge, price: 0.048915, dir: PriceAlignDown, want: "0.04891"},
		{name: "已经是 tick 整数倍时不变（向上）", spec: doge, price: 0.48, dir: PriceAlignUp, want: "0.48000"},
		{name: "已经是 tick 整数倍时不变（向下）", spec: doge, price: 0.48, dir: PriceAlignDown, want: "0.48000"},
		{name: "ETH 向上对齐两位小数", spec: ethSpec, price: 1700.005, dir: PriceAlignUp, want: "1700.01"},
		{name: "ETH 向下对齐两位小数", spec: ethSpec, price: 1700.005, dir: PriceAlignDown, want: "1700.00"},
		{name: "对齐方向非法", spec: doge, price: 0.48, dir: 0, wantErr: "invalid price align direction"},
		{name: "价格为零", spec: doge, price: 0, dir: PriceAlignDown, wantErr: "must be positive"},
		{name: "价格为负", spec: doge, price: -1, dir: PriceAlignDown, wantErr: "must be positive"},
		{name: "价格向下越过一个 tick", spec: doge, price: 0.000005, dir: PriceAlignDown, wantErr: "below one price step"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, value, err := c.spec.AlignTriggerPrice(c.price, c.dir)
			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, text)
			parsed, err := parseDecimal("aligned", text)
			require.NoError(t, err)
			assert.Zero(t, parsed.Cmp(value), "字符串与精确值一致")
		})
	}
}
