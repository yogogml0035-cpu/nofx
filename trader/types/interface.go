package types

import (
	"errors"
	"fmt"
	"nofx/logger"
	"time"
)

// ClosedPnLRecord represents a single closed position record from exchange
type ClosedPnLRecord struct {
	Symbol      string    // Trading pair (e.g., "BTCUSDT")
	Side        string    // "long" or "short"
	EntryPrice  float64   // Entry price
	ExitPrice   float64   // Exit/close price
	Quantity    float64   // Position size
	RealizedPnL float64   // Realized profit/loss
	Fee         float64   // Trading fee/commission
	Leverage    int       // Leverage used
	EntryTime   time.Time // Position open time
	ExitTime    time.Time // Position close time
	OrderID     string    // Close order ID
	CloseType   string    // "manual", "stop_loss", "take_profit", "liquidation", "unknown"
	ExchangeID  string    // Exchange-specific position ID
}

// TradeRecord represents a single trade/fill from exchange
// Used for reconstructing position history with unified algorithm
type TradeRecord struct {
	TradeID      string    // Unique trade ID from exchange
	Symbol       string    // Trading pair (e.g., "BTCUSDT")
	Side         string    // "BUY" or "SELL"
	PositionSide string    // "LONG", "SHORT", or "BOTH" (for one-way mode)
	OrderAction  string    // "open_long", "open_short", "close_long", "close_short" (from exchange Dir field)
	Price        float64   // Execution price
	Quantity     float64   // Executed quantity
	RealizedPnL  float64   // Realized PnL (non-zero for closing trades)
	Fee          float64   // Trading fee/commission
	Time         time.Time // Trade execution time
}

// Trader Unified trader interface
// Supports multiple trading platforms (Binance, Bybit, OKX, Gate, etc.)
type Trader interface {
	// GetBalance Get account balance
	GetBalance() (map[string]interface{}, error)

	// GetPositions Get all positions
	GetPositions() ([]map[string]interface{}, error)

	// OpenLong Open long position
	OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// OpenShort Open short position
	OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error)

	// CloseLong Close long position (quantity=0 means close all)
	CloseLong(symbol string, quantity float64) (map[string]interface{}, error)

	// CloseShort Close short position (quantity=0 means close all)
	CloseShort(symbol string, quantity float64) (map[string]interface{}, error)

	// SetLeverage Set leverage
	SetLeverage(symbol string, leverage int) error

	// SetMarginMode Set position mode (true=cross margin, false=isolated margin)
	SetMarginMode(symbol string, isCrossMargin bool) error

	// GetMarketPrice Get market price
	GetMarketPrice(symbol string) (float64, error)

	// SetStopLoss Set stop-loss order
	SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error

	// SetTakeProfit Set take-profit order
	SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error

	// CancelStopLossOrders Cancel only stop-loss orders (BUG fix: don't delete take-profit when adjusting stop-loss)
	CancelStopLossOrders(symbol string) error

	// CancelTakeProfitOrders Cancel only take-profit orders (BUG fix: don't delete stop-loss when adjusting take-profit)
	CancelTakeProfitOrders(symbol string) error

	// CancelAllOrders Cancel all pending orders for this symbol
	CancelAllOrders(symbol string) error

	// CancelStopOrders Cancel stop-loss/take-profit orders for this symbol (for adjusting stop-loss/take-profit positions)
	CancelStopOrders(symbol string) error

	// FormatQuantity Format quantity to correct precision
	FormatQuantity(symbol string, quantity float64) (string, error)

	// GetOrderStatus Get order status
	// Returns: status(FILLED/NEW/CANCELED), avgPrice, executedQty, commission
	GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error)

	// GetClosedPnL Get closed position PnL records from exchange
	// startTime: start time for query (usually last sync time)
	// limit: max number of records to return
	// Returns accurate exit price, fees, and close reason for positions closed externally
	GetClosedPnL(startTime time.Time, limit int) ([]ClosedPnLRecord, error)

	// GetOpenOrders Get open/pending orders from exchange
	// Returns stop-loss, take-profit, and limit orders that haven't been filled
	GetOpenOrders(symbol string) ([]OpenOrder, error)
}

