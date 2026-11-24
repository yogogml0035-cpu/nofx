package types

import (
	"fmt"
	"math"
)

// 适配器 GetBalance 返回的映射里的三个规范键。键名沿用各交易所适配器的既有输出，含义只在这里定义。
const (
	BalanceKeyPrincipal     = "totalWalletBalance"    // 本金
	BalanceKeyUnrealizedPnL = "totalUnrealizedProfit" // 未实现盈亏
	BalanceKeyAvailable     = "availableBalance"      // 可用于合约的额度
)

// balanceEquityKeys 适配器直接报告权益时使用的键，按优先级排列（各交易所的写法不一）。
// 不报告权益的适配器（Gate、Binance、OKX、Aster），权益由本金加未实现盈亏得出。
var balanceEquityKeys = [...]string{"totalEquity", "total_equity"}

// AccountBalance 账户读数的规范口径。本金、可用于合约的额度与权益只在这里定义，
// 交易路径、API 与助手不各自按键名取数。
type AccountBalance struct {
	Principal     float64 // 本金：不含未实现盈亏，已实现盈亏计入
	UnrealizedPnL float64 // 未实现盈亏
	Available     float64 // 可用于合约的额度
	Equity        float64 // 权益：适配器报告的值，没有则为 Principal + UnrealizedPnL
}

// InitialBalance 创建或重建 trader 初始资金时记录的读数：当时的本金。
// 初始资金是盈亏统计的基准，不随当时的浮动盈亏变动。
func (b AccountBalance) InitialBalance() float64 {
	return b.Principal
}

// ParseAccountBalance 把适配器 GetBalance 的返回值解析为规范口径。
// 三个规范键缺失或不是有限数值都报错：账户读数决定能否开仓与开多大，不按 0 处理。
func ParseAccountBalance(m map[string]interface{}) (AccountBalance, error) {
	var balance AccountBalance
	var err error
	if balance.Principal, err = balanceField(m, BalanceKeyPrincipal); err != nil {
		return AccountBalance{}, err
	}
	if balance.UnrealizedPnL, err = balanceField(m, BalanceKeyUnrealizedPnL); err != nil {
		return AccountBalance{}, err
	}
	if balance.Available, err = balanceField(m, BalanceKeyAvailable); err != nil {
		return AccountBalance{}, err
	}

	balance.Equity = balance.Principal + balance.UnrealizedPnL
	for _, key := range balanceEquityKeys {
		if reported, ok := m[key].(float64); ok && reported > 0 {
			balance.Equity = reported
			break
		}
	}
	return balance, nil
}

// ReadAccountBalance 读取并解析适配器的账户读数：交易所请求失败、读数不符合规范口径都返回错误。
// 交易路径、API 与助手都经它取数，不各自调用 GetBalance 再解析。
func ReadAccountBalance(t Trader) (AccountBalance, error) {
	raw, err := t.GetBalance()
	if err != nil {
		return AccountBalance{}, fmt.Errorf("failed to get account balance: %w", err)
	}
	balance, err := ParseAccountBalance(raw)
	if err != nil {
		return AccountBalance{}, fmt.Errorf("unreadable account balance: %w", err)
	}
	return balance, nil
}

// ReadInitialBalance 读取交易所账户的当前本金，作为创建或重建 trader 时记录的初始资金。
func ReadInitialBalance(t Trader) (float64, error) {
	balance, err := ReadAccountBalance(t)
	if err != nil {
		return 0, err
	}
	return balance.InitialBalance(), nil
}

func balanceField(m map[string]interface{}, key string) (float64, error) {
	value, ok := m[key].(float64)
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("balance field %q is missing or not a finite number (%T)", key, m[key])
	}
	return value, nil
}
