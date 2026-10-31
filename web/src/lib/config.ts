export type LoginPanel = {
  enabled: boolean
  title: string
  body: string
}

export type LoginFooter = {
  enabled: boolean
  text: string
}

export type CaptchaConfig = {
  enabled: boolean
  captchaId: string
  required: boolean
}

export type PublicConfig = {
  siteTitle: string
  panel: LoginPanel
  footer: LoginFooter
  captcha: CaptchaConfig
}

export const emptyPanel: LoginPanel = { enabled: true, title: "", body: "" }

export const emptyFooter: LoginFooter = { enabled: false, text: "" }

export const emptyCaptcha: CaptchaConfig = { enabled: false, captchaId: "", required: false }

export async function loadPublicConfig(): Promise<PublicConfig> {
  try {
    const res = await fetch("/api/public/config")
    if (!res.ok) return { siteTitle: "", panel: emptyPanel, footer: emptyFooter, captcha: emptyCaptcha }
    const data = (await res.json()) as {
      site_title?: string
      login_panel?: LoginPanel
      login_footer?: LoginFooter
      captcha?: { enabled?: boolean; captcha_id?: string; required?: boolean }
    }
    return {
      siteTitle: data.site_title ?? "",
      panel: data.login_panel ?? emptyPanel,
      footer: data.login_footer ?? emptyFooter,
      captcha: {
        enabled: data.captcha?.enabled ?? false,
        captchaId: data.captcha?.captcha_id ?? "",
        required: data.captcha?.required ?? false,
      },
    }
  } catch {
    return { siteTitle: "", panel: emptyPanel, footer: emptyFooter, captcha: emptyCaptcha }
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
