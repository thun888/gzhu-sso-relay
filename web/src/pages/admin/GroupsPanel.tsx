import { useCallback, useEffect, useMemo, useState } from "react"
import { ChevronRight, Plus, Search, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import type { Group } from "./client-types"
import { ConfirmDialog, Empty, Panel, Select, api, groupPath } from "./shared"

type Member = { id: string; name: string; email: string; status: number }

type GroupNode = { group: Group; children: GroupNode[] }

function buildTree(groups: Group[]): GroupNode[] {
  const nodes = new Map<number, GroupNode>(
    groups.map((group) => [group.id, { group, children: [] }]),
  )
  const parentOf = (node: GroupNode) =>
    node.group.parent_id == null ? undefined : nodes.get(node.group.parent_id)

  // 祖先链回到自己说明数据成环，这类组按顶级展示，避免渲染时无限递归
  const cyclic = new Set<number>()
  nodes.forEach((node) => {
    const seen = new Set<number>([node.group.id])
    for (let cursor = parentOf(node); cursor; cursor = parentOf(cursor)) {
      if (seen.has(cursor.group.id)) {
        cyclic.add(node.group.id)
        return
      }
      seen.add(cursor.group.id)
    }
  })

  const roots: GroupNode[] = []
  nodes.forEach((node) => {
    const parent = cyclic.has(node.group.id) ? undefined : parentOf(node)
    if (parent) parent.children.push(node)
    else roots.push(node)
  })

  const sort = (list: GroupNode[]) => {
    list.sort(
      (a, b) =>
        Number(a.group.is_upstream) - Number(b.group.is_upstream) ||
        a.group.code.localeCompare(b.group.code),
    )
    list.forEach((node) => sort(node.children))
  }
  sort(roots)
  return roots
}

export function GroupsPanel({ onError }: { onError: (message: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([])
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})

  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [code, setCode] = useState("")
  const [parentID, setParentID] = useState("")
  const [creating, setCreating] = useState(false)

  const [memberGroup, setMemberGroup] = useState<Group | null>(null)
  const [members, setMembers] = useState<Member[]>([])
  const [keyword, setKeyword] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])

  const [leaderGroup, setLeaderGroup] = useState<Group | null>(null)
  const [leaders, setLeaders] = useState<Member[]>([])
  const [leaderKeyword, setLeaderKeyword] = useState("")
  const [leaderCandidates, setLeaderCandidates] = useState<Member[]>([])

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

  useEffect(() => {
    const query = leaderKeyword.trim()
    if (!leaderGroup || query.length < 2) {
      setLeaderCandidates([])
      return
    }

    const timer = setTimeout(async () => {
      try {
        const data = await api<{ users: Member[] }>(
          `/api/admin/users?limit=8&q=${encodeURIComponent(query)}`,
        )
        setLeaderCandidates(data.users.filter((user) => !leaders.some((l) => l.id === user.id)))
      } catch {
        setLeaderCandidates([])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [leaderKeyword, leaderGroup, leaders])

  const counts = useMemo(
    () => ({
      manual: groups.filter((group) => !group.is_upstream).length,
      upstream: groups.filter((group) => group.is_upstream).length,
    }),
    [groups],
  )

  const parentOptions = useMemo(
    () =>
      groups
        .filter((group) => !group.is_upstream)
        .sort((a, b) => a.name.localeCompare(b.name, "zh-Hans-CN")),
    [groups],
  )

  const rows = useMemo(() => {
    const out: { node: GroupNode; depth: number }[] = []
    const walk = (list: GroupNode[], depth: number) => {
      list.forEach((node) => {
        out.push({ node, depth })
        if (expanded[node.group.id] ?? !node.group.is_upstream) walk(node.children, depth + 1)
      })
    }
    walk(buildTree(groups), 0)
    return out
  }, [groups, expanded])

  async function create() {
    setCreating(true)
    try {
      await api("/api/admin/groups", {
        method: "POST",
        body: JSON.stringify({
          name: name.trim(),
          code: code.trim(),
          parent_id: parentID === "" ? null : Number(parentID),
        }),
      })
      setName("")
      setCode("")
      setParentID("")
      setCreateOpen(false)
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

  function openLeaders(group: Group) {
    setLeaderGroup(group)
    setLeaders(group.leaders ?? [])
    setLeaderKeyword("")
    setLeaderCandidates([])
  }

  async function saveLeaders(next: Member[]) {
    if (!leaderGroup) return
    try {
      const data = await api<{ leaders: Member[] }>(`/api/admin/groups/${leaderGroup.id}/leaders`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: next.map((member) => member.id) }),
      })
      setLeaders(data.leaders)
      setGroups((prev) =>
        prev.map((group) =>
          group.id === leaderGroup.id ? { ...group, leaders: data.leaders } : group,
        ),
      )
      setLeaderKeyword("")
      setLeaderCandidates([])
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

  const childCount = pendingDelete
    ? groups.filter((group) => group.parent_id === pendingDelete.id).length
    : 0

  return (
    <div className="space-y-5">
      <Panel
        title="用户组"
        description="自定义组在前；上游组按身份层级折叠，登录时同步成员与父组"
        action={
          <Button className="h-9 w-auto px-3.5" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" strokeWidth={1.75} />
            新建用户组
          </Button>
        }
      >
        <div className="flex items-center gap-4 border-b border-border px-6 py-2.5 text-[12px] text-muted-foreground">
          <span>
            自定义 <span className="text-foreground">{counts.manual}</span>
          </span>
          <span>
            上游 <span className="text-foreground">{counts.upstream}</span>
          </span>
        </div>

        <div className="divide-y divide-border">
          {rows.map(({ node, depth }) => {
            const group = node.group
            const open = expanded[group.id] ?? !group.is_upstream
            const children = node.children.length

            return (
              <div
                key={group.id}
                className="flex items-center justify-between gap-4 py-2.5 pr-6"
                style={{ paddingLeft: 20 + depth * 18 }}
              >
                <div className="flex min-w-0 items-start gap-1">
                  <span className="flex size-5 shrink-0 items-center justify-center">
                    {children > 0 && (
                      <button
                        type="button"
                        aria-expanded={open}
                        aria-label={open ? `收起 ${group.name}` : `展开 ${group.name}`}
                        onClick={() =>
                          setExpanded((prev) => ({ ...prev, [group.id]: !open }))
                        }
                        className="rounded p-0.5 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                      >
                        <ChevronRight
                          className={cn("size-3.5 transition-transform", open && "rotate-90")}
                          strokeWidth={2.25}
                        />
                      </button>
                    )}
                  </span>
                  <div className="min-w-0 pt-0.5">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-[13px] text-foreground">{group.name}</span>
                      {group.is_upstream && (
                        <span className="shrink-0 rounded border border-border px-1.5 py-0.5 text-[11px] text-muted-foreground">
                          上游
                        </span>
                      )}
                      {!open && children > 0 && (
                        <span className="shrink-0 text-[11.5px] text-muted-foreground">
                          {children} 个下级
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 truncate font-mono text-[12px] text-muted-foreground">
                      {group.code}
                    </div>
                    {!group.is_upstream && (
                      <div className="mt-0.5 truncate text-[12px] text-muted-foreground">
                        {group.leaders.length > 0
                          ? `组长 ${group.leaders.map((leader) => leader.name).join("、")}`
                          : "未设组长"}
                      </div>
                    )}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-3 whitespace-nowrap">
                  {group.is_upstream ? (
                    <span className="text-[12.5px] text-muted-foreground">成员随登录同步</span>
                  ) : (
                    <>
                      <Button
                        variant="outline"
                        className="h-8 w-auto px-3"
                        onClick={() => openLeaders(group)}
                      >
                        组长
                      </Button>
                      <Button
                        variant="outline"
                        className="h-8 w-auto px-3"
                        onClick={() => openMembers(group)}
                      >
                        成员
                      </Button>
                    </>
                  )}
                  <button
                    className="text-[12.5px] text-destructive hover:underline"
                    onClick={() => setPendingDelete(group)}
                  >
                    删除
                  </button>
                </div>
              </div>
            )
          })}
        </div>

        {groups.length === 0 && <Empty>还没有用户组</Empty>}
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
          <label className="block">
            <span className="text-[12.5px] text-muted-foreground">父组</span>
            <Select
              className="mt-1.5 h-9 w-full"
              value={parentID}
              onChange={setParentID}
            >
              <option value="">无，作为顶级用户组</option>
              {parentOptions.map((group) => (
                <option key={group.id} value={String(group.id)}>
                  {groupPath(groups, group)}
                </option>
              ))}
            </Select>
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

      <Modal
        open={leaderGroup !== null}
        title={`组长 · ${leaderGroup?.name ?? ""}`}
        description="组长可以维护本组成员，任命后自动加入该组"
        onClose={() => setLeaderGroup(null)}
        width="max-w-[560px]"
        footer={
          <Button
            variant="outline"
            className="h-9 w-auto px-4"
            onClick={() => setLeaderGroup(null)}
          >
            关闭
          </Button>
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
              placeholder="输入学号或姓名添加组长"
              value={leaderKeyword}
              onChange={(event) => setLeaderKeyword(event.target.value)}
            />
          </div>

          {leaderCandidates.length > 0 && (
            <div className="overflow-hidden rounded-md border border-border">
              {leaderCandidates.map((user) => (
                <button
                  key={user.id}
                  onClick={() => saveLeaders([...leaders, user])}
                  className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left transition-colors hover:bg-secondary"
                >
                  <span className="min-w-0">
                    <span className="text-[13px] text-foreground">{user.name}</span>
                    <span className="ml-2 font-mono text-[12px] text-muted-foreground">{user.id}</span>
                  </span>
                  <span className="shrink-0 text-[12.5px] text-primary">设为组长</span>
                </button>
              ))}
            </div>
          )}

          {leaderKeyword.trim().length >= 2 && leaderCandidates.length === 0 && (
            <p className="text-[12px] text-muted-foreground">没有可添加的用户</p>
          )}

          <div className="divide-y divide-border rounded-md border border-border">
            {leaders.map((leader) => (
              <div key={leader.id} className="flex items-center justify-between gap-3 px-3 py-2">
                <div className="min-w-0">
                  <span className="text-[13px] text-foreground">{leader.name}</span>
                  <span className="ml-2 font-mono text-[12px] text-muted-foreground">{leader.id}</span>
                  {leader.status === 1 && (
                    <span className="ml-2 text-[11.5px] text-destructive">已禁用</span>
                  )}
                </div>
                <button
                  className="rounded p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  aria-label={`移除组长 ${leader.name}`}
                  onClick={() => saveLeaders(leaders.filter((item) => item.id !== leader.id))}
                >
                  <X className="size-3.5" strokeWidth={2} />
                </button>
              </div>
            ))}
          </div>

          {leaders.length === 0 && (
            <p className="py-6 text-center text-[12.5px] text-muted-foreground">该组还没有组长</p>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={pendingDelete !== null}
        title="删除用户组"
        message={`删除「${pendingDelete?.name ?? ""}」后，该组的成员关系与授权关联会一并移除${
          childCount > 0 ? `，${childCount} 个下级组会变为顶级组` : ""
        }，操作不可撤销。`}
        confirmLabel="删除"
        danger
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  )
}
