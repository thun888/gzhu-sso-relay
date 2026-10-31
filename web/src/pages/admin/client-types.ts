export type Member = { id: string; name: string; email: string; status: number }

export type Scope = { name: string; display_name: string; description: string }

export type Provider = { name: string; display_name: string }

export type Group = { id: number; name: string; code: string; is_upstream: boolean }

export type Client = {
  client_id: string
  name: string
  logo_url: string
  homepage_url: string
  description: string
  is_public: boolean
  is_private: boolean
  enabled: boolean
  redirect_urls: string[]
  required_bindings: string[]
  scopes: string[]
  group_ids: number[]
  users: Member[]
}
