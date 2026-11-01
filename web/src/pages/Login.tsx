import { useEffect, useRef, useState, type FormEvent } from "react"
import { Eye, EyeOff, Loader2, Lock, RefreshCw, User } from "lucide-react"
import { QRCodeSVG } from "qrcode.react"

import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { safeNext, type CaptchaConfig, type LoginFooter, type LoginPanel } from "@/lib/config"
import { createCaptcha, type CaptchaAnswer, type CaptchaInstance } from "@/lib/geetest"
import { cn } from "@/lib/utils"

type Props = {
  siteTitle: string
  panel: LoginPanel
  footer: LoginFooter
  captcha: CaptchaConfig
  qrLogin: boolean
}

type loginResponse = {
  error?: string
  captcha_required?: boolean
}

type qrChallenge = {
  id: string
  url: string
  expires_in: number
}

type qrPollResult = {
  status?: "pending" | "confirmed" | "failed"
  error?: string
}

type qrStatus = "loading" | "waiting" | "expired" | "failed"

function nextTarget() {
  return safeNext(new URLSearchParams(window.location.search).get("next") ?? "")
}

function usernameError(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ""
  if (trimmed.includes("@")) return "请填写学号或教工号，不要输入邮箱"
  if (!/^\d+$/.test(trimmed)) return "学号或教工号只能包含数字"
  return ""
}

