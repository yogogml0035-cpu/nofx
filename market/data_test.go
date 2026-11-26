package market

import (
	"math"
	"testing"
)

// generateTestKlines generates test K-line data
func generateTestKlines(count int) []Kline {
	klines := make([]Kline, count)
	for i := 0; i < count; i++ {
		// Generate simulated price data with some fluctuation
		basePrice := 100.0
		variance := float64(i%10) * 0.5
		open := basePrice + variance
		high := open + 1.0
		low := open - 0.5
		close := open + 0.3
		volume := 1000.0 + float64(i*100)

		klines[i] = Kline{
			OpenTime:  int64(i * 180000), // 5-minute interval
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
			CloseTime: int64((i+1)*180000 - 1),
		}
	}
	return klines
}

// TestCalculateATR tests ATR calculation function
func TestCalculateATR(t *testing.T) {
	tests := []struct {
		name       string
		klines     []Kline
		period     int
		expectZero bool
	}{
		{
			name: "Normal calculation - sufficient data",
			klines: []Kline{
				{High: 102.0, Low: 100.0, Close: 101.0},
				{High: 103.0, Low: 101.0, Close: 102.0},
				{High: 104.0, Low: 102.0, Close: 103.0},
				{High: 105.0, Low: 103.0, Close: 104.0},
				{High: 106.0, Low: 104.0, Close: 105.0},
				{High: 107.0, Low: 105.0, Close: 106.0},
				{High: 108.0, Low: 106.0, Close: 107.0},
				{High: 109.0, Low: 107.0, Close: 108.0},
				{High: 110.0, Low: 108.0, Close: 109.0},
				{High: 111.0, Low: 109.0, Close: 110.0},
				{High: 112.0, Low: 110.0, Close: 111.0},
				{High: 113.0, Low: 111.0, Close: 112.0},
				{High: 114.0, Low: 112.0, Close: 113.0},
				{High: 115.0, Low: 113.0, Close: 114.0},
				{High: 116.0, Low: 114.0, Close: 115.0},
			},
			period:     14,
			expectZero: false,
		},
		{
			name: "Insufficient data - equal to period",
			klines: []Kline{
				{High: 102.0, Low: 100.0, Close: 101.0},
				{High: 103.0, Low: 101.0, Close: 102.0},
			},
			period:     2,
			expectZero: true,
		},
		{
			name: "Insufficient data - less than period",
			klines: []Kline{
				{High: 102.0, Low: 100.0, Close: 101.0},
			},
			period:     14,
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atr := calculateATR(tt.klines, tt.period)

			if tt.expectZero {
				if atr != 0 {
					t.Errorf("calculateATR() = %.3f, expected 0 (insufficient data)", atr)
				}
			} else {
				if atr <= 0 {
					t.Errorf("calculateATR() = %.3f, expected > 0", atr)
				}
			}
		})
	}
}

// TestCalculateATR_TrueRange tests ATR True Range calculation correctness
func TestCalculateATR_TrueRange(t *testing.T) {
	// Create a simple test case, manually calculate expected ATR
	klines := []Kline{
		{High: 50.0, Low: 48.0, Close: 49.0}, // TR = 2.0
		{High: 51.0, Low: 49.0, Close: 50.0}, // TR = max(2.0, 2.0, 1.0) = 2.0
		{High: 52.0, Low: 50.0, Close: 51.0}, // TR = max(2.0, 2.0, 1.0) = 2.0
		{High: 53.0, Low: 51.0, Close: 52.0}, // TR = 2.0
		{High: 54.0, Low: 52.0, Close: 53.0}, // TR = 2.0
	}

	atr := calculateATR(klines, 3)

	// Expected calculation:
	// TR[1] = max(51-49, |51-49|, |49-49|) = 2.0
	// TR[2] = max(52-50, |52-50|, |50-50|) = 2.0
	// TR[3] = max(53-51, |53-51|, |51-51|) = 2.0
	// Initial ATR = (2.0 + 2.0 + 2.0) / 3 = 2.0
	// TR[4] = max(54-52, |54-52|, |52-52|) = 2.0
	// Smoothed ATR = (2.0*2 + 2.0) / 3 = 2.0

	expectedATR := 2.0
	tolerance := 0.01 // Allow small floating point error

	if math.Abs(atr-expectedATR) > tolerance {
		t.Errorf("calculateATR() = %.3f, want approximately %.3f", atr, expectedATR)
	}
}