// ProtectedOpenRequest 随单保护开仓的请求：名义价值与保护触发价由调用方（交易路径）决定，
// 张数换算、取价与提交由适配器完成。
type ProtectedOpenRequest struct {
	Symbol      string
	NotionalUSD float64 // 开仓名义价值（USDT）
	Leverage    int
	// IsCrossMargin 全仓为 true、逐仓为 false（trader 界面的设置）；提交订单前与 Leverage 一并生效
	IsCrossMargin bool
	StopLoss      float64 // 止损触发价
	TakeProfit    float64 // 止盈触发价
	MinNotional   float64 // 最小下单金额（USDT）；0 表示不设下限
	SlipRatio     float64 // 市价滑点上限（0.01 = 1%）；0 表示用交易所默认
}

// ProtectedOpenResult 随单保护开仓的返回：下单张数与实际成交值用于记录与展示。
type ProtectedOpenResult struct {
	OrderID   string
	Contracts string  // 下单张数（十进制字符串）
	Notional  float64 // 张数 × 价格的实际名义价值（USDT）
	FillPrice float64
	Quantity  float64 // 币数量（张数 × 每张币数量，展示与记账用）
	// StopLoss / TakeProfit 对齐到合约价格步长后的实际触发价（交易所接受的值）
	StopLoss   float64
	TakeProfit float64
}

// MarginSettingsApplier 按 trader 的设置一次性生效仓位的保证金模式与杠杆。
// 保证金模式与杠杆在交易所上是一次设置，分开设置时各缺一半信息（模式不知道杠杆、
// 杠杆不知道模式）。自动交易在开仓与启动网格前调用它，返回错误即不交易。
type MarginSettingsApplier interface {
	// ApplyMarginSettings 生效全仓（isCrossMargin 为 true）或逐仓与杠杆，并读回核对；
	// 交易所拒绝或读回与设置不一致时返回错误。
	ApplyMarginSettings(symbol string, isCrossMargin bool, leverage int) error
}

// ProtectedOpener 随开仓单一并提交止损止盈的开仓能力（保护不变量：仓位从存在那一刻起
// 就带有止损与止盈，成交后核对保护单存在，缺失即平仓）。实现该接口的适配器承载
// 取价与张数换算，并在提交订单前按请求里的保证金模式与杠杆生效仓位的设置；
// 自动交易的开仓路径要求适配器实现它，未实现的适配器不能开仓。
type ProtectedOpener interface {
	// OpenLongProtected 随单保护开多：换算张数并随单提交止损止盈。
	OpenLongProtected(req ProtectedOpenRequest) (ProtectedOpenResult, error)

	// OpenShortProtected 随单保护开空：换算张数并随单提交止损止盈。
	OpenShortProtected(req ProtectedOpenRequest) (ProtectedOpenResult, error)

	// VerifyOrderProtection 开仓成交后核对保护单存在；缺失返回错误（调用方立即平仓）。
	VerifyOrderProtection(symbol, orderID string) error
}

// PartialCloseStatus 描述一个持仓能否分批平仓。Supported 为 true 当且仅当交易所可精确
// 读到持仓张数与最小步长，且持仓张数严格大于一个最小步长（至少两步）。只有一步的仓位
// 任何一个小于 100% 的比例都会向下取整为 0 步，无法分批。
type PartialCloseStatus struct {
	Supported bool   // 能否分批
	Reason    string // 不能分批的原因；Supported 为 false 时非空
	Contracts string // 当前持仓张数（不带符号）
	SizeStep  string // 最小步长（张）
}

// PartialCloseRequest 分批平仓请求：比例由模型给出，张数换算与下单由适配器完成。
// Percent 取 (0, 100)；100% 是全平，必须走全平动作，适配器拒绝。
type PartialCloseRequest struct {
	Symbol  string
	Side    string  // "long" / "short"
	Percent float64 // (0, 100)
}

// PartialCloseResult 分批平仓的成交结果。
type PartialCloseResult struct {
	OrderID   string
	Contracts string  // 本次平掉的张数（不带符号）
	Remaining string  // 平后的剩余张数
	FillPrice float64 // 成交价
	Quantity  float64 // 平掉的基础币数量（展示与记账）
	Notional  float64 // Quantity × FillPrice
}

