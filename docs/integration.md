# OIDC 接入指南

relay 对外提供标准 OIDC 授权码流程。应用只需要 discovery 地址、`client_id`、`client_secret` 与回调地址即可接入。

## 注册应用

在管理后台「应用」页新建，或使用命令行：

```bash
./bin/relayctl -config config.yaml client create \
  -id gitea \
  -name "Git Server" \
  -redirect "https://git.example.edu.cn/user/oauth2/relay/callback" \
  -scopes openid,profile,email
```

| 参数 | 说明 |
| --- | --- |
| `client_id` | 唯一标识，只允许字母、数字、下划线、连字符，最长 64 |
| `name` | 授权确认页展示的应用名 |
| `redirect_urls` | 允许的回调地址，可多个；scheme 与 host 不区分大小写，路径与查询串必须完全一致。`127.0.0.1`、`localhost`、`[::1]` 忽略端口，也可填 `myapp://callback` 这类自定义 scheme |
| `scopes` | 允许申请的额外 scope，可留空 |
| `is_public` | 公开客户端不校验密钥，必须使用 PKCE |

创建成功后 `client_secret` 只显示一次，服务端只保存 Argon2id 哈希。

`openid`、`profile`、`email` 三个内置 scope 对所有应用默认开放，不需要勾选。管理后台仍可勾选，但对这三个内置 scope 不改变实际允许范围。

`groups` 不在默认开放范围内，需要管理员在应用详情里勾选后才能申请；用户首次申请时会出现一次授权确认页。

## 端点

| 端点 | 路径 |
| --- | --- |
| Issuer | `server.base_url` |
| Discovery | `<issuer>/.well-known/openid-configuration` |
| 授权 | `<issuer>/oidc/authorize` |
| 令牌 | `<issuer>/oidc/token` |
| 用户信息 | `<issuer>/oidc/userinfo` |
| JWKS | `<issuer>/oidc/jwks` |

discovery 声明的能力：

| 项目 | 值 |
| --- | --- |
| `response_types_supported` | `code` |
| `grant_types_supported` | `authorization_code` |
| `subject_types_supported` | `public` |
| `id_token_signing_alg_values_supported` | `RS256` |
| `scopes_supported` | `openid`、`profile`、`email`、`groups` |
| `token_endpoint_auth_methods_supported` | `client_secret_basic`、`client_secret_post`、`none` |
| `code_challenge_methods_supported` | `S256`、`plain` |

## 授权码流程

### 1. 跳转授权端点

```http
GET <issuer>/oidc/authorize?
  response_type=code&
  client_id=gitea&
  redirect_uri=https%3A%2F%2Fgit.example.edu.cn%2Fuser%2Foauth2%2Frelay%2Fcallback&
  scope=openid%20profile%20email&
  state=<随机值>&
  nonce=<随机值>&
  code_challenge=<PKCE challenge>&
  code_challenge_method=S256
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `client_id` | 是 | 已注册的应用标识 |
| `redirect_uri` | 是 | 必须与登记的回调地址一致，规则同上 |
| `response_type` | 是 | 只支持 `code` |
| `scope` | 是 | 空格分隔，必须包含 `openid` |
| `state` | 否 | 原样回传，用于防 CSRF |
| `nonce` | 否 | 写入 `id_token` 的 `nonce` |
| `code_challenge` | 公开客户端必填 | PKCE challenge |
| `code_challenge_method` | 否 | `S256` 或 `plain`，缺省 `plain` |

relay 依次检查：

1. `client_id` 是否存在，`redirect_uri` 是否在允许列表。
2. `response_type` 是否为 `code`。
3. `scope` 是否包含 `openid`，是否都在允许范围内。
4. 用户是否登录，未登录跳转 `/login?next=<原始请求>`。
5. 用户是否属于应用限定的用户组，不限定时直接通过。
6. 是否完成应用要求的强制绑定，缺少时跳转用户中心。
7. 是否已授予这些 scope，未授予时跳转确认页。
8. 公开客户端是否携带 PKCE。

全部通过后签发授权码，重定向到：

```text
<redirect_uri>?code=<授权码>&state=<原样回传>
```

授权码有效期 5 分钟，只能使用一次。

### 2. 换取令牌

```http
POST <issuer>/oidc/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic <base64(client_id:client_secret)>

