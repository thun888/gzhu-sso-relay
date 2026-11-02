import { useEffect, useState } from "react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { safeNext } from "@/lib/config"

type Preview = {
  action: string
  summary: string
  note: string
  danger: boolean
}

type State =
  | { kind: "loading" }
  | { kind: "ready"; preview: Preview }
  | { kind: "failed"; message: string }

export default function Confirm({ siteTitle }: { siteTitle: string }) {
  const params = new URLSearchParams(window.location.search)
  const kind = params.get("kind") ?? ""
  const token = params.get("token") ?? ""

  const [state, setState] = useState<State>({ kind: "loading" })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!kind || !token) {
      setState({ kind: "failed", message: "确认链接不完整" })
      return
    }

    fetch(`/api/confirm/${encodeURIComponent(kind)}?token=${encodeURIComponent(token)}`)
      .then(async (res) => {
        if (!res.ok) {
          const data = (await res.json().catch(() => null)) as { error?: string } | null
          setState({ kind: "failed", message: data?.error ?? "确认链接无效或已过期" })
          return
        }
        setState({ kind: "ready", preview: (await res.json()) as Preview })
      })
      .catch(() => setState({ kind: "failed", message: "网络异常，请稍后重试" }))
  }, [kind, token])

  async function submit() {
    if (pending) return
    setError("")
    setPending(true)
    try {
      const res = await fetch(
        `/api/confirm/${encodeURIComponent(kind)}?token=${encodeURIComponent(token)}`,
        { method: "POST" },
      )
      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        setError(data?.error ?? "确认失败，请稍后重试")
        return
      }
      const data = (await res.json()) as { next?: string }
      window.location.replace(safeNext(data.next ?? "/"))
    } catch {
      setError("网络异常，请稍后重试")
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
          <h1 className="text-[19px] font-medium text-foreground">无法完成确认</h1>
          <p className="mt-2 text-[13px] text-muted-foreground">{state.message}</p>
          <Button variant="outline" className="mt-7 h-10" onClick={() => window.location.replace("/")}>
            返回用户中心
          </Button>
        </div>
      </PageShell>
    )
  }

  const { preview } = state

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
          确认{preview.action}
        </h1>
        <p className="mt-1.5 text-[13px] leading-[1.8] text-muted-foreground">
          {preview.summary}
        </p>

        {preview.note && (
          <p className="mt-5 rounded-lg bg-secondary px-3 py-2 text-[12.5px] leading-[1.8] text-foreground">
            {preview.note}
          </p>
        )}

        <p className="mt-3 min-h-[18px] text-[12.5px] text-destructive" role="alert">
          {error}
        </p>

        <div className="mt-3 flex items-center gap-3">
          <Button
            variant="outline"
            className="h-11 flex-1"
            disabled={pending}
            onClick={() => window.location.replace("/")}
          >
            取消
          </Button>
          <Button
            variant={preview.danger ? "destructive" : "default"}
            className="h-11 flex-1"
            disabled={pending}
            onClick={submit}
          >
            {pending ? "处理中" : `确认${preview.action}`}
          </Button>
        </div>
      </div>
    </PageShell>
  )
}
