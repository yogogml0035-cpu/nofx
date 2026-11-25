import type { CreateTraderRequest } from '../../types'

// 初始余额输入框接受普通的十进制写法（可带正负号、可省略整数或小数部分）。
// 不用 Number() 直接判断：它会接受「0x10」「1e3」这类写法；也不用 parseFloat：它会放过「50abc」。
const DECIMAL_INPUT = /^[+-]?(?:\d+\.?\d*|\.\d+)$/

// parseInitialBalance 解析初始余额输入框的原文，必须大于 0；不合法时返回 null
export function parseInitialBalance(raw: string): number | null {
  const text = raw.trim()
  if (!DECIMAL_INPUT.test(text)) {
    return null
  }
  const value = Number(text)
  return Number.isFinite(value) && value > 0 ? value : null
}

// buildTraderUpdateRequest 把编辑表单的数据整理成更新请求。
// 初始余额是可选项：用户没有改过时表单不带这个字段，请求里也就没有它——
// 后端把「正值」当作明确给出，换绑交易所时要靠「没有给出」把旧账户的初始余额重置掉
export function buildTraderUpdateRequest(data: CreateTraderRequest) {
  return {
    name: data.name,
    ai_model_id: data.ai_model_id,
    exchange_id: data.exchange_id,
    strategy_id: data.strategy_id,
    scan_interval_minutes: data.scan_interval_minutes,
    is_cross_margin: data.is_cross_margin,
    show_in_competition: data.show_in_competition,
    // 表单没有给出初始余额时，请求里不带这个键
    ...(data.initial_balance === undefined
      ? {}
      : { initial_balance: data.initial_balance }),
  }
}
