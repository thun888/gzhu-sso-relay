import { useEffect, useState } from "react"

import { emptyCaptcha, emptyPanel, loadPublicConfig, type CaptchaConfig, type LoginPanel } from "@/lib/config"
import Account from "@/pages/Account"
import Admin from "@/pages/Admin"
import Consent from "@/pages/Consent"
import Login from "@/pages/Login"

export default function App() {
  const [siteTitle, setSiteTitle] = useState("")
  const [panel, setPanel] = useState<LoginPanel>(emptyPanel)
  const [captcha, setCaptcha] = useState<CaptchaConfig>(emptyCaptcha)

  useEffect(() => {
    let alive = true
    loadPublicConfig().then((config) => {
      if (!alive) return
      setSiteTitle(config.siteTitle)
      setPanel(config.panel)
      setCaptcha(config.captcha)
    })
    return () => {
      alive = false
    }
  }, [])

  const path = window.location.pathname
  if (path === "/login") return <Login siteTitle={siteTitle} panel={panel} captcha={captcha} />
  if (path === "/consent") return <Consent siteTitle={siteTitle} />
  if (path === "/admin") return <Admin siteTitle={siteTitle} />
  return <Account siteTitle={siteTitle} />
}
