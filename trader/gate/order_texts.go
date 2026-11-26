package gate

// 下单备注（Gate 订单的 text 字段）：成交记录（my_trades）原样带回，是区分「谁开的仓」的唯一线上依据。
// 同一个交易所账户上可能有多个下单入口——AI trader、助手手动下单、网格、用户在 App 或其他工具里下单——
// 仓位归属（计划文档 D40）按备注判定，所以 AI trader 的开仓必须用它专属的备注。
const (
	// aiTraderOpenText AI trader 的随单保护开仓。SyncOrdersFromGate 只为带这个备注的开仓成交建立持仓记录。
	aiTraderOpenText = "t-nofx-ai"
	// assistantOpenText 助手手动下单与网格的普通市价开仓（不带保护，不属于任何 AI trader）
	assistantOpenText = "t-nofx"
	// closeText 市价全平（trader、助手、回撤与保护兜底的紧急平仓共用）
	closeText = "t-nofx-close"
	// partialCloseText 分批平仓
	partialCloseText = "t-nofx-partial-close"
)
