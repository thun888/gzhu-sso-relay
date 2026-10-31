import { useCallback, useEffect, useMemo, useState } from "react"
import { Plus, Search, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import { ConfirmDialog, Empty, Panel, api } from "./shared"

type Group = {
  id: number
  name: string
  code: string
  is_upstream: boolean
  parent_id?: number | null
}

type Member = { id: string; name: string; email: string; status: number }

const tabs = [
  { key: "manual", label: "手动维护" },
  { key: "upstream", label: "上游同步" },
] as const

type TabKey = (typeof tabs)[number]["key"]

export function GroupsPanel({ onError }: { onError: (message: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([])
  const [tab, setTab] = useState<TabKey>("manual")

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [code, setCode] = useState("")
  const [creating, setCreating] = useState(false)

  const [memberGroup, setMemberGroup] = useState<Group | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [keyword, setKeyword] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])

  const [pendingDelete, setPendingDelete] = useState<Group | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await api<{ groups: Group[] }>("/api/admin/groups")
      setGroups(data.groups)
    } catch (err) {
      onError((err as Error).message)
    }
  }, [onError])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    const query = keyword.trim()
    if (!memberGroup || query.length < 2) {
      setCandidates([])
      return
    }

    const timer = setTimeout(async () => {
      try {
        const data = await api<{ users: Member[] }>(
          `/api/admin/users?limit=8&q=${encodeURIComponent(query)}`,
        )
        setCandidates(data.users.filter((user) => !members.some((m) => m.id === user.id)))
      } catch {
        setCandidates([])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [keyword, memberGroup, members])

  const counts = useMemo(
    () => ({
      manual: groups.filter((group) => !group.is_upstream).length,
      upstream: groups.filter((group) => group.is_upstream).length,
    }),
    [groups],
  )

  const visible = groups.filter((group) => group.is_upstream === (tab === "upstream"))

  async function create() {
    setCreating(true)
    try {
      await api("/api/admin/groups", {
        method: "POST",
        body: JSON.stringify({ name: name.trim(), code: code.trim() }),
      })
      setName("")
      setCode("")
      setCreateOpen(false)
      setTab("manual")
      await load()
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  async function remove(group: Group) {
    try {
      await api(`/api/admin/groups/${group.id}`, { method: "DELETE" })
      setPendingDelete(null)
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function openMembers(group: Group) {
    setMemberGroup(group)
    setMembers([])
    setKeyword("")
    setCandidates([])
    try {
      const data = await api<{ members: Member[] }>(`/api/admin/groups/${group.id}/members`)
      setMembers(data.members)
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function addMember(user: Member) {
    if (!memberGroup) return
    try {
      const data = await api<{ members: Member[] }>(`/api/admin/groups/${memberGroup.id}/members`, {
        method: "POST",
        body: JSON.stringify({ user_id: user.id }),
      })
      setMembers(data.members)
      setKeyword("")
      setCandidates([])
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function removeMember(member: Member) {
    if (!memberGroup) return
    try {
      await api(`/api/admin/groups/${memberGroup.id}/members/${encodeURIComponent(member.id)}`, {
        method: "DELETE",
      })
      setMembers((prev) => prev.filter((item) => item.id !== member.id))
    } catch (err) {
      onError((err as Error).message)
    }
  }

  function exportMembers() {
    if (!memberGroup || members.length === 0) return

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
    link.download = `${memberGroup.name}-成员.csv`
    link.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="space-y-5">
      <Panel
        title="用户组"
        description="手动维护的组由管理员管理成员；上游组按身份信息在登录时自动生成"
        action={
          <Button className="h-9 w-auto px-3.5" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" strokeWidth={1.75} />
            新建用户组
          </Button>
        }
      >
        <div className="flex items-center gap-1 border-b border-border px-4 py-2">
          {tabs.map((item) => (
            <button
              key={item.key}
              onClick={() => setTab(item.key)}
              className={cn(
                "rounded-md px-3 py-1.5 text-[12.5px] transition-colors",
                tab === item.key
                  ? "bg-secondary font-medium text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {item.label}
              <span className="ml-1.5 text-[11.5px] text-muted-foreground">{counts[item.key]}</span>
            </button>
          ))}
        </div>

        <div className="divide-y divide-border">
          {visible.map((group) => (
            <div key={group.id} className="flex items-center justify-between gap-4 px-6 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-[13px] text-foreground">{group.name}</span>
                  {group.is_upstream && (
                    <span className="rounded border border-border px-1.5 py-0.5 text-[11px] text-muted-foreground">
                      上游
                    </span>
                  )}
                </div>
                <div className="mt-0.5 font-mono text-[12px] text-muted-foreground">{group.code}</div>
              </div>
              <div className="flex shrink-0 items-center gap-3 whitespace-nowrap">
                {group.is_upstream ? (
                  <span className="text-[12.5px] text-muted-foreground">成员随登录同步</span>
                ) : (
                  <Button
                    variant="outline"
                    className="h-8 w-auto px-3"
                    onClick={() => openMembers(group)}
                  >
                    成员
                  </Button>
                )}
                <button
                  className="text-[12.5px] text-destructive hover:underline"
                  onClick={() => setPendingDelete(group)}
                >
                  删除
                </button>
              </div>
            </div>
          ))}
        </div>

        {visible.length === 0 && (
          <Empty>
            {tab === "manual" ? "还没有手动维护的用户组" : "还没有同步到上游用户组"}
          </Empty>
        )}
      </Panel>

      <Modal
        open={createOpen}
        title="新建用户组"
        description="成员由管理员手工维护，不会随登录变化"
        onClose={() => setCreateOpen(false)}
        footer={
          <>
            <Button
              variant="outline"
              className="h-9 w-auto px-4"
              onClick={() => setCreateOpen(false)}
            >
              取消
            </Button>
            <Button
              className="h-9 w-auto px-4"
              disabled={!name.trim() || !code.trim() || creating}
              onClick={create}
            >
              {creating ? "创建中" : "创建"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <label className="block">
            <span className="text-[12.5px] text-muted-foreground">组名</span>
            <Input
              className="mt-1.5 h-9"
              placeholder="例如 计算机学院"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </label>
          <label className="block">
            <span className="text-[12.5px] text-muted-foreground">标识</span>
            <Input
              className="mt-1.5 h-9 font-mono"
              placeholder="例如 cs"
              value={code}
              onChange={(event) => setCode(event.target.value)}
            />
            <span className="mt-1.5 block text-[12px] text-muted-foreground">
              用于程序引用的唯一标识，创建后不可修改
            </span>
          </label>
        </div>
      </Modal>

      <Modal
        open={memberGroup !== null}
        title={`成员 · ${memberGroup?.name ?? ""}`}
        description={`共 ${members.length} 人`}
        onClose={() => setMemberGroup(null)}
        width="max-w-[560px]"
        footer={
          <>
            <Button
              variant="outline"
              className="mr-auto h-9 w-auto px-4"
              disabled={members.length === 0}
              onClick={exportMembers}
            >
              导出 CSV
            </Button>
            <Button
              variant="outline"
              className="h-9 w-auto px-4"
              onClick={() => setMemberGroup(null)}
            >
              关闭
            </Button>
          </>
        }
      >
        <div className="space-y-3">
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
                    <span className="ml-2 font-mono text-[12px] text-muted-foreground">{user.id}</span>
                  </span>
                  <span className="shrink-0 text-[12.5px] text-primary">添加</span>
                </button>
              ))}
            </div>
          )}

          {keyword.trim().length >= 2 && candidates.length === 0 && (
            <p className="text-[12px] text-muted-foreground">没有可添加的用户</p>
          )}

          <div className="divide-y divide-border rounded-md border border-border">
            {members.map((member) => (
              <div key={member.id} className="flex items-center justify-between gap-3 px-3 py-2">
                <div className="min-w-0">
                  <span className="text-[13px] text-foreground">{member.name}</span>
                  <span className="ml-2 font-mono text-[12px] text-muted-foreground">{member.id}</span>
                  {member.status === 1 && (
                    <span className="ml-2 text-[11.5px] text-destructive">已禁用</span>
                  )}
                </div>
                <button
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  aria-label={`移除 ${member.name}`}
                  onClick={() => removeMember(member)}
                >
                  <X className="size-3.5" strokeWidth={2} />
                </button>
              </div>
            ))}
          </div>

          {members.length === 0 && (
            <p className="py-6 text-center text-[12.5px] text-muted-foreground">该组还没有成员</p>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={pendingDelete !== null}
        title="删除用户组"
        message={`删除「${pendingDelete?.name ?? ""}」后，该组的成员关系与授权关联会一并移除，操作不可撤销。`}
        confirmLabel="删除"
        danger
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  )
}
