import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { HelpCircle } from 'lucide-react'

// 卡头上下文帮助：常驻一个问号图标，点击切换说明气泡，点击外部关闭
export function HelpTip({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLSpanElement>(null)

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
          role="tooltip"
          className="absolute right-0 top-6 z-30 w-72 rounded-lg border border-white/10 bg-[#0B0E11] p-3 shadow-xl"
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
