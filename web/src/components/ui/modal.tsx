import { useEffect, type ReactNode } from "react"
import { X } from "lucide-react"

import { cn } from "@/lib/utils"

type Props = {
  open: boolean
  title: string
  description?: string
  width?: string
  footer?: ReactNode
  onClose: () => void
  children: ReactNode
}

export function Modal({ open, title, description, width, footer, onClose, children }: Props) {
  useEffect(() => {
    if (!open) return

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") onClose()
    }

    document.addEventListener("keydown", onKeyDown)
    const overflow = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      document.removeEventListener("keydown", onKeyDown)
      document.body.style.overflow = overflow
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
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