// PartialCloser 精确分批平仓能力。实现该接口的适配器承载持仓读取、张数换算与
// reduce-only 市价单提交；自动交易的分批平仓路径要求适配器实现它，未实现的适配器
// 在发单前被拒绝（fail-closed，不回落全平）。
type PartialCloser interface {
	// PartialCloseStatus 报告该持仓能否分批；读取失败返回错误。
	PartialCloseStatus(symbol, side string) (PartialCloseStatus, error)

	// ClosePartial 按比例平掉部分持仓；比例换算后不足一个最小步长时报错。
	ClosePartial(req PartialCloseRequest) (PartialCloseResult, error)
}

// ProtectionKind 保护单类型：止损或止盈
type ProtectionKind string

const (
	ProtectionStopLoss   ProtectionKind = "stop_loss"
	ProtectionTakeProfit ProtectionKind = "take_profit"
)

// PositionProtection 当前持仓的止损止盈读取结果。来源是交易所上该合约未触发的条件单：
// 同类多条并存时取距现价最近（最先触发）的一条作为有效值，计数给出实际条数。
// 止损与止盈都可读时 Known 为 true；否则模型不得修改该持仓的保护（旧保护无法保证被替换）。
type PositionProtection struct {
	StopLoss         float64 // 有效止损触发价；0 = 无
	TakeProfit       float64 // 有效止盈触发价；0 = 无
	StopLossOrders   int     // 同侧止损条件单条数
	TakeProfitOrders int     // 同侧止盈条件单条数
	Known            bool    // 止损与止盈都可读
	Reason           string  // Known=false 的原因（英文，给日志与提示词）
}

// ProtectionUpdateRequest 修改止损或止盈的请求。新价格由模型给出，适配器负责对齐价格步长、
// 挂新单、核对、撤销旧单；Principal 与 MaxLossPercent 用于执行时的亏损上限复核（止损）。
type ProtectionUpdateRequest struct {
	Symbol         string
	Side           string         // "long" / "short"
	Kind           ProtectionKind // 止损或止盈
	Price          float64        // 新的触发价（未对齐）
	Principal      float64        // 本金（D4 口径）
	MaxLossPercent float64        // 止损时的亏损上限（百分点）；止盈忽略
}

// ProtectionUpdateResult 改单结果：新保护单与它替换掉的旧保护单。
type ProtectionUpdateResult struct {
	OrderID          string // 新保护单 ID
	Kind             ProtectionKind
	OldPrice         float64  // 替换前生效的触发价；0 = 原先没有该类型的保护单
	NewPrice         float64  // 对齐后的实际触发价
	ReplacedOrderIDs []string // 被撤销的旧保护单 ID
}

// ProtectiveOrderManager 止损止盈管理能力：每轮读取持仓当前的止损止盈，按需撤旧建新。
// 自动交易的修改动作要求适配器实现它，未实现的适配器在发请求前被拒绝（fail-closed）。
type ProtectiveOrderManager interface {
	// PositionProtection 读取该持仓当前的止损止盈；读取失败返回错误。
	PositionProtection(symbol, side string) (PositionProtection, error)

	// UpdateProtection 把止盈或止损改到新价格：先挂新单并核对，再撤销旧单并核对；
	// 失败时恢复原保护，不留下无保护的持仓。
	UpdateProtection(req ProtectionUpdateRequest) (ProtectionUpdateResult, error)
}

// ErrPositionUnprotected 改单或恢复过程中保护单已无法确认存在：调用方必须立即平仓
// （保护不变量：持仓从存在那一刻起必须带有止损与止盈）。
var ErrPositionUnprotected = errors.New("position is not protected")

// OpenOrder represents a pending order on the exchange
type OpenOrder struct {
	OrderID      string  `json:"order_id"`
	Symbol       string  `json:"symbol"`
	Side         string  `json:"side"`          // BUY/SELL
	PositionSide string  `json:"position_side"` // LONG/SHORT
	Type         string  `json:"type"`          // LIMIT/STOP_MARKET/TAKE_PROFIT_MARKET
	Price        float64 `json:"price"`         // Order price (for limit orders)
	StopPrice    float64 `json:"stop_price"`    // Trigger price (for stop orders)
	Quantity     float64 `json:"quantity"`
	Status       string  `json:"status"` // NEW
}

