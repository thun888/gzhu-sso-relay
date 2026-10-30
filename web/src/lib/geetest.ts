export type CaptchaAnswer = {
  lot_number: string
  captcha_output: string
  pass_token: string
  gen_time: string
}

export type CaptchaInstance = {
  onReady: (callback: () => void) => CaptchaInstance
  onSuccess: (callback: () => void) => CaptchaInstance
  onError: (callback: () => void) => CaptchaInstance
  onClose: (callback: () => void) => CaptchaInstance
  getValidate: () => CaptchaAnswer | false
  showCaptcha: () => void
  reset: () => void
  destroy: () => void
}

type InitOptions = {
  captchaId: string
  product: "bind"
  language: "zho"
}

declare global {
  interface Window {
    initGeetest4?: (options: InitOptions, callback: (instance: CaptchaInstance) => void) => void
  }
}

const scriptURL = "https://static.geetest.com/v4/gt4.js"

let loading: Promise<void> | null = null

function loadScript(): Promise<void> {
  if (window.initGeetest4) return Promise.resolve()
  if (loading) return loading

  loading = new Promise<void>((resolve, reject) => {
    const script = document.createElement("script")
    script.src = scriptURL
    script.async = true
    script.addEventListener("load", () => resolve())
    script.addEventListener("error", () => {
      loading = null
      script.remove()
      reject(new Error("geetest 脚本加载失败"))
    })
    document.head.appendChild(script)
  })
  return loading
}

export async function createCaptcha(
  captchaId: string,
  onSuccess: (answer: CaptchaAnswer) => void,
  onError: () => void,
): Promise<CaptchaInstance> {
  await loadScript()
  if (!window.initGeetest4) throw new Error("geetest 未就绪")

  return new Promise((resolve, reject) => {
    window.initGeetest4?.({ captchaId, product: "bind", language: "zho" }, (instance) => {
      if (!instance) {
        reject(new Error("geetest 初始化失败"))
        return
      }
      instance
        .onSuccess(() => {
          const answer = instance.getValidate()
          if (answer) onSuccess(answer)
        })
        .onError(onError)
      resolve(instance)
    })
  })
}
