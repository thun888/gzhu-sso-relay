import { useState } from "react"
import { Link2, LayoutGrid, Users, UsersRound } from "lucide-react"

import { cn } from "@/lib/utils"
import { BindingsPanel } from "./admin/BindingsPanel"
import { ClientsPanel } from "./admin/ClientsPanel"
import { GroupsPanel } from "./admin/GroupsPanel"
import { UsersPanel } from "./admin/UsersPanel"

const sections = [
  { key: "users", label: "用户", icon: Users },
  { key: "groups", label: "用户组", icon: UsersRound },
  { key: "clients", label: "应用", icon: LayoutGrid },
  { key: "bindings", label: "绑定源", icon: Link2 },
] as const

type Section = (typeof sections)[number]["key"]

function initialSection(): Section {
  const fromURL = new URLSearchParams(window.location.search).get("section")
  return sections.some((item) => item.key === fromURL) ? (fromURL as Section) : "users"
}

export default function Admin({ siteTitle }: { siteTitle: string }) {
  const [section, setSection] = useState<Section>(initialSection)
  const [error, setError] = useState("")

  function select(key: Section) {
    setSection(key)
    setError("")
    const url = new URL(window.location.href)
    url.searchParams.set("section", key)
    window.history.replaceState(null, "", url)
  }

  const current = sections.find((item) => item.key === section) ?? sections[0]

  return (
    <div className="flex min-h-screen bg-[#f5f6f8]">
      <aside className="sticky top-0 flex h-screen w-[208px] shrink-0 flex-col border-r border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-5 py-4">
          <span className="size-1.5 rounded-full bg-primary" />
          <span className="truncate text-[13px] font-medium text-foreground">
            {siteTitle || "统一身份认证"}
          </span>
        </div>

        <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
          {sections.map((item) => (
            <button
              key={item.key}
              onClick={() => select(item.key)}
              className={cn(
                "flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-[13px] transition-colors",
                section === item.key
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-secondary hover:text-foreground",
              )}
            >
              <item.icon className="size-4" strokeWidth={1.75} />
              {item.label}
            </button>
          ))}
        </nav>

      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-border bg-card px-6">
          <div className="flex items-center gap-2 text-[13px]">
            <span className="text-muted-foreground">管理后台</span>
            <span className="text-muted-foreground/40">/</span>
            <h1 className="font-medium text-foreground">{current.label}管理</h1>
          </div>
          <a href="/" className="text-[12.5px] text-muted-foreground hover:text-foreground">
            返回用户中心
          </a>
        </header>

        <main className="min-w-0 flex-1 p-6">
          {error && (
            <p className="mb-4 rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-2.5 text-[12.5px] text-destructive">
              {error}
            </p>
          )}

          {section === "users" && <UsersPanel onError={setError} />}
          {section === "groups" && <GroupsPanel onError={setError} />}
          {section === "clients" && <ClientsPanel onError={setError} />}
          {section === "bindings" && <BindingsPanel onError={setError} />}
        </main>
      </div>
    </div>
  )
}
