import type { ReactNode } from "react"

import { cn } from "@/lib/utils"

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  })

  if (res.status === 401) {
    window.location.replace(`/login?next=${encodeURIComponent(window.location.pathname)}`)
    throw new Error("未登录")
  }
  if (res.status === 403) {
    window.location.replace("/")
    throw new Error("需要管理员权限")
  }
  if (!res.ok) {
    const data = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(data?.error ?? "请求失败")
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function Panel({
  title,
  description,
  action,
  children,
}: {
  title?: string
  description?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card">
      {(title || action) && (
        <header className="flex items-center justify-between gap-4 border-b border-border px-6 py-3.5">
          <div>
            <h2 className="text-[13.5px] font-medium text-foreground">{title}</h2>
            {description && (
              <p className="mt-0.5 text-[12px] text-muted-foreground">{description}</p>
            )}
          </div>
          {action}
        </header>
      )}
      {children}
    </section>
  )
}

export function FormRow({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className="flex items-start gap-8 border-b border-border px-6 py-4 last:border-b-0">
      <div className="w-[110px] shrink-0 pt-2 text-[13px] text-muted-foreground">{label}</div>
      <div className="min-w-0 flex-1">
        {children}
        {hint && <p className="mt-1.5 text-[12px] text-muted-foreground">{hint}</p>}
      </div>
    </div>
  )
}

export function Chip({
  active,
  label,
  onClick,
}: {
  active: boolean
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-md border px-2.5 py-1 text-[12px] transition-colors",
        active
          ? "border-primary/40 bg-primary/10 text-primary"
          : "border-border text-muted-foreground hover:text-foreground",
      )}
    >
      {label}
    </button>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="py-10 text-center text-[13px] text-muted-foreground">{children}</p>
}

export const thClass = "px-6 py-2.5 text-left text-[12px] font-normal text-muted-foreground"
export const tdClass = "px-6 py-3 text-[13px] border-t border-border"
