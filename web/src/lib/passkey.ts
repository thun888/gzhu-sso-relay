type CreationJSON = {
  publicKey: {
    challenge: string
    user: { id: string; name: string; displayName: string }
    excludeCredentials?: { id: string; type: string; transports?: string[] }[]
    [key: string]: unknown
  }
}

type RequestJSON = {
  publicKey: {
    challenge: string
    allowCredentials?: { id: string; type: string; transports?: string[] }[]
    [key: string]: unknown
  }
}

function toBytes(value: string): Uint8Array {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/")
  const raw = atob(padded + "===".slice((padded.length + 3) % 4))
  const bytes = new Uint8Array(raw.length)
  for (let i = 0; i < raw.length; i += 1) bytes[i] = raw.charCodeAt(i)
  return bytes
}

function toBase64Url(buffer: ArrayBuffer): string {
  let raw = ""
  for (const byte of new Uint8Array(buffer)) raw += String.fromCharCode(byte)
  return btoa(raw).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

// 浏览器只在安全上下文里开放 WebAuthn，普通 http 站点即使支持也无法使用。
export function passkeySupported(): boolean {
  return window.isSecureContext && typeof window.PublicKeyCredential !== "undefined"
}

export function defaultDeviceName(): string {
  const ua = navigator.userAgent
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Chrome\//.test(ua)
      ? "Chrome"
      : /Firefox\//.test(ua)
        ? "Firefox"
        : /Safari\//.test(ua)
          ? "Safari"
          : "浏览器"
  const system = /Windows/.test(ua)
    ? "Windows"
    : /iPhone|iPad|iPod/.test(ua)
      ? "iOS"
      : /Android/.test(ua)
        ? "Android"
        : /Macintosh|Mac OS X/.test(ua)
          ? "macOS"
          : /Linux/.test(ua)
            ? "Linux"
            : "设备"
  return `${browser} · ${system}`
}

export function passkeyErrorMessage(error: unknown): string {
  if (error instanceof DOMException) {
    switch (error.name) {
      case "InvalidStateError":
        return "该设备已经绑定过通行密钥"
      case "NotSupportedError":
        return "该设备或浏览器不支持通行密钥"
      case "SecurityError":
        return "当前环境不允许使用通行密钥"
      case "NotAllowedError":
        return "已取消或等待超时，请重试"
    }
  }
  return "通行密钥操作失败，请重试"
}

export async function createPasskey(publicKey: CreationJSON["publicKey"]): Promise<unknown> {
  const options = {
    ...publicKey,
    challenge: toBytes(publicKey.challenge),
    user: { ...publicKey.user, id: toBytes(publicKey.user.id) },
    excludeCredentials: publicKey.excludeCredentials?.map((item) => ({
      ...item,
      id: toBytes(item.id),
    })),
  } as unknown as PublicKeyCredentialCreationOptions

  const credential = (await navigator.credentials.create({ publicKey: options })) as
    | PublicKeyCredential
    | null
  if (!credential) throw new Error("passkey: 未返回凭据")

  const response = credential.response as AuthenticatorAttestationResponse
  return {
    id: credential.id,
    rawId: toBase64Url(credential.rawId),
    type: credential.type,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      attestationObject: toBase64Url(response.attestationObject),
      transports: response.getTransports?.() ?? [],
    },
  }
}

export async function getPasskey(publicKey: RequestJSON["publicKey"]): Promise<unknown> {
  const options = {
    ...publicKey,
    challenge: toBytes(publicKey.challenge),
    allowCredentials: publicKey.allowCredentials?.map((item) => ({
      ...item,
      id: toBytes(item.id),
    })),
  } as unknown as PublicKeyCredentialRequestOptions

  const credential = (await navigator.credentials.get({ publicKey: options })) as
    | PublicKeyCredential
    | null
  if (!credential) throw new Error("passkey: 未返回凭据")

  const response = credential.response as AuthenticatorAssertionResponse
  return {
    id: credential.id,
    rawId: toBase64Url(credential.rawId),
    type: credential.type,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64Url(response.clientDataJSON),
      authenticatorData: toBase64Url(response.authenticatorData),
      signature: toBase64Url(response.signature),
      userHandle: response.userHandle ? toBase64Url(response.userHandle) : undefined,
    },
  }
}
