export const loginMethods: Record<string, string> = {
  password: "密码登录",
  qr: "扫码登录",
  passkey: "通行密钥",
  session: "会话操作",
  oidc: "授权端点",
  consent: "同意页",
}

export const eventNames: Record<string, string> = {
  "login.success": "登录成功",
  "login.failure": "登录失败",
  logout: "退出登录",
  "session.revoke": "下线设备",
  "consent.grant": "确认授权",
  "authorize.allow": "授权应用",
  "authorize.deny": "拒绝授权",
  "authorize.revoke": "撤销授权",
  "app.login": "应用登录",
}

export const reasonNames: Record<string, string> = {
  bad_credentials: "账号或密码错误",
  account_disabled: "账号已停用",
  throttled: "尝试过于频繁",
  upstream_locked: "上游账号被锁定",
  upstream_challenge: "上游要求验证码",
  upstream_unavailable: "上游不可用",
  no_email_domain: "缺少邮箱域名",
  qr_unlinked: "微信未关联账号",
  no_access: "无权访问",
  binding_denied: "绑定不满足",
  user_denied: "用户拒绝",
  user: "用户操作",
  client: "应用发起",
}
