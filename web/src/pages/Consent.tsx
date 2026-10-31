import { useEffect, useState } from "react"
import { Check } from "lucide-react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { currentPath } from "@/lib/config"

type ConsentInfo = {
  client: { id: string; name: string }
  scopes: { name: string; display_name: string; description: string }[]
}

type State =
  | { kind: "loading" }
  | { kind: "ready"; info: ConsentInfo }
  | { kind: "failed"; message: string }

export default function Consent({ siteTitle }: { siteTitle: string }) {
  const [state, setState] = useState<State>({ kind: "loading" })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")

  const req = new URLSearchParams(window.location.search).get("req") ?? ""

  useEffect(() => {
    if (!req) {
      setState({ kind: "failed", message: "缺少授权请求参数" })
      return
    }

    fetch(`/api/consent?req=${encodeURIComponent(req)}`)
      .then(async (res) => {
        if (res.status === 401) {
          window.location.replace(`/login?next=${encodeURIComponent(currentPath())}`)
          return
        }
        if (!res.ok) {
          const data = (await res.json().catch(() => null)) as { error?: string } | null
          setState({ kind: "failed", message: data?.error ?? "授权请求不可用" })
          return
        }
        setState({ kind: "ready", info: (await res.json()) as ConsentInfo })
      })
      .catch(() => setState({ kind: "failed", message: "网络异常，请稍后重试" }))
  }, [req])

  async function approve() {
    if (pending) return
    setError("")
    setPending(true)
    try {
      const res = await fetch("/api/consent", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ req }),
      })

      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setError(data?.error ?? "授权失败，请稍后重试")
        return
      }

      const data = (await res.json()) as { continue: string }
      window.location.replace(data.continue)
    } catch {
      setError("网络异常，请稍后重试")
    } finally {
      setPending(false)
    }
  }

  async function deny() {
    if (pending) return
    setError("")
    setPending(true)
    try {
      const res = await fetch("/api/consent/deny", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ req }),
      })
      if (!res.ok) {
        window.location.replace("/")
        return
      }
      const data = (await res.json()) as { continue: string }
      window.location.replace(data.continue)
    } catch {
      window.location.replace("/")
    } finally {
      setPending(false)
    }
  }

  if (state.kind === "loading") {
    return (
      <PageShell>
        <p className="py-16 text-center text-[13px] text-muted-foreground">加载中</p>
      </PageShell>
    )
  }

  if (state.kind === "failed") {
    return (
      <PageShell>
        <div className="px-7 py-10 sm:px-10">
          <h1 className="text-[19px] font-medium text-foreground">无法继续授权</h1>
          <p className="mt-2 text-[13px] text-muted-foreground">{state.message}</p>
          <Button variant="outline" className="mt-7 h-10" onClick={() => window.location.replace("/")}>
            返回用户中心
          </Button>
        </div>
      </PageShell>
    )
  }

  const { info } = state

  return (
    <PageShell>
      <div className="px-7 py-8 sm:px-10">
        {siteTitle && (
          <div className="flex items-center gap-2">
            <span className="size-1.5 rounded-full bg-primary" />
            <span className="text-[12px] tracking-[0.12em] text-muted-foreground">{siteTitle}</span>
          </div>
        )}

        <h1 className="mt-6 text-[19px] font-medium tracking-tight text-foreground">
          {info.client.name} 请求访问你的账号
        </h1>
        <p className="mt-1.5 text-[13px] text-muted-foreground">
          同意后该应用可以获得以下信息，之后再次登录将不再询问
        </p>

        <ul className="mt-6 space-y-2">
          {info.scopes.map((scope) => (
            <li
              key={scope.name}
              className="flex items-start gap-3 rounded-lg border border-border px-4 py-3"
            >
              <Check className="mt-0.5 size-4 shrink-0 text-primary" strokeWidth={2} />
              <div className="min-w-0">
                <div className="text-[13.5px] text-foreground">{scope.display_name}</div>
                {scope.description && (
                  <div className="mt-0.5 text-[12px] text-muted-foreground">{scope.description}</div>
                )}
              </div>
            </li>
          ))}
        </ul>

        <p className="mt-3 min-h-[18px] text-[12.5px] text-destructive" role="alert">
          {error}
        </p>

        <div className="mt-4 flex items-center gap-3">
          <Button
            variant="outline"
            className="h-11 flex-1"
            disabled={pending}
            onClick={deny}
          >
            取消
          </Button>
          <Button className="h-11 flex-1" disabled={pending} onClick={approve}>
            {pending ? "授权中" : "同意并继续"}
          </Button>
        </div>
      </div>
    </PageShell>
  )
}
