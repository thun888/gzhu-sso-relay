import { useEffect, useState } from "react"
import { Link2 } from "lucide-react"

import { Empty, Panel, api } from "./shared"

type Provider = { name: string; display_name: string }

export function BindingsPanel({ onError }: { onError: (message: string) => void }) {
  const [providers, setProviders] = useState<Provider[]>([])

  useEffect(() => {
    api<{ providers: Provider[] }>("/api/admin/bindings")
      .then((data) => setProviders(data.providers))
      .catch((err: Error) => onError(err.message))
  }, [onError])

  return (
    <div className="space-y-5">
      <Panel
        title="可绑定的账号来源"
        description="在 config.yaml 的 bindings.providers 中声明，每个来源走标准 OAuth2 授权码流程"
      >
        <div className="divide-y divide-border">
          {providers.map((provider) => (
            <div key={provider.name} className="flex items-center gap-3 px-6 py-3.5">
              <Link2 className="size-4 text-muted-foreground" strokeWidth={1.75} />
              <div>
                <div className="text-[13px] text-foreground">{provider.display_name}</div>
                <div className="mt-0.5 font-mono text-[12px] text-muted-foreground">
                  {provider.name}
                </div>
              </div>
            </div>
          ))}
        </div>
        {providers.length === 0 && (
          <Empty>还没有配置绑定源，应用的「强制绑定」选项在配置之前不会出现</Empty>
        )}
      </Panel>
    </div>
  )
}
