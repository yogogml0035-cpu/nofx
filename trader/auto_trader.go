package trader

import (
	"fmt"
	"nofx/kernel"
	"nofx/logger"
	"nofx/mcp"
	_ "nofx/mcp/provider"
	"nofx/store"
	"nofx/trader/aster"
	"nofx/trader/binance"
	"nofx/trader/bitget"
	"nofx/trader/bybit"
	"nofx/trader/gate"
	"nofx/trader/indodax"
	"nofx/trader/kucoin"
	"nofx/trader/lighter"
	"nofx/trader/okx"
	"nofx/trader/types"
	"sync"
	"time"
)

func (at *AutoTrader) logTag() string {
	if at == nil {
		return "[trader_id=unknown]"
	}
	if at.name != "" {
		return fmt.Sprintf("[trader_id=%s trader_name=%s]", at.id, at.name)
	}
	return fmt.Sprintf("[trader_id=%s]", at.id)
}

func (at *AutoTrader) logInfof(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Infof("%s "+format, values...)
}

func (at *AutoTrader) logWarnf(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Warnf("%s "+format, values...)
}

func (at *AutoTrader) logErrorf(format string, args ...interface{}) {
	values := append([]interface{}{at.logTag()}, args...)
	logger.Errorf("%s "+format, values...)
}

// AutoTraderConfig auto trading configuration (simplified version - AI makes all decisions)
type AutoTraderConfig struct {
	// Trader identification
	ID      string // Trader unique identifier (for log directory, etc.)
	Name    string // Trader display name
	AIModel string // AI model: "qwen" or "deepseek"

	// Trading platform selection
	Exchange   string // Exchange type: "binance", "bybit", "okx", "bitget", "gate", "aster" or "lighter"
	ExchangeID string // Exchange account UUID (for multi-account support)

	// Binance API configuration
	BinanceAPIKey    string
	BinanceSecretKey string

	// Bybit API configuration
	BybitAPIKey    string
	BybitSecretKey string

	// OKX API configuration
	OKXAPIKey     string
	OKXSecretKey  string
	OKXPassphrase string

	// Bitget API configuration
	BitgetAPIKey     string
	BitgetSecretKey  string
	BitgetPassphrase string

	// Gate API configuration
	GateAPIKey    string
	GateSecretKey string

	// KuCoin API configuration
	KuCoinAPIKey     string
	KuCoinSecretKey  string
	KuCoinPassphrase string

	// Indodax API configuration
	IndodaxAPIKey    string
	IndodaxSecretKey string

	// Aster configuration
	AsterUser       string // Aster main wallet address
	AsterSigner     string // Aster API wallet address
	AsterPrivateKey string // Aster API wallet private key

	// LIGHTER configuration
	LighterWalletAddr       string // LIGHTER wallet address (L1 wallet)
	LighterPrivateKey       string // LIGHTER L1 private key (for account identification)
	LighterAPIKeyPrivateKey string // LIGHTER API Key private key (40 bytes, for transaction signing)
	LighterAPIKeyIndex      int    // LIGHTER API Key index (0-255)
	LighterTestnet          bool   // Whether to use testnet

	// AI configuration
	UseQwen     bool
	DeepSeekKey string
	QwenKey     string

	// Custom AI API configuration
	CustomAPIURL    string
	CustomAPIKey    string
	CustomModelName string

	// Scan configuration
	ScanInterval time.Duration // Scan interval (recommended 3 minutes)

	// Account configuration
	InitialBalance float64 // Initial balance (for P&L calculation, must be set manually)

	// Risk control (only as hints, AI can make autonomous decisions)
	MaxDailyLoss    float64       // Maximum daily loss percentage (hint)
	MaxDrawdown     float64       // Maximum drawdown percentage (hint)
	StopTradingTime time.Duration // Pause duration after risk control triggers

	// Position mode
	IsCrossMargin bool // true=cross margin mode, false=isolated margin mode

	// Competition visibility
	ShowInCompetition bool // Whether to show in competition page

	// Strategy configuration (use complete strategy config)
	StrategyConfig *store.StrategyConfig // Strategy configuration (includes coin sources, indicators, risk control, prompts, etc.)
}

