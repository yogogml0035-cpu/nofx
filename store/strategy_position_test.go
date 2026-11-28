package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// ⑧ strategy-config：开仓金额百分比（D23）与周期收敛（D25）。

func TestPositionPercentClampMatrix(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{"missing legacy config defaults to 10", 0, 10},
		{"negative defaults to 10", -5, 10},
		{"below floor clamps to 1", 0.5, 1},
		{"in range keeps step precision", 12.5, 12.5},
		{"floor stays", 1, 1},
		{"ceiling stays", 20, 20},
		{"above ceiling clamps to 20", 25, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := StrategyConfig{}
			cfg.RiskControl.PositionPercent = tt.input
			cfg.ClampLimits()
			if cfg.RiskControl.PositionPercent != tt.want {
				t.Fatalf("PositionPercent = %v, want %v", cfg.RiskControl.PositionPercent, tt.want)
			}
		})
	}
}

func TestDefaultStrategyConfigPositionFields(t *testing.T) {
	cfg := GetDefaultStrategyConfig("zh")
	if cfg.RiskControl.PositionPercent != 10 {
		t.Fatalf("default PositionPercent = %v, want 10", cfg.RiskControl.PositionPercent)
	}

	// 旧配置里的倍数字段与最小下单金额被忽略，不再出现在序列化结果中。
	legacy := `{"risk_control":{"btc_eth_max_position_value_ratio":5,"altcoin_max_position_value_ratio":1,"min_position_size":12}}`
	var parsed StrategyConfig
	if err := json.Unmarshal([]byte(legacy), &parsed); err != nil {
		t.Fatalf("unmarshal legacy config: %v", err)
	}
	parsed.ClampLimits()
	out, err := json.Marshal(parsed.RiskControl)
	if err != nil {
		t.Fatalf("marshal risk control: %v", err)
	}
	for _, gone := range []string{"btc_eth_max_position_value_ratio", "altcoin_max_position_value_ratio", "min_position_size"} {
		if strings.Contains(string(out), gone) {
			t.Fatalf("serialized risk control still contains %q: %s", gone, out)
		}
	}
	if !strings.Contains(string(out), `"position_percent":10`) {
		t.Fatalf("legacy config without percent should default to 10, got: %s", out)
	}
}

// MergeStrategyConfig 对不认识的键静默丢弃；把关函数必须按真实的结构体标签判断，
// 已知字段的零值与自由格式的键不能被误判，调用方的 patch 也不能被改动
func TestFindUnknownStrategyConfigField(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  string
	}{
		{name: "空的 patch", patch: `{}`, want: ""},
		{name: "嵌套结构里的已知字段", patch: `{"ai_config":{"risk_control":{"position_percent":15},"coin_source":{"static_coins":["BTCUSDT"]}}}`, want: ""},
		{name: "顶层的 AI 字段先归一再判断", patch: `{"risk_control":{"position_percent":15}}`, want: ""},
		{name: "已知字段的零值不是未知字段", patch: `{"custom_prompt":""}`, want: ""},
		{name: "自由格式的键不受限制", patch: `{"indicators":{"external_data_sources":[{"name":"x","headers":{"X-Any":"1"}}]}}`, want: ""},
		{name: "网格配置", patch: `{"grid_config":{"grid_count":20}}`, want: ""},
		{name: "已删除的字段", patch: `{"risk_control":{"min_position_size":20}}`, want: "min_position_size"},
		{name: "顶层的未知字段", patch: `{"whatever":1}`, want: "whatever"},
		{name: "更深一层的未知字段", patch: `{"ai_config":{"indicators":{"klines":{"primary_timeframe":"5m","magic":true}}}}`, want: "magic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var patch map[string]any
			if err := json.Unmarshal([]byte(c.patch), &patch); err != nil {
				t.Fatalf("unmarshal patch: %v", err)
			}
			before, _ := json.Marshal(patch)
			got, ok := FindUnknownStrategyConfigField(patch)
			if got != c.want || ok != (c.want != "") {
				t.Fatalf("FindUnknownStrategyConfigField(%s) = %q, %v; want %q", c.patch, got, ok, c.want)
			}
			after, _ := json.Marshal(patch)
			if string(before) != string(after) {
				t.Fatalf("the caller's patch must not be modified: %s -> %s", before, after)
			}
		})
	}
}

