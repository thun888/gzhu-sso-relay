# 配置参考

relay 只读取一个 YAML 文件，路径由启动参数 `-config` 指定，缺省为当前目录的 `config.yaml`。

配置分两层：程序内置默认值，再用文件内容覆盖。没有写出的字段保留默认值，显式写成空字符串会覆盖默认值并可能触发校验失败。

```bash
./relay -config /etc/relay/config.yaml
```

## 最小可用配置

```yaml
server:
  base_url: "https://sso.example.edu.cn"
  email_domains:
    "": "gzhu.edu.cn"

secret_key: "<32 字节密钥的 base64 或 hex>"

database:
  dsn: "postgres://relay:password@127.0.0.1:5432/relay?sslmode=disable"

redis:
  addr: "127.0.0.1:6379"
```

## server

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `addr` | `":8080"` | HTTP 监听地址 |
| `base_url` | `"http://localhost:8080"` | 外部访问地址，即 OIDC issuer；末尾斜杠会被去掉 |
| `login_url` | `"/login"` | 未登录访问授权端点时跳转的登录页；为空时回落为 `/login` |
| `consent_url` | `"/consent"` | 首次授权时的确认页 |
| `account_url` | `"/"` | 登录后的用户中心，也是强制绑定时跳转的页面 |
| `site_title` | `"广州大学"` | 登录卡片与用户中心的品牌行；留空则整行隐藏 |
| `email_domains` | 无，必填 | 邮箱后缀映射，见下文 |
| `signing_key_path` | `"./data/signing_key.pem"` | RS256 私钥路径，不存在时自动生成 |
| `trusted_proxies` | `["127.0.0.1", "::1"]` | 信任 `X-Forwarded-For` 的来源地址 |
| `login_panel.enabled` | `true` | 登录页右侧文案区开关 |
| `login_panel.title` | `"统一身份认证门户"` | 右侧区域标题 |
| `login_panel.body` | 模板文案 | 右侧区域正文，支持换行 |
| `login_footer.enabled` | `true` | 登录页页脚开关 |
| `login_footer.text` | `"© 广州大学"` | 页脚文案，支持换行 |

`login_url`、`consent_url`、`account_url` 保持默认值即可。显式留空会导致跳转异常；其中 `login_url` 为空时会被重置为 `/login`，另外两个不会自动修正。

### base_url

`base_url` 同时决定三件事：

1. `id_token` 的 `iss` 与 discovery 文档中的 issuer。
2. 绑定回调地址：`<base_url>/bindings/<provider>/callback`。
3. 会话 cookie 是否带 `Secure`，判断依据是前缀是否为 `https://`。

部署在反向代理之后时，填外部用户看到的地址，不要填容器内地址。

### email_domains

按上游返回的 `sub` 做前缀匹配，取最长匹配的前缀。邮箱生成为 `<sub>@<后缀>`，只在用户首次登录创建账号时写入。

```yaml
email_domains:
  "1": "gzhu.edu.cn"       # sub 以 1 开头
  "3": "e.gzhu.edu.cn"     # sub 以 3 开头
  "": "gzhu.edu.cn"        # 兜底，匹配所有 sub
```

| 规则 | 行为 |
| --- | --- |
| 多个前缀都能匹配 | 取最长前缀 |
| 键为空字符串 | 匹配所有 `sub`，通常作为兜底 |
| 没有任何前缀匹配 | 登录失败，提示该学号没有匹配的邮箱域名配置 |
| 后缀为空 | 启动校验失败 |

`sub` 来自融合门户的 `MOBILE` 字段，不一定是学号。上线前先用 `casprobe` 确认实际取值，再决定前缀。

### signing_key_path

- 文件不存在时生成 2048 位 RSA 私钥，PKCS#8 PEM，权限 `0600`，目录权限 `0700`。
- 已有文件支持 PKCS#8 与 PKCS#1 两种 PEM。
- `kid` 由公钥模数哈希前 16 位生成，更换密钥后 `kid` 随之改变。
- 多实例部署必须共享同一把密钥，否则各实例签发的令牌无法互相校验。
- 丢失密钥后所有下游应用缓存的 JWKS 与已签发令牌全部失效。

### trusted_proxies

Gin 的 `ClientIP` 只在请求来自这些地址时才采信 `X-Forwarded-For`。该值影响登录限流与验证码标记的 IP 维度。

| 写法 | 行为 |
| --- | --- |
| 不写 | 使用默认的 `127.0.0.1` 与 `::1` |
| 加入代理地址 | 代理转发的真实 IP 生效 |
| `[]` | 一律使用 TCP 连接地址，忽略转发头 |

反向代理不在本机时，必须加入代理地址。否则所有请求会被当成同一个 IP，验证码与限流会互相影响。

### login_panel

```yaml
login_panel:
  enabled: true
  title: "统一身份认证门户"
  body: "第一行\n第二行"
```

`enabled` 为 `false`，或 `title` 与 `body` 都为空时，登录卡片收成单栏。

### login_footer

