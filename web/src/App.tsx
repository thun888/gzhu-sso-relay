import { useEffect, useState } from "react"

import {
  emptyCaptcha,
  emptyFooter,
  emptyPanel,
  inlineConfig,
  loadPublicConfig,
  type PublicConfig,
} from "@/lib/config"
import Account from "@/pages/Account"
import Admin from "@/pages/Admin"
import Confirm from "@/pages/Confirm"
import Consent from "@/pages/Consent"
import Leader from "@/pages/Leader"
import Login from "@/pages/Login"

export default function App() {
  const [config, setConfig] = useState<PublicConfig | null>(() => inlineConfig())

  useEffect(() => {
    if (config) return

    let alive = true
    loadPublicConfig().then((loaded) => {
      if (alive) setConfig(loaded)
    })
    return () => {
      alive = false
    }
  }, [config])

  const siteTitle = config?.siteTitle ?? ""
  const panel = config?.panel ?? emptyPanel
  const footer = config?.footer ?? emptyFooter
  const captcha = config?.captcha ?? emptyCaptcha
  const qrLogin = config?.qrLogin ?? false
  const passkey = config?.passkey ?? false

  const path = window.location.pathname
  if (path === "/login")
    return (
      <Login
        siteTitle={siteTitle}
        panel={panel}
        footer={footer}
        captcha={captcha}
        qrLogin={qrLogin}
        passkey={passkey}
      />
    )
  if (path === "/consent") return <Consent siteTitle={siteTitle} />
  if (path === "/confirm") return <Confirm siteTitle={siteTitle} />
  if (path === "/admin") return <Admin siteTitle={siteTitle} />
  if (path === "/groups") return <Leader siteTitle={siteTitle} />
  return <Account siteTitle={siteTitle} passkey={passkey} />
}
