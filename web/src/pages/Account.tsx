import { useEffect, useState, type ReactNode } from "react"
import { Link2, Loader2, PenLine, ShieldCheck } from "lucide-react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import { currentPath, safeNext } from "@/lib/config"
import {
  createPasskey,
  defaultDeviceName,
  passkeyErrorMessage,
  passkeySupported,
} from "@/lib/passkey"

type Me = {
  id: string
  name: string
  email: string
  type: number
  mail_enabled?: boolean
  belong_class: string
  groups: { id: number; name: string; code: string }[]
  led_groups: { id: number; name: string; code: string }[]
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

const emailErrors: Record<string, string> = {
  invalid: "确认链接无效或已过期",
  taken: "该邮箱已被其他账号使用",
  failed: "切换邮箱失败，请稍后重试",
}

function Row({ label, value, action }: { label: string; value: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-6 border-b border-border py-3 last:border-b-0">
      <dt className="text-[13px] text-muted-foreground">{label}</dt>
      <dd className="flex items-center gap-2 text-right text-[13.5px] break-all text-foreground">
        {value}
        {action}
      </dd>
    </div>
  )
}

function formatDate(value: string) {
  return new Date(value).toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  })
}

export default function Account({ siteTitle, passkey }: { siteTitle: string; passkey: boolean }) {
  const [me, setMe] = useState<Me | null>(null)
  const [notice, setNotice] = useState("")
  const [emailOpen, setEmailOpen] = useState(false)
  const [newEmail, setNewEmail] = useState("")
  const [emailBusy, setEmailBusy] = useState(false)
  const [emailMsg, setEmailMsg] = useState("")
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteMsg, setDeleteMsg] = useState("")
  const [devices, setDevices] = useState<PasskeyDevice[]>([])
  const [deviceOpen, setDeviceOpen] = useState(false)
  const [deviceAddOpen, setDeviceAddOpen] = useState(false)
  const [deviceName, setDeviceName] = useState("")
  const [deviceBusy, setDeviceBusy] = useState(false)
  const [deviceMsg, setDeviceMsg] = useState("")
  const [confirmId, setConfirmId] = useState("")

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
    if (passkeyEnabled) void loadDevices()
  }, [passkeyEnabled])

  useEffect(() => {
    if (bindError && bindErrors[bindError]) setNotice(bindErrors[bindError])
  }, [bindError])

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
    setDeviceAddOpen(true)
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

      setDeviceAddOpen(false)
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
      setDeleteOpen(false)
      setNotice(`确认邮件已发送到 ${me?.email ?? "账号邮箱"}，请在 30 分钟内打开邮件中的链接完成注销`)
    } finally {
      setDeleteBusy(false)
    }
  }

  if (!me) {
    return (
      <PageShell>
        <p className="py-16 text-center text-[13px] text-muted-foreground">加载中</p>
      </PageShell>
    )
  }

  return (
    <PageShell>
      <div className="px-7 py-8 sm:px-10">
        {siteTitle && (
          <div className="flex items-center gap-2">
            <span className="size-1.5 rounded-full bg-primary" />
            <span className="text-[12px] tracking-[0.12em] text-muted-foreground">{siteTitle}</span>
          </div>
        )}

        <div className="mt-6 flex items-center justify-between gap-4">
          <div>
            <h1 className="text-[21px] font-medium tracking-tight text-foreground">
              你好，{me.name}
            </h1>
            <p className="mt-1.5 text-[13px] text-muted-foreground">已登录统一身份认证</p>
          </div>
          <ShieldCheck className="size-7 text-primary/70" strokeWidth={1.5} />
        </div>

        {notice && (
          <p className="mt-4 rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">
            {notice}
          </p>
        )}

        <dl className="mt-6 border-t border-border">
          <Row label="学号" value={me.id} />
          <Row label="姓名" value={me.name} />
          <Row
            label="邮箱"
            value={me.email}
            action={
              me.mail_enabled ? (
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
          {me.belong_class && <Row label="班级" value={me.belong_class} />}
        </dl>

        {passkeyEnabled && (
          <div className="mt-6">
            <div className="text-[12px] text-muted-foreground">通行密钥</div>
            <div className="mt-2 flex items-center justify-between gap-4 rounded-lg border border-border px-4 py-3">
              <div className="min-w-0">
                <div className="text-[13.5px] text-foreground">
                  {devices.length > 0 ? `已绑定 ${devices.length} 台设备` : "未绑定设备"}
                </div>
                <div className="text-[12px] text-muted-foreground">
                  用指纹、面容或设备 PIN 代替密码登录
                </div>
              </div>
              <Button
                variant="outline"
                size="sm"
                className="h-8 shrink-0"
                onClick={() => {
                  setDeviceMsg("")
                  setConfirmId("")
                  setDeviceOpen(true)
                }}
              >
                管理
              </Button>
            </div>
          </div>
        )}

        <Modal
          open={emailOpen}
          title="修改邮箱"
          description={`当前邮箱 ${me.email}`}
          onClose={() => setEmailOpen(false)}
          footer={
            <>
              <Button variant="outline" className="h-9 w-auto px-4" onClick={() => setEmailOpen(false)}>
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
                placeholder={me.email}
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
              <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">{emailMsg}</p>
            )}
          </div>
        </Modal>

        <Modal
          open={deviceOpen}
          title="通行密钥"
          description="每台设备一条记录，登录时按浏览器提示选择"
          onClose={() => {
            setDeviceOpen(false)
            setDeviceAddOpen(false)
          }}
          footer={
            <>
              <Button
                variant="outline"
                className="h-9 w-auto px-4"
                onClick={() => {
                  setDeviceOpen(false)
                  setDeviceAddOpen(false)
                }}
              >
                关闭
              </Button>
              <Button className="h-9 w-auto px-4" onClick={startAddDevice}>
                添加设备
              </Button>
            </>
          }
        >
          <div className="space-y-2">
            {devices.length === 0 ? (
              <p className="py-6 text-center text-[12.5px] text-muted-foreground">
                还没有绑定任何设备
              </p>
            ) : (
              devices.map((device) => (
                <div
                  key={device.id}
                  className="flex items-center justify-between gap-3 rounded-lg border border-border px-3.5 py-2.5"
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
        </Modal>

        <Modal
          open={deviceAddOpen}
          layer={1}
          title="添加通行密钥"
          description="为这台设备起个名字"
          onClose={() => setDeviceAddOpen(false)}
          footer={
            <>
              <Button
                variant="outline"
                className="h-9 w-auto px-4"
                onClick={() => setDeviceAddOpen(false)}
              >
                取消
              </Button>
              <Button className="h-9 w-auto px-4" disabled={deviceBusy} onClick={addDevice}>
                {deviceBusy && <Loader2 className="size-3.5 animate-spin" strokeWidth={1.75} />}
                {deviceBusy ? "等待设备确认" : "开始添加"}
              </Button>
            </>
          }
        >
          <div className="space-y-3">
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
            <p className="text-[12px] leading-[1.7] text-muted-foreground">
              点击开始添加后，按浏览器提示使用指纹、面容、设备 PIN 或安全密钥完成验证。同一账号可以绑定多台设备。
            </p>
            {deviceMsg && (
              <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">
                {deviceMsg}
              </p>
            )}
          </div>
        </Modal>

        {me.groups.length > 0 && (
          <div className="mt-6">
            <div className="text-[12px] text-muted-foreground">用户组</div>
            <div className="mt-2 flex flex-wrap gap-2">
              {me.groups.map((group) => (
                <span
                  key={group.id}
                  className="rounded-md bg-secondary px-2 py-1 text-[12px] text-secondary-foreground"
                >
                  {group.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {me.bindings.length > 0 && (
          <div className="mt-6">
            <div className="text-[12px] text-muted-foreground">账号绑定</div>
            <div className="mt-2 space-y-2">
              {me.bindings.map((item) => {
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
          </div>
        )}

        <div className="mt-8 flex items-center justify-between gap-3">
          <Button variant="outline" className="h-10" onClick={logout}>
            退出登录
          </Button>
          <div className="flex items-center gap-3">
            {me.led_groups.length > 0 && (
              <Button variant="outline" className="h-10" asChild>
                <a href="/groups">我管理的用户组</a>
              </Button>
            )}
            {me.type === 1 ? (
              <Button asChild className="h-10">
                <a href="/admin">管理后台</a>
              </Button>
            ) : (
              me.led_groups.length === 0 && (
                <span className="text-[12px] text-muted-foreground">普通用户</span>
              )
            )}
          </div>
        </div>

        {me.type !== 1 && me.mail_enabled && (
          <div className="mt-10 flex flex-wrap items-start justify-between gap-3 border-t border-border pt-5">
            <div className="min-w-0">
              <div className="text-[13px] text-foreground">注销账号</div>
              <p className="mt-1 text-[12px] leading-[1.7] text-muted-foreground">
                永久删除本站保存的账号数据与授权记录，需要邮件二次确认。
              </p>
            </div>
            <Button
              variant="outline"
              className="h-9 w-auto shrink-0 px-4 text-destructive hover:text-destructive"
              onClick={() => {
                setDeleteMsg("")
                setDeleteOpen(true)
              }}
            >
              注销
            </Button>
          </div>
        )}

        <Modal
          open={deleteOpen}
          title="注销账号"
          description={`确认邮件将发送到 ${me.email}`}
          onClose={() => setDeleteOpen(false)}
          footer={
            <>
              <Button variant="outline" className="h-9 w-auto px-4" onClick={() => setDeleteOpen(false)}>
                取消
              </Button>
              <Button
                variant="destructive"
                className="h-9 w-auto px-4"
                disabled={deleteBusy}
                onClick={requestDeletion}
              >
                {deleteBusy ? "发送中" : "发送确认邮件"}
              </Button>
            </>
          }
        >
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
              所有已登录设备会立即退出。注销后再次登录会重新建立空白账号，原有绑定与授权不会恢复。
            </p>
            {deleteMsg && (
              <p className="rounded-lg bg-secondary px-3 py-2 text-[12.5px] text-foreground">{deleteMsg}</p>
            )}
          </div>
        </Modal>
      </div>
    </PageShell>
  )
}
