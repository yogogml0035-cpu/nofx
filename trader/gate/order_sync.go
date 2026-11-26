package gate

import (
	"fmt"
	"math/big"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"nofx/trader/syncloop"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antihax/optional"
	"github.com/gate/gateapi-go/v7"
)

// GateTrade represents a trade record from Gate fill history
type GateTrade struct {
	Symbol      string
	TradeID     string
	OrderID     string
	Side        string // buy or sell
	FillPrice   float64
	FillQty     float64 // In base currency (e.g., ETH), not contracts
	Fee         float64
	FeeAsset    string
	ExecTime    time.Time
	ProfitLoss  float64
	OrderType   string
	OrderAction string // open_long, open_short, close_long, close_short
	OrderText   string // 订单备注原样带回（下单入口的区分依据，见 order_texts.go）
}

// GetTrades retrieves trade/fill records from Gate
func (t *GateTrader) GetTrades(startTime time.Time, limit int) ([]GateTrade, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		limit = 100 // Gate max limit
	}

	opts := &gateapi.GetMyTradesOpts{
		Limit: optional.NewInt32(int32(limit)),
	}

	// Get trades from Gate API
	trades, _, err := t.client.FuturesApi.GetMyTrades(t.ctx, "usdt", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}

	logger.Infof("📥 Received %d trades from Gate", len(trades))

	result := make([]GateTrade, 0, len(trades))

	for _, trade := range trades {
		// Filter by start time
		createTime := int64(trade.CreateTime)
		if createTime < startTime.Unix() {
			continue
		}

		fillPrice, err := strconv.ParseFloat(trade.Price, 64)
		if err != nil || fillPrice == 0 {
			logger.Infof("⚠️  Gate trade %d: fillPrice parse issue - raw='%s' parsed=%.8f err=%v",
				trade.Id, trade.Price, fillPrice, err)
		}

		// 张数换算为标的币数量：元数据经 GetContractSpec 精确解析，失败即报错
		spec, err := t.GetContractSpec(trade.Contract)
		if err != nil {
			return nil, fmt.Errorf("failed to get contract info for %s: %w", trade.Contract, err)
		}

		// Convert contract size to actual quantity
		size, err := parseRequiredSize("trade size", trade.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to parse trade %d: %w", trade.Id, err)
		}
		closeSize, err := parseOptionalSize("trade close_size", trade.CloseSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse trade %d: %w", trade.Id, err)
		}
		sizeRat, err := parseDecimal("trade size", trade.Size)
		if err != nil {
			return nil, fmt.Errorf("failed to parse trade %d: %w", trade.Id, err)
		}
		fillQty := spec.contractQuantity(new(big.Rat).Abs(sizeRat))

		// Determine side and order action based on size and close_size
		// Gate close_size field determines if trade is opening or closing:
		// close_size=0 && size>0: Open long
		// close_size=0 && size<0: Open short
		// close_size>0 && size>0: Close short (and possibly open long if size > close_size)
		// close_size<0 && size<0: Close long (and possibly open short if |size| > |close_size|)
		side := "BUY"
		orderAction := "open_long"

		if size > 0 {
			side = "BUY"
			if closeSize > 0 {
				// Closing short position
				orderAction = "close_short"
			} else {
				// Opening long position
				orderAction = "open_long"
			}
		} else if size < 0 {
			side = "SELL"
			if closeSize < 0 {
				// Closing long position
				orderAction = "close_long"
			} else {
				// Opening short position
				orderAction = "open_short"
			}
		}

		// Calculate fee (Gate returns fee as negative value)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)
		if fee < 0 {
			fee = -fee
		}

		// For closed positions, estimate PnL (Gate doesn't directly provide it in trade record)
		pnl := 0.0
		if strings.Contains(orderAction, "close") {
			// PnL would need to be calculated from position history
			// For now, we leave it as 0 and let position builder handle it
		}

		gateTrade := GateTrade{
			Symbol:      trade.Contract,
			TradeID:     fmt.Sprintf("%d", trade.Id),
			OrderID:     trade.OrderId,
			Side:        side,
			FillPrice:   fillPrice,
			FillQty:     fillQty,
			Fee:         fee,
			FeeAsset:    "USDT",
			ExecTime:    time.Unix(createTime, 0).UTC(),
			ProfitLoss:  pnl,
			OrderType:   "MARKET",
			OrderAction: orderAction,
			OrderText:   trade.Text,
		}

		result = append(result, gateTrade)
	}

	return result, nil
}