// AutoTrader automatic trader
type AutoTrader struct {
	id                    string // Trader unique identifier
	name                  string // Trader display name
	aiModel               string // AI model name
	exchange              string // Trading platform type (binance/bybit/etc)
	exchangeID            string // Exchange account UUID
	showInCompetition     bool   // Whether to show in competition page
	config                AutoTraderConfig
	trader                Trader // Use Trader interface (supports multiple platforms)
	mcpClient             mcp.AIClient
	store                 *store.Store           // Data storage (decision records, etc.)
	strategyEngine        *kernel.StrategyEngine // Strategy engine (uses strategy configuration)
	cycleNumber           int                    // Current cycle number
	initialBalance        float64
	dailyPnL              float64
	customPrompt          string // Custom trading strategy prompt
	overrideBasePrompt    bool   // Whether to override base prompt
	lastResetTime         time.Time
	stopUntil             time.Time
	isRunning             bool
	isRunningMutex        sync.RWMutex       // Mutex to protect isRunning flag
	startTime             time.Time          // System start time
	callCount             int                // AI call count
	positionFirstSeenTime map[string]int64   // Position first seen time (symbol_side -> timestamp in milliseconds)
	stopMonitorCh         chan struct{}      // Used to stop monitoring goroutine
	monitorWg             sync.WaitGroup     // Used to wait for monitoring goroutine to finish
	peakPnLCache          map[string]float64 // Peak profit cache (symbol -> peak P&L percentage)
	peakPnLCacheMutex     sync.RWMutex       // Cache read-write lock
	// selfOpenedPositions 记录本进程刚开出的仓位（归一化 symbol_side），补上「开仓成交 → OrderSync
	// 把持仓落到 DB」之间的归属空窗（D40：归属不了就不管理，这段时间仓位仍是自己的）
	selfOpenedPositions map[string]bool
	selfOpenedMutex     sync.RWMutex
	// consecutiveRejectedRounds 连续出现被校验丢弃的决策的轮数（见 noteRejectedDecisions）
	consecutiveRejectedRounds int
	// openBlockReason 非空时本轮禁止开仓（例如有仓位失去止损且没能平掉）；每轮开始时清空
	openBlockReason       string
	lastBalanceSyncTime   time.Time  // Last balance sync time
	userID                string     // User ID
	gridState             *GridState // Grid trading state (only used when StrategyType == "grid_trading")
	consecutiveAIFailures int        // Consecutive AI call failures
	safeMode              bool       // Safe mode: no new positions, protect existing ones
	safeModeReason        string     // Why safe mode was activated
}