export default function Login({ siteTitle, panel, footer, captcha, qrLogin }: Props) {
  const [mode, setMode] = useState<"password" | "qr">("password")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [revealed, setRevealed] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")
  const [gated, setGated] = useState(false)
  const [ready, setReady] = useState(false)
  const [qrImage, setQrImage] = useState<qrChallenge | null>(null)
  const [qrStatus, setQrStatus] = useState<qrStatus>("loading")
  const [qrError, setQrError] = useState("")
  const [qrAttempt, setQrAttempt] = useState(0)
  const [qrGated, setQrGated] = useState(false)
  const [qrCaptchaPending, setQrCaptchaPending] = useState(false)

  const widget = useRef<CaptchaInstance | null>(null)
  const submitRef = useRef<(answer?: CaptchaAnswer) => void>(() => {})
  const qrStartRef = useRef<(answer?: CaptchaAnswer) => void>(() => {})
  const captchaTargetRef = useRef<"password" | "qr">("password")
  const qrCaptchaOpenRef = useRef(false)
  const usernameRef = useRef<HTMLInputElement | null>(null)

  const needCaptcha = captcha.enabled && (captcha.required || gated || qrGated)
  const showPanel = panel.enabled && Boolean(panel.title || panel.body)
  const showFooter = footer.enabled && Boolean(footer.text.trim())
  const usernameInvalid = usernameError(username)

  useEffect(() => {
    submitRef.current = (answer) => {
      void submit(answer)
    }
  })

  useEffect(() => {
    if (!needCaptcha || !captcha.captchaId) return

    let alive = true
    createCaptcha(
      captcha.captchaId,
      (answer) => {
        if (captchaTargetRef.current === "qr") {
          qrCaptchaOpenRef.current = false
          setQrCaptchaPending(false)
          qrStartRef.current(answer)
        } else submitRef.current(answer)
      },
      () => setError("验证加载异常，请刷新页面后重试"),
      () => {
        if (!qrCaptchaOpenRef.current) return
        qrCaptchaOpenRef.current = false
        setQrCaptchaPending(false)
        setQrStatus("failed")
        setQrError("已取消验证，请点击刷新二维码")
      },
    )
      .then((instance) => {
        if (!alive) {
          instance.destroy()
          return
        }
        instance.onReady(() => setReady(true))
        widget.current = instance
      })
      .catch(() => {
        if (alive) setError("验证组件加载失败，请刷新页面后重试")
      })

    return () => {
      alive = false
      widget.current?.destroy()
      widget.current = null
    }
  }, [needCaptcha, captcha.captchaId])

  // 二维码生成被要求先过验证码时，等组件就绪后自动弹出，不用用户再点一次。
  useEffect(() => {
    if (mode !== "qr" || !qrCaptchaPending || !ready || !widget.current) return
    qrCaptchaOpenRef.current = true
    widget.current.showCaptcha()
  }, [mode, qrCaptchaPending, ready])

  useEffect(() => {
    if (mode !== "qr" || !qrLogin) return

    let alive = true
    let timer: number | undefined

    setQrImage(null)
    setQrError("")
    setQrStatus("loading")
    setQrCaptchaPending(false)

    const poll = async (id: string) => {
      try {
        const res = await fetch(`/api/login/qrcode/${encodeURIComponent(id)}`, { cache: "no-store" })
        const data = (await res.json().catch(() => null)) as qrPollResult | null
        if (!alive) return

        if (res.ok && data?.status === "confirmed") {
          window.location.replace(nextTarget())
          return
        }
        if (res.status === 410) {
          setQrStatus("expired")
          return
        }
        if (!res.ok) {
          setQrStatus("failed")
          setQrError(data?.error ?? "扫码登录失败，请刷新后重试")
          return
        }
      } catch {
        // 网络抖动继续轮询，状态交给下一次请求判断
      }
      if (!alive) return
      timer = window.setTimeout(() => void poll(id), 2000)
    }

    const start = async (answer?: CaptchaAnswer) => {
      try {
        const res = await fetch("/api/login/qrcode", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ captcha: answer }),
        })
        const data = (await res.json().catch(() => null)) as
          | (qrChallenge & { error?: string; captcha_required?: boolean })
          | null
        if (!alive) return
        if (data?.captcha_required) {
          widget.current?.reset()
          captchaTargetRef.current = "qr"
          setQrGated(true)
          setQrCaptchaPending(true)
          return
        }
        if (!res.ok || !data?.id) {
          setQrStatus("failed")
          setQrError(data?.error ?? "二维码获取失败，请刷新后重试")
          return
        }
        widget.current?.reset()
        setQrImage({ id: data.id, url: data.url, expires_in: data.expires_in })
        setQrStatus("waiting")
        void poll(data.id)
      } catch {
        if (!alive) return
        setQrStatus("failed")
        setQrError("网络异常，请刷新后重试")
      }
    }

    qrStartRef.current = (answer) => void start(answer)
    void start()

    return () => {
      alive = false
      if (timer) window.clearTimeout(timer)
    }
  }, [mode, qrLogin, qrAttempt])

  async function submit(answer?: CaptchaAnswer) {
    if (pending) return

    setError("")
    setPending(true)
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim(), password, captcha: answer }),
      })

      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as loginResponse | null
        setError(data?.error ?? "登录失败，请稍后重试")
        if (data?.captcha_required) setGated(true)
        if (answer) widget.current?.reset()
        return
      }

      window.location.replace(nextTarget())
    } catch {
      setError("网络异常，请稍后重试")
    } finally {
      setPending(false)
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return

    if (usernameInvalid) {
      usernameRef.current?.focus()
      return
    }

    if (needCaptcha) {
      if (!ready || !widget.current) {
        setError("验证正在加载，请稍后重试")
        return
      }
      setError("")
      captchaTargetRef.current = "password"
      widget.current.showCaptcha()
      return
    }

    void submit()
  }

  const qrHint =
    qrStatus === "waiting"
      ? "使用企业微信『扫一扫』扫描二维码"
      : qrStatus === "expired"
        ? "二维码已失效，请刷新"
        : qrStatus === "failed"
          ? qrError
          : qrCaptchaPending
            ? "请先完成验证"
            : "正在生成二维码"

  return (
    <div
      className="relative min-h-screen w-full bg-cover bg-center bg-no-repeat"
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

              {qrLogin && (
                <div className="mt-6 grid grid-cols-2 gap-1 rounded-lg border border-border/60 bg-muted p-1">
                  {(
                    [
                      ["password", "账号登录"],
                      ["qr", "微信扫码"],
                    ] as const
                  ).map(([value, label]) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => setMode(value)}
                      className={cn(
                        "h-8 rounded-md text-[13px] transition-colors",
                        mode === value
                          ? "bg-card text-foreground shadow-[0_1px_2px_rgba(16,24,40,0.08)]"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              )}

              {mode === "qr" ? (
                <div className="mt-5">
                  <div className="relative mx-auto flex size-[200px] items-center justify-center overflow-hidden rounded-xl border border-border bg-white">
                    {qrImage ? (
                      <QRCodeSVG value={qrImage.url} size={176} level="M" />
                    ) : qrStatus === "loading" ? (
                      <Loader2 className="size-5 animate-spin text-muted-foreground" />
                    ) : null}
                    {qrStatus === "expired" && (
                      <div className="absolute inset-0 flex items-center justify-center bg-white/90">
                        <span className="text-[13px] text-muted-foreground">二维码已失效</span>
                      </div>
                    )}
                  </div>

                  <p
                    className={cn(
                      "mt-3 min-h-[18px] text-center text-[12.5px] leading-[18px]",
                      qrStatus === "failed" ? "text-destructive" : "text-muted-foreground",
                    )}
                    role="status"
                  >
                    {qrHint}
                  </p>

                  <Button
                    type="button"
                    variant="outline"
                    className="mt-2 h-11 w-full"
                    disabled={qrStatus === "loading"}
                    onClick={() => setQrAttempt((value) => value + 1)}
                  >
                    <RefreshCw className="size-4" strokeWidth={1.75} />
                    刷新二维码
                  </Button>
                </div>
              ) : (
                <form className="mt-7" onSubmit={handleSubmit}>
                  <div className="relative">
                    <User
                      className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground"
                      strokeWidth={1.75}
                    />
                    <Input
                      ref={usernameRef}
                      className="h-11 pl-10"
                      placeholder="请输入教工号或学号"
                      aria-label="教工号或学号"
                      aria-invalid={Boolean(usernameInvalid)}
                      aria-describedby={usernameInvalid ? "username-error" : undefined}
                      autoComplete="username"
                      autoFocus
                      required
                      value={username}
                      onChange={(event) => setUsername(event.target.value)}
                    />
                  </div>
                  {usernameInvalid && (
                    <p
                      id="username-error"
                      className="mt-1.5 text-[12.5px] leading-[18px] text-destructive"
                    >
                      {usernameInvalid}
                    </p>
                  )}

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

                  <p
                    className="mt-2.5 mb-2.5 min-h-[18px] text-[12.5px] leading-[18px] text-destructive"
                    role="alert"
                  >
                    {error}
                  </p>

                  <Button type="submit" className="h-11 w-full" disabled={pending}>
                    {pending ? "登录中" : "登录"}
                  </Button>
                </form>
              )}
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

      {showFooter && (
        <p className="absolute inset-x-0 bottom-6 text-center text-[12px] leading-[1.8] whitespace-pre-line text-muted-foreground">
          {footer.text}
        </p>
      )}
    </div>
  )
}
