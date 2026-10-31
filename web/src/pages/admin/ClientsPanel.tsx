import { useCallback, useEffect, useState } from "react"
import { Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Modal } from "@/components/ui/modal"
import { cn } from "@/lib/utils"
import { ClientDetail } from "./ClientDetail"
import type { Client, Group, Provider, Scope } from "./client-types"
import { Empty, Panel, api } from "./shared"

const emptyForm = { name: "", redirect: "", isPrivate: false }

export function ClientsPanel({ onError }: { onError: (message: string) => void }) {
  const [clients, setClients] = useState<Client[]>([])
  const [scopes, setScopes] = useState<Scope[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [groups, setGroups] = useState<Group[]>([])

  const [active, setActive] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [form, setForm] = useState(emptyForm)
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState("")

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
    setCreating(true)
    try {
      const data = await api<{ client_id: string; client_secret: string }>("/api/admin/clients", {
        method: "POST",
        body: JSON.stringify({
          name: form.name.trim(),
          redirect_urls: [form.redirect.trim()],
          scopes: ["openid", "profile"],
          is_private: form.isPrivate,
        }),
      })
      setCreated(
        data.client_secret
          ? `${data.client_id}　${data.client_secret}`
          : `${data.client_id}（公开客户端无需密钥）`,
      )
      setForm(emptyForm)
      setCreateOpen(false)
      await load()
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const current = clients.find((client) => client.client_id === active)
  if (current) {
    return (
      <ClientDetail
        key={current.client_id}
        client={current}
        scopes={scopes}
        providers={providers}
        groups={groups}
        onBack={() => setActive(null)}
        onReload={load}
        onError={onError}
      />
    )
  }

  return (
    <div className="space-y-5">
      <Panel
        title="应用"
        description={`共 ${clients.length} 个 · 点击应用名进入配置`}
        action={
          <Button className="h-9 w-auto px-3.5" onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" strokeWidth={1.75} />
            新建应用
          </Button>
        }
      >
        {created && (
          <div className="flex items-start justify-between gap-4 border-b border-border bg-primary/5 px-6 py-3">
            <div className="min-w-0">
              <div className="text-[12px] text-muted-foreground">
                已创建，client_id 与密钥只显示这一次
              </div>
              <div className="mt-1 font-mono text-[12.5px] break-all text-primary">{created}</div>
            </div>
            <button
              className="shrink-0 text-[12px] text-muted-foreground hover:text-foreground"
              onClick={() => setCreated("")}
            >
              关闭
            </button>
          </div>
        )}

        <div className="divide-y divide-border">
          {clients.map((client) => (
            <div key={client.client_id} className="flex items-center gap-4 px-6 py-4">
              {client.logo_url ? (
                <img
                  src={client.logo_url}
                  alt={client.name}
                  width={44}
                  height={44}
                  className="rounded-lg border border-border object-cover"
                />
              ) : (
                <div className="flex size-11 items-center justify-center rounded-lg bg-secondary text-[15px] font-medium text-muted-foreground">
                  {client.name.slice(0, 1)}
                </div>
              )}

              <button
                className="min-w-0 flex-1 text-left"
                onClick={() => setActive(client.client_id)}
              >
                <div className="truncate text-[13.5px] font-medium text-primary hover:underline">
                  {client.name}
                </div>
                {client.description && (
                  <div className="mt-0.5 truncate text-[12px] text-muted-foreground">
                    {client.description}
                  </div>
                )}
              </button>

              {!client.enabled && (
                <span className="shrink-0 rounded bg-destructive/10 px-1.5 py-0.5 text-[11.5px] text-destructive">
                  已停用
                </span>
              )}

              <span
                className={cn(
                  "shrink-0 rounded px-1.5 py-0.5 text-[11.5px]",
                  client.is_private
                    ? "bg-secondary text-muted-foreground"
                    : "bg-primary/10 text-primary",
                )}
              >
                {client.is_private ? "受限" : "全员"}
              </span>

              <Button
                variant="outline"
                className="h-8 w-auto shrink-0 px-3"
                onClick={() => setActive(client.client_id)}
              >
                编辑
              </Button>
            </div>
          ))}
        </div>

        {clients.length === 0 && <Empty>还没有接入应用</Empty>}
      </Panel>

      <Modal
        open={createOpen}
        title="新建应用"
        description="client_id 与密钥在创建后自动生成，只显示一次"
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
              disabled={!form.name.trim() || !form.redirect.trim() || creating}
              onClick={create}
            >
              {creating ? "创建中" : "创建"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <label className="block">
            <span className="block text-[12.5px] text-muted-foreground">应用名称</span>
            <Input
              className="mt-1.5 h-9"
              placeholder="例如 Gitea"
              value={form.name}
              onChange={(event) => setForm({ ...form, name: event.target.value })}
              autoFocus
            />
          </label>
          <label className="block">
            <span className="block text-[12.5px] text-muted-foreground">回调地址</span>
            <Input
              className="mt-1.5 h-9 font-mono text-[12.5px]"
              placeholder="https://example.com/callback"
              value={form.redirect}
              onChange={(event) => setForm({ ...form, redirect: event.target.value })}
            />
            <span className="mt-1.5 block text-[12px] text-muted-foreground">
              必须与客户端发起授权时使用的地址完全一致，创建后还能继续添加
            </span>
          </label>
          <label className="flex items-center gap-2 text-[12.5px] text-muted-foreground">
            <input
              type="checkbox"
              checked={form.isPrivate}
              onChange={(event) => setForm({ ...form, isPrivate: event.target.checked })}
            />
            私有应用，创建后到应用详情里配置用户与用户组白名单
          </label>
        </div>
      </Modal>
    </div>
  )
}
