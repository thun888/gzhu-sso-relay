import { useCallback, useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { Empty, Panel, api, tdClass, thClass } from "./shared"

type AdminUser = {
  id: string
  name: string
  email: string
  type: number
  status: number
  last_login_at?: string
}

export function UsersPanel({ onError }: { onError: (message: string) => void }) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [keyword, setKeyword] = useState("")
  const [loading, setLoading] = useState(true)

  const load = useCallback(
    async (query: string) => {
      setLoading(true)
      try {
        const data = await api<{ users: AdminUser[] }>(
          `/api/admin/users?limit=100&q=${encodeURIComponent(query)}`,
        )
        setUsers(data.users)
      } catch (err) {
        onError((err as Error).message)
      } finally {
        setLoading(false)
      }
    },
    [onError],
  )

  useEffect(() => {
    load("")
  }, [load])

  async function patch(id: string, body: Record<string, number>) {
    try {
      await api(`/api/admin/users/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body: JSON.stringify(body),
      })
      await load(keyword)
    } catch (err) {
      onError((err as Error).message)
    }
  }

  return (
    <Panel
      title="用户"
      description="登录过的账号会自动入库，默认是普通用户"
      action={
        <form
          className="flex items-center gap-2"
          onSubmit={(event) => {
            event.preventDefault()
            load(keyword)
          }}
        >
          <Input
            className="h-9 w-[220px]"
            placeholder="按学号或姓名搜索"
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
          />
          <Button type="submit" variant="outline" className="h-9 w-auto px-4">
            搜索
          </Button>
        </form>
      }
    >
      <table className="w-full">
        <thead>
          <tr>
            <th className={thClass}>学号</th>
            <th className={thClass}>姓名</th>
            <th className={thClass}>邮箱</th>
            <th className={thClass}>角色</th>
            <th className={thClass}>状态</th>
            <th className={cn(thClass, "text-right")}>操作</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id} className="transition-colors hover:bg-secondary/40">
              <td className={cn(tdClass, "font-mono text-[12.5px]")}>{user.id}</td>
              <td className={tdClass}>{user.name}</td>
              <td className={cn(tdClass, "text-muted-foreground")}>{user.email}</td>
              <td className={tdClass}>
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[11.5px]",
                    user.type === 1
                      ? "bg-primary/10 text-primary"
                      : "bg-secondary text-muted-foreground",
                  )}
                >
                  {user.type === 1 ? "管理员" : "普通用户"}
                </span>
              </td>
              <td className={tdClass}>
                <span className={user.status === 1 ? "text-destructive" : "text-muted-foreground"}>
                  {user.status === 1 ? "已禁用" : "正常"}
                </span>
              </td>
              <td className={cn(tdClass, "text-right whitespace-nowrap")}>
                <button
                  className="text-[12.5px] text-primary hover:underline"
                  onClick={() => patch(user.id, { type: user.type === 1 ? 0 : 1 })}
                >
                  {user.type === 1 ? "取消管理员" : "设为管理员"}
                </button>
                <button
                  className="ml-4 text-[12.5px] text-muted-foreground hover:underline"
                  onClick={() => patch(user.id, { status: user.status === 1 ? 0 : 1 })}
                >
                  {user.status === 1 ? "启用" : "禁用"}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!loading && users.length === 0 && <Empty>没有匹配的用户</Empty>}
    </Panel>
  )
}
