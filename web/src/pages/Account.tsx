import { useEffect, useState, type ReactNode } from "react"
import { Link2, PenLine, ShieldCheck } from "lucide-react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import { currentPath, safeNext } from "@/lib/config"

type Me = {
  id: string
  name: string
  email: string
  type: number
  mail_enabled?: boolean
  belong_class: string
  groups: { id: number; name: string; code: string }[]
  bindings: { provider: string; display_name: string; bound: boolean; account?: string }[]
}

const bindErrors: Record<string, string> = {
  state: "绑定会话已失效，请重新发起",
  denied: "你取消了在绑定源上的授权",
  exchange: "绑定源未返回有效身份",
  store: "保存绑定关系失败",
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

export default function Account({ siteTitle }: { siteTitle: string }) {
  const [me, setMe] = useState<Me | null>(null)
  const [notice, setNotice] = useState("")
  const [emailOpen, setEmailOpen] = useState(false)
  const [newEmail, setNewEmail] = useState("")
  const [emailBusy, setEmailBusy] = useState(false)
  const [emailMsg, setEmailMsg] = useState("")
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteBusy, setDeleteBusy] = useState(false)
  const [deleteMsg, setDeleteMsg] = useState("")

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
          {me.type === 1 ? (
            <Button asChild className="h-10">
              <a href="/admin">管理后台</a>
            </Button>
          ) : (
            <span className="text-[12px] text-muted-foreground">普通用户</span>
          )}
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
              <li>账号密码与个人资料快照</li>
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
