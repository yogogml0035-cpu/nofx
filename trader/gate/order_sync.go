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
		if err != nil || fillPrice <= 0 {
			// 成交价是持仓记账的唯一价格来源，解析失败不能用 0 继续：跳过这笔，下次同步重试。
			logger.Infof("⚠️  Skip Gate trade %d: invalid fill price raw=%q: %v", trade.Id, trade.Price, err)
			continue
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

		// 同一笔成交只应用一次：订单行存在即视为已处理（持仓写入失败时会连同订单/成交行回滚，
		// 下一次同步从头重放）。这里绝不能再重放平仓成交——部分平仓后记录仍是 OPEN，
		// 每次轮询都重放会反复累加手续费/盈亏，并把分批平仓的记录提前关掉。
		existing, err := orderStore.GetOrderByExchangeID(exchangeID, trade.TradeID)
		if err != nil {
			logger.Infof("  ⚠️ Skip trade %s: failed to check existing order: %v", trade.TradeID, err)
			continue
		}
		if existing != nil {
			continue
		}

		// 仓位归属（计划文档 D40）：同一个账户上可能有多个下单入口（助手、网格、用户在 App 或其他工具里
		// 下单）。只有带 AI trader 专属备注的开仓成交才是本 trader 开出的仓位，才记到它名下；
		// 其余开仓成交不属于任何 trader（它们自带或不带保护都由下单的人自己负责）。
		if isOpening && trade.OrderText != aiTraderOpenText {
			logger.Debugf("  ⏭️ Skip %s %s trade %s: opened outside this trader (text=%q)", trade.OrderAction, symbol, trade.TradeID, trade.OrderText)
			continue
		}
		// 平仓成交只作用于本 trader 已有持仓记录的仓位：别的入口开的仓被平掉，与本 trader 无关，
		// 不记到它的成交历史与盈亏统计里。仓位已经被对账转为 RECONCILED 时，迟到的平仓成交回填进去
		// （否则出场价/盈亏永久丢失）；当前 OPEN 记录晚于成交时间时，说明这条成交属于上一笔仓位。
		backfillPositionID := int64(0)
		if !isOpening {
			openPos, posErr := positionStore.GetOpenPositionBySymbol(traderID, symbol, positionSide)
			if posErr != nil {
				logger.Infof("  ⚠️ Skip %s %s trade %s: cannot determine position ownership: %v",
					trade.OrderAction, symbol, trade.TradeID, posErr)
				continue
			}
			staleForCurrent := openPos != nil && openPos.EntryTime > 0 && execTimeMs+5000 < openPos.EntryTime
			if openPos == nil || staleForCurrent {
				reconciled, recErr := positionStore.GetBackfillableReconciledPosition(
					traderID, symbol, positionSide, startTime.UnixMilli())
				if recErr != nil {
					logger.Infof("  ⚠️ Skip %s %s trade %s: failed to query reconciled position: %v",
						trade.OrderAction, symbol, trade.TradeID, recErr)
					continue
				}
				if reconciled == nil {
					logger.Debugf("  ⏭️ Skip %s %s trade %s: no open position of this trader to close", trade.OrderAction, symbol, trade.TradeID)
					continue
				}
				backfillPositionID = reconciled.ID
			}
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
			logger.Infof("  ⚠️ Failed to sync fill for trade %s, rolling back order row: %v", trade.TradeID, err)
			if delErr := orderStore.DeleteSyncedTrade(exchangeID, trade.TradeID); delErr != nil {
				logger.Warnf("  ⚠️ Failed to roll back synced trade %s: %v", trade.TradeID, delErr)
			}
			continue
		}

		// 迟到的平仓成交回填到对账留下的 RECONCILED 记录（出场价/盈亏不会再丢），
		// 不经过 PositionBuilder：那条 OPEN 记录已经不存在了。
		if backfillPositionID > 0 {
			applied, backfillErr := positionStore.BackfillReconciledClose(
				backfillPositionID, trade.FillQty, trade.FillPrice, trade.ProfitLoss, trade.Fee, execTimeMs, trade.TradeID)
			if backfillErr != nil || !applied {
				logger.Infof("  ⚠️ Failed to backfill reconciled position for trade %s (applied=%v): %v",
					trade.TradeID, applied, backfillErr)
				if delErr := orderStore.DeleteSyncedTrade(exchangeID, trade.TradeID); delErr != nil {
					logger.Warnf("  ⚠️ Failed to roll back synced trade %s: %v", trade.TradeID, delErr)
				}
				continue
			}
			syncedCount++
			logger.Infof("  ✅ Synced trade (backfilled reconciled position): %s %s %s qty=%.6f price=%.6f pnl=%.2f action=%s",
				trade.TradeID, symbol, side, trade.FillQty, trade.FillPrice, trade.ProfitLoss, trade.OrderAction)
			continue
		}

		if trade.FillPrice <= 0 {
			logger.Infof("  ⚠️ WARNING: trade %s has FillPrice=%.10f (invalid), rolling back so it can be replayed", trade.TradeID, trade.FillPrice)
			if delErr := orderStore.DeleteSyncedTrade(exchangeID, trade.TradeID); delErr != nil {
				logger.Warnf("  ⚠️ Failed to roll back synced trade %s: %v", trade.TradeID, delErr)
			}
			continue
		}
		if err := posBuilder.ProcessTrade(
			traderID, exchangeID, exchangeType,
			symbol, positionSide, trade.OrderAction,
			trade.FillQty, trade.FillPrice, trade.Fee, trade.ProfitLoss,
			execTimeMs, trade.TradeID,
		); err != nil {
			// 持仓没写进去：订单行与成交行一起回滚，下一次同步从头重放这笔成交。
			// 开仓更关键——订单行存在会让后续同步跳过它，仓位就永远不属于本 trader（重启后无人管理）。
			logger.Infof("  ⚠️ Failed to sync position for trade %s, rolling back: %v", trade.TradeID, err)
			if delErr := orderStore.DeleteSyncedTrade(exchangeID, trade.TradeID); delErr != nil {
				logger.Warnf("  ⚠️ Failed to roll back synced trade %s: %v", trade.TradeID, delErr)
			}
			continue
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
