import { useEffect, useState } from "react"
import { Link2, ShieldCheck } from "lucide-react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { currentPath, safeNext } from "@/lib/config"

type Me = {
  id: string
  name: string
  email: string
  type: number
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

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-6 border-b border-border py-3 last:border-b-0">
      <dt className="text-[13px] text-muted-foreground">{label}</dt>
      <dd className="text-right text-[13.5px] break-all text-foreground">{value}</dd>
    </div>
  )
}

export default function Account({ siteTitle }: { siteTitle: string }) {
  const [me, setMe] = useState<Me | null>(null)
  const [notice, setNotice] = useState("")

  const params = new URLSearchParams(window.location.search)
  const highlight = params.get("bind") ?? ""
  const bound = params.get("bound") ?? ""
  const bindError = params.get("bind_error") ?? ""
  const resumeTo = params.get("next") ?? ""

  useEffect(() => {
    loadMe()
  }, [])

  useEffect(() => {
    if (bindError && bindErrors[bindError]) setNotice(bindErrors[bindError])
  }, [bindError])

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

  async function logout() {
    await fetch("/api/logout", { method: "POST" })
    window.location.replace("/login")
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
          <Row label="邮箱" value={me.email} />
          {me.belong_class && <Row label="班级" value={me.belong_class} />}
        </dl>

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

        <div className="mt-8 flex items-center justify-between">
          <Button variant="outline" className="h-10" onClick={logout}>
            退出登录
          </Button>
          <span className="text-[12px] text-muted-foreground">
            {me.type === 1 ? "管理员" : "普通用户"}
          </span>
        </div>
      </div>
    </PageShell>
  )
}
