import { useEffect, useRef, useState, type FormEvent } from "react"
import { Eye, EyeOff, Fingerprint, Loader2, Lock, RefreshCw, User } from "lucide-react"
import { QRCodeSVG } from "qrcode.react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { safeNext, type CaptchaConfig, type LoginFooter, type LoginPanel } from "@/lib/config"
import { createCaptcha, type CaptchaAnswer, type CaptchaInstance } from "@/lib/geetest"
import {
  getPasskey,
  passkeyErrorMessage,
  passkeySupported,
} from "@/lib/passkey"
import { cn } from "@/lib/utils"

type Props = {
  siteTitle: string
  panel: LoginPanel
  footer: LoginFooter
  captcha: CaptchaConfig
  qrLogin: boolean
  passkey: boolean
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

type passkeyOptions = {
  publicKey?: Parameters<typeof getPasskey>[0]
  error?: string
  captcha_required?: boolean
}

// 预取的挑战只在两分钟内直接复用，留足余量给服务端五分钟的过期时间。
const passkeyPrefetchTTL = 120_000

function nextTarget() {
  return safeNext(new URLSearchParams(window.location.search).get("next") ?? "")
}

const forgotPasswordUrl = "https://newcas.gzhu.edu.cn/cas/pwd?locale=zh_CN"

const deleteErrors: Record<string, string> = {
  invalid: "确认链接无效或已过期",
  forbidden: "管理员账号不能自助注销",
  failed: "注销失败，请稍后重试",
}

function accountNotice() {
  const params = new URLSearchParams(window.location.search)
  if (params.get("account_deleted")) return "账号已注销，本站保存的账号数据已删除"
  return deleteErrors[params.get("account_delete_error") ?? ""] ?? ""
}

function usernameError(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return ""
  if (trimmed.includes("@")) return "请填写学号或教工号，不要输入邮箱"
  if (!/^\d+$/.test(trimmed)) return "学号或教工号只能包含数字"
  return ""
}

type loginMode = "password" | "qr" | "passkey"
type captchaTarget = "password" | "qr" | "passkey"

export default function Login({ siteTitle, panel, footer, captcha, qrLogin, passkey }: Props) {
  const [mode, setMode] = useState<loginMode>("password")
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
  const [passkeyBusy, setPasskeyBusy] = useState(false)
  const [passkeyError, setPasskeyError] = useState("")
  const [passkeyGated, setPasskeyGated] = useState(false)
  const [captchaPending, setCaptchaPending] = useState<captchaTarget | null>(null)

  const widget = useRef<CaptchaInstance | null>(null)
  const submitRef = useRef<(answer?: CaptchaAnswer) => void>(() => {})
  const qrStartRef = useRef<(answer?: CaptchaAnswer) => void>(() => {})
  const passkeyRef = useRef<(answer?: CaptchaAnswer) => void>(() => {})
  const prefetched = useRef<{ options: passkeyOptions; at: number } | null>(null)
  const prefetching = useRef<Promise<void> | null>(null)
  const captchaTargetRef = useRef<captchaTarget>("password")
  const qrCaptchaOpenRef = useRef(false)
  const usernameRef = useRef<HTMLInputElement | null>(null)

  const showPasskey = passkey && passkeySupported()
  const tabs: { value: loginMode; label: string }[] = [
    { value: "password", label: "账号登录" },
    ...(qrLogin ? [{ value: "qr", label: "微信扫码" } as const] : []),
    ...(showPasskey ? [{ value: "passkey", label: "通行密钥" } as const] : []),
  ]
  const needCaptcha = captcha.enabled && (captcha.required || gated || qrGated || passkeyGated)
  const showPanel = panel.enabled && Boolean(panel.title || panel.body)
  const showFooter = footer.enabled && Boolean(footer.text.trim())
  const usernameInvalid = usernameError(username)
  const notice = accountNotice()

  function captchaFailed(message: string) {
    setCaptchaPending(null)
    if (captchaTargetRef.current === "qr") {
      setQrStatus("failed")
      setQrError(message)
      return
    }
    if (captchaTargetRef.current === "passkey") {
      setPasskeyError(message)
      return
    }
    setError(message)
  }

  function showWidget() {
    const instance = widget.current
    if (!instance) return false

    try {
      instance.reset()
      instance.showCaptcha()
      return true
    } catch {
      captchaFailed("验证码打开失败，请刷新页面后重试")
      return false
    }
  }

  // 服务端要求验证时立刻唤起；组件还没就绪就先排队，等 ready 后自动弹出。
  function openCaptcha(target: captchaTarget) {
    captchaTargetRef.current = target
    qrCaptchaOpenRef.current = target === "qr"
    if (target === "qr") {
      setQrGated(true)
    } else if (target === "passkey") {
      setPasskeyGated(true)
    } else {
      setGated(true)
    }

    if (ready && widget.current) {
      if (showWidget()) setCaptchaPending(null)
      return
    }
    setCaptchaPending(target)
  }

  useEffect(() => {
    submitRef.current = (answer) => {
      void submit(answer)
    }
  })

  useEffect(() => {
    passkeyRef.current = (answer) => {
      void passkeyLogin(answer)
    }
  })

  useEffect(() => {
    prefetchPasskey()
  }, [showPasskey])

  useEffect(() => {
    if (!needCaptcha || !captcha.captchaId) return

    let alive = true
    createCaptcha(
      captcha.captchaId,
      (answer) => {
        setCaptchaPending(null)
        if (captchaTargetRef.current === "qr") {
          qrCaptchaOpenRef.current = false
          qrStartRef.current(answer)
          return
        }
        if (captchaTargetRef.current === "passkey") {
          passkeyRef.current(answer)
          return
        }
        submitRef.current(answer)
      },
      () => captchaFailed("验证加载异常，请刷新页面后重试"),
      () => {
        setCaptchaPending(null)
        if (!qrCaptchaOpenRef.current) return
        qrCaptchaOpenRef.current = false
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
        if (alive) captchaFailed("验证组件加载失败，请刷新页面后重试")
      })

    return () => {
      alive = false
      widget.current?.destroy()
      widget.current = null
    }
  }, [needCaptcha, captcha.captchaId])

  // 组件还在加载时先排队，就绪后自动弹出，不用用户再点一次。
  useEffect(() => {
    if (!captchaPending || !ready || !widget.current) return
    showWidget()
    setCaptchaPending(null)
  }, [captchaPending, ready])

  useEffect(() => {
    if (mode !== "qr" || !qrLogin) return

    let alive = true
    let timer: number | undefined

    setQrImage(null)
    setQrError("")
    setQrStatus("loading")

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
          openCaptcha("qr")
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
        if (data?.captcha_required) openCaptcha("password")
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

  async function requestPasskeyOptions(answer?: CaptchaAnswer) {
    const res = await fetch("/api/login/passkey/options", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ captcha: answer }),
    })
    return { ok: res.ok, options: (await res.json().catch(() => null)) as passkeyOptions | null }
  }

  // 取一次可用的挑战：有新鲜的预取就直接用，否则发新请求。
  // 先等掉还在路上的预取，避免两条响应的 Set-Cookie 交错，让挑战和 cookie 对不上。
  async function takePasskeyOptions(answer?: CaptchaAnswer) {
    if (prefetching.current) await prefetching.current

    const cached = prefetched.current
    prefetched.current = null
    if (!answer && cached && Date.now() - cached.at < passkeyPrefetchTTL) {
      return { ok: true, options: cached.options }
    }
    return requestPasskeyOptions(answer)
  }

  // 挑战先取好，点击时只剩系统弹窗这一段等待。
  function prefetchPasskey() {
    if (!showPasskey || prefetching.current) return

    const cached = prefetched.current
    if (cached && Date.now() - cached.at < passkeyPrefetchTTL) return

    prefetching.current = requestPasskeyOptions()
      .then(({ ok, options }) => {
        if (ok && options?.publicKey) prefetched.current = { options, at: Date.now() }
      })
      .catch(() => {})
      .finally(() => {
        prefetching.current = null
      })
  }

  // 切到通行密钥的同时就发起验证，省掉再点一次。
  function selectMode(next: loginMode) {
    setMode(next)
    if (next === "passkey" && mode !== "passkey") void passkeyLogin()
  }

  async function passkeyLogin(answer?: CaptchaAnswer) {
    if (passkeyBusy) return

    setPasskeyError("")
    setPasskeyBusy(true)
    try {
      const { ok, options } = await takePasskeyOptions(answer)

      if (options?.captcha_required) {
        openCaptcha("passkey")
        return
      }
      if (!ok || !options?.publicKey) {
        setPasskeyError(options?.error ?? "无法发起通行密钥登录")
        prefetchPasskey()
        return
      }

      const credential = await getPasskey(options.publicKey)
      const finish = await fetch("/api/login/passkey", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(credential),
      })
      const result = (await finish.json().catch(() => null)) as loginResponse | null

      if (!finish.ok) {
        setPasskeyError(result?.error ?? "通行密钥登录失败，请重试")
        if (result?.captcha_required) openCaptcha("passkey")
        prefetchPasskey()
        return
      }

      window.location.replace(nextTarget())
    } catch (err) {
      setPasskeyError(passkeyErrorMessage(err))
    } finally {
      setPasskeyBusy(false)
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
      setError("")
      openCaptcha("password")
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
          : captchaPending === "qr"
            ? "请先完成验证"
            : "正在生成二维码"

  return (
    <PageShell
      width={showPanel ? "max-w-[800px]" : "max-w-[400px]"}
      footer={showFooter ? footer.text : undefined}
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

          {notice && (
            <p className="mt-4 rounded-lg bg-secondary px-3 py-2 text-[12.5px] leading-[18px] text-foreground">
              {notice}
            </p>
          )}

          {tabs.length > 1 && (
            <div
              className={cn(
                "mt-6 grid gap-1 rounded-lg border border-border/60 bg-muted p-1",
                tabs.length === 3 ? "grid-cols-3" : "grid-cols-2",
              )}
            >
              {tabs.map((tab) => (
                <button
                  key={tab.value}
                  type="button"
                  onClick={() => selectMode(tab.value)}
                  onPointerEnter={() => {
                    if (tab.value === "passkey") prefetchPasskey()
                  }}
                  onFocus={() => {
                    if (tab.value === "passkey") prefetchPasskey()
                  }}
                  className={cn(
                    "h-8 rounded-md text-[13px] transition-colors",
                    mode === tab.value
                      ? "bg-card text-foreground shadow-[0_1px_2px_rgba(16,24,40,0.08)]"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          )}

          {mode === "passkey" ? (
            <div className="mt-6 flex min-h-[200px] flex-col items-center justify-center">
              <button
                type="button"
                disabled={passkeyBusy}
                onClick={() => void passkeyLogin()}
                aria-label="使用通行密钥登录"
                className="flex size-[76px] items-center justify-center rounded-full border border-border bg-muted text-primary transition-colors hover:border-primary/40 hover:bg-primary/5 disabled:opacity-70"
              >
                {passkeyBusy ? (
                  <Loader2 className="size-7 animate-spin" strokeWidth={1.5} />
                ) : (
                  <Fingerprint className="size-7" strokeWidth={1.5} />
                )}
              </button>
              <p
                className={cn(
                  "mt-4 min-h-[18px] px-2 text-center text-[12.5px] leading-[18px]",
                  passkeyError ? "text-destructive" : "text-muted-foreground",
                )}
                role="status"
              >
                {passkeyError ||
                  (passkeyBusy
                    ? "请在设备上完成验证"
                    : "点击按系统提示使用指纹、面容或设备 PIN")}
              </p>
            </div>
          ) : mode === "qr" ? (
            <div className="mt-5">
              <div className="relative mx-auto flex size-[200px] items-center justify-center overflow-hidden rounded-xl border border-border bg-white">
                {qrImage ? (
                  <QRCodeSVG value={qrImage.url} size={176} level="M" />
                ) : qrStatus === "loading" ? (
                  <Loader2 className="size-5 animate-spin text-muted-foreground" />
                ) : null}
                {qrStatus === "expired" && (
                  <div className="absolute inset-0 flex items-center justify-center bg-white/90">
                    <span className="text-[13px] text-zinc-600">二维码已失效</span>
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

              <a
                href={forgotPasswordUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="mt-3.5 block text-center text-[12.5px] text-muted-foreground transition-colors hover:text-primary"
              >
                忘记密码
              </a>
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
    </PageShell>
  )
}