// TestIsStaleData_NormalData tests that normal fluctuating data returns false
func TestIsStaleData_NormalData(t *testing.T) {
	klines := []Kline{
		{Close: 100.0, Volume: 1000},
		{Close: 100.5, Volume: 1200},
		{Close: 99.8, Volume: 900},
		{Close: 100.2, Volume: 1100},
		{Close: 100.1, Volume: 950},
	}

	result := isStaleData(klines, "BTCUSDT")

	if result {
		t.Error("Expected false for normal fluctuating data, got true")
	}
}

// TestIsStaleData_PriceFreezeWithZeroVolume tests that frozen price + zero volume returns true
func TestIsStaleData_PriceFreezeWithZeroVolume(t *testing.T) {
	klines := []Kline{
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
	}

	result := isStaleData(klines, "DOGEUSDT")

	if !result {
		t.Error("Expected true for frozen price + zero volume, got false")
	}
}

// TestIsStaleData_PriceFreezeWithVolume tests that frozen price but normal volume returns false
func TestIsStaleData_PriceFreezeWithVolume(t *testing.T) {
	klines := []Kline{
		{Close: 100.0, Volume: 1000},
		{Close: 100.0, Volume: 1200},
		{Close: 100.0, Volume: 900},
		{Close: 100.0, Volume: 1100},
		{Close: 100.0, Volume: 950},
	}

	result := isStaleData(klines, "STABLECOIN")

	if result {
		t.Error("Expected false for frozen price but normal volume (low volatility market), got true")
	}
}

// TestIsStaleData_InsufficientData tests that insufficient data (<5 klines) returns false
func TestIsStaleData_InsufficientData(t *testing.T) {
	klines := []Kline{
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
	}

	result := isStaleData(klines, "BTCUSDT")

	if result {
		t.Error("Expected false for insufficient data (<5 klines), got true")
	}
}

// TestIsStaleData_ExactlyFiveKlines tests edge case with exactly 5 klines
func TestIsStaleData_ExactlyFiveKlines(t *testing.T) {
	// Stale case: exactly 5 frozen klines with zero volume
	staleKlines := []Kline{
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
		{Close: 100.0, Volume: 0},
	}

	result := isStaleData(staleKlines, "TESTUSDT")
	if !result {
		t.Error("Expected true for exactly 5 frozen klines with zero volume, got false")
	}

	// Normal case: exactly 5 klines with fluctuation
	normalKlines := []Kline{
		{Close: 100.0, Volume: 1000},
		{Close: 100.1, Volume: 1100},
		{Close: 99.9, Volume: 900},
		{Close: 100.0, Volume: 1000},
		{Close: 100.05, Volume: 950},
	}

	result = isStaleData(normalKlines, "TESTUSDT")
	if result {
		t.Error("Expected false for exactly 5 normal klines, got true")
	}
}

// TestIsStaleData_WithinTolerance tests price changes within tolerance (0.01%)
func TestIsStaleData_WithinTolerance(t *testing.T) {
	// Price changes within 0.01% tolerance should be treated as frozen
	basePrice := 10000.0
	tolerance := 0.0001                        // 0.01%
	smallChange := basePrice * tolerance * 0.5 // Half of tolerance

	klines := []Kline{
		{Close: basePrice, Volume: 1000},
		{Close: basePrice + smallChange, Volume: 1000},
		{Close: basePrice - smallChange, Volume: 1000},
		{Close: basePrice, Volume: 1000},
		{Close: basePrice + smallChange, Volume: 1000},
	}

	result := isStaleData(klines, "BTCUSDT")

	// Should return false because there's normal volume despite tiny price changes
	if result {
		t.Error("Expected false for price within tolerance but with volume, got true")
	}
}

// TestIsStaleData_MixedScenario tests realistic scenario with some history before freeze
func TestIsStaleData_MixedScenario(t *testing.T) {
	// Simulate: normal trading → suddenly freezes
	klines := []Kline{
		{Close: 100.0, Volume: 1000}, // Normal
		{Close: 100.5, Volume: 1200}, // Normal
		{Close: 100.2, Volume: 1100}, // Normal
		{Close: 50.0, Volume: 0},     // Freeze starts
		{Close: 50.0, Volume: 0},     // Frozen
		{Close: 50.0, Volume: 0},     // Frozen
		{Close: 50.0, Volume: 0},     // Frozen
		{Close: 50.0, Volume: 0},     // Frozen (last 5 are all frozen)
	}

	result := isStaleData(klines, "DOGEUSDT")

	// Should detect stale data based on last 5 klines
	if !result {
		t.Error("Expected true for frozen last 5 klines with zero volume, got false")
	}
}

