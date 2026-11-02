import { useEffect, useState } from "react"
import { Moon, Sun } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

function isDark() {
  return document.documentElement.classList.contains("dark")
}

function storedTheme() {
  try {
    return localStorage.getItem("theme")
  } catch {
    return null
  }
}

export function ThemeToggle({ className }: { className?: string }) {
  const [dark, setDark] = useState(isDark)

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)")
    const sync = () => {
      if (storedTheme()) return
      document.documentElement.classList.toggle("dark", media.matches)
      document.documentElement.style.colorScheme = media.matches ? "dark" : "light"
      setDark(media.matches)
    }
    media.addEventListener("change", sync)
    return () => media.removeEventListener("change", sync)
  }, [])

  function toggle() {
    const next = !dark
    document.documentElement.classList.toggle("dark", next)
    document.documentElement.style.colorScheme = next ? "dark" : "light"
    try {
      localStorage.setItem("theme", next ? "dark" : "light")
    } catch {
      // 隐私模式下无法写入时仅在当前页面生效
    }
    setDark(next)
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={dark ? "切换到浅色模式" : "切换到深色模式"}
      title={dark ? "切换到浅色模式" : "切换到深色模式"}
      onClick={toggle}
      className={cn(
        "rounded-full text-muted-foreground hover:bg-card/80 hover:text-foreground",
        className,
      )}
    >
      {dark ? <Sun className="size-4" strokeWidth={1.75} /> : <Moon className="size-4" strokeWidth={1.75} />}
    </Button>
  )
}