// SyncOrdersFromGate syncs Gate exchange order history to local database
// Also creates/updates position records to ensure orders/fills/positions data consistency
// exchangeID: Exchange account UUID (from exchanges.id)
// exchangeType: Exchange type ("gate")
func (t *GateTrader) SyncOrdersFromGate(traderID string, exchangeID string, exchangeType string, st *store.Store) error {
	if st == nil {
		return fmt.Errorf("store is nil")
	}

	// Get recent trades (last 24 hours)
	startTime := time.Now().Add(-24 * time.Hour)

	logger.Infof("🔄 Syncing Gate trades from: %s", startTime.Format(time.RFC3339))

	// Use GetTrades method to fetch trade records
	trades, err := t.GetTrades(startTime, 100)
	if err != nil {
		return fmt.Errorf("failed to get trades: %w", err)
	}

	logger.Infof("📥 Received %d trades from Gate", len(trades))

	// Sort trades by time ASC (oldest first) for proper position building
	sort.Slice(trades, func(i, j int) bool {
		return trades[i].ExecTime.UnixMilli() < trades[j].ExecTime.UnixMilli()
	})

	// Process trades one by one (no transaction to avoid deadlock)
	orderStore := st.Order()
	positionStore := st.Position()
	posBuilder := store.NewPositionBuilder(positionStore)
	syncedCount := 0

	for _, trade := range trades {
		// Normalize symbol (Gate uses BTC_USDT, normalize to BTCUSDT)
		symbol := market.Normalize(strings.ReplaceAll(trade.Symbol, "_", ""))

		// Determine position side from order action
		positionSide := "LONG"
		if strings.Contains(trade.OrderAction, "short") {
			positionSide = "SHORT"
		}

		execTimeMs := trade.ExecTime.UTC().UnixMilli()
		isOpening := trade.OrderAction == "open_long" || trade.OrderAction == "open_short"

		// 仓位归属（计划文档 D40）：同一个账户上可能有多个下单入口（助手、网格、用户在 App 或其他工具里
		// 下单）。只有带 AI trader 专属备注的开仓成交才是本 trader 开出的仓位，才记到它名下；
		// 其余开仓成交不属于任何 trader（它们自带或不带保护都由下单的人自己负责）。
		if isOpening && trade.OrderText != aiTraderOpenText {
			logger.Debugf("  ⏭️ Skip %s %s trade %s: opened outside this trader (text=%q)", trade.OrderAction, symbol, trade.TradeID, trade.OrderText)
			continue
		}
		// 平仓成交只作用于本 trader 已有持仓记录的仓位：别的入口开的仓被平掉，与本 trader 无关，
		// 不记到它的成交历史与盈亏统计里。
		if !isOpening {
			if openPos, err := positionStore.GetOpenPositionBySymbol(traderID, symbol, positionSide); err == nil && openPos == nil {
				logger.Debugf("  ⏭️ Skip %s %s trade %s: no open position of this trader to close", trade.OrderAction, symbol, trade.TradeID)
				continue
			}
		}

		// Check if trade already exists (use exchangeID which is UUID, not exchange type)
		existing, err := orderStore.GetOrderByExchangeID(exchangeID, trade.TradeID)
		if err == nil && existing != nil {
			// Order exists, but still try to update position for close trades
			// This handles the case where order was created but position update failed
			if strings.HasPrefix(trade.OrderAction, "close_") && trade.FillPrice > 0 {
				if err := posBuilder.ProcessTrade(
					traderID, exchangeID, exchangeType,
					symbol, positionSide, trade.OrderAction,
					trade.FillQty, trade.FillPrice, trade.Fee, trade.ProfitLoss,
					execTimeMs, trade.TradeID,
				); err != nil {
					logger.Infof("  ⚠️ Retry position update for existing trade %s failed: %v", trade.TradeID, err)
				}
			}
			continue
		}

		// Normalize side for storage
		side := strings.ToUpper(trade.Side)

		// Create order record
		orderRecord := &store.TraderOrder{
			TraderID:        traderID,
			ExchangeID:      exchangeID,   // UUID
			ExchangeType:    exchangeType, // Exchange type
			ExchangeOrderID: trade.TradeID,
			Symbol:          symbol,
			Side:            side,
			PositionSide:    "BOTH", // Gate uses one-way position mode
			Type:            trade.OrderType,
			OrderAction:     trade.OrderAction,
			Quantity:        trade.FillQty,
			Price:           trade.FillPrice,
			Status:          "FILLED",
			FilledQuantity:  trade.FillQty,
			AvgFillPrice:    trade.FillPrice,
			Commission:      trade.Fee,
			FilledAt:        execTimeMs,
			CreatedAt:       execTimeMs,
			UpdatedAt:       execTimeMs,
		}

		// Insert order record
		if err := orderStore.CreateOrder(orderRecord); err != nil {
			logger.Infof("  ⚠️ Failed to sync trade %s: %v", trade.TradeID, err)
			continue
		}

		// Create fill record - use UTC time in milliseconds
		fillRecord := &store.TraderFill{
			TraderID:        traderID,
			ExchangeID:      exchangeID,   // UUID
			ExchangeType:    exchangeType, // Exchange type
			OrderID:         orderRecord.ID,
			ExchangeOrderID: trade.OrderID,
			ExchangeTradeID: trade.TradeID,
			Symbol:          symbol,
			Side:            side,
			Price:           trade.FillPrice,
			Quantity:        trade.FillQty,
			QuoteQuantity:   trade.FillPrice * trade.FillQty,
			Commission:      trade.Fee,
			CommissionAsset: trade.FeeAsset,
			RealizedPnL:     trade.ProfitLoss,
			IsMaker:         false,
			CreatedAt:       execTimeMs,
		}

		if err := orderStore.CreateFill(fillRecord); err != nil {
			logger.Infof("  ⚠️ Failed to sync fill for trade %s: %v", trade.TradeID, err)
		}

		// Create/update position record using PositionBuilder
		positionUpdated := false
		if trade.FillPrice <= 0 {
			logger.Infof("  ⚠️ WARNING: trade %s has FillPrice=%.10f (invalid), skipping position update", trade.TradeID, trade.FillPrice)
		} else if err := posBuilder.ProcessTrade(
			traderID, exchangeID, exchangeType,
			symbol, positionSide, trade.OrderAction,
			trade.FillQty, trade.FillPrice, trade.Fee, trade.ProfitLoss,
			execTimeMs, trade.TradeID,
		); err != nil {
			logger.Infof("  ⚠️ Failed to sync position for trade %s: %v", trade.TradeID, err)
		} else {
			positionUpdated = true
			logger.Infof("  📍 Position updated for trade: %s (action: %s, qty: %.6f, price: %.10f)", trade.TradeID, trade.OrderAction, trade.FillQty, trade.FillPrice)
		}
		if isOpening && !positionUpdated {
			// 本 trader 的开仓成交没写进持仓记录：把刚建的订单行与成交行删掉，下一次同步从头重放这笔成交。
			// 否则订单行已存在会让后续同步跳过开仓成交，这个仓位就永远不属于本 trader（重启后无人管理）。
			if delErr := orderStore.DeleteSyncedTrade(exchangeID, trade.TradeID); delErr != nil {
				logger.Warnf("  ⚠️ Failed to roll back synced trade %s: %v", trade.TradeID, delErr)
			}
		}

		syncedCount++
		logger.Infof("  ✅ Synced trade: %s %s %s qty=%.6f price=%.6f pnl=%.2f fee=%.6f action=%s",
			trade.TradeID, symbol, side, trade.FillQty, trade.FillPrice, trade.ProfitLoss, trade.Fee, trade.OrderAction)
	}

	logger.Infof("✅ Gate order sync completed: %d new trades synced", syncedCount)
	return nil
}

// StartOrderSync starts background order sync task for Gate
func (t *GateTrader) StartOrderSync(traderID string, exchangeID string, exchangeType string, st *store.Store, interval time.Duration, stop <-chan struct{}) {
	syncloop.Run(stop, interval, "Gate", func() error {
		return t.SyncOrdersFromGate(traderID, exchangeID, exchangeType, st)
	})
}
