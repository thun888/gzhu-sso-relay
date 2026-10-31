import { useCallback, useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { Chip, Empty, FormRow, Panel, api, tdClass, thClass } from "./shared"

type Scope = { name: string; display_name: string; description: string }
type Provider = { name: string; display_name: string }
type Group = { id: number; name: string; code: string }

type Client = {
  client_id: string
  name: string
  is_public: boolean
  redirect_urls: string[]
  required_bindings: string[]
  scopes: string[]
  group_ids: number[]
}

const emptyForm = {
  client_id: "",
  name: "",
  redirect: "",
  scopes: [] as string[],
  bindings: [] as string[],
  isPublic: false,
}

export function ClientsPanel({ onError }: { onError: (message: string) => void }) {
  const [clients, setClients] = useState<Client[]>([])
  const [scopes, setScopes] = useState<Scope[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [created, setCreated] = useState("")
  const [expanded, setExpanded] = useState<string | null>(null)
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
                  {client.scopes.length > 0 ? client.scopes.join(" · ") : "无"}
                </td>
                <td className={cn(tdClass, "text-muted-foreground")}>
                  {client.required_bindings.length > 0 ? client.required_bindings.join(" · ") : "无"}
                </td>
                <td className={cn(tdClass, "text-right whitespace-nowrap")}>
                  <button
                    className="text-[12.5px] text-primary hover:underline"
                    onClick={() => setExpanded(expanded === client.client_id ? null : client.client_id)}
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
                  <td colSpan={5} className="border-t border-border bg-secondary/30 px-6 py-4">
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
                        <div className="text-[12px] text-muted-foreground">
                          限定用户组（全不选表示不限制）
                        </div>
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
                            <span className="text-[12px] text-muted-foreground">还没有用户组</span>
                          )}
                        </div>
                      </div>

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
