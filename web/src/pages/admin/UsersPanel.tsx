import { useCallback, useEffect, useState } from "react"
import { RotateCcw, Search } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import type { Group } from "./client-types"
import { ConfirmDialog, Empty, Panel, Select, api, groupPath } from "./shared"

type AdminUser = {
  id: string
  name: string
  email: string
  type: number
  status: number
  last_login_at?: string
}

type Filters = { q: string; type: string; status: string; group: string }

type Pending = {
  id: string
  body: Record<string, number>
  title: string
  message: string
  label: string
  danger?: boolean
}

const initialFilters: Filters = { q: "", type: "", status: "", group: "" }

const headClass =
  "px-4 py-2.5 text-left text-[12px] font-normal text-muted-foreground first:pl-6 last:pr-6"
const cellClass = "border-t border-border px-4 py-3 text-[13px] first:pl-6 last:pr-6"

function formatTime(value?: string) {
  if (!value) return "从未登录"
  const date = new Date(value)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function UsersPanel({ onError }: { onError: (message: string) => void }) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [keyword, setKeyword] = useState("")
  const [filters, setFilters] = useState<Filters>(initialFilters)
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState<Pending | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ limit: "100" })
      if (filters.q) params.set("q", filters.q)
      if (filters.type) params.set("type", filters.type)
      if (filters.status) params.set("status", filters.status)
      if (filters.group) params.set("group_id", filters.group)

      const data = await api<{ users: AdminUser[] }>(`/api/admin/users?${params}`)
      setUsers(data.users)
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setLoading(false)
    }
  }, [filters, onError])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    api<{ groups: Group[] }>("/api/admin/groups")
      .then((data) => setGroups(data.groups))
      .catch(() => undefined)
  }, [])

  async function patch(id: string, body: Record<string, number>) {
    try {
      await api(`/api/admin/users/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body: JSON.stringify(body),
      })
      setPending(null)
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  function toggleAdmin(user: AdminUser) {
    if (user.type === 1) {
      setPending({
        id: user.id,
        body: { type: 0 },
        title: "取消管理员",
        message: `「${user.name}」将失去管理后台的全部权限，确定继续？`,
        label: "取消管理员",
      })
      return
    }
    patch(user.id, { type: 1 })
  }

  function toggleStatus(user: AdminUser) {
    if (user.status === 1) {
      patch(user.id, { status: 0 })
      return
    }
    setPending({
      id: user.id,
      body: { status: 1 },
      title: "禁用账号",
      message: `禁用后「${user.name}」无法登录，已登录的会话会在下次请求时失效。`,
      label: "禁用",
      danger: true,
    })
  }

  const filtered = Boolean(filters.q || filters.type || filters.status || filters.group)

  return (
    <Panel title="用户" description={`共 ${users.length} 人`}>
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-6 py-3">
        <form
          className="flex items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            setFilters((prev) => ({ ...prev, q: keyword.trim() }))
          }}
        >
          <div className="relative">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
              strokeWidth={1.75}
            />
            <Input
              className="h-9 w-[220px] pl-9"
              placeholder="学号或姓名"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
            />
          </div>
        </form>

        <Select
          value={filters.type}
          onChange={(value) => setFilters((prev) => ({ ...prev, type: value }))}
        >
          <option value="">全部角色</option>
          <option value="1">管理员</option>
          <option value="0">普通用户</option>
        </Select>

        <Select
          value={filters.status}
          onChange={(value) => setFilters((prev) => ({ ...prev, status: value }))}
        >
          <option value="">全部状态</option>
          <option value="0">正常</option>
          <option value="1">已禁用</option>
        </Select>

        <Select
          value={filters.group}
          onChange={(value) => setFilters((prev) => ({ ...prev, group: value }))}
        >
          <option value="">全部用户组</option>
          {groups.map((group) => (
            <option key={group.id} value={String(group.id)}>
              {groupPath(groups, group)}
            </option>
          ))}
        </Select>

        {filtered && (
          <Button
            variant="ghost"
            className="h-9 w-auto px-2.5 text-muted-foreground"
            onClick={() => {
              setKeyword("")
              setFilters(initialFilters)
            }}
          >
            <RotateCcw className="size-3.5" strokeWidth={1.75} />
            重置
          </Button>
        )}
      </div>

      <table className="w-full table-fixed">
        <colgroup>
          <col className="w-[150px]" />
          <col className="w-[110px]" />
          <col />
          <col className="w-[92px]" />
          <col className="w-[84px]" />
          <col className="w-[148px]" />
          <col className="w-[180px]" />
        </colgroup>
        <thead>
          <tr>
            <th className={headClass}>学号</th>
            <th className={headClass}>姓名</th>
            <th className={headClass}>邮箱</th>
            <th className={headClass}>角色</th>
            <th className={headClass}>状态</th>
            <th className={headClass}>最后登录</th>
            <th className={cn(headClass, "text-right")}>操作</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id} className="transition-colors hover:bg-secondary/40">
              <td className={cn(cellClass, "truncate font-mono text-[12.5px]")}>{user.id}</td>
              <td className={cn(cellClass, "truncate")}>{user.name}</td>
              <td className={cn(cellClass, "truncate text-muted-foreground")}>{user.email}</td>
              <td className={cellClass}>
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[11.5px]",
                    user.type === 1
                      ? "bg-primary/10 text-primary"
                      : "bg-secondary text-muted-foreground",
                  )}
                >
                  {user.type === 1 ? "管理员" : "普通"}
                </span>
              </td>
              <td className={cellClass}>
                <span className={user.status === 1 ? "text-destructive" : "text-muted-foreground"}>
                  {user.status === 1 ? "已禁用" : "正常"}
                </span>
              </td>
              <td className={cn(cellClass, "text-[12.5px] text-muted-foreground")}>
                {formatTime(user.last_login_at)}
              </td>
              <td className={cn(cellClass, "text-right whitespace-nowrap")}>
                <button
                  className={cn(
                    "text-[12.5px] hover:underline",
                    user.type === 1 ? "text-muted-foreground" : "text-primary",
                  )}
                  onClick={() => toggleAdmin(user)}
                >
                  {user.type === 1 ? "取消管理员" : "设为管理员"}
                </button>
                <button
                  className={cn(
                    "ml-4 text-[12.5px] hover:underline",
                    user.status === 1 ? "text-primary" : "text-destructive",
                  )}
                  onClick={() => toggleStatus(user)}
                >
                  {user.status === 1 ? "启用" : "禁用"}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {!loading && users.length === 0 && (
        <Empty>{filtered ? "没有匹配的用户" : "还没有用户"}</Empty>
      )}

      <ConfirmDialog
        open={pending !== null}
        title={pending?.title ?? ""}
        message={pending?.message ?? ""}
        confirmLabel={pending?.label}
        danger={pending?.danger}
        onCancel={() => setPending(null)}
        onConfirm={() => pending && patch(pending.id, pending.body)}
      />
    </Panel>
  )
}