```yaml
login_footer:
  enabled: true
  text: "© 广州大学\n技术支持：网络中心"
```

页脚显示在登录卡片下方并居中。`enabled` 为 `false`，或 `text` 去掉空白后为空时不显示，登录卡片的居中位置不变。

## secret_key

必填，32 字节密钥，支持 base64 或 hex 编码。

```bash
openssl rand -base64 32
# 或
openssl rand -hex 32
```

该密钥用于 AES-GCM 加密 `oauth_accounts.profile_data`（上游资料快照与第三方绑定的原始身份信息）。绑定流程只保存身份快照，不落盘 access token。历史明文数据读取时自动兼容，下次写入时转为密文。更换该密钥后已加密数据无法解密。该值不要与 `signing_key_path` 混用：前者是对称加密密钥，后者是 OIDC 签名私钥。

## database

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `dsn` | 无，必填 | PostgreSQL 连接串，GORM postgres 驱动 |

启动时执行 `AutoMigrate`，需要建表与改表权限。表结构只增不减，不会删除已有列。

连接池固定为最大 25、空闲 10、连接最长 1 小时，不通过配置文件调整。

```yaml
database:
  dsn: "postgres://relay:password@127.0.0.1:5432/relay?sslmode=disable"
```

生产环境按数据库要求设置 `sslmode`，不要直接照抄 `disable`。

## redis

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `addr` | `"localhost:6379"` | Redis 地址 |
| `password` | `""` | 密码，无密码留空 |
| `db` | `0` | 逻辑库编号 |

Redis 不持久化业务数据，但承载全部短期状态。启动时不会主动探测 Redis，连接失败时服务仍能监听，`/healthz` 返回 503，登录与授权会失败。

| 键前缀 | TTL | 用途 |
| --- | --- | --- |
| `relay:session:` | 12 小时 | 登录会话 |
| `relay:code:` | 5 分钟 | OIDC 授权码，读取即删除 |
| `relay:consent:` | 10 分钟 | 待确认的授权请求 |
| `relay:bindstate:` | 10 分钟 | 账号绑定流程的 state |
| `relay:authfail:` | 15 分钟 | 上游认证尝试计数，上限 5 次 |
| `relay:captcha:` | `captcha.fail_window` | 需要验证码的 IP 标记 |

切换 Redis 库或清空数据会使用户重新登录，并丢失进行中的授权与绑定流程，不影响数据库中的用户与授权记录。

## upstream

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `cas_base_url` | `"https://newcas.gzhu.edu.cn"` | CAS 登录地址 |
| `portal_base_url` | `"https://newmy.gzhu.edu.cn"` | 融合门户地址 |
| `timeout` | `30s` | 单次上游登录链路的总超时，必须为正数 |

```yaml
upstream:
  cas_base_url: "https://newcas.gzhu.edu.cn"
  portal_base_url: "https://newmy.gzhu.edu.cn"
  timeout: 30s
```

一次登录会依次访问 CAS 登录页、提交账密、打开门户首页、请求用户信息，全部在同一个超时窗口内完成。上游响应慢时适当调大，不建议超过 `60s`。

上游返回要求验证码、账号被锁定、账号密码错误时，relay 会区分错误并返回不同提示，不会把这些情况混为「服务不可用」。

## captcha

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `enabled` | `false` | 是否启用极验 |
| `always` | `false` | 是否每次登录都要求验证 |
| `captcha_id` | `""` | 极验验证 ID，启用时必填 |
| `captcha_key` | `""` | 极验私钥，只在服务端使用 |
| `fail_window` | `24h` | 失败标记保留时长，必须为正数 |

三种组合：

| enabled | always | 行为 |
| --- | --- | --- |
| `false` | 任意 | 不加载验证码，不记录失败 |
| `true` | `false` | 某个 IP 登录失败后，在 `fail_window` 内要求验证码；该 IP 任意一次登录成功即解除 |
| `true` | `true` | 每次登录都要求验证码，不再记录失败次数 |

`always: true` 适合完全对公网开放的部署。前端从 `https://static.geetest.com/v4/gt4.js` 加载组件，挑战接口在 `*.geetest.com`、`*.geevisit.com`、`*.gsensebot.com` 之间依次重试，浏览器需要能访问这三个域名。反代或防火墙只放行单个域名时，验证码会停在加载中。

## bindings

声明可绑定的第三方账号来源。每个来源走标准 OAuth2 授权码流程，只要对方支持 authorization code 就能通过配置接入。

```yaml
bindings:
  providers:
    - name: gitea
      display_name: Git Server
      client_id: "..."
      client_secret: "..."
      auth_url: "https://git.example.edu.cn/login/oauth/authorize"
      token_url: "https://git.example.edu.cn/login/oauth/access_token"
      userinfo_url: "https://git.example.edu.cn/api/v1/user"
      id_field: "login"
      name_field: "login"
```

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是 | 内部标识，用于 URL 与 API；不能重复 |
| `display_name` | 否 | 用户中心展示名，建议填写 |
| `client_id` | 是 | 对方应用的客户端 ID |
| `client_secret` | 是 | 对方应用的客户端密钥 |
| `auth_url` | 是 | 授权端点 |
| `token_url` | 是 | 令牌端点 |
| `userinfo_url` | 是 | 用户信息端点 |
| `scopes` | 否 | 请求的 scope 列表，以空格拼接后发送 |
| `id_field` | 否 | 用户信息中作为唯一标识的字段，默认 `id` |
| `name_field` | 否 | 作为展示名的字段，默认与 `id_field` 相同 |