// NewAutoTrader creates an automatic trader
// st parameter is used to store decision records to database
func NewAutoTrader(config AutoTraderConfig, st *store.Store, userID string) (*AutoTrader, error) {
	// Set default values
	if config.ID == "" {
		config.ID = "default_trader"
	}
	if config.Name == "" {
		config.Name = "Default Trader"
	}
	if config.AIModel == "" {
		if config.UseQwen {
			config.AIModel = "qwen"
		} else {
			config.AIModel = "deepseek"
		}
	}

	// Initialize AI client based on provider
	var mcpClient mcp.AIClient
	aiModel := config.AIModel
	if config.UseQwen && aiModel == "" {
		aiModel = "qwen"
	}

	// Resolve API key (provider-specific overrides)
	apiKey := config.CustomAPIKey
	customURL := config.CustomAPIURL
	switch aiModel {
	case "qwen":
		if config.QwenKey != "" {
			apiKey = config.QwenKey
		}
	case "deepseek", "":
		if config.DeepSeekKey != "" {
			apiKey = config.DeepSeekKey
		}
	}

	// Create client via registry (covers all registered providers)
	if aiModel == "custom" {
		mcpClient = mcp.New()
	} else if aiModel == "" {
		aiModel = "deepseek"
		mcpClient = mcp.NewAIClientByProvider(aiModel)
	} else {
		mcpClient = mcp.NewAIClientByProvider(aiModel)
	}
	if mcpClient == nil {
		mcpClient = mcp.New()
	}

	mcpClient.SetAPIKey(apiKey, customURL, config.CustomModelName)
	logger.Infof("🤖 [%s] Using %s AI", config.Name, aiModel)

	if config.CustomAPIURL != "" || config.CustomModelName != "" {
		logger.Infof("🔧 [%s] Custom config - URL: %s, Model: %s", config.Name, config.CustomAPIURL, config.CustomModelName)
	}

	// Set default trading platform
	if config.Exchange == "" {
		config.Exchange = "binance"
	}

	// Create corresponding trader based on configuration
	var trader Trader
	var err error

	// Record position mode (general)
	marginModeStr := "Cross Margin"
	if !config.IsCrossMargin {
		marginModeStr = "Isolated Margin"
	}
	logger.Infof("📊 [%s] Position mode: %s", config.Name, marginModeStr)

	switch config.Exchange {
	case "binance":
		logger.Infof("🏦 [%s] Using Binance Futures trading", config.Name)
		trader = binance.NewFuturesTrader(config.BinanceAPIKey, config.BinanceSecretKey, userID)
	case "bybit":
		logger.Infof("🏦 [%s] Using Bybit Futures trading", config.Name)
		trader = bybit.NewBybitTrader(config.BybitAPIKey, config.BybitSecretKey)
	case "okx":
		logger.Infof("🏦 [%s] Using OKX Futures trading", config.Name)
		trader = okx.NewOKXTrader(config.OKXAPIKey, config.OKXSecretKey, config.OKXPassphrase)
	case "bitget":
		logger.Infof("🏦 [%s] Using Bitget Futures trading", config.Name)
		trader = bitget.NewBitgetTrader(config.BitgetAPIKey, config.BitgetSecretKey, config.BitgetPassphrase)
	case "gate":
		logger.Infof("🏦 [%s] Using Gate.io Futures trading", config.Name)
		trader = gate.NewGateTrader(config.GateAPIKey, config.GateSecretKey)
	case "kucoin":
		logger.Infof("🏦 [%s] Using KuCoin Futures trading", config.Name)
		trader = kucoin.NewKuCoinTrader(config.KuCoinAPIKey, config.KuCoinSecretKey, config.KuCoinPassphrase)
	case "aster":
		logger.Infof("🏦 [%s] Using Aster trading", config.Name)
		trader, err = aster.NewAsterTrader(config.AsterUser, config.AsterSigner, config.AsterPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Aster trader: %w", err)
		}
	case "lighter":
		logger.Infof("🏦 [%s] Using LIGHTER trading", config.Name)

		if config.LighterWalletAddr == "" || config.LighterAPIKeyPrivateKey == "" {
			return nil, fmt.Errorf("Lighter requires wallet address and API Key private key")
		}

		// Lighter only supports mainnet (testnet disabled)
		trader, err = lighter.NewLighterTraderV2(
			config.LighterWalletAddr,
			config.LighterAPIKeyPrivateKey,
			config.LighterAPIKeyIndex,
			false, // Always use mainnet for Lighter
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize LIGHTER trader: %w", err)
		}
		logger.Infof("✓ LIGHTER trader initialized successfully")
	case "indodax":
		logger.Infof("🏦 [%s] Using Indodax Spot trading", config.Name)
		trader = indodax.NewIndodaxTrader(config.IndodaxAPIKey, config.IndodaxSecretKey)
	default:
		return nil, fmt.Errorf("unsupported trading platform: %s", config.Exchange)
	}

	// Initial balance: keep a valid value, rebuild an invalid one from the account principal
	if err := resolveInitialBalance(trader, st, userID, &config); err != nil {
		return nil, fmt.Errorf("[%s] %w", config.Name, err)
	}

	// Get last cycle number (for recovery)
	var cycleNumber int
	if st != nil {
		cycleNumber, _ = st.Decision().GetLastCycleNumber(config.ID)
		logger.Infof("📊 [%s] Decision records will be stored to database", config.Name)
	}

	// Create strategy engine (must have strategy config)
	if config.StrategyConfig == nil {
		return nil, fmt.Errorf("[%s] strategy not configured", config.Name)
	}
	strategyEngine := kernel.NewStrategyEngine(config.StrategyConfig)
	logger.Infof("✓ [%s] Using strategy engine (strategy configuration loaded)", config.Name)

	return &AutoTrader{
		id:                    config.ID,
		name:                  config.Name,
		aiModel:               config.AIModel,
		exchange:              config.Exchange,
		exchangeID:            config.ExchangeID,
		showInCompetition:     config.ShowInCompetition,
		config:                config,
		trader:                trader,
		mcpClient:             mcpClient,
		store:                 st,
		strategyEngine:        strategyEngine,
		cycleNumber:           cycleNumber,
		initialBalance:        config.InitialBalance,
		lastResetTime:         time.Now(),
		startTime:             time.Now(),
		callCount:             0,
		isRunning:             false,
		positionFirstSeenTime: make(map[string]int64),
		stopMonitorCh:         make(chan struct{}),
		monitorWg:             sync.WaitGroup{},
		peakPnLCache:          make(map[string]float64),
		peakPnLCacheMutex:     sync.RWMutex{},
		selfOpenedPositions:   make(map[string]bool),
		selfOpenedMutex:       sync.RWMutex{},
		lastBalanceSyncTime:   time.Now(),
		userID:                userID,
	}, nil
}

