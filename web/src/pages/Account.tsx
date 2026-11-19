import { useEffect, useState, type ReactNode } from "react"
import { OverlayScrollbarsComponent } from "overlayscrollbars-react"
import { ChevronLeft, ChevronRight, Link2, Loader2, PenLine, ShieldCheck } from "lucide-react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import { currentPath, safeNext } from "@/lib/config"
import { eventNames, loginMethods, reasonNames } from "@/lib/auth-labels"
import {
  createPasskey,
  defaultDeviceName,
  passkeyErrorMessage,
  passkeySupported,
} from "@/lib/passkey"

type GroupBrief = { id: number; name: string; code: string; is_upstream: boolean }

type Me = {
  id: string
  name: string
  email: string
  type: number
  mail_enabled?: boolean
  avatar?: string
  belong_class: string
  groups: GroupBrief[]
  led_groups: GroupBrief[]
  bindings: { provider: string; display_name: string; bound: boolean; account?: string }[]
}

const bindErrors: Record<string, string> = {
  state: "绑定会话已失效，请重新发起",
  denied: "你取消了在绑定源上的授权",
  exchange: "绑定源未返回有效身份",
  store: "保存绑定关系失败",
}

type PasskeyDevice = {
  id: string
  name: string
  synced: boolean
  created_at: string
  last_used_at: string | null
}

type LoginSession = {
  id: string
  method: string
  ip: string
  user_agent: string
  created_at: string
  current: boolean
}

type Authorization = {
  client_id: string
  name: string
  homepage_url: string
  scopes: string[]
  granted_at: string
  last_used_at: string | null
}

type AuthEvent = {
  id: number
  occurred_at: string
  kind: string
  client_name?: string
  result: string
  reason?: string
}

type View =
  | "main"
  | "profile"
  | "passkey"
  | "sessions"
  | "authorizations"
  | "bindings"
  | "events"
  | "delete"

const sectionViews = new Set<View>([
  "profile",
  "passkey",
  "sessions",
  "authorizations",
  "bindings",
  "events",
  "delete",
])

const scrollOptions = {
  scrollbars: {
    theme: "os-theme-relay",
    autoHide: "never",
    autoHideDelay: 700,
    clickScroll: true,
  },
} as const

function parseView(): View {
  const value = new URLSearchParams(window.location.search).get("section")
  return value && sectionViews.has(value as View) ? (value as View) : "main"
}

function sectionURL(view: View) {
  const url = new URL(window.location.href)
  if (view === "main") url.searchParams.delete("section")
  else url.searchParams.set("section", view)
  return url.pathname + url.search + url.hash
}

const emailErrors: Record<string, string> = {
  invalid: "确认链接无效或已过期",
  taken: "该邮箱已被其他账号使用",
  failed: "切换邮箱失败，请稍后重试",
}

function Row({ label, value, action }: { label: string; value: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-6 border-b border-border py-2.5 last:border-b-0">
      <dt className="text-[13px] text-muted-foreground">{label}</dt>
      <dd className="flex items-center gap-2 text-right text-[13.5px] break-all text-foreground">
        {value}
        {action}
      </dd>
    </div>
  )
}

function SettingTile({
  label,
  hint,
  onClick,
}: {
  label: string
  hint: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex h-[70px] items-center justify-between gap-3 rounded-lg border border-border px-4 py-3 text-left transition-colors hover:bg-secondary/50"
    >
      <div className="min-w-0">
        <div className="truncate text-[13px] text-foreground">{label}</div>
        <div className="mt-0.5 truncate text-[12px] text-muted-foreground">{hint}</div>
      </div>
      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" strokeWidth={2.25} />
    </button>
  )
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  })
}

function formatDateTime(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
}