grant_type=authorization_code&
code=<授权码>&
redirect_uri=<与授权时一致>&
code_verifier=<PKCE verifier>
```

也可以把凭据放在表单里：

```text
client_id=gitea&client_secret=<secret>
```

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `grant_type` | 是 | 固定 `authorization_code` |
| `code` | 是 | 授权码 |
| `redirect_uri` | 否 | 传了就必须与授权时一致 |
| `client_id` | 是 | 使用 Basic 认证时从头部取 |
| `client_secret` | 机密客户端必填 | 公开客户端忽略 |
| `code_verifier` | 有 challenge 时必填 | PKCE verifier |

响应：

```json
{
  "access_token": "<JWT>",
  "token_type": "Bearer",
  "expires_in": 3600,
  "id_token": "<JWT>",
  "scope": "openid profile email"
}
```

没有 `refresh_token`。access token 与 id token 的有效期都是 1 小时。

### 3. 获取用户信息

```http
GET <issuer>/oidc/userinfo
Authorization: Bearer <access_token>
```

返回当前 scope 允许的 claims：

```json
{
  "sub": "20230001",
  "preferred_username": "20230001",
  "name": "张三",
  "belong_class": "计算机 231",
  "email": "13800000000@gzhu.edu.cn",
  "email_verified": true,
  "groups": ["major:计算机科学与技术", "unit:计算机学院", "user_type:学生"]
}
```

无效、过期或用户已被禁用时返回 401：

```http
WWW-Authenticate: Bearer error="invalid_token"
```

只接受 access token。携带 `id_token` 会因 `token_use` 不是 `access` 而被拒绝。

## scope 与 claims

| scope | 产生的 claims |
| --- | --- |
| `openid` | `sub` |
| `profile` | `preferred_username`、`name`、`belong_class` |
| `email` | `email`、`email_verified` |
| `groups` | `groups` |

| claim | 来源 | 说明 |
| --- | --- | --- |
| `sub` | 本地用户 ID | 学号或教工号，稳定不变 |
| `preferred_username` | 本地用户 ID | 与 `sub` 相同 |
| `name` | 门户 `USER_NAME` | 姓名 |
| `belong_class` | 门户 `BELONG_CLASS` | 班级，上游未返回时省略 |
| `email` | 首次登录时生成 | `<门户 MOBILE>@<域名后缀>` |
| `email_verified` | 固定 | 恒为 `true` |
| `groups` | 用户组 | 用户所在组的 `code` 数组，含祖先组；无组时省略 |

未授予的 scope 不产生任何对应 claim。例如只申请 `openid`，`id_token` 与 userinfo 只有 `sub`。

## 令牌

### ID Token

| claim | 说明 |
| --- | --- |
| `iss` | `server.base_url` |
| `aud` | `[client_id]` |
| `sub` | 学号 |
| `iat`、`exp` | 签发与过期时间，有效期 1 小时 |
| `auth_time` | 本次授权码的签发时间 |
| `nonce` | 授权请求携带时原样写入 |
| `name` 等 | 由 scope 决定 |

使用 RS256 签名，`kid` 对应 JWKS 中的公钥。下游应通过 discovery 获取 `jwks_uri` 并缓存公钥，不要硬编码。

### Access Token

| claim | 说明 |
| --- | --- |
| `iss` | `server.base_url` |
| `sub` | 学号 |
| `aud` | `[client_id]` |
| `scope` | 空格分隔的已授予 scope |
| `token_use` | 固定 `access` |
| `iat`、`exp` | 有效期 1 小时 |

access token 是自包含 JWT，资源服务可以离线校验签名与过期时间，也可以调用 userinfo 获取最新身份。离线校验无法感知用户禁用，需要即时生效时应调用 userinfo。

### 缓存建议

| 资源 | 缓存 |
| --- | --- |
| discovery | 5 分钟 |
| JWKS | 1 小时，遇到未知 `kid` 时立即刷新 |

## 错误

授权端点在 `redirect_uri` 校验通过后，错误通过重定向返回：

```text
<redirect_uri>?error=<code>&error_description=<说明>&state=<原样回传>
```

| error | 原因 |
| --- | --- |
| `unsupported_response_type` | `response_type` 不是 `code` |
| `invalid_scope` | 缺少 `openid`，或申请了未被允许的 scope |
| `access_denied` | 用户不在应用限定的用户组内，或在确认页取消授权 |
| `invalid_request` | 不支持的 PKCE 方法，或公开客户端未携带 PKCE |

`client_id` 未知或应用已停用时返回 400：

```json
{"error": "unauthorized_client", "error_description": "未知的 client_id"}
```

`redirect_uri` 不在允许列表时不跳转，直接返回 400 JSON：

```json
{"error": "invalid_request", "error_description": "redirect_uri 不在允许列表"}
```

令牌端点的错误：

| error | HTTP | 原因 |
| --- | --- | --- |
| `invalid_request` | 400 | 表单无法解析 |
| `invalid_client` | 401 | 缺少凭据、客户端不存在或密钥错误 |
| `unsupported_grant_type` | 400 | 不是 `authorization_code` |
| `invalid_grant` | 400 | 授权码无效、已被使用、过期、与客户端或回调不匹配、PKCE 失败、用户被禁用 |
| `too_many_requests` | 429 | 同一来源短时间内失败次数过多，等 5 分钟后重试 |
| `server_error` | 500 | 签发令牌失败 |

## 接入示例

通用 OIDC 客户端配置：

| 字段 | 值 |
| --- | --- |
| Issuer | `https://sso.example.edu.cn` |
| Discovery | `https://sso.example.edu.cn/.well-known/openid-configuration` |
| Client ID | 管理后台创建时填写 |
| Client Secret | 创建时返回，只显示一次 |
| Scopes | `openid profile email` |
| Redirect URI | 应用自身的回调地址，需与登记值一致 |
| PKCE | 公开客户端必须启用 |