// resolveInitialBalance 校验并补齐初始资金。配置里的初始资金大于 0 时照用；不大于 0 是无效值
// （存量 trader 在交易所读数为 0 时被记成 0），此时读交易所账户的当前本金，以它重建、写回数据库
// 并记录日志。读不到本金或本金不为正时返回错误：没有有效的初始资金，盈亏与开仓金额都无从计算，
// trader 不加载。错误链里带着 types.ErrBalanceUnreadable 或 types.ErrPrincipalNotPositive，
// 上层按类型识别原因，不靠匹配文案。
func resolveInitialBalance(t Trader, st *store.Store, userID string, config *AutoTraderConfig) error {
	if config.InitialBalance > 0 {
		return nil
	}
	invalid := config.InitialBalance

	principal, err := types.ReadInitialBalance(t)
	if err != nil {
		return fmt.Errorf("initial balance %.2f is not valid: %w", invalid, err)
	}

	config.InitialBalance = principal
	logger.Warnf("⚠️ [%s] Initial balance %.2f is not valid, rebuilt from the current account principal: %.2f USDT",
		config.Name, invalid, principal)
	if st != nil {
		if err := st.Trader().UpdateInitialBalance(userID, config.ID, principal); err != nil {
			logger.Warnf("⚠️ [%s] Failed to save the rebuilt initial balance to database: %v", config.Name, err)
		}
	}
	return nil
}

