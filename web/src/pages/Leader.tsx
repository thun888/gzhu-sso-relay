import { useCallback, useEffect, useState } from "react"
import { Search, UsersRound, X } from "lucide-react"

import { ThemeToggle } from "@/components/theme-toggle"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { Empty, Panel, api, thClass, tdClass } from "./admin/shared"

type Member = { id: string; name: string; email: string; status: number }
type LedGroup = { id: number; name: string; code: string }

export default function Leader({ siteTitle }: { siteTitle: string }) {
  const [groups, setGroups] = useState<LedGroup[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [leaders, setLeaders] = useState<Member[]>([])
  const [keyword, setKeyword] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(true)

  const group = groups.find((item) => item.id === selected)

  useEffect(() => {
    let alive = true
    api<{ groups: LedGroup[] }>("/api/leader/groups", undefined, "需要组长权限")
      .then((data) => {
        if (!alive) return
        setGroups(data.groups)
        setSelected(data.groups[0]?.id ?? null)
      })
      .catch((err) => {
        if (alive) setError((err as Error).message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [])

  const loadMembers = useCallback(async (groupID: number) => {
    try {
      const data = await api<{ members: Member[]; leaders: Member[] }>(
        `/api/leader/groups/${groupID}/members`,
        undefined,
        "需要组长权限",
      )
      setMembers(data.members)
      setLeaders(data.leaders)
    } catch (err) {
      setError((err as Error).message)
    }
  }, [])

  useEffect(() => {
    setKeyword("")
    setCandidates([])
    if (selected == null) {
      setMembers([])
      setLeaders([])
      return
    }
    loadMembers(selected)
  }, [selected, loadMembers])

  useEffect(() => {
    const query = keyword.trim()
    if (selected == null || query.length < 2) {
      setCandidates([])
      return
    }

    const timer = setTimeout(async () => {
      try {
        const data = await api<{ users: Member[] }>(
          `/api/leader/users?limit=8&q=${encodeURIComponent(query)}`,
          undefined,
          "需要组长权限",
        )
        setCandidates(data.users.filter((user) => !members.some((m) => m.id === user.id)))
      } catch {
        setCandidates([])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [keyword, selected, members])

  async function addMember(user: Member) {
    if (selected == null) return
    try {
      const data = await api<{ members: Member[]; leaders: Member[] }>(
        `/api/leader/groups/${selected}/members`,
        { method: "POST", body: JSON.stringify({ user_id: user.id }) },
        "需要组长权限",
      )
      setMembers(data.members)
      setLeaders(data.leaders)
      setKeyword("")
      setCandidates([])
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function removeMember(member: Member) {
    if (selected == null) return
    try {
      await api(
        `/api/leader/groups/${selected}/members/${encodeURIComponent(member.id)}`,
        { method: "DELETE" },
        "需要组长权限",
      )
      setMembers((prev) => prev.filter((item) => item.id !== member.id))
    } catch (err) {
      setError((err as Error).message)
    }
  }

  function exportMembers() {
    if (!group || members.length === 0) return

    const rows = [
      ["学号", "姓名", "邮箱"],
      ...members.map((member) => [member.id, member.name, member.email]),
    ]
    const csv =
      "\uFEFF" +
      rows.map((row) => row.map((cell) => `"${cell.replace(/"/g, '""')}"`).join(",")).join("\n")

    const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }))
    const link = document.createElement("a")
    link.href = url
    link.download = `${group.name}-成员.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  function select(id: number) {
    setError("")
    setSelected(id)
  }

  return (
    <div className="flex min-h-screen bg-[#f5f6f8] dark:bg-background">
      <aside className="sticky top-0 flex h-screen w-[208px] shrink-0 flex-col border-r border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-5 py-4">
          <span className="size-1.5 rounded-full bg-primary" />
          <span className="truncate text-[13px] font-medium text-foreground">
            {siteTitle || "统一身份认证"}
          </span>
        </div>

        <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
          {groups.map((item) => (
            <button
              key={item.id}
              onClick={() => select(item.id)}
              className={cn(
                "flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-[13px] transition-colors",
                selected === item.id
                  ? "bg-primary/10 font-medium text-primary"
                  : "text-muted-foreground hover:bg-secondary hover:text-foreground",
              )}
            >
              <UsersRound className="size-4 shrink-0" strokeWidth={1.75} />
              <span className="min-w-0">
                <span className="block truncate">{item.name}</span>
                <span className="block truncate font-mono text-[11px] opacity-70">{item.code}</span>
              </span>
            </button>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-border bg-card px-6">
          <div className="flex items-center gap-2 text-[13px]">
            <span className="text-muted-foreground">组长工作台</span>
            <span className="text-muted-foreground/40">/</span>
            <h1 className="font-medium text-foreground">{group?.name ?? "我的用户组"}</h1>
          </div>
          <div className="flex items-center gap-1">
            <a href="/" className="text-[12.5px] text-muted-foreground hover:text-foreground">
              返回用户中心
            </a>
            <ThemeToggle />
          </div>
        </header>

        <main className="min-w-0 flex-1 p-6">
          {error && (
            <p className="mb-4 rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-2.5 text-[12.5px] text-destructive">
              {error}
            </p>
          )}

          {loading ? (
            <Empty>加载中</Empty>
          ) : groups.length === 0 ? (
            <Empty>你不是任何用户组的组长</Empty>
          ) : (
            group && (
              <Panel
                title={group.name}
                description={`${group.code} · 共 ${members.length} 人`}
                action={
                  <Button
                    variant="outline"
                    className="h-9 w-auto px-3.5"
                    disabled={members.length === 0}
                    onClick={exportMembers}
                  >
                    导出 CSV
                  </Button>
                }
              >
                <div className="space-y-3 p-6">
                  <div className="relative">
                    <Search
                      className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
                      strokeWidth={1.75}
                    />
                    <Input
                      className="h-9 pl-9"
                      placeholder="输入学号或姓名添加成员"
                      value={keyword}
                      onChange={(event) => setKeyword(event.target.value)}
                    />
                  </div>

                  {candidates.length > 0 && (
                    <div className="overflow-hidden rounded-md border border-border">
                      {candidates.map((user) => (
                        <button
                          key={user.id}
                          onClick={() => addMember(user)}
                          className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-secondary"
                        >
                          <span className="min-w-0">
                            <span className="text-[13px] text-foreground">{user.name}</span>
                            <span className="ml-2 font-mono text-[12px] text-muted-foreground">
                              {user.id}
                            </span>
                          </span>
                          <span className="shrink-0 text-[12.5px] text-primary">添加</span>
                        </button>
                      ))}
                    </div>
                  )}

                  {keyword.trim().length >= 2 && candidates.length === 0 && (
                    <p className="text-[12px] text-muted-foreground">没有可添加的用户</p>
                  )}

                  <div className="overflow-hidden rounded-md border border-border">
                    <table className="w-full">
                      <thead>
                        <tr>
                          <th className={thClass}>学号</th>
                          <th className={thClass}>姓名</th>
                          <th className={thClass}>邮箱</th>
                          <th className={cn(thClass, "w-16 text-right")} />
                        </tr>
                      </thead>
                      <tbody>
                        {members.map((member) => {
                          const isLeader = leaders.some((leader) => leader.id === member.id)
                          return (
                            <tr key={member.id}>
                              <td className={cn(tdClass, "font-mono text-[12.5px]")}>{member.id}</td>
                              <td className={tdClass}>
                                {member.name}
                                {isLeader && (
                                  <span className="ml-2 rounded border border-primary/30 bg-primary/5 px-1.5 py-0.5 text-[11px] text-primary">
                                    组长
                                  </span>
                                )}
                                {member.status === 1 && (
                                  <span className="ml-2 text-[11.5px] text-destructive">已禁用</span>
                                )}
                              </td>
                              <td className={cn(tdClass, "text-muted-foreground")}>{member.email}</td>
                              <td className={cn(tdClass, "text-right")}>
                                {!isLeader && (
                                  <button
                                    className="rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                                    aria-label={`移除 ${member.name}`}
                                    onClick={() => removeMember(member)}
                                  >
                                    <X className="size-3.5" strokeWidth={2} />
                                  </button>
                                )}
                              </td>
                            </tr>
                          )
                        })}
                      </tbody>
                    </table>
                  </div>

                  {members.length === 0 && (
                    <p className="py-6 text-center text-[12.5px] text-muted-foreground">
                      该组还没有成员
                    </p>
                  )}
                </div>
              </Panel>
            )
          )}
        </main>
      </div>
    </div>
  )
}
