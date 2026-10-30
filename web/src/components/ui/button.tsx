import type { ButtonHTMLAttributes } from "react"
import { cn } from "@/lib/utils"

export function Button({ className, ...props }: ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={cn(
        "inline-flex h-11 w-full items-center justify-center rounded-lg bg-brand px-4",
        "text-[14px] font-medium text-white transition-colors",
        "hover:bg-brand-hover focus-visible:ring-4 focus-visible:ring-brand/20 focus-visible:outline-none",
        "disabled:pointer-events-none disabled:opacity-60",
        className,
      )}
      {...props}
    />
  )
}
