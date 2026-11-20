import type { ReactNode } from "react"

import { ThemeToggle } from "@/components/theme-toggle"
import { Card } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

export function PageShell({
  width = "max-w-[560px]",
  compact = false,
  footer,
  children,
}: {
  width?: string
  compact?: boolean
  footer?: string
  children: ReactNode
}) {
  return (
    <div
      className="relative h-dvh w-full overflow-hidden bg-cover bg-center bg-no-repeat"
      style={{ backgroundImage: "url(/background.webp)" }}
    >
      <div className="pointer-events-none absolute inset-0 hidden bg-black/50 dark:block" />
      <ThemeToggle className="fixed top-4 right-4 z-20" />
      <ScrollArea className="h-full">
        <div
          className={cn(
            "relative flex min-h-full items-center justify-center px-5",
            footer ? "pt-10 pb-20" : compact ? "py-4" : "py-10",
          )}
        >
          <Card
            className={cn(
              "w-full gap-0 overflow-hidden border-border/70 bg-card/95 py-0 backdrop-blur-sm",
              "shadow-[0_1px_2px_rgba(16,24,40,0.05),0_28px_72px_-28px_rgba(16,24,40,0.28)]",
              width,
            )}
          >
            {children}
          </Card>
        </div>
      </ScrollArea>
      {footer && (
        <p className="absolute inset-x-0 bottom-6 text-center text-[12px] leading-[1.8] whitespace-pre-line text-muted-foreground">
          {footer}
        </p>
      )}
    </div>
  )
}
