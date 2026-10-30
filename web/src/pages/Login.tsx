import { useState, type FormEvent } from "react"
import { Eye, EyeOff, Lock, User } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { safeNext, type LoginPanel } from "@/lib/config"
import { cn } from "@/lib/utils"

type Props = {
  siteTitle: string
  panel: LoginPanel
}

function nextTarget() {
  return safeNext(new URLSearchParams(window.location.search).get("next") ?? "")
}

export default function Login({ siteTitle, panel }: Props) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [revealed, setRevealed] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")

  const showPanel = panel.enabled && Boolean(panel.title || panel.body)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    setError("")
    setPending(true)
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim(), password }),
      })

      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setError(data?.error ?? "登录失败，请稍后重试")
        return
      }

      window.location.replace(nextTarget())
    } catch {
      setError("网络异常，请稍后重试")
    } finally {
      setPending(false)
    }
  }

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
            showPanel ? "max-w-[800px]" : "max-w-[400px]",
          )}
        >
          <div className={cn(showPanel && "md:grid md:grid-cols-[minmax(0,1fr)_296px]")}>
            <div className="px-7 py-9 sm:px-10">
              {siteTitle && (
                <div className="flex items-center gap-2">
                  <span className="size-1.5 rounded-full bg-primary" />
                  <span className="text-[12px] tracking-[0.12em] text-muted-foreground">
                    {siteTitle}
                  </span>
                </div>
              )}

              <h1
                className={cn(
                  "text-[21px] font-medium tracking-tight text-foreground",
                  siteTitle && "mt-6",
                )}
              >
                统一身份认证
              </h1>
              <p className="mt-1.5 text-[13px] text-muted-foreground">
                欢迎回来，请使用以下方式登录
              </p>

              <form className="mt-7" onSubmit={handleSubmit}>
                <div className="relative">
                  <User
                    className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground"
                    strokeWidth={1.75}
                  />
                  <Input
                    className="h-11 pl-10"
                    placeholder="请输入教工号或学号"
                    aria-label="教工号或学号"
                    autoComplete="username"
                    autoFocus
                    required
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                  />
                </div>

                <div className="relative mt-3">
                  <Lock
                    className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground"
                    strokeWidth={1.75}
                  />
                  <Input
                    className="h-11 pr-11 pl-10"
                    type={revealed ? "text" : "password"}
                    placeholder="请输入密码"
                    aria-label="密码"
                    autoComplete="current-password"
                    required
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={revealed ? "隐藏密码" : "显示密码"}
                    onClick={() => setRevealed((value) => !value)}
                    className="absolute top-1/2 right-2 -translate-y-1/2 text-muted-foreground hover:bg-transparent"
                  >
                    {revealed ? (
                      <EyeOff className="size-4" strokeWidth={1.75} />
                    ) : (
                      <Eye className="size-4" strokeWidth={1.75} />
                    )}
                  </Button>
                </div>

                <p className="mt-2 min-h-[18px] text-[12.5px] text-destructive" role="alert">
                  {error}
                </p>

                <Button
                  type="submit"
                  className="h-11 w-full"
                  disabled={pending}
                >
                  {pending ? "登录中" : "登录"}
                </Button>
              </form>
            </div>

            {showPanel && (
              <aside className="hidden flex-col justify-center gap-3 border-l border-border bg-muted px-7 py-10 md:flex">
                {panel.title && (
                  <div className="text-[15px] font-medium text-foreground">{panel.title}</div>
                )}
                {panel.body && (
                  <p className="text-[13px] leading-[1.9] whitespace-pre-line text-muted-foreground">
                    {panel.body}
                  </p>
                )}
              </aside>
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}