命令行验证：

```bash
# 1. 浏览器打开授权地址，登录并同意，从回调地址取 code

# 2. 换取令牌
curl -s -u gitea:<client_secret> \
  -d grant_type=authorization_code \
  -d code=<code> \
  -d redirect_uri=https://git.example.edu.cn/user/oauth2/relay/callback \
  https://sso.example.edu.cn/oidc/token

# 3. 读取用户信息
curl -s -H "Authorization: Bearer <access_token>" \
  https://sso.example.edu.cn/oidc/userinfo
```

## 能力边界

| 项目 | 现状 |
| --- | --- |
| 授权模式 | 仅 authorization code |
| 刷新令牌 | 不支持；令牌过期后重新走 authorize，已有会话时无需再次输入密码 |
| PKCE | 支持 `S256` 与 `plain`；公开客户端强制 |
| 令牌撤销 | 不提供 revocation 端点 |
| 令牌自省 | 不提供 introspection 端点 |
| 单点退出 | 不提供 end_session 端点 |
| 动态注册 | 不支持，应用需在管理后台创建 |
| 自定义 scope | 当前只内置 `openid`、`profile`、`email`，新增 scope 不会产生额外 claim |
| 用户组与绑定 | 每次授权重新检查，已签发令牌不受影响 |
| 应用停用 | 管理后台可停用应用，停用后授权与换取令牌都返回 `unauthorized_client`，配置与授权记录保留 |

## 上线检查清单

- [ ] `server.base_url` 为外部 HTTPS 地址，与 discovery 中的 issuer 一致。
- [ ] 应用回调地址已登记，且与代码中的值完全一致。
- [ ] 机密客户端的 `client_secret` 保存在服务端环境变量或密钥管理中。
- [ ] 申请了 `openid`，需要姓名时加 `profile`，需要邮箱时加 `email`，需要按组判断权限时加 `groups`。
- [ ] 公开客户端启用 PKCE，使用 `S256`。
- [ ] 下游校验 `iss`、`aud`、`exp`，并通过 JWKS 校验签名。
- [ ] 令牌过期后重新发起授权，不依赖 refresh token。
