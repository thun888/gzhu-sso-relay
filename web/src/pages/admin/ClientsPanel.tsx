import { useCallback, useEffect, useState } from "react"
import { Search, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { Chip, Empty, FormRow, Panel, api, tdClass, thClass } from "./shared"

type Scope = { name: string; display_name: string; description: string }
type Provider = { name: string; display_name: string }
type Group = { id: number; name: string; code: string }

type Member = { id: string; name: string; email: string; status: number }

type Client = {
  client_id: string
  name: string
  is_public: boolean
  is_private: boolean
  redirect_urls: string[]
  required_bindings: string[]
  scopes: string[]
  group_ids: number[]
  users: Member[]
}

const emptyForm = {
  client_id: "",
  name: "",
  redirect: "",
  scopes: [] as string[],
  bindings: [] as string[],
  isPublic: false,
  isPrivate: false,
}

export function ClientsPanel({ onError }: { onError: (message: string) => void }) {
  const [clients, setClients] = useState<Client[]>([])
  const [scopes, setScopes] = useState<Scope[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [created, setCreated] = useState("")
  const [expanded, setExpanded] = useState<string | null>(null)
  const [userQuery, setUserQuery] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])
  const [form, setForm] = useState(emptyForm)

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

  useEffect(() => {
    const query = userQuery.trim()
    const client = clients.find((item) => item.client_id === expanded)
    if (!client || query.length < 2) {
      setCandidates([])
      return
    }

    const timer = setTimeout(async () => {
      try {
        const data = await api<{ users: Member[] }>(
          `/api/admin/users?limit=8&q=${encodeURIComponent(query)}`,
        )
        setCandidates(data.users.filter((user) => !client.users.some((m) => m.id === user.id)))
      } catch {
        setCandidates([])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [userQuery, expanded, clients])

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
          is_private: form.isPrivate,
        }),
      })
      setCreated(
        data.client_secret ? `${data.client_id}　${data.client_secret}` : `${data.client_id}（公开客户端无需密钥）`,
      )
      setForm(emptyForm)
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

  async function savePrivate(clientID: string, next: boolean) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(clientID)}`, {
        method: "PATCH",
        body: JSON.stringify({ is_private: next }),
      })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function saveUsers(client: Client, ids: string[]) {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}/users`, {
        method: "PUT",
        body: JSON.stringify({ user_ids: ids }),
      })
      await load()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  function addUser(client: Client, user: Member) {
    setUserQuery("")
    setCandidates([])
    saveUsers(client, [...client.users.map((item) => item.id), user.id])
  }

  function removeUser(client: Client, id: string) {
    saveUsers(
      client,
      client.users.filter((item) => item.id !== id).map((item) => item.id),
    )
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
    <div className="space-y-5">
      <Panel
        title="接入应用"
        description={`共 ${clients.length} 个 · 点击「权限」即时生效，无需保存`}
      >
        <table className="w-full">
          <thead>
            <tr>
              <th className={thClass}>应用</th>
              <th className={thClass}>类型</th>
              <th className={thClass}>访问范围</th>
              <th className={thClass}>scope</th>
              <th className={thClass}>强制绑定</th>
              <th className={cn(thClass, "text-right")}>操作</th>
            </tr>
          </thead>
          <tbody>
            {clients.map((client) => (
              <tr key={client.client_id} className="align-top">
                <td className={tdClass}>
                  <div className="text-foreground">{client.name}</div>
                  <div className="mt-0.5 font-mono text-[12px] text-muted-foreground">
                    {client.client_id}
                  </div>
                </td>
                <td className={tdClass}>
                  <span
                    className={cn(
                      "rounded px-1.5 py-0.5 text-[11.5px]",
                      client.is_public
                        ? "bg-secondary text-muted-foreground"
                        : "bg-primary/10 text-primary",
                    )}
                  >
                    {client.is_public ? "公开" : "机密"}
                  </span>
                </td>
                <td className={cn(tdClass, "text-muted-foreground")}>
                  {client.is_private
                    ? `受限 · ${client.group_ids.length} 组 / ${client.users.length} 人`
                    : "全员"}
                </td>
                <td className={cn(tdClass, "text-muted-foreground")}>
                  {client.scopes.length > 0 ? client.scopes.join(" · ") : "无"}
                </td>
                <td className={cn(tdClass, "text-muted-foreground")}>
                  {client.required_bindings.length > 0 ? client.required_bindings.join(" · ") : "无"}
                </td>
                <td className={cn(tdClass, "text-right whitespace-nowrap")}>
                  <button
                    className="text-[12.5px] text-primary hover:underline"
                    onClick={() => {
                      setUserQuery("")
                      setCandidates([])
                      setExpanded(expanded === client.client_id ? null : client.client_id)
                    }}
                  >
                    {expanded === client.client_id ? "收起" : "权限"}
                  </button>
                  <button
                    className="ml-4 text-[12.5px] text-muted-foreground hover:underline"
                    onClick={() => remove(client.client_id)}
                  >
                    删除
                  </button>
                </td>
              </tr>
            ))}
            {clients.map((client) =>
              expanded === client.client_id ? (
                <tr key={`${client.client_id}-detail`}>
                  <td colSpan={6} className="border-t border-border bg-secondary/30 px-6 py-4">
                    <div className="space-y-3">
                      <div>
                        <div className="text-[12px] text-muted-foreground">允许申请的 scope</div>
                        <div className="mt-1.5 flex flex-wrap gap-1.5">
                          {scopes.map((scope) => (
                            <Chip
                              key={scope.name}
                              active={client.scopes.includes(scope.name)}
                              label={scope.display_name}
                              onClick={() =>
                                saveScopes(
                                  client.client_id,
                                  client.scopes.includes(scope.name)
                                    ? client.scopes.filter((item) => item !== scope.name)
                                    : [...client.scopes, scope.name],
                                )
                              }
                            />
                          ))}
                        </div>
                      </div>

                      {providers.length > 0 && (
                        <div>
                          <div className="text-[12px] text-muted-foreground">授权前必须绑定</div>
                          <div className="mt-1.5 flex flex-wrap gap-1.5">
                            {providers.map((provider) => (
                              <Chip
                                key={provider.name}
                                active={client.required_bindings.includes(provider.name)}
                                label={provider.display_name}
                                onClick={() =>
                                  saveBindings(
                                    client.client_id,
                                    client.required_bindings.includes(provider.name)
                                      ? client.required_bindings.filter((item) => item !== provider.name)
                                      : [...client.required_bindings, provider.name],
                                  )
                                }
                              />
                            ))}
                          </div>
                        </div>
                      )}

                      <div>
                        <div className="text-[12px] text-muted-foreground">访问范围</div>
                        <div className="mt-1.5 flex flex-wrap gap-1.5">
                          <Chip
                            active={!client.is_private}
                            label="所有登录用户"
                            onClick={() => savePrivate(client.client_id, false)}
                          />
                          <Chip
                            active={client.is_private}
                            label="仅指定用户与用户组"
                            onClick={() => savePrivate(client.client_id, true)}
                          />
                        </div>
                      </div>

                      {client.is_private && (
                        <div className="space-y-3 rounded-lg border border-border bg-card px-3 py-3">
                          <div>
                            <div className="text-[12px] text-muted-foreground">允许访问的用户组</div>
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
                              {groups.length === 0 && (
                                <span className="text-[12px] text-muted-foreground">
                                  还没有用户组
                                </span>
                              )}
                            </div>
                          </div>

                          <div>
                            <div className="text-[12px] text-muted-foreground">用户白名单</div>
                            {client.users.length > 0 && (
                              <div className="mt-1.5 flex flex-wrap gap-1.5">
                                {client.users.map((user) => (
                                  <span
                                    key={user.id}
                                    className="inline-flex items-center gap-1.5 rounded-md border border-border py-0.5 pr-1 pl-2 text-[12px] text-foreground"
                                  >
                                    {user.name}
                                    <span className="font-mono text-[11px] text-muted-foreground">
                                      {user.id}
                                    </span>
                                    <button
                                      aria-label={`移除 ${user.name}`}
                                      className="rounded p-0.5 text-muted-foreground hover:text-destructive"
                                      onClick={() => removeUser(client, user.id)}
                                    >
                                      <X className="size-3" strokeWidth={2} />
                                    </button>
                                  </span>
                                ))}
                              </div>
                            )}

                            <div className="relative mt-2 max-w-[320px]">
                              <Search
                                className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
                                strokeWidth={1.75}
                              />
                              <Input
                                className="h-8 pl-8 text-[12.5px]"
                                placeholder="搜索学号或姓名添加"
                                value={userQuery}
                                onChange={(event) => setUserQuery(event.target.value)}
                              />
                            </div>

                            {candidates.length > 0 && (
                              <div className="mt-1.5 max-w-[320px] overflow-hidden rounded-md border border-border">
                                {candidates.map((user) => (
                                  <button
                                    key={user.id}
                                    onClick={() => addUser(client, user)}
                                    className="flex w-full items-center justify-between gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-secondary"
                                  >
                                    <span className="min-w-0 text-[12.5px] text-foreground">
                                      {user.name}
                                      <span className="ml-2 font-mono text-[11.5px] text-muted-foreground">
                                        {user.id}
                                      </span>
                                    </span>
                                    <span className="shrink-0 text-[12px] text-primary">添加</span>
                                  </button>
                                ))}
                              </div>
                            )}

                            {client.users.length === 0 && client.group_ids.length === 0 && (
                              <p className="mt-2 text-[11.5px] text-destructive">
                                白名单为空，当前没有任何账号能授权该应用
                              </p>
                            )}
                          </div>
                        </div>
                      )}

                      <div className="text-[12px] text-muted-foreground">
                        回调地址：{client.redirect_urls.join("，") || "未设置"}
                      </div>
                    </div>
                  </td>
                </tr>
              ) : null,
            )}
          </tbody>
        </table>
        {clients.length === 0 && <Empty>还没有接入应用</Empty>}
      </Panel>

      <Panel title="新建应用" description="创建后会生成 client_secret，仅显示一次">
        <FormRow label="client_id" hint="字母、数字、下划线或连字符，创建后不可修改">
          <Input
            className="h-9 max-w-[320px] font-mono"
            placeholder="例如 gitea"
            value={form.client_id}
            onChange={(event) => setForm({ ...form, client_id: event.target.value })}
          />
        </FormRow>
        <FormRow label="应用名称">
          <Input
            className="h-9 max-w-[320px]"
            placeholder="例如 Gitea"
            value={form.name}
            onChange={(event) => setForm({ ...form, name: event.target.value })}
          />
        </FormRow>
        <FormRow label="回调地址" hint="多个地址用逗号或空格分隔，必须是完整的 http(s) 地址">
          <Input
            className="h-9"
            placeholder="https://git.example.com/user/oauth2/relay/callback"
            value={form.redirect}
            onChange={(event) => setForm({ ...form, redirect: event.target.value })}
          />
        </FormRow>
        <FormRow label="允许 scope">
          <div className="flex flex-wrap gap-1.5">
            {scopes.map((scope) => (
              <Chip
                key={scope.name}
                active={form.scopes.includes(scope.name)}
                label={scope.display_name}
                onClick={() => setForm({ ...form, scopes: toggle(form.scopes, scope.name) })}
              />
            ))}
          </div>
        </FormRow>
        {providers.length > 0 && (
          <FormRow label="强制绑定" hint="用户授权前必须先绑定这些第三方账号">
            <div className="flex flex-wrap gap-1.5">
              {providers.map((provider) => (
                <Chip
                  key={provider.name}
                  active={form.bindings.includes(provider.name)}
                  label={provider.display_name}
                  onClick={() => setForm({ ...form, bindings: toggle(form.bindings, provider.name) })}
                />
              ))}
            </div>
          </FormRow>
        )}
        <FormRow label="访问范围" hint="私有应用只有白名单里的用户或用户组能授权">
          <label className="flex items-center gap-2 text-[12.5px] text-muted-foreground">
            <input
              type="checkbox"
              checked={form.isPrivate}
              onChange={(event) => setForm({ ...form, isPrivate: event.target.checked })}
            />
            私有应用
          </label>
        </FormRow>
        <FormRow label="客户端类型">
          <label className="flex items-center gap-2 text-[12.5px] text-muted-foreground">
            <input
              type="checkbox"
              checked={form.isPublic}
              onChange={(event) => setForm({ ...form, isPublic: event.target.checked })}
            />
            公开客户端（不校验密钥，必须使用 PKCE）
          </label>
        </FormRow>

        {created && (
          <div className="border-b border-border bg-primary/5 px-6 py-3">
            <div className="text-[12px] text-muted-foreground">已创建，请立即保存，之后无法查看</div>
            <div className="mt-1 font-mono text-[12.5px] break-all text-primary">{created}</div>
          </div>
        )}

        <div className="flex justify-end px-6 py-3">
          <Button
            className="h-9 w-auto px-4"
            disabled={!form.client_id.trim() || !form.name.trim()}
            onClick={create}
          >
            创建应用
          </Button>
        </div>
      </Panel>
    </div>
  )
}

function toggle(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((item) => item !== value) : [...list, value]
}
