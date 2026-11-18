import { motion } from 'framer-motion'
import {
  Zap,
  Lightbulb,
  Search,
  Bot,
} from 'lucide-react'

interface SuggestionCard {
  icon: JSX.Element
  title: string
  subtitle: string
  cmd: string
}

interface WelcomeScreenProps {
  language: string
  onSend: (cmd: string) => void
}

export function WelcomeScreen({ language, onSend }: WelcomeScreenProps) {
  const suggestions: SuggestionCard[] = language === 'zh'
    ? [
        { icon: <Bot size={18} />, title: '创建加密 Agent', subtitle: '强势标的 + 严格风控', cmd: '创建一个加密货币趋势交易 Agent，默认选择强势标的，严格风控' },
        { icon: <Zap size={18} />, title: '一句话建策略', subtitle: '从想法到 Agent', cmd: '我想做 BTC 强趋势突破，帮我生成策略和Agent' },
        { icon: <Search size={18} />, title: 'AI500 精选', subtitle: '查看 AI 评分精选币种', cmd: '展示 AI500 精选币种' },
        { icon: <Lightbulb size={18} />, title: '行情策略建议', subtitle: 'BTC/ETH/主流币', cmd: '当前 BTC、ETH 行情适合什么策略？' },
      ]
    : [
        { icon: <Bot size={18} />, title: 'Create Crypto Agent', subtitle: 'Strong symbols + strict risk', cmd: 'Create a crypto trend-following agent with strong symbols and strict risk control' },
        { icon: <Zap size={18} />, title: 'One-line strategy', subtitle: 'Idea to agent', cmd: 'I want a BTC breakout strategy; build the strategy and agent' },
        { icon: <Search size={18} />, title: 'AI500 Picks', subtitle: 'AI-scored coin selection', cmd: 'Show me the AI500 picks' },
        { icon: <Lightbulb size={18} />, title: 'Market strategy', subtitle: 'BTC/ETH/majors', cmd: 'What strategy fits BTC and ETH now?' },
      ]

  return (
    <div style={{
      maxWidth: 640,
      margin: '0 auto',
      padding: '0 20px',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      height: '100%',
      minHeight: 400,
    }}>
      {/* Logo / greeting */}
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, ease: 'easeOut' }}
        style={{ textAlign: 'center', marginBottom: 40 }}
      >
        <div style={{
          width: 56,
          height: 56,
          borderRadius: 16,
          background: 'linear-gradient(135deg, rgba(240,185,11,0.12), rgba(0,229,160,0.06))',
          border: '1px solid rgba(240,185,11,0.15)',
          display: 'grid',
          placeItems: 'center',
          margin: '0 auto 16px',
          fontSize: 24,
        }}>
          ⚡
        </div>
        <h1 style={{
          fontSize: 22,
          fontWeight: 700,
          color: '#f0f0f8',
          margin: '0 0 8px',
          letterSpacing: '-0.02em',
        }}>
          {language === 'zh' ? '快速创建你的交易 Agent' : 'Create your trading agent'}
        </h1>
        <p style={{
          fontSize: 13.5,
          color: '#5c5c72',
          margin: 0,
          lineHeight: 1.5,
        }}>
          {language === 'zh'
            ? '加密货币永续合约 — 用自然语言描述策略即可'
            : 'Crypto perpetual futures — describe the strategy in plain English'}
        </p>
      </motion.div>

      {/* Suggestion cards grid */}
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, delay: 0.1, ease: 'easeOut' }}
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(2, 1fr)',
          gap: 10,
          width: '100%',
          maxWidth: 520,
        }}
      >
        {suggestions.map((s, i) => (
          <button
            key={i}
            onClick={() => onSend(s.cmd)}
            className="suggestion-card"
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              gap: 6,
              padding: '16px 14px',
              background: 'rgba(255,255,255,0.02)',
              border: '1px solid rgba(255,255,255,0.06)',
              borderRadius: 14,
              cursor: 'pointer',
              textAlign: 'left',
              fontFamily: 'inherit',
              transition: 'all 0.2s ease',
            }}
          >
            <div style={{ color: '#F0B90B', opacity: 0.7 }}>
              {s.icon}
            </div>
            <div>
              <div style={{ fontSize: 13, fontWeight: 600, color: '#d0d0e0', marginBottom: 2 }}>
                {s.title}
              </div>
              <div style={{ fontSize: 11.5, color: '#5c5c72' }}>
                {s.subtitle}
              </div>
            </div>
          </button>
        ))}
      </motion.div>
    </div>
  )
}