// Run runs the automatic trading main loop
func (at *AutoTrader) Run() error {
	at.isRunningMutex.Lock()
	at.isRunning = true
	at.isRunningMutex.Unlock()

	at.stopMonitorCh = make(chan struct{})
	at.startTime = time.Now()

	logger.Info("🚀 AI-driven automatic trading system started")
	at.logInfof("💰 Initial balance: %.2f USDT", at.initialBalance)
	if cadence := scanTimeframe(at.config.StrategyConfig); cadence == "" {
		at.logWarnf("⚙️  Primary timeframe missing/invalid; scan interval fallback: %v", at.scanFallbackInterval())
	} else {
		at.logInfof("⚙️  Scan aligned to %s close + %v (UTC)", cadence, scanCloseDelay)
	}
	logger.Info("🤖 AI will make full decisions on leverage, position size, stop loss/take profit, etc.")

	at.monitorWg.Add(1)
	defer at.monitorWg.Done()

	// Start drawdown monitoring
	at.startDrawdownMonitor()

	// Start Lighter order sync if using Lighter exchange
	if at.exchange == "lighter" {
		if lighterTrader, ok := at.trader.(*lighter.LighterTraderV2); ok && at.store != nil {
			lighterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Lighter order+position sync enabled (every 30s)")
		}
	}

	// Start Bybit order sync if using Bybit exchange
	if at.exchange == "bybit" {
		if bybitTrader, ok := at.trader.(*bybit.BybitTrader); ok && at.store != nil {
			bybitTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Bybit order+position sync enabled (every 30s)")
		}
	}

	// Start OKX order sync if using OKX exchange
	if at.exchange == "okx" {
		if okxTrader, ok := at.trader.(*okx.OKXTrader); ok && at.store != nil {
			okxTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 OKX order+position sync enabled (every 30s)")
		}
	}

	// Start Bitget order sync if using Bitget exchange
	if at.exchange == "bitget" {
		if bitgetTrader, ok := at.trader.(*bitget.BitgetTrader); ok && at.store != nil {
			bitgetTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Bitget order+position sync enabled (every 30s)")
		}
	}

	// Start Aster order sync if using Aster exchange
	if at.exchange == "aster" {
		if asterTrader, ok := at.trader.(*aster.AsterTrader); ok && at.store != nil {
			asterTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Aster order+position sync enabled (every 30s)")
		}
	}

	// Start Binance order sync if using Binance exchange
	if at.exchange == "binance" {
		if binanceTrader, ok := at.trader.(*binance.FuturesTrader); ok && at.store != nil {
			binanceTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Binance order+position sync enabled (every 30s)")
		}
	}

	// Start Gate order sync if using Gate exchange
	if at.exchange == "gate" {
		if gateTrader, ok := at.trader.(*gate.GateTrader); ok && at.store != nil && !at.IsGridStrategy() {
			// 网格不消费 trader_positions，也不按备注区分下单入口：开启同步会把同一个账户上
			// AI trader 的成交记到网格名下（污染记录）。网格的仓位归属按网格品种判定（D40）。
			gateTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 Gate order+position sync enabled (every 30s)")
		}
	}

	// Start KuCoin order sync if using KuCoin exchange
	if at.exchange == "kucoin" {
		if kucoinTrader, ok := at.trader.(*kucoin.KuCoinTrader); ok && at.store != nil {
			kucoinTrader.StartOrderSync(at.id, at.exchangeID, at.exchange, at.store, 30*time.Second, at.stopMonitorCh)
			at.logInfof("🔄 KuCoin order+position sync enabled (every 30s)")
		}
	}

	// Check if this is a grid trading strategy
	isGridStrategy := at.IsGridStrategy()
	if isGridStrategy {
		at.logInfof("🔲 Grid trading strategy detected, initializing grid...")
		if err := at.InitializeGrid(); err != nil {
			at.logErrorf("❌ Failed to initialize grid: %v", err)
			return fmt.Errorf("grid initialization failed: %w", err)
		}
	}

	// Execute immediately on first run
	if isGridStrategy {
		if err := at.RunGridCycle(); err != nil {
			at.logErrorf("❌ Grid execution failed: %v", err)
		}
	} else {
		if err := at.runCycle(); err != nil {
			at.logErrorf("❌ Execution failed: %v", err)
		}
	}

	for {
		at.isRunningMutex.RLock()
		running := at.isRunning
		at.isRunningMutex.RUnlock()

		if !running {
			break
		}

		// 等待下一个 K 线收线边界（+ 固定延迟）；主周期缺失时回退到旧 ScanInterval。
		// 每轮重新读取：引擎可能在首轮原地归一化周期别名（1小时→1h），下一轮起即可对齐。
		cadence := scanTimeframe(at.config.StrategyConfig)
		wait := at.scanFallbackInterval()
		if cadence != "" {
			next, err := nextScanTime(time.Now(), cadence, scanCloseDelay)
			if err != nil {
				at.logWarnf("⚠️ Scan schedule unavailable (%v); fallback to %v", err, wait)
			} else {
				wait = time.Until(next)
				if wait < 0 {
					wait = 0
				}
				at.logInfof("⏰ Next scan at %s (UTC)", next.Format(time.RFC3339))
			}
		}
		if cadence == "" {
			at.logInfof("⏰ Next scan in %v (ScanInterval fallback)", wait)
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			if isGridStrategy {
				if err := at.RunGridCycle(); err != nil {
					at.logErrorf("❌ Grid execution failed: %v", err)
				}
			} else {
				if err := at.runCycle(); err != nil {
					at.logErrorf("❌ Execution failed: %v", err)
				}
			}
		case <-at.stopMonitorCh:
			timer.Stop()
			at.logInfof("⏹ Stop signal received, exiting automatic trading main loop")
			return nil
		}
	}

	return nil
}