// LimitOrderRequest represents a limit order request for grid trading
type LimitOrderRequest struct {
	Symbol       string  `json:"symbol"`
	Side         string  `json:"side"`          // BUY/SELL
	PositionSide string  `json:"position_side"` // LONG/SHORT (for hedge mode)
	Price        float64 `json:"price"`         // Limit price
	Quantity     float64 `json:"quantity"`
	Leverage     int     `json:"leverage"`
	PostOnly     bool    `json:"post_only"`   // Maker only order
	ReduceOnly   bool    `json:"reduce_only"` // Reduce position only
	ClientID     string  `json:"client_id"`   // Client order ID for tracking
}

// LimitOrderResult represents the result of placing a limit order
type LimitOrderResult struct {
	OrderID      string  `json:"order_id"`
	ClientID     string  `json:"client_id"`
	Symbol       string  `json:"symbol"`
	Side         string  `json:"side"`
	PositionSide string  `json:"position_side"`
	Price        float64 `json:"price"`
	Quantity     float64 `json:"quantity"`
	Status       string  `json:"status"` // NEW, PARTIALLY_FILLED, FILLED, CANCELED
}

// GridTrader extends Trader interface with limit order support for grid trading
// Exchanges that support grid trading should implement this interface
type GridTrader interface {
	Trader

	// PlaceLimitOrder places a limit order at specified price
	// Returns order ID and status
	PlaceLimitOrder(req *LimitOrderRequest) (*LimitOrderResult, error)

	// CancelOrder cancels a specific order by ID
	CancelOrder(symbol, orderID string) error

	// GetOrderBook gets current order book (for price validation)
	// Returns best bid/ask prices
	GetOrderBook(symbol string, depth int) (bids, asks [][]float64, err error)
}

// GridTraderAdapter wraps a basic Trader to provide GridTrader interface
// Uses stop orders as a fallback when limit orders aren't directly available
type GridTraderAdapter struct {
	Trader
}

// NewGridTraderAdapter creates an adapter for basic Trader
func NewGridTraderAdapter(t Trader) *GridTraderAdapter {
	return &GridTraderAdapter{Trader: t}
}

// PlaceLimitOrder implements limit order using available methods
// For exchanges without native limit order support, this uses conditional orders
func (a *GridTraderAdapter) PlaceLimitOrder(req *LimitOrderRequest) (*LimitOrderResult, error) {
	// CRITICAL FIX: Set leverage before placing order
	if req.Leverage > 0 {
		if err := a.Trader.SetLeverage(req.Symbol, req.Leverage); err != nil {
			logger.Warnf("[Grid] Failed to set leverage %dx: %v", req.Leverage, err)
			// Continue anyway - some exchanges don't require explicit leverage setting
		}
	}

	// Use SetStopLoss/SetTakeProfit as conditional limit orders
	// For buy orders below current price, use stop-loss mechanism
	// For sell orders above current price, use take-profit mechanism
	var err error
	if req.Side == "BUY" {
		err = a.Trader.SetStopLoss(req.Symbol, "SHORT", req.Quantity, req.Price)
	} else {
		err = a.Trader.SetTakeProfit(req.Symbol, "LONG", req.Quantity, req.Price)
	}
	if err != nil {
		return nil, err
	}
	return &LimitOrderResult{
		OrderID:      req.ClientID,
		ClientID:     req.ClientID,
		Symbol:       req.Symbol,
		Side:         req.Side,
		PositionSide: req.PositionSide,
		Price:        req.Price,
		Quantity:     req.Quantity,
		Status:       "NEW",
	}, nil
}

// CancelOrder cancels a specific order
func (a *GridTraderAdapter) CancelOrder(symbol, orderID string) error {
	// Try to use CancelOrder if trader supports it directly
	if canceler, ok := a.Trader.(interface {
		CancelOrder(symbol, orderID string) error
	}); ok {
		return canceler.CancelOrder(symbol, orderID)
	}

	// For traders that only support CancelAllOrders, log a warning
	// This is a limitation - we cannot cancel individual orders
	logger.Warnf("[Grid] Trader does not support individual order cancellation, "+
		"cannot cancel order %s. Consider using exchange-specific GridTrader implementation.", orderID)

	// Return error instead of canceling all orders
	return fmt.Errorf("individual order cancellation not supported for this exchange")
}

// GetOrderBook returns empty order book (not supported in basic Trader)
func (a *GridTraderAdapter) GetOrderBook(symbol string, depth int) (bids, asks [][]float64, err error) {
	// Not supported, return empty
	return nil, nil, nil
}
