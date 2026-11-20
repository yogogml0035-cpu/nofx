import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { HelpCircle } from 'lucide-react'

// 视口安全边距：气泡与屏幕边缘至少保留的空隙
const VIEWPORT_PADDING = 8
// 气泡理想宽度（小屏自动收窄到视口内）
const TIP_WIDTH = 288

// 卡头上下文帮助：常驻一个问号图标，点击切换说明气泡；
// 气泡按锚点实测定位——水平 clamp 进视口、下方空间不足时向上翻转，resize/scroll 实时重算
export function HelpTip({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [style, setStyle] = useState<CSSProperties | null>(null)
  const rootRef = useRef<HTMLSpanElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)
  const tipRef = useRef<HTMLDivElement>(null)

  // 依据锚点与气泡实测位置计算气泡样式（相对组件根容器）
  const place = useCallback(() => {
    const root = rootRef.current
    const btn = btnRef.current
    if (!root || !btn) return
    const rootRect = root.getBoundingClientRect()
    const anchor = btn.getBoundingClientRect()
    const vw = window.innerWidth
    const vh = window.innerHeight
    const width = Math.min(TIP_WIDTH, vw - VIEWPORT_PADDING * 2)
    const height = tipRef.current?.offsetHeight ?? 0

    // 水平：默认与锚点左缘对齐并略向左偏，超界时 clamp 进视口
    let left = anchor.left - rootRect.left - VIEWPORT_PADDING
    left = Math.max(
      VIEWPORT_PADDING - rootRect.left,
      Math.min(left, vw - VIEWPORT_PADDING - width - rootRect.left)
    )

    // 垂直：优先向下展开，下方放不下且上方更宽裕时向上翻转
    const belowOk =
      anchor.bottom + VIEWPORT_PADDING + height <= vh - VIEWPORT_PADDING
    const aboveOk = anchor.top - VIEWPORT_PADDING - height >= VIEWPORT_PADDING
    const below = belowOk || !aboveOk

    setStyle({
      width,
      ...(below
        ? { top: anchor.bottom - rootRect.top + 6, bottom: undefined }
        : { top: undefined, bottom: rootRect.bottom - anchor.top + 6 }),
      left,
      right: undefined,
    })
  }, [])

  useLayoutEffect(() => {
    if (!open) {
      setStyle(null)
      return
    }
    place()
    // 展开期间视口变化/页面滚动时保持气泡贴住锚点且不出屏
    const reposition = () => place()
    window.addEventListener('resize', reposition)
    window.addEventListener('scroll', reposition, true)
    return () => {
      window.removeEventListener('resize', reposition)
      window.removeEventListener('scroll', reposition, true)
    }
  }, [open, place, label])

  useEffect(() => {
    if (!open) return
    const onDocClick = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onDocClick)
    return () => document.removeEventListener('mousedown', onDocClick)
  }, [open])

  return (
    <span ref={rootRef} className="relative inline-flex" data-testid="help-tip">
      <button
        ref={btnRef}
        type="button"
        aria-label={label}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="shrink-0 text-nofx-text-muted/70 transition hover:text-nofx-gold"
      >
        <HelpCircle className="h-3.5 w-3.5" />
      </button>
      {open && (
        <div
          ref={tipRef}
          role="tooltip"
          style={style ?? { visibility: 'hidden' }}
          className="absolute top-6 z-30 rounded-lg border border-white/10 bg-[#0B0E11] p-3 shadow-xl"
        >
          <div className="mb-1.5 text-[10px] font-mono uppercase tracking-wider text-nofx-gold">
            {label}
          </div>
          <div className="space-y-1.5 text-[11px] leading-relaxed text-nofx-text-muted">
            {children}
          </div>
        </div>
      )}
    </span>
  )
}