// Stop stops the automatic trading
func (at *AutoTrader) Stop() {
	at.isRunningMutex.Lock()
	if !at.isRunning {
		at.isRunningMutex.Unlock()
		return
	}
	at.isRunning = false
	at.isRunningMutex.Unlock()

	close(at.stopMonitorCh) // Notify monitoring goroutine to stop
	at.monitorWg.Wait()     // Wait for monitoring goroutine to finish
	logger.Info("⏹ Automatic trading system stopped")
}

// GetID gets trader ID
func (at *AutoTrader) GetID() string {
	return at.id
}

// GetUnderlyingTrader returns the underlying Trader interface implementation
// This is used by grid trading and other components that need direct exchange access
func (at *AutoTrader) GetUnderlyingTrader() Trader {
	return at.trader
}

// GetName gets trader name
func (at *AutoTrader) GetName() string {
	return at.name
}

// GetAIModel gets AI model
func (at *AutoTrader) GetAIModel() string {
	return at.aiModel
}

// GetExchange gets exchange
func (at *AutoTrader) GetExchange() string {
	return at.exchange
}

// GetShowInCompetition returns whether trader should be shown in competition
func (at *AutoTrader) GetShowInCompetition() bool {
	return at.showInCompetition
}

// SetShowInCompetition sets whether trader should be shown in competition
func (at *AutoTrader) SetShowInCompetition(show bool) {
	at.showInCompetition = show
}

// SetCustomPrompt sets custom trading strategy prompt
func (at *AutoTrader) SetCustomPrompt(prompt string) {
	at.customPrompt = prompt
}

// SetOverrideBasePrompt sets whether to override base prompt
func (at *AutoTrader) SetOverrideBasePrompt(override bool) {
	at.overrideBasePrompt = override
}

// GetSystemPromptTemplate gets current system prompt template name (from strategy config)
func (at *AutoTrader) GetSystemPromptTemplate() string {
	if at.strategyEngine != nil {
		config := at.strategyEngine.GetConfig()
		if config.CustomPrompt != "" {
			return "custom"
		}
	}
	return "strategy"
}

// GetCandidateCoins returns the current candidate coin set from the trader's strategy engine.
func (at *AutoTrader) GetCandidateCoins() ([]kernel.CandidateCoin, error) {
	if at.strategyEngine == nil {
		return nil, fmt.Errorf("strategy engine not configured")
	}
	return at.strategyEngine.GetCandidateCoins()
}

// GetStrategyConfig returns the current strategy config used by the trader.
func (at *AutoTrader) GetStrategyConfig() *store.StrategyConfig {
	if at.strategyEngine == nil {
		return at.config.StrategyConfig
	}
	return at.strategyEngine.GetConfig()
}

// GetStore gets data store (for external access to decision records, etc.)
func (at *AutoTrader) GetStore() *store.Store {
	return at.store
}

// calculatePnLPercentage calculates P&L percentage (based on margin, automatically considers leverage)
// Return rate = Unrealized P&L / Margin x 100%
func calculatePnLPercentage(unrealizedPnl, marginUsed float64) float64 {
	if marginUsed > 0 {
		return (unrealizedPnl / marginUsed) * 100
	}
	return 0.0
}
