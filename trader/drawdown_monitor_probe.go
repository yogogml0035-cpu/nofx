//go:build livecheck

package trader

// 子任务 drawdown-monitor 的真实验证桥（计划文档 10.2 第 6 行「带仓的只读真实读数」）：
// 把未导出的回撤监控入口暴露给任务 probe/ 目录下的独立脚本。
// 只参与带 livecheck tag 的构建；正常构建、make test、make build 都不编译这个文件
// （探针代码不进构建产物与单元测试，规格 backend/quality-guidelines.md 的探针分离约定）。
// 真实验证不构造 store（无 sqlite）：归属判定的 DB 子句由单元测试覆盖（临时库 + 真实查询），
// 这里的归属走网格子句——网格品种上的持仓视为本 trader 开出的。

import (
	"sync"

	"nofx/store"
	"nofx/trader/types"
)

// NewDrawdownProbeTrader 构造探针用的 AutoTrader：网格子句归属（gridSymbol 上的持仓视为本 trader 的），
// 不带 store（isOwnPosition 的 store 子句不会走到）。
func NewDrawdownProbeTrader(t types.Trader, gridSymbol string) *AutoTrader {
	return &AutoTrader{
		trader: t,
		config: AutoTraderConfig{StrategyConfig: &store.StrategyConfig{
			StrategyType: "grid_trading",
			GridConfig:   &store.GridStrategyConfig{Symbol: gridSymbol},
		}},
		peakPnLCache:      make(map[string]float64),
		peakPnLCacheMutex: sync.RWMutex{},
	}
}

// RunDrawdownMonitorOnce 执行一轮回撤监控（只读一轮：首次见到仓位时峰值 = 当前值、回撤为 0，
// 无论价格如何都不会触发平仓）
func RunDrawdownMonitorOnce(at *AutoTrader) {
	at.checkPositionDrawdown()
}

// ProbeOwnPosition 归属判定探针：真实读数上回答「这个仓位是否视为本 trader 开出的」
func ProbeOwnPosition(at *AutoTrader, symbol, side string) bool {
	return at.isOwnPosition(symbol, side)
}

// ProbePeakCache 峰值缓存读数（键 symbol_side → 峰值收益率百分比）
func ProbePeakCache(at *AutoTrader) map[string]float64 {
	return at.GetPeakPnLCache()
}
