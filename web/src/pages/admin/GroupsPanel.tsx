import { useCallback, useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Empty, FormRow, Panel, api } from "./shared"

type Group = { id: number; name: string; code: string; parent_id?: number | null }

export function GroupsPanel({ onError }: { onError: (message: string) => void }) {
  const [groups, setGroups] = useState<Group[]>([])
  const [name, setName] = useState("")
  const [code, setCode] = useState("")
  const [editing, setEditing] = useState<number | null>(null)
  const [members, setMembers] = useState("")

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

  async function create() {
    try {
      await api("/api/admin/groups", {
        method: "POST",
        body: JSON.stringify({ name: name.trim(), code: code.trim() }),
      })
      setName("")
      setCode("")
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function remove(id: number) {
    try {
      await api(`/api/admin/groups/${id}`, { method: "DELETE" })
      if (editing === id) setEditing(null)
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function openMembers(id: number) {
    if (editing === id) {
      setEditing(null)
      return
    }
    try {
      const data = await api<{ user_ids: string[] }>(`/api/admin/groups/${id}/members`)
      setMembers(data.user_ids.join(", "))
      setEditing(id)
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function saveMembers(id: number) {
    const ids = members
      .split(/[,，\s]+/)
      .map((item) => item.trim())
      .filter(Boolean)

    try {
      await api(`/api/admin/groups/${id}/members`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: ids }),
      })
      setEditing(null)
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  return (
    <div className="space-y-5">
      <Panel title="新建用户组" description="用户组用于控制哪些人可以访问哪些应用">
        <FormRow label="组名">
          <Input
            className="h-9 max-w-[320px]"
            placeholder="例如 计算机学院"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </FormRow>
        <FormRow label="标识" hint="用于程序引用的唯一 slug，创建后不建议修改">
          <Input
            className="h-9 max-w-[320px] font-mono"
            placeholder="例如 cs"
            value={code}
            onChange={(event) => setCode(event.target.value)}
          />
        </FormRow>
        <div className="flex justify-end px-6 py-3">
          <Button
            className="h-9 w-auto px-4"
            disabled={!name.trim() || !code.trim()}
            onClick={create}
          >
            创建
          </Button>
        </div>
      </Panel>

      <Panel title="全部用户组" description={`共 ${groups.length} 个`}>
        <div className="divide-y divide-border">
          {groups.map((group) => (
            <div key={group.id}>
              <div className="flex items-center justify-between gap-4 px-6 py-3">
                <div className="min-w-0">
                  <div className="text-[13px] text-foreground">{group.name}</div>
                  <div className="mt-0.5 font-mono text-[12px] text-muted-foreground">
                    {group.code}
                  </div>
                </div>
                <div className="flex items-center gap-4 whitespace-nowrap">
                  <button
                    className="text-[12.5px] text-primary hover:underline"
                    onClick={() => openMembers(group.id)}
                  >
                    {editing === group.id ? "收起" : "成员"}
                  </button>
                  <button
                    className="text-[12.5px] text-muted-foreground hover:underline"
                    onClick={() => remove(group.id)}
                  >
                    删除
                  </button>
                </div>
              </div>

              {editing === group.id && (
                <div className="border-t border-border bg-secondary/30 px-6 py-3">
                  <Input
                    className="h-9"
                    placeholder="学号，逗号或空格分隔"
                    value={members}
                    onChange={(event) => setMembers(event.target.value)}
                  />
                  <div className="mt-2 flex items-center justify-between">
                    <span className="text-[12px] text-muted-foreground">
                      保存会整体替换该组成员
                    </span>
                    <Button className="h-8 w-auto px-4" onClick={() => saveMembers(group.id)}>
                      保存成员
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
        {groups.length === 0 && <Empty>还没有用户组</Empty>}
      </Panel>
    </div>
  )
}
