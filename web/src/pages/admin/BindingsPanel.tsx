import { useEffect, useState } from "react"
import { Link2, Search, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import type { Group, Member } from "./client-types"
import { Chip, Empty, Panel, api, groupPath } from "./shared"

type BindingProvider = {
  name: string
  display_name: string
  is_private: boolean
  group_ids: number[]
  users: Member[]
}

function sameSet<T>(a: T[], b: T[]) {
  return a.length === b.length && a.every((item) => b.includes(item))
}

function ProviderCard({
  provider,
  groups,
  onError,
  onSaved,
}: {
  provider: BindingProvider
  groups: Group[]
  onError: (message: string) => void
  onSaved: (provider: BindingProvider) => void
}) {
  const [isPrivate, setIsPrivate] = useState(provider.is_private)
  const [groupIds, setGroupIds] = useState<number[]>(provider.group_ids)
  const [members, setMembers] = useState<Member[]>(provider.users)
  const [query, setQuery] = useState("")
  const [candidates, setCandidates] = useState<Member[]>([])
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setIsPrivate(provider.is_private)
    setGroupIds(provider.group_ids)
    setMembers(provider.users)
    setQuery("")
    setCandidates([])
  }, [provider])

  useEffect(() => {
    const keyword = query.trim()
    if (!isPrivate || keyword.length < 2) {
      setCandidates([])
      return
    }
    const timer = setTimeout(async () => {
      try {
        const data = await api<{ users: Member[] }>(
          `/api/admin/users?limit=8&q=${encodeURIComponent(keyword)}`,
        )
        setCandidates(data.users.filter((user) => !members.some((m) => m.id === user.id)))
      } catch {
        setCandidates([])
      }
    }, 250)
    return () => clearTimeout(timer)
  }, [query, isPrivate, members])

  const dirty =
    isPrivate !== provider.is_private ||
    !sameSet(groupIds, provider.group_ids) ||
    !sameSet(
      members.map((m) => m.id),
      provider.users.map((u) => u.id),
    )

  function reset() {
    setIsPrivate(provider.is_private)
    setGroupIds(provider.group_ids)
    setMembers(provider.users)
    setQuery("")
    setCandidates([])
  }

  async function save() {
    setSaving(true)
    try {
      const updated = await api<BindingProvider>(
        `/api/admin/bindings/${encodeURIComponent(provider.name)}`,
        {
          method: "PUT",
          body: JSON.stringify({
            is_private: isPrivate,
            group_ids: groupIds,
            user_ids: members.map((m) => m.id),
          }),
        },
      )
      onSaved(updated)
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="px-6 py-4">
      <div className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          <Link2 className="mt-0.5 size-4 shrink-0 text-muted-foreground" strokeWidth={1.75} />
          <div className="min-w-0">
            <div className="text-[13px] text-foreground">{provider.display_name}</div>
            <div className="mt-0.5 font-mono text-[12px] text-muted-foreground">
              {provider.name}
            </div>
          </div>
        </div>
        {dirty && (
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" className="h-7 w-auto px-2.5 text-[12px]" disabled={saving} onClick={reset}>
              放弃
            </Button>
            <Button className="h-7 w-auto px-3 text-[12px]" disabled={saving} onClick={save}>
              {saving ? "保存中" : "保存"}
            </Button>
          </div>
        )}
      </div>

      <div className="mt-3 space-y-3">
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
              <div className="text-[12px] text-muted-foreground">允许使用的用户组</div>
              <div className="mt-1.5 flex flex-wrap gap-1.5">
                {groups.map((group) => (
                  <Chip
                    key={group.id}
                    active={groupIds.includes(group.id)}
                    label={group.name}
                    title={groupPath(groups, group)}
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
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </div>

              {candidates.length > 0 && (
                <div className="mt-1.5 max-w-[320px] overflow-hidden rounded-md border border-border bg-card">
                  {candidates.map((user) => (
                    <button
                      key={user.id}
                      className="flex w-full items-center justify-between gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-secondary"
                      onClick={() => {
                        setQuery("")
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
                  白名单为空，当前没有任何账号能在用户中心看到该绑定源
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export function BindingsPanel({ onError }: { onError: (message: string) => void }) {
  const [providers, setProviders] = useState<BindingProvider[]>([])
  const [groups, setGroups] = useState<Group[]>([])

  useEffect(() => {
    Promise.all([
      api<{ providers: BindingProvider[] }>("/api/admin/bindings"),
      api<{ groups: Group[] }>("/api/admin/groups"),
    ])
      .then(([providerData, groupData]) => {
        setProviders(providerData.providers)
        setGroups(groupData.groups)
      })
      .catch((err: Error) => onError(err.message))
  }, [onError])

  function replace(updated: BindingProvider) {
    setProviders(providers.map((item) => (item.name === updated.name ? updated : item)))
  }

  return (
    <div className="space-y-5">
      <Panel
        title="可绑定的账号来源"
        description="在 config.yaml 的 bindings.providers 中声明，每个来源走标准 OAuth2 授权码流程。访问范围决定谁能在用户中心看到该来源"
      >
        <div className="divide-y divide-border">
          {providers.map((provider) => (
            <ProviderCard
              key={provider.name}
              provider={provider}
              groups={groups}
              onError={onError}
              onSaved={replace}
            />
          ))}
        </div>
        {providers.length === 0 && (
          <Empty>还没有配置绑定源，应用的「强制绑定」选项在配置之前不会出现</Empty>
        )}
      </Panel>
    </div>
  )
}
