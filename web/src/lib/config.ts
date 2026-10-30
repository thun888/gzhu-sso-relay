export type LoginPanel = {
  enabled: boolean
  title: string
  body: string
}

export type PublicConfig = {
  siteTitle: string
  panel: LoginPanel
}

export const emptyPanel: LoginPanel = { enabled: true, title: "", body: "" }

export async function loadPublicConfig(): Promise<PublicConfig> {
  try {
    const res = await fetch("/api/public/config")
    if (!res.ok) return { siteTitle: "", panel: emptyPanel }
    const data = (await res.json()) as { site_title?: string; login_panel?: LoginPanel }
    return {
      siteTitle: data.site_title ?? "",
      panel: data.login_panel ?? emptyPanel,
    }
  } catch {
    return { siteTitle: "", panel: emptyPanel }
  }
}

export function safeNext(raw: string): string {
  if (!raw.startsWith("/") || raw.startsWith("//") || raw.includes("\\")) {
    return "/"
  }
  return raw
}

export function currentPath(): string {
  return window.location.pathname + window.location.search
}
