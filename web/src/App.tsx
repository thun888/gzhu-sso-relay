import { useEffect, useState } from "react"

import { emptyPanel, loadPublicConfig, type LoginPanel } from "@/lib/config"
import Account from "@/pages/Account"
import Consent from "@/pages/Consent"
import Login from "@/pages/Login"

export default function App() {
  const [siteTitle, setSiteTitle] = useState("")
  const [panel, setPanel] = useState<LoginPanel>(emptyPanel)

  useEffect(() => {
    let alive = true
    loadPublicConfig().then((config) => {
      if (!alive) return
      setSiteTitle(config.siteTitle)
      setPanel(config.panel)
    })
    return () => {
      alive = false
    }
  }, [])

  const path = window.location.pathname
  if (path === "/login") return <Login siteTitle={siteTitle} panel={panel} />
  if (path === "/consent") return <Consent siteTitle={siteTitle} />
  return <Account siteTitle={siteTitle} />
}
