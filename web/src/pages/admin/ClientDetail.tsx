import { useEffect, useRef, useState } from "react"
import { ArrowLeft, Copy, Search, Upload, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { Client, Group, Member, Provider, Scope } from "./client-types"
import { Chip, ConfirmDialog, api } from "./shared"

type Props = {
  client: Client
  scopes: Scope[]
  providers: Provider[]
  groups: Group[]
  onBack: () => void
  onReload: () => Promise<void>
  onError: (message: string) => void
}

function Section({
  title,
  description,
  action,
  children,
}: {
  title: string
  description?: string
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className="border-t border-border px-6 py-5">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-[14px] font-medium text-foreground">{title}</h2>
          {description && (
            <p className="mt-1 text-[12px] leading-relaxed text-muted-foreground">{description}</p>
          )}
        </div>
        {action}
      </div>
      <div className="mt-3">{children}</div>
    </section>
  )
}

function Logo({ client, size }: { client: Client; size: number }) {
  if (client.logo_url) {
    return (
      <img
        src={client.logo_url}
        alt={client.name}
        width={size}
        height={size}
        className="rounded-lg border border-border object-cover"
      />
    )
  }
  return (
    <div
      className="flex items-center justify-center rounded-lg bg-secondary text-[15px] font-medium text-muted-foreground"
      style={{ width: size, height: size }}
    >
      {client.name.slice(0, 1)}
    </div>
  )
}

function sameSet<T>(a: T[], b: T[]) {
  return a.length === b.length && a.every((item) => b.includes(item))
}

export function ClientDetail({
  client,
  scopes,
  providers,
  groups,
  onBack,
  onReload,
  onError,
}: Props) {
  const [name, setName] = useState(client.name)
  const [homepage, setHomepage] = useState(client.homepage_url)
  const [description, setDescription] = useState(client.description)
  const [redirects, setRedirects] = useState(client.redirect_urls)
  const [selectedScopes, setSelectedScopes] = useState(client.scopes)
  const [bindings, setBindings] = useState(client.required_bindings)
  const [isPrivate, setIsPrivate] = useState(client.is_private)
  const [groupIds, setGroupIds] = useState<number[]>(client.group_ids)
  const [members, setMembers] = useState(client.users)

  const [redirect, setRedirect] = useState("")
  const [rotated, setRotated] = useState("")
  const [copied, setCopied] = useState(false)
  const [pendingDelete, setPendingDelete] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [saving, setSaving] = useState(false)

  const [userQuery, setUserQuery] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])

  const logoInput = useRef<HTMLInputElement | null>(null)

  function resetDrafts() {
    setName(client.name)
    setHomepage(client.homepage_url)
    setDescription(client.description)
    setRedirects(client.redirect_urls)
    setSelectedScopes(client.scopes)
    setBindings(client.required_bindings)
    setIsPrivate(client.is_private)
    setGroupIds(client.group_ids)
    setMembers(client.users)
  }

  useEffect(() => {
    resetDrafts()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client])

  useEffect(() => {
    const query = userQuery.trim()
    if (!isPrivate || query.length < 2) {
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
  }, [userQuery, isPrivate, members])

  const dirty =
    name.trim() !== client.name ||
    homepage.trim() !== client.homepage_url ||
    description.trim() !== client.description ||
    redirects.join("\n") !== client.redirect_urls.join("\n") ||
    !sameSet(selectedScopes, client.scopes) ||
    !sameSet(bindings, client.required_bindings) ||
    isPrivate !== client.is_private ||
    !sameSet(groupIds, client.group_ids) ||
    !sameSet(
      members.map((m) => m.id),
      client.users.map((u) => u.id),
    )

  async function save() {
    setSaving(true)
    try {
      const body: Record<string, unknown> = {}
      if (name.trim() !== client.name) body.name = name.trim()
      if (homepage.trim() !== client.homepage_url) body.homepage_url = homepage.trim()
      if (description.trim() !== client.description) body.description = description.trim()
      if (redirects.join("\n") !== client.redirect_urls.join("\n")) body.redirect_urls = redirects
      if (!sameSet(bindings, client.required_bindings)) body.required_bindings = bindings
      if (isPrivate !== client.is_private) body.is_private = isPrivate
      if (Object.keys(body).length > 0) {
        await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}`, {
          method: "PATCH",
          body: JSON.stringify(body),
        })
      }

      if (!sameSet(selectedScopes, client.scopes)) {
        await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}/scopes`, {
          method: "PUT",
          body: JSON.stringify({ scopes: selectedScopes }),
        })
      }

      if (isPrivate) {
        if (!sameSet(groupIds, client.group_ids)) {
          await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}/groups`, {
            method: "PUT",
            body: JSON.stringify({ group_ids: groupIds }),
          })
        }
        if (
          !sameSet(
            members.map((m) => m.id),
            client.users.map((u) => u.id),
          )
        ) {
          await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}/users`, {
            method: "PUT",
            body: JSON.stringify({ user_ids: members.map((m) => m.id) }),
          })
        }
      }

      await onReload()
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  async function rotateSecret() {
    try {
      const data = await api<{ client_id: string; client_secret: string }>(
        `/api/admin/clients/${encodeURIComponent(client.client_id)}/secret`,
        { method: "POST" },
      )
      setRotated(data.client_secret)
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function uploadLogo(file: File) {
    if (file.size > 1<<20) {
      onError("图片不能超过 1MB")
      return
    }
    setUploading(true)
    try {
      const form = new FormData()
      form.append("file", file)
      const res = await fetch(`/api/admin/clients/${encodeURIComponent(client.client_id)}/logo`, {
        method: "POST",
        body: form,
      })
      if (!res.ok) {
        const data = (await res.json().catch(() => null)) as { error?: string } | null
        throw new Error(data?.error ?? "上传失败")
      }
      await onReload()
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setUploading(false)
    }
  }

  async function removeLogo() {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}/logo`, {
        method: "DELETE",
      })
      await onReload()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  async function removeClient() {
    try {
      await api(`/api/admin/clients/${encodeURIComponent(client.client_id)}`, { method: "DELETE" })
      setPendingDelete(false)
      onBack()
    } catch (err) {
      onError((err as Error).message)
    }
  }

  function copyClientID() {
    navigator.clipboard?.writeText(client.client_id).then(
      () => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      },
      () => undefined,
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <button
          onClick={onBack}
          className="flex items-center gap-1.5 text-[12.5px] text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3.5" strokeWidth={2} />
          应用
        </button>
        <Button
          variant="outline"
          className="h-8 w-auto border-destructive/30 px-3 text-destructive hover:bg-destructive/5"
          onClick={() => setPendingDelete(true)}
        >
          删除应用
        </Button>
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex items-center gap-4 px-6 py-5">
          <Logo client={client} size={52} />
          <div className="min-w-0">
            <h1 className="truncate text-[17px] font-medium text-foreground">{client.name}</h1>
            <p className="mt-0.5 text-[12px] text-muted-foreground">
              {client.description || "未填写描述"}
            </p>
          </div>
        </div>

        <Section title="基本信息">
          <div className="flex flex-col gap-5 sm:flex-row">
            <div className="shrink-0">
              <div className="text-[12px] text-muted-foreground">应用图标</div>
              <div className="mt-1.5 flex flex-col items-start gap-2">
                <Logo client={client} size={72} />
                <div className="flex items-center gap-3">
                  <Button
                    variant="outline"
                    className="h-7 w-auto px-2.5 text-[12px]"
                    disabled={uploading}
                    onClick={() => logoInput.current?.click()}
                  >
                    <Upload className="size-3" strokeWidth={1.75} />
                    {uploading ? "上传中" : "上传"}
                  </Button>
                  {client.logo_url && (
                    <button
                      className="text-[11.5px] text-muted-foreground hover:text-destructive"
                      onClick={removeLogo}
                    >
                      移除
                    </button>
                  )}
                </div>
              </div>
              <input
                ref={logoInput}
                type="file"
                accept="image/png,image/jpeg,image/gif,image/webp"
                className="hidden"
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (file) uploadLogo(file)
                  event.target.value = ""
                }}
              />
            </div>

            <div className="min-w-0 flex-1 space-y-3">
              <label className="block">
                <span className="block text-[12px] text-muted-foreground">应用名称</span>
                <Input
                  className="mt-1 h-9 max-w-[360px]"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                />
              </label>
              <label className="block">
                <span className="block text-[12px] text-muted-foreground">主页地址</span>
                <Input
                  className="mt-1 h-9 max-w-[360px]"
                  placeholder="https://example.com"
                  value={homepage}
                  onChange={(event) => setHomepage(event.target.value)}
                />
              </label>
              <label className="block">
                <span className="block text-[12px] text-muted-foreground">描述</span>
                <Input
                  className="mt-1 h-9 max-w-[480px]"
                  placeholder="一句话说明这个应用做什么"
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                />
              </label>
            </div>
          </div>
        </Section>

        <Section title="Client ID" description="公开标识，写在授权请求的 URL 里，不是密钥">
          <div className="flex items-center gap-2">
            <code className="rounded-md border border-border bg-secondary/50 px-2.5 py-1.5 font-mono text-[12.5px] text-foreground">
              {client.client_id}
            </code>
            <Button variant="outline" className="h-8 w-auto px-2.5" onClick={copyClientID}>
              <Copy className="size-3.5" strokeWidth={1.75} />
              {copied ? "已复制" : "复制"}
            </Button>
          </div>
        </Section>

        <Section
          title="客户端密钥"
          description={
            client.is_public
              ? "公开客户端不使用密钥，依赖 PKCE 校验"
              : "服务端只保存哈希，创建后无法查看原文；需要新密钥时重新生成"
          }
          action={
            client.is_public ? undefined : (
              <Button variant="outline" className="h-8 w-auto px-3" onClick={rotateSecret}>
                生成新密钥
              </Button>
            )
          }
        >
          {rotated ? (
            <div className="rounded-md border border-primary/30 bg-primary/5 px-3 py-2.5">
              <div className="text-[11.5px] text-muted-foreground">
                新密钥只显示这一次，旧的已经失效
              </div>
              <div className="mt-1 font-mono text-[12.5px] break-all text-primary">{rotated}</div>
            </div>
          ) : (
            <p className="text-[12.5px] text-muted-foreground">
              {client.is_public ? "无需密钥" : "已设置 · ••••••••••••"}
            </p>
          )}
        </Section>

        <Section title="回调地址" description="授权完成后跳回的地址，必须完全匹配">
          <div className="space-y-2">
            {redirects.map((item) => (
              <div
                key={item}
                className="flex items-center justify-between gap-3 rounded-md border border-border px-3 py-2"
              >
                <span className="min-w-0 truncate font-mono text-[12px] text-foreground">{item}</span>
                <button
                  className="shrink-0 text-[12px] text-destructive hover:underline"
                  onClick={() => setRedirects(redirects.filter((url) => url !== item))}
                >
                  删除
                </button>
              </div>
            ))}
            {redirects.length === 0 && (
              <p className="text-[12.5px] text-muted-foreground">还没有回调地址</p>
            )}

            <form
              className="flex items-center gap-2 pt-1"
              onSubmit={(event) => {
                event.preventDefault()
                const value = redirect.trim()
                if (!value) return
                if (redirects.includes(value)) return
                setRedirect("")
                setRedirects([...redirects, value])
              }}
            >
              <Input
                className="h-9 max-w-[420px] font-mono text-[12.5px]"
                placeholder="https://example.com/callback"
                value={redirect}
                onChange={(event) => setRedirect(event.target.value)}
              />
              <Button type="submit" variant="outline" className="h-9 w-auto px-3">
                添加
              </Button>
            </form>
          </div>
        </Section>

        <Section title="权限">
          <div className="space-y-4">
            <div>
              <div className="text-[12px] text-muted-foreground">允许申请的 scope</div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {scopes.map((scope) => (
                  <Chip
                    key={scope.name}
                    active={selectedScopes.includes(scope.name)}
                    label={scope.display_name}
                    onClick={() =>
                      setSelectedScopes(
                        selectedScopes.includes(scope.name)
                          ? selectedScopes.filter((item) => item !== scope.name)
                          : [...selectedScopes, scope.name],
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
                      active={bindings.includes(provider.name)}
                      label={provider.display_name}
                      onClick={() =>
                        setBindings(
                          bindings.includes(provider.name)
                            ? bindings.filter((item) => item !== provider.name)
                            : [...bindings, provider.name],
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
                <Chip active={!isPrivate} label="所有登录用户" onClick={() => setIsPrivate(false)} />
                <Chip active={isPrivate} label="仅指定用户与用户组" onClick={() => setIsPrivate(true)} />
              </div>
            </div>

            {isPrivate && (
              <div className="space-y-3 rounded-lg border border-border bg-secondary/20 px-3 py-3">
                <div>
                  <div className="text-[12px] text-muted-foreground">允许访问的用户组</div>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {groups.map((group) => (
                      <Chip
                        key={group.id}
                        active={groupIds.includes(group.id)}
                        label={group.name}
                        onClick={() =>
                          setGroupIds(
                            groupIds.includes(group.id)
                              ? groupIds.filter((id) => id !== group.id)
                              : [...groupIds, group.id],
                          )
                        }
                      />
                    ))}
                    {groups.length === 0 && (
                      <span className="block text-[12px] text-muted-foreground">还没有用户组</span>
                    )}
                  </div>
                </div>

                <div>
                  <div className="text-[12px] text-muted-foreground">用户白名单</div>
                  {members.length > 0 && (
                    <div className="mt-1.5 flex flex-wrap gap-1.5">
                      {members.map((user) => (
                        <span
                          key={user.id}
                          className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card py-0.5 pr-1 pl-2 text-[12px] text-foreground"
                        >
                          {user.name}
                          <span className="font-mono text-[11px] text-muted-foreground">{user.id}</span>
                          <button
                            aria-label={`移除 ${user.name}`}
                            className="rounded p-0.5 text-muted-foreground hover:text-destructive"
                            onClick={() => setMembers(members.filter((m) => m.id !== user.id))}
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
                    <div className="mt-1.5 max-w-[320px] overflow-hidden rounded-md border border-border bg-card">
                      {candidates.map((user) => (
                        <button
                          key={user.id}
                          className="flex w-full items-center justify-between gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-secondary"
                          onClick={() => {
                            setUserQuery("")
                            setCandidates([])
                            setMembers([...members, user])
                          }}
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

                  {members.length === 0 && groupIds.length === 0 && (
                    <p className="mt-2 text-[11.5px] text-destructive">
                      白名单为空，当前没有任何账号能授权该应用
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>
        </Section>
      </div>

      {dirty && (
        <div className="sticky bottom-4 z-10">
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-border bg-card px-5 py-3 shadow-lg shadow-black/5">
            <span className="text-[12.5px] text-muted-foreground">
              有未保存的修改，点击保存后生效
            </span>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                className="h-8 w-auto px-3"
                disabled={saving}
                onClick={resetDrafts}
              >
                放弃修改
              </Button>
              <Button className="h-8 w-auto px-4" disabled={saving} onClick={save}>
                {saving ? "保存中…" : "保存全部修改"}
              </Button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete}
        title="删除应用"
        message={`删除「${client.name}」后，它的 scope、用户组与白名单关联会一并移除，操作不可撤销。`}
        confirmLabel="删除"
        danger
        onCancel={() => setPendingDelete(false)}
        onConfirm={removeClient}
      />
    </div>
  )
}
