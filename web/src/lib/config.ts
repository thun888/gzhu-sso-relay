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
  qrLogin: boolean
  passkey: boolean
}

export const emptyPanel: LoginPanel = { enabled: true, title: "", body: "" }

export const emptyFooter: LoginFooter = { enabled: false, text: "" }

export const emptyCaptcha: CaptchaConfig = { enabled: false, captchaId: "", required: false }

type ConfigPayload = {
  site_title?: string
  login_panel?: LoginPanel
  login_footer?: LoginFooter
  qr_login?: boolean
  passkey?: boolean
  captcha?: { enabled?: boolean; captcha_id?: string; required?: boolean }
}

declare global {
  interface Window {
    __RELAY_CONFIG__?: ConfigPayload
  }
}

function toConfig(data: ConfigPayload): PublicConfig {
  return {
    siteTitle: data.site_title ?? "",
    panel: data.login_panel ?? emptyPanel,
    footer: data.login_footer ?? emptyFooter,
    qrLogin: data.qr_login ?? false,
    passkey: data.passkey ?? false,
    captcha: {
      enabled: data.captcha?.enabled ?? false,
      captchaId: data.captcha?.captcha_id ?? "",
      required: data.captcha?.required ?? false,
    },
  }
}

// 服务端把配置内联在 index.html 里，首帧就能读到；开发模式下没有，回落到接口。
export function inlineConfig(): PublicConfig | null {
  const payload = window.__RELAY_CONFIG__
  return payload ? toConfig(payload) : null
}

export async function loadPublicConfig(): Promise<PublicConfig> {
  try {
    const res = await fetch("/api/public/config")
    if (!res.ok) return toConfig({})
    return toConfig((await res.json()) as ConfigPayload)
  } catch {
    return toConfig({})
  }
}

export function safeNext(raw: string): string {
  if (!raw.startsWith("/") || raw.startsWith("\\") || raw.startsWith("//")) {
    return "/"
  }
  return raw
}

export function currentPath(): string {
  return window.location.pathname + window.location.search
}