接入前需要在对方应用里登记回调地址：

```text
<server.base_url>/bindings/<name>/callback
```

例如 `https://sso.example.edu.cn/bindings/gitea/callback`。回调必须与登记值完全一致，否则对方会拒绝。

绑定只保存对方返回的身份标识与用户信息快照，不保存 access token 与 refresh token。字段值支持字符串与数字，数字会转成十进制字符串。

访问范围不在配置文件里，由后台「绑定源」页面维护（数据表 `binding_acls`）。未配置的来源对所有登录用户开放；设为私有后，只有用户白名单或指定用户组里的账号能在用户中心看到它。

## 完整示例

```yaml
server:
  addr: ":8080"
  base_url: "https://sso.example.edu.cn"
  login_url: "/login"
  consent_url: "/consent"
  account_url: "/"
  site_title: "广州大学"
  email_domains:
    "1": "gzhu.edu.cn"
    "": "e.gzhu.edu.cn"
  signing_key_path: "./data/signing_key.pem"
  trusted_proxies:
    - "127.0.0.1"
    - "::1"
  login_panel:
    enabled: true
    title: "统一身份认证门户"
    body: "账号问题请联系网络中心。"
  login_footer:
    enabled: true
    text: "© 广州大学"

secret_key: "<openssl rand -base64 32 的输出>"

database:
  dsn: "postgres://relay:password@127.0.0.1:5432/relay?sslmode=disable"

redis:
  addr: "127.0.0.1:6379"
  password: ""
  db: 0

upstream:
  cas_base_url: "https://newcas.gzhu.edu.cn"
  portal_base_url: "https://newmy.gzhu.edu.cn"
  timeout: 30s

captcha:
  enabled: false
  always: false
  captcha_id: ""
  captcha_key: ""
  fail_window: 24h

bindings:
  providers:
    - name: gitea
      display_name: Git Server
      client_id: "<gitea client id>"
      client_secret: "<gitea client secret>"
      auth_url: "https://git.example.edu.cn/login/oauth/authorize"
      token_url: "https://git.example.edu.cn/login/oauth/access_token"
      userinfo_url: "https://git.example.edu.cn/api/v1/user"
      id_field: "login"
      name_field: "login"
```

## 启动校验

启动时一次性检查所有必填项，错误信息会列出缺失字段。

| 报错 | 原因 |
| --- | --- |
| `缺少必需项: database.dsn` | 未配置数据库连接串 |
| `缺少必需项: secret_key` | 未配置对称加密密钥 |
| `缺少必需项: server.base_url` | 未配置 issuer |
| `缺少必需项: server.email_domains` | 邮箱映射为空 |
| `缺少必需项: captcha.captcha_id, captcha.captcha_key` | 启用验证码但未填凭据 |
| `server.email_domains[""] 的后缀不能为空` | 映射值为空字符串 |
| `upstream.timeout 必须为正数` | 超时小于等于 0 |
| `captcha.fail_window 必须为正数` | 窗口小于等于 0 |
| `配置: 解析 ...` | YAML 语法错误 |
| `时长 "x" 不合法` | `timeout` 或 `fail_window` 未使用 `30s`、`1m` 这类格式 |

`relay` 与 `relayctl` 读取同一份配置并执行相同校验。`relayctl` 只连接数据库，不启动 HTTP 服务。

## 变更影响

| 改动 | 影响 |
| --- | --- |
| `base_url` | issuer 变化，已签发令牌的 `iss` 不再匹配；下游需要更新 discovery 地址 |
| `signing_key_path` 内容 | `kid` 与签名变化，旧令牌与下游缓存的 JWKS 失效 |
| `secret_key` | 已加密的数据无法解密 |
| `email_domains` | 只影响之后首次登录的新用户，已有用户邮箱不变 |
| `trusted_proxies` | 立即影响 IP 识别，进而影响限流与验证码 |
| `bindings.providers` | 增加或修改后需重启；删除后已存在的绑定记录仍保留在数据库中 |
| `captcha` | 修改后需重启；`always` 打开后不再累计失败次数 |

## 反向代理

推荐由 Nginx 或同类代理终止 TLS：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

对应配置：

```yaml
server:
  base_url: "https://sso.example.edu.cn"
  trusted_proxies:
    - "127.0.0.1"
    - "::1"
```

relay 不读取 `X-Forwarded-Proto` 判断协议，cookie 的 `Secure` 只由 `base_url` 前缀决定。代理到 relay 的链路可以是 HTTP，但 `base_url` 必须写成外部用户访问的 HTTPS 地址。
