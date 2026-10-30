import type { ReactNode } from "react"

import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"

export function PageShell({ width = "max-w-[560px]", children }: { width?: string; children: ReactNode }) {
  return (
    <div
      className="min-h-screen w-full bg-cover bg-center bg-no-repeat"
      style={{ backgroundImage: "url(/background.webp)" }}
    >
      <div className="flex min-h-screen items-center justify-center px-5 py-10">
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
    </div>
  )
}