func TestNormalizeTimeframeD25(t *testing.T) {
	accepted := map[string]string{
		"10s": "10s", "1m": "1m", "5m": "5m", "15m": "15m", "30m": "30m",
		"1h": "1h", "4h": "4h", "8h": "8h", "1d": "1d", "7d": "7d", "1w": "1w", "30d": "30d",
		"10秒": "10s", "5分钟": "5m", "1小时": "1h", "4小时": "4h",
		"1天": "1d", "7天": "7d", "1周": "1w", "30天": "30d", "1个月": "30d",
		" 15M ": "15m", "1W": "1w",
	}
	for input, want := range accepted {
		if got := normalizeTimeframe(input); got != want {
			t.Errorf("normalizeTimeframe(%q) = %q, want %q", input, got, want)
		}
	}

	// D25 弃用的周期与文档外值归空
	for _, input := range []string{"3m", "2h", "6h", "12h", "3d", "1mo", "15s", "3分钟", "2小时", "6小时", "12小时", "3天"} {
		if got := normalizeTimeframe(input); got != "" {
			t.Errorf("normalizeTimeframe(%q) = %q, want empty", input, got)
		}
	}
}

func TestNormalizeProductSchemaDropsDeprecatedTimeframes(t *testing.T) {
	cfg := StrategyConfig{}
	cfg.Indicators.Klines.PrimaryTimeframe = "12h"
	cfg.Indicators.Klines.SelectedTimeframes = []string{"5m", "12h", "5m", "1mo"}
	cfg.NormalizeProductSchema()
	if cfg.Indicators.Klines.PrimaryTimeframe != "" {
		t.Fatalf("deprecated 12h must normalize to empty, got %q", cfg.Indicators.Klines.PrimaryTimeframe)
	}
	if len(cfg.Indicators.Klines.SelectedTimeframes) != 1 || cfg.Indicators.Klines.SelectedTimeframes[0] != "5m" {
		t.Fatalf("selected timeframes should collapse to [5m], got %v", cfg.Indicators.Klines.SelectedTimeframes)
	}
}

func TestStrategyClampWarningsCoversPositionPercent(t *testing.T) {
	before := GetDefaultStrategyConfig("zh")
	before.RiskControl.PositionPercent = 30
	after := GetDefaultStrategyConfig("zh")
	after.RiskControl.PositionPercent = 30
	after.ClampLimits() // clamps 30 -> 20

	warnings := StrategyClampWarnings(before, after, "zh")
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "开仓金额百分比") && strings.Contains(w, "20.00") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected position percent clamp warning, got %v", warnings)
	}
}

func TestStrategyClampWarningsTimeframes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		before       string
		after        string
		lang         string
		wantContains string
	}{
		{"unsupported cleared zh", "3m", "", "zh", "不在支持范围"},
		{"unsupported cleared en", "3m", "", "en", "is not supported"},
		{"alias normalized zh", "1小时", "1h", "zh", "已从"},
		{"alias normalized en", "1小时", "1h", "en", "adjusted from"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := StrategyConfig{Language: tc.lang}
			before.Indicators.Klines.PrimaryTimeframe = tc.before
			after := before
			after.Indicators.Klines.PrimaryTimeframe = tc.after
			warnings := StrategyClampWarnings(before, after, tc.lang)
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.wantContains) {
				t.Fatalf("expected one timeframe warning containing %q, got %v", tc.wantContains, warnings)
			}
		})
	}

	// 保留集合内的值不产生告警
	before := StrategyConfig{Language: "zh"}
	before.Indicators.Klines.PrimaryTimeframe = "1h"
	after := before
	after.Indicators.Klines.SelectedTimeframes = []string{"5m", "15m"}
	if warnings := StrategyClampWarnings(before, after, "zh"); len(warnings) != 1 || !strings.Contains(warnings[0], "多周期") {
		t.Fatalf("expected only the selected-timeframes warning, got %v", warnings)
	}
	if warnings := StrategyClampWarnings(after, after, "zh"); len(warnings) != 0 {
		t.Fatalf("expected no warnings for an unchanged config, got %v", warnings)
	}
}