export default function Account({ siteTitle, passkey }: { siteTitle: string; passkey: boolean }) {
  const [me, setMe] = useState<Me | null>(null)
  const [notice, setNotice] = useState("")
  const [emailOpen, setEmailOpen] = useState(false)
  const [newEmail, setNewEmail] = useState("")
  const [emailBusy, setEmailBusy] = useState(false)
  const [emailMsg, setEmailMsg] = useState("")
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteMsg, setDeleteMsg] = useState("")
  const [devices, setDevices] = useState<PasskeyDevice[]>([])
  const [deviceFormOpen, setDeviceFormOpen] = useState(false)
  const [deviceName, setDeviceName] = useState("")
  const [deviceBusy, setDeviceBusy] = useState(false)
  const [deviceMsg, setDeviceMsg] = useState("")
  const [confirmId, setConfirmId] = useState("")
  const [upstreamOpen, setUpstreamOpen] = useState(false)
  const [avatarFailed, setAvatarFailed] = useState(false)
  const [loginSessions, setLoginSessions] = useState<LoginSession[]>([])
  const [authorizations, setAuthorizations] = useState<Authorization[]>([])
  const [authEvents, setAuthEvents] = useState<AuthEvent[]>([])
  const [view, setView] = useState<View>(parseView)

  const passkeyEnabled = passkey && passkeySupported()

  const params = new URLSearchParams(window.location.search)
  const highlight = params.get("bind") ?? ""
  const bound = params.get("bound") ?? ""
  const bindError = params.get("bind_error") ?? ""
  const emailChanged = params.get("email_changed") ?? ""
  const emailError = params.get("email_error") ?? ""
  const resumeTo = params.get("next") ?? ""

  useEffect(() => {
    loadMe()
  }, [])

  useEffect(() => {
    void loadSecurity()
  }, [])

  useEffect(() => {
    if (passkeyEnabled) void loadDevices()
  }, [passkeyEnabled])

  useEffect(() => {
    if (bindError && bindErrors[bindError]) setNotice(bindErrors[bindError])
  }, [bindError])

  useEffect(() => {
    if (!highlight) return
    history.replaceState({ accountView: "bindings" }, "", sectionURL("bindings"))
    setView("bindings")
  }, [highlight])

  useEffect(() => {
    const onPop = () => setView(parseView())
    window.addEventListener("popstate", onPop)
    return () => window.removeEventListener("popstate", onPop)
  }, [])

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [view])

  useEffect(() => {
    if (emailChanged) setNotice("邮箱已更新")
    if (emailError && emailErrors[emailError]) setNotice(emailErrors[emailError])
  }, [emailChanged, emailError])

  async function loadMe() {
    const res = await fetch("/api/me")
    if (res.status === 401) {
      window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
      return
    }
    if (!res.ok) {
      setNotice("加载用户信息失败")
      return
    }
    setMe((await res.json()) as Me)

    // 绑定是授权流程中的一个中转步骤，完成后要回到原来那个应用的授权请求。
    if (bound && resumeTo) {
      window.location.replace(safeNext(resumeTo))
      return
    }
    if (bound) setNotice(`已绑定 ${bound}`)
  }

  async function unbind(provider: string) {
    const res = await fetch(`/api/bindings/${encodeURIComponent(provider)}`, { method: "DELETE" })
    if (!res.ok) {
      setNotice("解除绑定失败")
      return
    }
    setNotice("已解除绑定")
    await loadMe()
  }

  async function loadSecurity() {
    const [sessionRes, authRes, eventRes] = await Promise.all([
      fetch("/api/me/sessions"),
      fetch("/api/me/authorizations"),
      fetch("/api/me/events?limit=10"),
    ])
    if (sessionRes.status === 401) {
      window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
      return
    }
    if (sessionRes.ok) setLoginSessions((await sessionRes.json()) as LoginSession[])
    if (authRes.ok) setAuthorizations((await authRes.json()) as Authorization[])
    if (eventRes.ok) {
      const data = (await eventRes.json().catch(() => null)) as { events?: AuthEvent[] } | null
      setAuthEvents(data?.events ?? [])
    }
  }

  async function revokeSession(id: string) {
    const res = await fetch(`/api/me/sessions/${encodeURIComponent(id)}`, { method: "DELETE" })
    if (!res.ok) {
      setNotice("下线设备失败")
      return
    }
    setNotice("该设备已下线")
    await loadSecurity()
  }

  async function revokeAuthorization(clientId: string) {
    const res = await fetch(`/api/me/authorizations/${encodeURIComponent(clientId)}`, { method: "DELETE" })
    if (!res.ok) {
      setNotice("撤销授权失败")
      return
    }
    setNotice("已撤销该应用的授权")
    await loadSecurity()
  }

  async function loadDevices() {
    const res = await fetch("/api/passkeys")
    if (res.status === 401) {
      window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
      return
    }
    if (!res.ok) return
    const data = (await res.json().catch(() => null)) as { passkeys?: PasskeyDevice[] } | null
    setDevices(data?.passkeys ?? [])
  }

  function startAddDevice() {
    setDeviceMsg("")
    setDeviceName(defaultDeviceName())
    setDeviceFormOpen(true)
  }

  async function addDevice() {
    setDeviceBusy(true)
    setDeviceMsg("")
    try {
      const res = await fetch("/api/passkeys/options", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: deviceName.trim() }),
      })
      if (res.status === 401) {
        window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
        return
      }
      const options = (await res.json().catch(() => null)) as
        | { publicKey?: Parameters<typeof createPasskey>[0]; error?: string }
        | null
      if (!res.ok || !options?.publicKey) {
        setDeviceMsg(options?.error ?? "添加失败，请稍后重试")
        return
      }

      const credential = await createPasskey(options.publicKey)
      const created = await fetch("/api/passkeys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(credential),
      })
      const body = (await created.json().catch(() => null)) as
        | { name?: string; error?: string }
        | null
      if (!created.ok) {
        setDeviceMsg(body?.error ?? "添加失败，请稍后重试")
        return
      }

      setDeviceFormOpen(false)
      setNotice(`已添加通行密钥「${body?.name ?? deviceName}」`)
      await loadDevices()
    } catch (error) {
      setDeviceMsg(passkeyErrorMessage(error))
    } finally {
      setDeviceBusy(false)
    }
  }

  async function removeDevice(device: PasskeyDevice) {
    const res = await fetch(`/api/passkeys/${encodeURIComponent(device.id)}`, { method: "DELETE" })
    if (!res.ok) {
      setNotice("解除通行密钥失败")
      return
    }
    setConfirmId("")
    setNotice(`已解除「${device.name}」`)
    await loadDevices()
  }

  async function submitEmail() {
    setEmailBusy(true)
    setEmailMsg("")
    try {
      const res = await fetch("/api/email/change", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: newEmail.trim() }),
      })
      if (res.status === 401) {
        window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
        return
      }
      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setEmailMsg(data?.error ?? "提交失败，请稍后重试")
        return
      }
      setEmailOpen(false)
      setNotice("确认邮件已发送到新邮箱，请在 30 分钟内打开邮件中的链接完成确认")
      setNewEmail("")
    } finally {
      setEmailBusy(false)
    }
  }

  async function logout() {
    await fetch("/api/logout", { method: "POST" })
    window.location.replace("/login")
  }

  async function requestDeletion() {
    setDeleteBusy(true)
    setDeleteMsg("")
    try {
      const res = await fetch("/api/account", { method: "DELETE" })
      if (res.status === 401) {
        window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
        return
      }
      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setDeleteMsg(data?.error ?? "提交失败，请稍后重试")
        return
      }
      goMain()
      setNotice(`确认邮件已发送到 ${me?.email ?? "账号邮箱"}，请在 30 分钟内打开邮件中的链接完成注销`)
    } finally {
      setDeleteBusy(false)
    }
  }

  function navigate(next: View) {
    if (next === view) return
    history.pushState({ accountView: next }, "", sectionURL(next))
    setView(next)
  }

  function goMain() {
    history.replaceState({ accountView: "main" }, "", sectionURL("main"))
    setView("main")
  }

  function goBack() {
    if (history.state?.accountView) {
      history.back()
      return
    }
    goMain()
  }

  if (!me) {
    return (
      <PageShell>
        <p className="py-16 text-center text-[13px] text-muted-foreground">加载中</p>
      </PageShell>
    )
  }

  const localGroups = me.groups.filter((group) => !group.is_upstream)
  const upstreamGroups = me.groups.filter((group) => group.is_upstream)

  const groupChip = (group: GroupBrief) => (
    <span
      key={group.id}
      className="rounded-md bg-secondary px-2 py-1 text-[12px] text-secondary-foreground"
    >
      {group.name}
    </span>
  )

  const user = me

  function subHeader(title: string, description?: string) {
    return (
      <>
        <button
          type="button"
          onClick={goBack}
          className="-ml-1 flex items-center gap-1 text-[12px] text-muted-foreground transition-colors hover:text-foreground"
        >
          <ChevronLeft className="size-4" strokeWidth={2} />
          返回
        </button>
        <h1 className="mt-4 text-[21px] font-medium tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1.5 text-[13px] text-muted-foreground">{description}</p>}
      </>
    )
  }

  function subpageMeta() {
    switch (view) {
      case "profile":
        return { title: "个人资料", description: "账号信息与所属用户组" }
      case "passkey":
        return { title: "通行密钥", description: "用指纹、面容或设备 PIN 代替密码登录" }
      case "sessions":
        return { title: "登录设备", description: `${loginSessions.length} 台设备使用当前账号` }
      case "authorizations":
        return { title: "已授权应用", description: `${authorizations.length} 个应用` }
      case "bindings":
        return { title: "账号绑定", description: "部分应用要求先完成绑定" }
      case "events":
        return { title: "登录与授权记录", description: "最近 10 条" }
      case "delete":
        return { title: "注销账号", description: `确认邮件将发送到 ${user.email}` }
      default:
        return { title: "", description: "" }
    }
  }

  function renderSubPage() {
    switch (view) {
      case "profile":
        return (
          <>
            <dl className="overflow-hidden rounded-lg border border-border px-4">
              <Row label="学号" value={user.id} />
              <Row label="姓名" value={user.name} />
              <Row
                label="邮箱"
                value={user.email}
                action={
                  user.mail_enabled ? (
                    <button
                      type="button"
                      title="修改邮箱别名"
                      onClick={() => {
                        setEmailMsg("")
                        setNewEmail("")
                        setEmailOpen(true)
                      }}
                      className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                    >
                      <PenLine className="size-3.5" strokeWidth={1.75} />
                    </button>
                  ) : undefined
                }
              />
              {user.belong_class && <Row label="班级" value={user.belong_class} />}
            </dl>

            {user.groups.length > 0 && (
              <div className="mt-4">
                <div className="text-[12px] text-muted-foreground">用户组</div>
                {localGroups.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-2">{localGroups.map(groupChip)}</div>
                )}
                {upstreamGroups.length > 0 && (
                  <div className="mt-2">
                    <button
                      type="button"
                      aria-expanded={upstreamOpen}
                      onClick={() => setUpstreamOpen((open) => !open)}
                      className="flex items-center gap-1 text-[12px] text-muted-foreground transition-colors hover:text-foreground"
                    >
                      <ChevronRight
                        className={cn("size-3.5 transition-transform", upstreamOpen && "rotate-90")}
                        strokeWidth={2.25}
                      />
                      上游用户组 {upstreamGroups.length}
                    </button>
                    {upstreamOpen && (
                      <div className="mt-1.5 flex flex-wrap gap-2">{upstreamGroups.map(groupChip)}</div>
                    )}
                  </div>
                )}
              </div>
            )}

            {user.type !== 1 && user.mail_enabled && (
              <div className="mt-5 flex flex-wrap items-start justify-between gap-3 rounded-lg border border-destructive/30 px-4 py-3">
                <div className="min-w-0">
                  <div className="text-[13px] text-foreground">注销账号</div>
                  <p className="mt-1 text-[12px] leading-[1.7] text-muted-foreground">
                    永久删除账号数据，需要邮件确认。
                  </p>
                </div>
                <Button
                  variant="outline"
                  className="h-8 w-auto shrink-0 px-3 text-destructive hover:text-destructive"
                  onClick={() => {
                    setDeleteMsg("")
                    navigate("delete")
                  }}
                >
                  注销
                </Button>
              </div>
            )}

            <Modal
              open={emailOpen}
              title="修改邮箱"
              description={`当前邮箱 ${user.email}`}
              onClose={() => setEmailOpen(false)}
              footer={
                <>
                  <Button
                    variant="outline"
                    className="h-9 w-auto px-4"
                    onClick={() => setEmailOpen(false)}
                  >
                    取消
                  </Button>
                  <Button
                    className="h-9 w-auto px-4"
                    disabled={!newEmail.trim() || emailBusy}
                    onClick={submitEmail}
                  >
                    {emailBusy ? "发送中" : "发送确认邮件"}
                  </Button>
                </>
              }
            >
              <div className="space-y-3">
                <label className="block">
                  <span className="text-[12.5px] text-muted-foreground">新邮箱</span>
                  <Input
                    className="mt-1.5 h-9"
                    placeholder={user.email}
                    value={newEmail}
                    onChange={(e) => setNewEmail(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && newEmail.trim() && !emailBusy) submitEmail()
                    }}
                  />
                </label>
                <p className="text-[12px] text-muted-foreground">
                  只能修改 @ 前的别名部分，后缀需保持不变；确认邮件将发送到新邮箱，打开链接进入确认页并点击确认后才生效。
                </p>
                {emailMsg && (
                  <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">
                    {emailMsg}
                  </p>
                )}
              </div>
            </Modal>
          </>
        )

      case "passkey":
        return (
          <>
            <div className="space-y-2">
              {devices.length === 0 ? (
                <p className="rounded-lg border border-border px-4 py-8 text-center text-[12.5px] text-muted-foreground">
                  还没有绑定任何设备
                </p>
              ) : (
                devices.map((device) => (
                  <div
                    key={device.id}
                    className="flex items-center justify-between gap-3 rounded-lg border border-border px-4 py-3"
                  >
                    <div className="min-w-0">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate text-[13px] text-foreground">{device.name}</span>
                        {device.synced && (
                          <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[11px] text-muted-foreground">
                            可同步
                          </span>
                        )}
                      </div>
                      <div className="mt-0.5 text-[11.5px] text-muted-foreground">
                        {formatDate(device.created_at)} 添加
                        {device.last_used_at ? ` · ${formatDate(device.last_used_at)} 使用` : ""}
                      </div>
                    </div>
                    {confirmId === device.id ? (
                      <div className="flex shrink-0 items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-8 text-destructive hover:text-destructive"
                          onClick={() => removeDevice(device)}
                        >
                          确认解除
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-8 text-muted-foreground"
                          onClick={() => setConfirmId("")}
                        >
                          取消
                        </Button>
                      </div>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 shrink-0 text-muted-foreground"
                        onClick={() => setConfirmId(device.id)}
                      >
                        解除
                      </Button>
                    )}
                  </div>
                ))
              )}
            </div>
            <p className="mt-3 text-[12px] leading-[1.7] text-muted-foreground">
              私钥保存在设备上，本站只保存公钥。解除后该设备无法再用它登录，密码登录不受影响。
            </p>
            {deviceFormOpen ? (
              <div className="mt-5 rounded-lg border border-border p-4">
                <label className="block">
                  <span className="text-[12.5px] text-muted-foreground">设备名称</span>
                  <Input
                    className="mt-1.5 h-9"
                    maxLength={40}
                    value={deviceName}
                    onChange={(e) => setDeviceName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !deviceBusy) addDevice()
                    }}
                  />
                </label>
                <p className="mt-2 text-[12px] leading-[1.7] text-muted-foreground">
                  点击开始添加后，按浏览器提示使用指纹、面容、设备 PIN 或安全密钥完成验证。
                </p>
                {deviceMsg && (
                  <p className="mt-2 rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">
                    {deviceMsg}
                  </p>
                )}
                <div className="mt-3 flex justify-end gap-2">
                  <Button
                    variant="outline"
                    className="h-9 w-auto px-4"
                    onClick={() => setDeviceFormOpen(false)}
                  >
                    取消
                  </Button>
                  <Button className="h-9 w-auto px-4" disabled={deviceBusy} onClick={addDevice}>
                    {deviceBusy && <Loader2 className="size-3.5 animate-spin" strokeWidth={1.75} />}
                    {deviceBusy ? "等待设备确认" : "开始添加"}
                  </Button>
                </div>
              </div>
            ) : (
              <Button className="mt-5 h-9 w-auto px-4" onClick={startAddDevice}>
                添加设备
              </Button>
            )}
          </>
        )

      case "sessions":
        return (
          <>
            <div className="space-y-2">
              {loginSessions.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center justify-between gap-4 rounded-lg border border-border px-4 py-3"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 text-[13.5px] text-foreground">
                      {loginMethods[item.method] ?? "登录会话"}
                      {item.current && (
                        <span className="rounded bg-secondary px-1.5 py-0.5 text-[11px] text-muted-foreground">
                          本机
                        </span>
                      )}
                    </div>
                    <div className="truncate text-[12px] text-muted-foreground" title={item.user_agent}>
                      {item.ip || "未知地址"} · {formatDateTime(item.created_at)}
                    </div>
                  </div>
                  {!item.current && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-8 shrink-0 text-muted-foreground"
                      onClick={() => revokeSession(item.id)}
                    >
                      下线
                    </Button>
                  )}
                </div>
              ))}
              {loginSessions.length === 0 && (
                <p className="rounded-lg border border-border px-4 py-8 text-center text-[12.5px] text-muted-foreground">
                  暂无登录设备
                </p>
              )}
            </div>
            <p className="mt-3 text-[12px] leading-[1.7] text-muted-foreground">
              当前设备需要退出登录才能下线，其他设备可以单独下线。
            </p>
          </>
        )

      case "authorizations":
        return (
          <>
            <div className="space-y-2">
              {authorizations.map((item) => (
                <div
                  key={item.client_id}
                  className="flex items-center justify-between gap-4 rounded-lg border border-border px-4 py-3"
                >
                  <div className="min-w-0">
                    <div className="truncate text-[13.5px] text-foreground">{item.name}</div>
                    <div className="truncate text-[12px] text-muted-foreground">
                      {item.scopes.join(" · ")}
                      {" · "}
                      {item.last_used_at ? `最近使用 ${formatDateTime(item.last_used_at)}` : "尚未使用"}
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 shrink-0 text-muted-foreground"
                    onClick={() => revokeAuthorization(item.client_id)}
                  >
                    撤销
                  </Button>
                </div>
              ))}
              {authorizations.length === 0 && (
                <p className="rounded-lg border border-border px-4 py-8 text-center text-[12.5px] text-muted-foreground">
                  还没有授权任何应用
                </p>
              )}
            </div>
            <p className="mt-3 text-[12px] leading-[1.7] text-muted-foreground">
              撤销后该应用需要重新获得授权，已签发的刷新令牌同时失效。
            </p>
          </>
        )

      case "bindings":
        return (
          <>
            <div className="space-y-2">
              {user.bindings.map((item) => {
                const needsAttention = !item.bound && highlight === item.provider
                return (
                  <div
                    key={item.provider}
                    className={cn(
                      "flex items-center justify-between gap-4 rounded-lg border px-4 py-3",
                      needsAttention ? "border-primary/60 bg-primary/5" : "border-border",
                    )}
                  >
                    <div className="min-w-0">
                      <div className="text-[13.5px] text-foreground">{item.display_name}</div>
                      <div className="truncate text-[12px] text-muted-foreground">
                        {item.bound ? item.account : needsAttention ? "该应用要求先完成绑定" : "未绑定"}
                      </div>
                    </div>
                    {item.bound ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 text-muted-foreground"
                        onClick={() => unbind(item.provider)}
                      >
                        解除
                      </Button>
                    ) : (
                      <Button size="sm" className="h-8" asChild>
                        <a
                          href={`/api/bindings/${encodeURIComponent(item.provider)}/start?next=${encodeURIComponent(
                            currentPath(),
                          )}`}
                        >
                          <Link2 className="size-3.5" />
                          去绑定
                        </a>
                      </Button>
                    )}
                  </div>
                )
              })}
            </div>
          </>
        )

      case "events":
        return (
          <>
            <div>
              {authEvents.length === 0 ? (
                <p className="rounded-lg border border-border px-4 py-8 text-center text-[12.5px] text-muted-foreground">
                  暂无记录
                </p>
              ) : (
                <div className="divide-y divide-border rounded-lg border border-border px-4">
                  {authEvents.map((item) => (
                    <div
                      key={item.id}
                      className="flex items-baseline justify-between gap-4 py-3 text-[12.5px]"
                    >
                      <div className="min-w-0 truncate">
                        <span className={item.result === "success" ? "text-foreground" : "text-destructive"}>
                          {eventNames[item.kind] ?? item.kind}
                        </span>
                        {item.client_name && (
                          <span className="text-muted-foreground"> · {item.client_name}</span>
                        )}
                        {item.reason && (
                          <span className="text-muted-foreground">
                            {" · "}
                            {reasonNames[item.reason] ?? item.reason}
                          </span>
                        )}
                      </div>
                      <div className="shrink-0 text-[12px] text-muted-foreground">
                        {formatDateTime(item.occurred_at)}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        )

      case "delete":
        return (
          <>
            <div className="space-y-3">
              <p className="text-[12.5px] leading-[1.8] text-foreground">
                注销后，本站保存的以下数据会被永久删除，且无法恢复：
              </p>
              <ul className="list-disc space-y-1 pl-5 text-[12.5px] leading-[1.8] text-muted-foreground">
                <li>账号密码、通行密钥与个人资料快照</li>
                <li>第三方账号绑定</li>
                <li>用户组归属</li>
                <li>应用授权记录与登录通知记录</li>
              </ul>
              <p className="text-[12px] leading-[1.7] text-muted-foreground">
                登录与授权事件会去除账号、IP 与设备信息后保留，用于安全审计。
              </p>
              <p className="text-[12px] leading-[1.7] text-muted-foreground">
                所有已登录设备会立即退出。注销后再次登录会重新建立空白账号，原有绑定与授权不会恢复。
              </p>
              {deleteMsg && (
                <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">{deleteMsg}</p>
              )}
              <Button
                variant="destructive"
                className="h-9 w-auto px-4"
                disabled={deleteBusy}
                onClick={requestDeletion}
              >
                {deleteBusy ? "发送中" : "发送确认邮件"}
              </Button>
            </div>
          </>
        )

      default:
        return null
    }
  }

  if (view !== "main") {
    const meta = subpageMeta()
    return (
      <PageShell compact>
        <div className="flex h-[calc(100dvh_-_2rem)] max-h-[540px] flex-col overflow-hidden">
          <div key={view} className="account-view-enter flex min-h-0 flex-1 flex-col">
            <div className="shrink-0 px-7 pt-6">{subHeader(meta.title, meta.description)}</div>
            <div className="min-h-0 flex-1">
              <OverlayScrollbarsComponent className="h-full" options={scrollOptions}>
                <div className="px-7 pb-6 pt-4">{renderSubPage()}</div>
              </OverlayScrollbarsComponent>
            </div>
          </div>
        </div>
      </PageShell>
    )
  }

  return (
    <PageShell compact>
      <div className="flex h-[calc(100dvh_-_2rem)] max-h-[540px] flex-col overflow-hidden">
        <div key={view} className="account-view-enter flex min-h-0 flex-1 flex-col">
          <div className="shrink-0 px-7 pt-6">
            {siteTitle && (
              <div className="flex items-center gap-2">
                <span className="size-1.5 rounded-full bg-primary" />
                <span className="text-[12px] tracking-[0.12em] text-muted-foreground">
                  {siteTitle}
                </span>
              </div>
            )}

            <div className="mt-4 flex items-center justify-between gap-4">
              <div>
                <h1 className="text-[21px] font-medium tracking-tight text-foreground">
                  你好，{me.name}
                </h1>
                <p className="mt-1.5 text-[13px] text-muted-foreground">
                  {me.id}
                  {me.belong_class ? ` · ${me.belong_class}` : ""}
                </p>
              </div>
              {me.avatar && !avatarFailed ? (
                <img
                  src={me.avatar}
                  alt=""
                  referrerPolicy="no-referrer"
                  onError={() => setAvatarFailed(true)}
                  className="size-12 shrink-0 rounded-full border border-border bg-secondary object-cover"
                />
              ) : (
                <ShieldCheck className="size-7 text-primary/70" strokeWidth={1.5} />
              )}
            </div>
          </div>

          <div className="min-h-0 flex-1">
            <OverlayScrollbarsComponent className="h-full" options={scrollOptions}>
              <div className="space-y-4 px-7 pb-4 pt-4">
                {notice && (
                  <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">
                    {notice}
                  </p>
                )}

                <div>
                  <div className="text-[12px] text-muted-foreground">账号设置</div>
                  <div className="mt-2 grid grid-cols-2 gap-2">
                    <SettingTile
                      label="个人资料"
                      hint="姓名、邮箱与用户组"
                      onClick={() => navigate("profile")}
                    />
                    {passkeyEnabled && (
                      <SettingTile
                        label="通行密钥"
                        hint={devices.length > 0 ? `已绑定 ${devices.length} 台` : "未绑定"}
                        onClick={() => {
                          setDeviceMsg("")
                          setConfirmId("")
                          setDeviceFormOpen(false)
                          navigate("passkey")
                        }}
                      />
                    )}
                    <SettingTile
                      label="登录设备"
                      hint={loginSessions.length > 0 ? `${loginSessions.length} 台` : "暂无记录"}
                      onClick={() => navigate("sessions")}
                    />
                    <SettingTile
                      label="已授权应用"
                      hint={authorizations.length > 0 ? `${authorizations.length} 个` : "暂无授权"}
                      onClick={() => navigate("authorizations")}
                    />
                    {me.bindings.length > 0 && (
                      <SettingTile
                        label="账号绑定"
                        hint={`${me.bindings.filter((item) => item.bound).length} 个已绑定`}
                        onClick={() => navigate("bindings")}
                      />
                    )}
                    <SettingTile
                      label="登录记录"
                      hint={
                        authEvents[0]
                          ? (eventNames[authEvents[0].kind] ?? authEvents[0].kind)
                          : "暂无记录"
                      }
                      onClick={() => navigate("events")}
                    />
                  </div>
                </div>
              </div>
            </OverlayScrollbarsComponent>
          </div>

          <div className="shrink-0 border-t border-border px-7 py-4">
            <div className="flex items-center justify-between gap-3">
              <Button variant="outline" className="h-9" onClick={logout}>
                退出登录
              </Button>
              <div className="flex items-center gap-2">
                {me.led_groups.length > 0 && (
                  <Button variant="outline" className="h-9" asChild>
                    <a href="/groups">我管理的用户组</a>
                  </Button>
                )}
                {me.type === 1 ? (
                  <Button asChild className="h-9">
                    <a href="/admin">管理后台</a>
                  </Button>
                ) : (
                  me.led_groups.length === 0 && (
                    <span className="text-[12px] text-muted-foreground">普通用户</span>
                  )
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </PageShell>
  )
}
