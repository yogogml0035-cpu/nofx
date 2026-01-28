package kernel

import (
	"testing"
)

func TestValidateJSONFormat_TildeInReasoning(t *testing.T) {
	// 测试：reasoning字段中包含波浪符号应该被允许
	jsonWithTildeInReasoning := `[{
		"symbol": "BTCUSDT",
		"action": "wait",
		"confidence": 10,
		"reasoning": "15m均线纠缠(EMA8~EMA13~EMA20),属于死水震荡,无明确趋势方向。"
	}]`

	err := validateJSONFormat(jsonWithTildeInReasoning)
	if err != nil {
		t.Errorf("Expected no error for tilde in reasoning field, got: %v", err)
	}
}

func TestValidateJSONFormat_TildeInNumericField(t *testing.T) {
	// 测试：数值字段中包含波浪符号应该被拒绝
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "tilde in leverage",
			json: `[{"symbol": "BTCUSDT", "action": "OPEN_NEW", "leverage": 3~5, "reasoning": "test"}]`,
		},
		{
			name: "tilde in position_size_usd",
			json: `[{"symbol": "BTCUSDT", "action": "OPEN_NEW", "position_size_usd": 100~200, "reasoning": "test"}]`,
		},
		{
			name: "tilde in stop_loss",
			json: `[{"symbol": "BTCUSDT", "action": "OPEN_NEW", "stop_loss": 40000~41000, "reasoning": "test"}]`,
		},
		{
			name: "tilde in confidence",
			json: `[{"symbol": "BTCUSDT", "action": "wait", "confidence": 70~80, "reasoning": "test"}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateJSONFormat(tc.json)
			if err == nil {
				t.Errorf("Expected error for tilde in numeric field, got nil")
			}
		})
	}
}

func TestValidateJSONFormat_ValidJSON(t *testing.T) {
	// 测试：正常的JSON应该通过验证
	validJSON := `[{
		"symbol": "ETHUSDT",
		"action": "OPEN_NEW",
		"leverage": 3,
		"position_size_usd": 500,
		"stop_loss": 2900,
		"take_profit": 3100,
		"confidence": 75,
		"reasoning": "技术突破，持仓量增加，多周期共振"
	}]`

	err := validateJSONFormat(validJSON)
	if err != nil {
		t.Errorf("Expected no error for valid JSON, got: %v", err)
	}
}
