import { useEffect, useRef, type ReactNode } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

type Props = {
  open: boolean
  title: string
  description?: string
  width?: string
  layer?: number
  footer?: ReactNode
  onClose: () => void
  children: ReactNode
}

// 打开的模态框栈顶才会响应 Esc，嵌套时不会一次关掉两层。
const openLayers: symbol[] = []

export function Modal({ open, title, description, width, layer = 0, footer, onClose, children }: Props) {
  const idRef = useRef(Symbol("modal"))

  useEffect(() => {
    if (!open) return

    const id = idRef.current
    openLayers.push(id)

    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "Escape") return
      if (openLayers[openLayers.length - 1] !== id) return
      onClose()
    }

    document.addEventListener("keydown", onKeyDown)
    const overflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.removeEventListener("keydown", onKeyDown)
      const index = openLayers.indexOf(id)
      if (index >= 0) openLayers.splice(index, 1)
      document.body.style.overflow = overflow
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 flex items-center justify-center p-4"
      style={{ zIndex: 50 + layer * 10 }}
    >
      <div className="absolute inset-0 bg-zinc-900/25" onClick={onClose} aria-hidden />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cn(
          "relative flex w-full flex-col overflow-hidden rounded-xl border border-border bg-card",
          "shadow-[0_24px_64px_-24px_rgba(16,24,40,0.35)]",
          width ?? "max-w-[520px]",
        )}
      >
        <header className="flex items-start justify-between gap-4 border-b border-border px-5 py-3.5">
          <div className="min-w-0">
            <h2 className="truncate text-[14px] font-medium text-foreground">{title}</h2>
            {description && (
              <p className="mt-0.5 text-[12px] text-muted-foreground">{description}</p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          >
            <X className="size-4" strokeWidth={1.75} />
          </button>
        </header>

        <div className="max-h-[68vh] overflow-y-auto px-5 py-4">{children}</div>

        {footer && (
          <footer className="flex items-center justify-end gap-2 border-t border-border px-5 py-3">
            {footer}
          </footer>
        )}
      </div>
    </div>
  )
}
