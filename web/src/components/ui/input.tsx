import type { InputHTMLAttributes } from "react"
import { cn } from "@/lib/utils"

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "h-11 w-full rounded-lg border border-zinc-200 bg-white text-[14px] text-zinc-900",
        "placeholder:text-zinc-400 transition outline-none",
        "focus:border-brand focus:ring-4 focus:ring-brand/10",
        "disabled:bg-zinc-50 disabled:text-zinc-400",
        className,
      )}
      {...props}
    />
  )
}