// TestIsStaleData_EmptyKlines tests edge case with empty slice
func TestIsStaleData_EmptyKlines(t *testing.T) {
	klines := []Kline{}

	result := isStaleData(klines, "BTCUSDT")

	if result {
		t.Error("Expected false for empty klines, got true")
	}
}

func TestCalculateDonchian(t *testing.T) {
	// Create test klines with known high/low values
	klines := []Kline{
		{High: 100, Low: 90},
		{High: 105, Low: 88},
		{High: 102, Low: 92},
		{High: 108, Low: 85},
		{High: 103, Low: 91},
	}

	upper, lower := ExportCalculateDonchian(klines, 5)

	if upper != 108 {
		t.Errorf("Expected upper = 108, got %v", upper)
	}
	if lower != 85 {
		t.Errorf("Expected lower = 85, got %v", lower)
	}
}

func TestCalculateDonchian_PartialPeriod(t *testing.T) {
	klines := []Kline{
		{High: 100, Low: 90},
		{High: 105, Low: 88},
	}

	upper, lower := ExportCalculateDonchian(klines, 10)

	// Should use all available klines when period > len(klines)
	if upper != 105 {
		t.Errorf("Expected upper = 105, got %v", upper)
	}
	if lower != 88 {
		t.Errorf("Expected lower = 88, got %v", lower)
	}
}

func TestCalculateDonchian_InvalidPeriod(t *testing.T) {
	klines := []Kline{
		{High: 100, Low: 90},
	}

	// Zero period should return (0, 0)
	upper, lower := ExportCalculateDonchian(klines, 0)
	if upper != 0 || lower != 0 {
		t.Errorf("Expected (0, 0) for zero period, got (%v, %v)", upper, lower)
	}

	// Negative period should return (0, 0)
	upper, lower = ExportCalculateDonchian(klines, -1)
	if upper != 0 || lower != 0 {
		t.Errorf("Expected (0, 0) for negative period, got (%v, %v)", upper, lower)
	}
}

func TestCalculateBoxData(t *testing.T) {
	// Create synthetic kline data
	klines := make([]Kline, 500)
	for i := 0; i < 500; i++ {
		basePrice := 100.0
		klines[i] = Kline{
			High:  basePrice + float64(i%10),
			Low:   basePrice - float64(i%10),
			Close: basePrice,
		}
	}

	box := ExportCalculateBoxData(klines, 100.0)

	if box.ShortUpper == 0 || box.ShortLower == 0 {
		t.Error("Short box should not be zero")
	}
	if box.MidUpper == 0 || box.MidLower == 0 {
		t.Error("Mid box should not be zero")
	}
	if box.LongUpper == 0 || box.LongLower == 0 {
		t.Error("Long box should not be zero")
	}
	if box.CurrentPrice != 100.0 {
		t.Errorf("Expected CurrentPrice = 100.0, got %v", box.CurrentPrice)
	}
}

// 短周期（10s）下陈旧判定窗口至少 15 分钟：30 根（5 分钟）全部冻结不足以判陈旧
func TestIsStaleData_ShortTimeframeUsesFifteenMinuteWindow(t *testing.T) {
	short := make([]Kline, 30)
	for i := range short {
		short[i] = Kline{OpenTime: int64(1700000000000 + i*10000), Close: 100, Volume: 0}
	}
	if isStaleData(short, "DOGEUSDT") {
		t.Error("30 根 10s（5 分钟）冻结不应判为陈旧")
	}

	long := make([]Kline, 100)
	for i := range long {
		long[i] = Kline{OpenTime: int64(1700000000000 + i*10000), Close: 100, Volume: 0}
	}
	if !isStaleData(long, "DOGEUSDT") {
		t.Error("100 根 10s（约 16.7 分钟）冻结 + 零成交应判为陈旧")
	}
}

// 1m 周期的窗口是 15 根：14 根冻结不判、15 根冻结判
func TestIsStaleData_MinuteTimeframeWindow(t *testing.T) {
	build := func(n int) []Kline {
		klines := make([]Kline, n)
		for i := range klines {
			klines[i] = Kline{OpenTime: int64(1700000000000 + i*60000), Close: 100, Volume: 0}
		}
		return klines
	}
	if isStaleData(build(14), "DOGEUSDT") {
		t.Error("14 根 1m 不应判为陈旧")
	}
	if !isStaleData(build(15), "DOGEUSDT") {
		t.Error("15 根 1m 冻结 + 零成交应判为陈旧")
	}
}
