import { useCallback, useEffect, useState } from "react"
import { RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { eventNames, loginMethods, reasonNames } from "@/lib/auth-labels"
import { Empty, Panel, Select, api, tdClass, thClass } from "./shared"

type AuditEvent = {
  id: number
  occurred_at: string
  kind: string
  user_id?: string
  actor_id?: string
  client_id?: string
  client_name?: string
  method?: string
  result: string
  reason?: string
  ip?: string
  user_agent?: string
  scopes?: string[]
}

type Filters = {
  user_id: string
  client_id: string
  kind: string
  result: string
  from: string
  to: string
}

const initialFilters: Filters = { user_id: "", client_id: "", kind: "", result: "", from: "", to: "" }

function formatTime(value: string) {
  const date = new Date(value)
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function resultLabel(event: AuditEvent) {
  const base = event.result === "success" ? "成功" : event.result === "denied" ? "拒绝" : "失败"
  if (!event.reason) return base
  return `${base} · ${reasonNames[event.reason] ?? event.reason}`
}

export function AuditPanel({ onError }: { onError: (message: string) => void }) {
  const [draft, setDraft] = useState<Filters>(initialFilters)
  const [filters, setFilters] = useState<Filters>(initialFilters)
  const [events, setEvents] = useState<AuditEvent[]>([])
  const [cursor, setCursor] = useState(0)
  const [loading, setLoading] = useState(true)
  const [more, setMore] = useState(false)

  const load = useCallback(
    async (next: number) => {
      if (next > 0) setMore(true)
      else setLoading(true)

      try {
        const params = new URLSearchParams({ limit: "50" })
        if (next > 0) params.set("cursor", String(next))
        if (filters.user_id) params.set("user_id", filters.user_id)
        if (filters.client_id) params.set("client_id", filters.client_id)
        if (filters.kind) params.set("kind", filters.kind)
        if (filters.result) params.set("result", filters.result)
        if (filters.from) params.set("from", filters.from)
        if (filters.to) params.set("to", filters.to)

        const data = await api<{ events: AuditEvent[]; next_cursor: number }>(`/api/admin/events?${params}`)
        setEvents((prev) => (next > 0 ? [...prev, ...data.events] : data.events))
        setCursor(data.next_cursor)
      } catch (err) {
        onError((err as Error).message)
      } finally {
        setLoading(false)
        setMore(false)
      }
    },
    [filters, onError],
  )

  useEffect(() => {
    void load(0)
  }, [load])

  function update(key: keyof Filters, value: string) {
    setDraft((prev) => ({ ...prev, [key]: value }))
  }

  return (
    <Panel
      title="登录与授权记录"
      description="事件只追加，账号注销后会去除账号、IP 与设备信息"
      action={
        <Button variant="outline" size="sm" className="h-8 w-auto px-3" onClick={() => void load(0)}>
          <RefreshCw className="size-3.5" />
          刷新
        </Button>
      }
    >
      <div className="flex flex-wrap items-end gap-2 border-b border-border px-6 py-3">
        <Input
          className="h-9 w-[140px]"
          placeholder="学号"
          value={draft.user_id}
          onChange={(e) => update("user_id", e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") setFilters({ ...draft })
          }}
        />
        <Input
          className="h-9 w-[150px]"
          placeholder="client_id"
          value={draft.client_id}
          onChange={(e) => update("client_id", e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") setFilters({ ...draft })
          }}
        />
        <Select value={draft.kind} onChange={(value) => update("kind", value)}>
          <option value="">全部事件</option>
          {Object.entries(eventNames).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </Select>
        <Select value={draft.result} onChange={(value) => update("result", value)}>
          <option value="">全部结果</option>
          <option value="success">成功</option>
          <option value="failure">失败</option>
          <option value="denied">拒绝</option>
        </Select>
        <Input
          className="h-9 w-[150px]"
          type="date"
          value={draft.from}
          onChange={(e) => update("from", e.target.value)}
        />
        <Input
          className="h-9 w-[150px]"
          type="date"
          value={draft.to}
          onChange={(e) => update("to", e.target.value)}
        />
        <Button className="h-9 w-auto px-4" onClick={() => setFilters({ ...draft })}>
          查询
        </Button>
        <Button
          variant="ghost"
          className="h-9 w-auto px-3"
          onClick={() => {
            setDraft(initialFilters)
            setFilters({ ...initialFilters })
          }}
        >
          重置
        </Button>
      </div>

      <ScrollArea className="w-full">
        <div className="pb-2">
          <table className="w-full min-w-[900px] border-collapse">
            <thead>
              <tr>
                <th className={thClass}>时间</th>
                <th className={thClass}>事件</th>
                <th className={thClass}>用户</th>
                <th className={thClass}>应用</th>
                <th className={thClass}>结果</th>
                <th className={thClass}>来源</th>
              </tr>
            </thead>
            <tbody>
              {events.map((item) => (
                <tr key={item.id}>
                  <td className={`${tdClass} whitespace-nowrap text-muted-foreground`}>
                    {formatTime(item.occurred_at)}
                  </td>
                  <td className={tdClass}>{eventNames[item.kind] ?? item.kind}</td>
                  <td className={tdClass}>
                    <div className="text-foreground">{item.user_id || "匿名"}</div>
                    {item.actor_id && item.actor_id !== item.user_id && (
                      <div className="text-[12px] text-muted-foreground">操作者 {item.actor_id}</div>
                    )}
                  </td>
                  <td className={tdClass}>
                    <div className="text-foreground">{item.client_name || item.client_id || "本站"}</div>
                    {item.client_name && item.client_id && (
                      <div className="text-[12px] text-muted-foreground">{item.client_id}</div>
                    )}
                  </td>
                  <td
                    className={`${tdClass} ${
                      item.result === "success" ? "text-muted-foreground" : "text-destructive"
                    }`}
                  >
                    {resultLabel(item)}
                  </td>
                  <td className={`${tdClass} text-muted-foreground`}>
                    <div className="whitespace-nowrap">
                      {item.ip || "未知地址"}
                      {item.method ? ` · ${loginMethods[item.method] ?? item.method}` : ""}
                    </div>
                    {item.user_agent && (
                      <div className="max-w-[260px] truncate text-[12px]" title={item.user_agent}>
                        {item.user_agent}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </ScrollArea>

      {!loading && events.length === 0 && <Empty>没有符合条件的记录</Empty>}
      {loading && <Empty>加载中</Empty>}
      {cursor > 0 && (
        <div className="flex justify-center border-t border-border py-3">
          <Button
            variant="outline"
            size="sm"
            className="h-8 w-auto px-4"
            disabled={more}
            onClick={() => void load(cursor)}
          >
            {more ? "加载中" : "加载更多"}
          </Button>
        </div>
      )}
    </Panel>
  )
}
