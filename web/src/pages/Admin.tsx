import { useCallback, useEffect, useState } from "react"

import { PageShell } from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

type AdminUser = {
  id: string
  name: string
  email: string
  type: number
  status: number
  last_login_at?: string
}

type Group = { id: number; name: string; code: string; parent_id?: number | null }
type Scope = { name: string; display_name: string; description: string }
type Provider = { name: string; display_name: string }

type Client = {
  client_id: string
  name: string
  is_public: boolean
  redirect_urls: string[]
  required_bindings: string[]
  scopes: string[]
  group_ids: number[]
}

type Tab = "users" | "groups" | "clients"

async function api<T>(path: string, init?: RequestInit): Promise<T> {
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

export default function Admin({ siteTitle }: { siteTitle: string }) {
  const [tab, setTab] = useState<Tab>(() => {
    const fromURL = new URLSearchParams(window.location.search).get("tab")
    return fromURL === "groups" || fromURL === "clients" ? fromURL : "users"
  })
  const [error, setError] = useState("")

  return (
    <PageShell width="max-w-[1000px]">
      <div className="px-7 py-8 sm:px-10">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            {siteTitle && <span className="size-1.5 rounded-full bg-primary" />}
            <span className="text-[13px] font-medium text-foreground">
              {siteTitle ? `${siteTitle} · 管理后台` : "管理后台"}
            </span>
          </div>
          <a href="/" className="text-[12.5px] text-muted-foreground hover:text-foreground">
            返回用户中心
          </a>
        </div>

        <div className="mt-5 flex gap-1 rounded-lg bg-secondary p-1">
          {(
            [
              ["users", "用户"],
              ["groups", "用户组"],
              ["clients", "应用"],
            ] as [Tab, string][]
          ).map(([key, label]) => (
            <button
              key={key}
              onClick={() => {
                setTab(key)
                setError("")
              }}
              className={cn(
                "flex-1 rounded-md px-3 py-1.5 text-[13px] transition-colors",
                tab === key
                  ? "bg-background text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {label}
            </button>
          ))}
        </div>

        {error && (
          <p className="mt-4 rounded-lg bg-destructive/10 px-3 py-2 text-[12.5px] text-destructive">
            {error}
          </p>
        )}

        <div className="mt-5">
          {tab === "users" && <UsersTab onError={setError} />}
          {tab === "groups" && <GroupsTab onError={setError} />}
          {tab === "clients" && <ClientsTab onError={setError} />}
        </div>
      </div>
    </PageShell>
  )
}

function UsersTab({ onError }: { onError: (message: string) => void }) {
  const [users, setUsers] = useState<AdminUser[]>([])
  const [keyword, setKeyword] = useState("")

  const load = useCallback(
    async (q: string) => {
      try {
        const data = await api<{ users: AdminUser[] }>(`/api/admin/users?limit=100&q=${encodeURIComponent(q)}`)
        setUsers(data.users)
      } catch (err) {
        onError((err as Error).message)
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
    <div>
      <form
        className="flex gap-2"
        onSubmit={(event) => {
          event.preventDefault()
          load(keyword)
        }}
      >
        <Input
          className="h-9"
          placeholder="按学号或姓名搜索"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
        />
        <Button type="submit" variant="outline" className="h-9 w-auto px-4">
          搜索
        </Button>
      </form>

      <table className="mt-4 w-full text-[13px]">
        <thead>
          <tr className="text-left text-[12px] text-muted-foreground">
            <th className="pb-2 font-normal">学号</th>
            <th className="pb-2 font-normal">姓名</th>
            <th className="pb-2 font-normal">角色</th>
            <th className="pb-2 font-normal">状态</th>
            <th className="pb-2 text-right font-normal">操作</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <tr key={user.id} className="border-t border-border">
              <td className="py-2.5 pr-3">{user.id}</td>
              <td className="py-2.5 pr-3">{user.name}</td>
              <td className="py-2.5 pr-3">
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[11.5px]",
                    user.type === 1 ? "bg-primary/10 text-primary" : "bg-secondary text-muted-foreground",
                  )}
                >
                  {user.type === 1 ? "管理员" : "普通用户"}
                </span>
              </td>
              <td className="py-2.5 pr-3">
                <span className={cn(user.status === 1 ? "text-destructive" : "text-muted-foreground")}>
                  {user.status === 1 ? "已禁用" : "正常"}
                </span>
              </td>
              <td className="py-2.5 text-right whitespace-nowrap">
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
          {users.length === 0 && (
            <tr>
              <td colSpan={5} className="py-8 text-center text-muted-foreground">
                没有匹配的用户
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

function GroupsTab({ onError }: { onError: (message: string) => void }) {
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
        body: JSON.stringify({ name, code }),
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
    <div>
      <div className="flex flex-wrap gap-2">
        <Input
          className="h-9 w-[180px]"
          placeholder="组名"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <Input
          className="h-9 w-[140px]"
          placeholder="标识 slug"
          value={code}
          onChange={(event) => setCode(event.target.value)}
        />
        <Button className="h-9 w-auto px-4" disabled={!name.trim() || !code.trim()} onClick={create}>
          新建用户组
        </Button>
      </div>

      <div className="mt-4 space-y-2">
        {groups.map((group) => (
          <div key={group.id} className="rounded-lg border border-border">
            <div className="flex items-center justify-between gap-4 px-4 py-2.5">
              <div className="min-w-0">
                <div className="text-[13.5px] text-foreground">{group.name}</div>
                <div className="text-[12px] text-muted-foreground">{group.code}</div>
              </div>
              <div className="flex items-center gap-4 whitespace-nowrap">
                <button
                  className="text-[12.5px] text-primary hover:underline"
                  onClick={() => (editing === group.id ? setEditing(null) : openMembers(group.id))}
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
              <div className="border-t border-border px-4 py-3">
                <Input
                  className="h-9"
                  placeholder="学号，逗号分隔"
                  value={members}
                  onChange={(event) => setMembers(event.target.value)}
                />
                <div className="mt-2 flex justify-end">
                  <Button className="h-8 w-auto px-4" onClick={() => saveMembers(group.id)}>
                    保存成员
                  </Button>
                </div>
              </div>
            )}
          </div>
        ))}
        {groups.length === 0 && (
          <p className="py-8 text-center text-[13px] text-muted-foreground">还没有用户组</p>
        )}
      </div>
    </div>
  )
}

function ClientsTab({ onError }: { onError: (message: string) => void }) {
  const [clients, setClients] = useState<Client[]>([])
  const [scopes, setScopes] = useState<Scope[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [created, setCreated] = useState("")
  const [editing, setEditing] = useState<string | null>(null)

  const [form, setForm] = useState({
    client_id: "",
    name: "",
    redirect: "",
    scopes: [] as string[],
    bindings: [] as string[],
    isPublic: false,
  })

  const load = useCallback(async () => {
    try {
      const [clientData, scopeData, providerData, groupData] = await Promise.all([
        api<{ clients: Client[] }>("/api/admin/clients"),
        api<{ scopes: Scope[] }>("/api/admin/scopes"),
        api<{ providers: Provider[] }>("/api/admin/bindings"),
        api<{ groups: Group[] }>("/api/admin/groups"),
      ])
      setClients(clientData.clients)
      setScopes(scopeData.scopes)
      setProviders(providerData.providers)
      setGroups(groupData.groups)
    } catch (err) {
      onError((err as Error).message)
    }
  }, [onError])

  useEffect(() => {
    load()
  }, [load])

  async function create() {
    try {
      const data = await api<{ client_id: string; client_secret: string }>("/api/admin/clients", {
        method: "POST",
        body: JSON.stringify({
          client_id: form.client_id.trim(),
          name: form.name.trim(),
          redirect_urls: form.redirect
            .split(/[\s,]+/)
            .map((item) => item.trim())
            .filter(Boolean),
          scopes: form.scopes,
          required_bindings: form.bindings,
          is_public: form.isPublic,
        }),
      })
      setCreated(data.client_secret ? `${data.client_id} / ${data.client_secret}` : `${data.client_id}（公开客户端）`)
      setForm({ client_id: "", name: "", redirect: "", scopes: [], bindings: [], isPublic: false })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function remove(clientID: string) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(clientID)}`, { method: "DELETE" })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function saveScopes(clientID: string, next: string[]) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(clientID)}/scopes`, {
        method: "PUT",
        body: JSON.stringify({ scopes: next }),
      })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function saveGroups(clientID: string, next: number[]) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(clientID)}/groups`, {
        method: "PUT",
        body: JSON.stringify({ group_ids: next }),
      })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function saveBindings(clientID: string, next: string[]) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(clientID)}`, {
        method: "PATCH",
        body: JSON.stringify({ required_bindings: next }),
      })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  return (
    <div>
      <div className="rounded-lg border border-border p-4">
        <div className="text-[12px] text-muted-foreground">新建应用</div>
        <div className="mt-2 flex flex-wrap gap-2">
          <Input
            className="h-9 w-[160px]"
            placeholder="client_id"
            value={form.client_id}
            onChange={(event) => setForm({ ...form, client_id: event.target.value })}
          />
          <Input
            className="h-9 w-[160px]"
            placeholder="应用名称"
            value={form.name}
            onChange={(event) => setForm({ ...form, name: event.target.value })}
          />
          <Input
            className="h-9 flex-1"
            placeholder="回调地址，多个用逗号分隔"
            value={form.redirect}
            onChange={(event) => setForm({ ...form, redirect: event.target.value })}
          />
        </div>

        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-[12px] text-muted-foreground">允许的 scope</span>
          {scopes.map((scope) => (
            <Chip
              key={scope.name}
              active={form.scopes.includes(scope.name)}
              label={scope.display_name}
              onClick={() =>
                setForm({
                  ...form,
                  scopes: toggle(form.scopes, scope.name),
                })
              }
            />
          ))}
        </div>

        {providers.length > 0 && (
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <span className="mr-1 text-[12px] text-muted-foreground">强制绑定</span>
            {providers.map((provider) => (
              <Chip
                key={provider.name}
                active={form.bindings.includes(provider.name)}
                label={provider.display_name}
                onClick={() =>
                  setForm({
                    ...form,
                    bindings: toggle(form.bindings, provider.name),
                  })
                }
              />
            ))}
          </div>
        )}

        <div className="mt-3 flex items-center justify-between">
          <label className="flex items-center gap-2 text-[12.5px] text-muted-foreground">
            <input
              type="checkbox"
              checked={form.isPublic}
              onChange={(event) => setForm({ ...form, isPublic: event.target.checked })}
            />
            公开客户端（不校验密钥，必须使用 PKCE）
          </label>
          <Button
            className="h-9 w-auto px-4"
            disabled={!form.client_id.trim() || !form.name.trim()}
            onClick={create}
          >
            创建
          </Button>
        </div>

        {created && (
          <p className="mt-3 rounded-md bg-primary/10 px-3 py-2 text-[12px] break-all text-primary">
            已创建，密钥只显示一次：{created}
          </p>
        )}
      </div>

      <div className="mt-4 space-y-2">
        {clients.map((client) => (
          <div key={client.client_id} className="rounded-lg border border-border">
            <div className="flex items-center justify-between gap-4 px-4 py-2.5">
              <div className="min-w-0">
                <div className="text-[13.5px] text-foreground">
                  {client.name}
                  {client.is_public && (
                    <span className="ml-2 rounded bg-secondary px-1.5 py-0.5 text-[11px] text-muted-foreground">
                      公开
                    </span>
                  )}
                </div>
                <div className="truncate text-[12px] text-muted-foreground">{client.client_id}</div>
              </div>
              <div className="flex items-center gap-4 whitespace-nowrap">
                <button
                  className="text-[12.5px] text-primary hover:underline"
                  onClick={() => setEditing(editing === client.client_id ? null : client.client_id)}
                >
                  {editing === client.client_id ? "收起" : "权限"}
                </button>
                <button
                  className="text-[12.5px] text-muted-foreground hover:underline"
                  onClick={() => remove(client.client_id)}
                >
                  删除
                </button>
              </div>
            </div>

            {editing === client.client_id && (
              <div className="space-y-3 border-t border-border px-4 py-3">
                <div>
                  <div className="text-[12px] text-muted-foreground">scope</div>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {scopes.map((scope) => (
                      <Chip
                        key={scope.name}
                        active={client.scopes.includes(scope.name)}
                        label={scope.display_name}
                        onClick={() =>
                          saveScopes(client.client_id, toggle(client.scopes, scope.name))
                        }
                      />
                    ))}
                  </div>
                </div>

                {providers.length > 0 && (
                  <div>
                    <div className="text-[12px] text-muted-foreground">强制绑定</div>
                    <div className="mt-1.5 flex flex-wrap gap-1.5">
                      {providers.map((provider) => (
                        <Chip
                          key={provider.name}
                          active={client.required_bindings.includes(provider.name)}
                          label={provider.display_name}
                          onClick={() =>
                            saveBindings(client.client_id, toggle(client.required_bindings, provider.name))
                          }
                        />
                      ))}
                    </div>
                  </div>
                )}

                <div>
                  <div className="text-[12px] text-muted-foreground">限定用户组（全不选表示不限制）</div>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {groups.map((group) => (
                      <Chip
                        key={group.id}
                        active={client.group_ids.includes(group.id)}
                        label={group.name}
                        onClick={() =>
                          saveGroups(
                            client.client_id,
                            client.group_ids.includes(group.id)
                              ? client.group_ids.filter((id) => id !== group.id)
                              : [...client.group_ids, group.id],
                          )
                        }
                      />
                    ))}
                  </div>
                </div>

                <div className="text-[12px] text-muted-foreground">
                  回调地址：{client.redirect_urls.join("，") || "未设置"}
                </div>
              </div>
            )}
          </div>
        ))}
        {clients.length === 0 && (
          <p className="py-8 text-center text-[13px] text-muted-foreground">还没有接入应用</p>
        )}
      </div>
    </div>
  )
}

function Chip({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-md border px-2 py-1 text-[12px] transition-colors",
        active
          ? "border-primary/40 bg-primary/10 text-primary"
          : "border-border text-muted-foreground hover:text-foreground",
      )}
    >
      {label}
    </button>
  )
}

function toggle(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((item) => item !== value) : [...list, value]
}
