# 使用与运维

## 页面入口

| 路径 | 说明 | 权限 |
| --- | --- | --- |
| `/login` | 登录页 | 公开 |
| `/` | 用户中心 | 已登录 |
| `/consent?req=<id>` | 应用授权确认 | 已登录 |
| `/admin` | 管理后台 | 管理员 |

前端为单页应用，未命中的路径回落到 `index.html`。`/api/`、`/oidc/` 与 `/healthz` 不参与回落。

## 登录

### 流程

1. 用户打开 `/login`，或从应用跳转到 `/login?next=<站内路径>`。
2. 输入学号或教工号与密码。
3. relay 先校验本地 Argon2id 密码哈希。
4. 本地未命中时，使用同一组账密访问上游 CAS，再打开融合门户拉取用户信息。
5. 本地命中但该账号被标记为「上次上游验证被拒」时，先用同一组账密同步回访上游复核，通过才放行。
6. 首次登录的账号自动创建，邮箱按 `server.email_domains` 生成。
7. 登录成功后写入 `relay_session` cookie，跳回 `next`；没有 `next` 时回到用户中心。

上游登录成功但本地密码不匹配时，relay 会用新密码更新本地哈希。上游改密后，用户下次登录会自动同步。
本地命中后的登录会按 `upstream.refresh_rate` 的概率异步回访上游刷新资料；回访被上游拒绝（密码已改）时，
relay 会标记该账号，之后的本地命中登录都会先同步复核上游，旧密码不再长期有效。

### 扫码登录

登录页切换到「微信扫码」后，relay 向上游申请一个 uuid 并把它渲染成二维码，二维码内容是
`<cas_base_url>/cas/qyQrLogin?uuid=<id>`。用户用微信扫码后，上游把微信身份绑定到该 uuid，
relay 每 2 秒用同一份 cookie 向上游查询一次状态，确认后跟进上游返回的跳转拿到门户会话，
再走与账密登录相同的资料落库与会话下发。`upstream.qr_login` 为 `false` 时登录页不显示这个入口。

扫码登录的账号归属按两种情况处理：

| 情况 | 处理 |
| --- | --- |
| 门户返回的手机号已绑定过本地账号 | 直接登录该账号，并刷新资料快照与上游用户组 |
| 首次扫码 | 借用扫码建立的 SSO 会话为门户申请一张票据并校验，取回学号后创建本地账号 |

首次扫码创建的账号没有本地密码，之后用账密登录时会回访上游并补上密码哈希。
上游没返回学号（票据校验失败或主账号不是纯数字）时，提示用户先用账号密码登录一次。

生成二维码时 relay 会向浏览器下发一个 HttpOnly 绑定 cookie，轮询与确认结果只发给持有
该 cookie 的浏览器，防止借他人二维码在受害者浏览器里种会话。同一 IP 每分钟最多生成 12
个二维码；IP 命中验证码策略时，生成二维码也要先通过极验验证。

| 提示 | HTTP | 原因 |
| --- | --- | --- |
| 二维码已失效，请刷新后重试 | 410 | 超过 3 分钟未完成扫码 |
| 二维码已失效，请刷新后重试 | 403 | 轮询请求缺少或不匹配绑定 cookie |
| 该微信还未关联统一身份认证账号，请先用账号密码登录一次 | 409 | 上游资料里拿不到可用于建账号的学号 |
| 上游认证服务不可用，请改用账号密码登录 | 502 | 轮询、门户首页或用户信息接口失败 |
| 该账号已被停用 | 403 | 本地账号被管理员禁用 |
| 获取二维码过于频繁，请稍后再试 | 429 | 同一 IP 每分钟超过 12 次生成 |

### 会话

| 项目 | 值 |
| --- | --- |
| Cookie 名 | `relay_session` |
| 有效期 | 12 小时 |
| HttpOnly | 是 |
| Secure | `base_url` 为 HTTPS 时开启 |
| 存储 | Redis，键前缀 `relay:session:` |

退出登录会删除 Redis 中的会话并清除 cookie。重新登录会生成新令牌，旧令牌不会自动失效。
管理员禁用某个账号时，relay 会立即清空该账号的全部会话，无需等待 12 小时。

### 失败提示

| 提示 | HTTP | 原因 |
| --- | --- | --- |
| 账号或密码错误 | 401 | 本地与上游都拒绝 |
| 该账号已被停用 | 403 | 本地账号被管理员禁用 |
| 尝试次数过多，请稍后再试 | 429 | 同一账号 15 分钟内触发上游超过 5 次 |
| 上游账号已被锁定，请稍后再试 | 429 | CAS 返回账号锁定 |
| 上游要求验证码，无法自动登录，请联系管理员 | 502 | CAS 要求验证码或其他额外校验 |
| 上游认证服务不可用 | 502 | CAS 或门户不可达、响应异常、缺少必要字段 |
| 该学号没有匹配的邮箱域名配置 | 500 | `server.email_domains` 没有匹配到前缀 |
| 请先完成验证 | 400 | 验证码未通过或已过期 |
| 验证服务不可用，请稍后再试 | 502 | 极验校验接口不可用 |

同一账号的 5 次上限只统计需要访问上游的登录（本地未命中，或本地命中但需复核过期标记）。普通本地命中不消耗次数，登录成功后会清零。

### 验证码

验证码按来访 IP 生效，不按账号。`server.trusted_proxies` 配置错误时，所有用户可能共享同一个 IP 标记。

| 配置 | 行为 |
| --- | --- |
| `captcha.enabled: false` | 不显示验证码 |
| `captcha.enabled: true`，`always: false` | 该 IP 曾登录失败时显示；成功一次后解除 |
| `captcha.enabled: true`，`always: true` | 每次登录都显示 |

## 用户中心

`/` 展示当前登录用户的学号、姓名、邮箱、班级、用户组与第三方账号绑定状态。

### 修改邮箱别名

用户中心点击邮箱旁的编辑图标可申请修改邮箱，规则如下：

1. 只允许修改 `@` 前的别名，后缀必须保持为该学号在 `server.email_domains` 中匹配到的后缀。
2. 新邮箱不能与当前邮箱相同，也不能与任何其他账号的邮箱重复。
3. 提交后 relay 向新邮箱发送确认邮件，链接有效期 30 分钟；再次申请会使旧链接失效。
4. 在邮件中点击确认链接后才正式写入 `users.email`，之后 `/api/me`、OIDC 的 `email` claim 与各类通知邮件均以新邮箱为准。

需要配置 `smtp` 才能使用；未配置时接口返回 503。

| 查询参数（回跳用户中心时） | 含义 |
| --- | --- |
| `email_changed=1` | 邮箱已切换成功 |
| `email_error=invalid` | 确认链接无效或已过期 |
| `email_error=taken` | 等待确认期间该邮箱已被其他账号使用 |
| `email_error=failed` | 写入数据库失败 |

### 第三方账号绑定

绑定源来自 `bindings.providers` 配置，未配置时页面不显示该区域。来源若被设为私有，只有命中的用户在用户中心能看到该行，其他人看不到也无法发起绑定。

1. 点击「去绑定」，跳转到对应平台的授权页。
2. 在平台确认授权后回到 `/bindings/<provider>/callback`。
3. relay 用授权码换取用户信息，保存身份标识与展示名。
4. 回到用户中心并提示绑定成功。

| 查询参数 | 含义 |
| --- | --- |
| `bound=<provider>` | 绑定成功 |
| `bind=<provider>` | 应用要求先绑定，该行会高亮 |
| `bind_error=state` | state 不存在或已过期，有效期 10 分钟 |
| `bind_error=denied` | 用户在平台上取消了授权 |
| `bind_error=exchange` | 换取令牌或用户信息失败 |
| `bind_error=store` | 写入数据库失败 |

解除绑定立即删除 `oauth_accounts` 中的记录。若某个应用设置了强制绑定，用户下次授权前必须重新绑定。

### 自动跳转

从授权流程跳来时，地址会带 `bind=<provider>&next=<授权请求>`。绑定完成后页面会自动回到 `next` 继续授权，不需要用户手动点击。

## 应用授权流程

下游应用按标准 OIDC 授权码流程接入，细节见 [integration.md](integration.md)。relay 侧的处理顺序如下：

1. 校验 `client_id` 与 `redirect_uri`。
2. 校验 `response_type=code`，`scope` 必须包含 `openid` 且不超过应用允许的范围。
3. 未登录时跳转 `/login?next=<原始授权请求>`。
4. 检查应用限定的用户组，不在组内返回 `access_denied`。
5. 检查应用要求的强制绑定，账号无权使用该绑定源时返回 `access_denied`，缺少绑定时跳转用户中心。
6. 检查用户是否已授予这些 scope，未授予时跳转 `/consent?req=<id>`。
7. 校验 PKCE，公开客户端必须携带。
8. 签发授权码并跳回 `redirect_uri`。

用户同意一次后，授权记录长期有效。应用之后申请新的 scope 时，会再次进入确认页。用户组与强制绑定在每次授权时重新检查。在确认页点「取消」会带 `error=access_denied` 回跳应用，待确认的授权请求随即作废。

## 管理后台

`/admin` 需要 `type=1` 的管理员。页面分四块：用户、用户组、应用、绑定源。

### 用户

- 按学号或姓名搜索，界面最多展示 100 条，接口上限 200 条。
- 只展示已经登录过一次的账号。
- 可切换管理员身份，可禁用或启用账号。

禁用后，该账号无法再次登录，新的授权请求会被拒绝，已建立的会话会被立即清空。
已经签发的 access token 在 1 小时有效期内仍可能被下游离线校验通过，在线校验 userinfo 会立即失效。

### 用户组

| 操作 | 说明 |
| --- | --- |
| 新建 | `name` 为展示名，`code` 为唯一标识，创建后不建议修改 |
| 成员 | 输入学号，逗号或空格分隔；保存会整体替换该组成员 |
| 删除 | 同时删除成员关系与应用的用户组限制 |

用户组只有被应用引用时才影响访问。应用一个组都不选，表示不限制。组可以有父组，子组成员同时具备父组的访问资格。应用申请 `groups` scope 后，用户的组标识会随 `id_token` 与 userinfo 返回，包含祖先组。

### 应用

| 操作 | 说明 |
| --- | --- |
| 新建 | 需要 `client_id`、名称、回调地址；`client_id` 只允许字母、数字、下划线与连字符，最长 64 |
| 权限 | 勾选允许申请的 scope、强制绑定源、限定用户组，点击即时生效 |
| 停用 | 停用后应用无法发起授权，也无法用授权码换取令牌，配置与授权记录保留，可随时恢复 |
| 删除 | 同时删除 scope、用户组、用户白名单、授权记录与登录通知；已签发令牌在有效期内仍可离线校验通过 |
| 客户端密钥 | 创建时显示一次，服务端只保存哈希；没有轮换接口，需要更换时删除重建 |

公开客户端不校验密钥，必须使用 PKCE。机密客户端支持 `client_secret_basic` 与 `client_secret_post`。

### 绑定源

来源本身在 `bindings.providers` 中声明，增加或修改需要改配置文件并重启 relay。每个来源的访问范围在后台配置，存于 `binding_acls`：

| 访问范围 | 效果 |
| --- | --- |
| 所有登录用户 | 用户中心对全部登录用户显示该来源 |
| 仅指定用户与用户组 | 只有用户白名单或指定用户组（含祖先组）里的账号能看到并使用；白名单为空时没有任何账号可用 |

未配置过访问控制的来源等同于对所有登录用户开放。无权限的用户看不到该行，直接访问 `/api/bindings/<provider>/start` 返回 403。应用把该来源设为强制绑定时，无权限的账号在授权环节按 `access_denied` 回跳，不会陷入绑定死循环。

## 命令行

### relayctl

```bash
relayctl [-config 路径] <命令>
```

| 命令 | 说明 |
| --- | --- |
| `client create -id <client_id> -name <名称> -redirect <地址[,地址]> [-scopes a,b] [-public]` | 创建应用，输出一次 `client_secret` |
| `client list` | 列出应用、类型与回调地址 |
| `client delete -id <client_id>` | 删除应用及其 scope、用户组关联 |
| `user promote -id <学号> [-revoke]` | 设为管理员，`-revoke` 取消 |

示例：

```bash
./bin/relayctl -config config.yaml client create \
  -id gitea \
  -name "Git Server" \
  -redirect "https://git.example.edu.cn/user/oauth2/relay/callback" \
  -scopes openid,profile,email

./bin/relayctl -config config.yaml user promote -id 20230001
```

`user promote` 要求目标用户已经登录过。`relayctl` 与 `relay` 使用同一份配置并通过相同校验。

### casprobe

探测上游 CAS 与门户，输出 relay 保留的用户信息字段。

```bash
GZHU_USERNAME=<学号> GZHU_PASSWORD=<密码> ./bin/casprobe
```

输出示例：

```json
{
  "sub": "13800000000",
  "name": "张三",
  "belong_class": "计算机 231"
}
```

`sub` 来自门户 `MOBILE` 字段，也是邮箱前缀的来源。上游改版、登录突然全部返回 502 时，先用它确认字段是否仍然存在。

## API 一览

所有响应为 JSON，错误统一为 `{"error": "说明"}`。

### 公开与 OIDC

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 数据库与 Redis 健康检查 |
| GET | `/.well-known/openid-configuration` | OIDC discovery |
| GET | `/oidc/jwks` | 签名公钥 |
| GET | `/oidc/authorize` | 授权端点 |
| POST | `/oidc/token` | 令牌端点，支持授权码与刷新令牌 |
| POST | `/oidc/revoke` | 撤销令牌 |
| GET | `/oidc/userinfo` | 用户信息端点 |
| GET | `/api/public/config` | 登录页所需的公开配置 |
| POST | `/api/login` | 登录 |
| POST | `/api/login/qrcode` | 生成扫码登录二维码，返回 `id`、`url` 与有效期 |
| GET | `/api/login/qrcode/<id>` | 查询扫码状态，`pending` 表示等待中，`confirmed` 时下发会话 cookie |
| POST | `/api/logout` | 退出登录 |

### 登录后

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me` | 当前用户、用户组与绑定状态 |
| POST | `/api/email/change` | 申请修改邮箱别名，发送确认邮件 |
| GET | `/api/email/confirm?token=` | 邮件确认链接，正式切换邮箱 |
| GET | `/api/consent?req=` | 查询待确认授权 |
| POST | `/api/consent` | 同意授权，返回继续地址 |
| POST | `/api/consent/deny` | 拒绝授权，返回带 `error=access_denied` 的回跳地址 |
| GET | `/api/bindings/<provider>/start?next=` | 发起绑定 |
| DELETE | `/api/bindings/<provider>` | 解除绑定 |
| GET | `/bindings/<provider>/callback` | 绑定源回调 |

### 管理员

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/admin/users?q=&limit=` | 搜索用户 |
| PATCH | `/api/admin/users/<id>` | 修改 `type` 或 `status` |
| GET | `/api/admin/groups` | 用户组列表 |
| POST | `/api/admin/groups` | 新建用户组 |
| PATCH | `/api/admin/groups/<id>` | 修改名称、标识、父组 |
| DELETE | `/api/admin/groups/<id>` | 删除用户组 |
| GET | `/api/admin/groups/<id>/members` | 组成员 |
| PUT | `/api/admin/groups/<id>/members` | 整体替换组成员 |
| GET | `/api/admin/scopes` | 全部 scope |
| GET | `/api/admin/bindings` | 已配置的绑定源与各自的访问控制 |
| PUT | `/api/admin/bindings/<provider>` | 保存绑定源访问范围、用户组与用户白名单 |
| GET | `/api/admin/clients` | 应用列表 |
| POST | `/api/admin/clients` | 新建应用 |
| PATCH | `/api/admin/clients/<id>` | 修改名称、回调、强制绑定、启用状态、客户端类型 |
| DELETE | `/api/admin/clients/<id>` | 删除应用 |
| PUT | `/api/admin/clients/<id>/scopes` | 整体替换允许的 scope |
| PUT | `/api/admin/clients/<id>/groups` | 整体替换限定用户组 |

## 日常运维

### 日志

访问日志与业务错误输出到标准输出。登录失败会记录学号、IP 与原因，不记录密码。容器部署时直接收集 stdout。

### 数据备份

| 对象 | 说明 |
| --- | --- |
| PostgreSQL | 用户、用户组、应用、授权记录、绑定关系 |
| `server.signing_key_path` | OIDC 签名私钥，丢失后需重新签发并更新下游 |
| `secret_key` | 对称加密密钥，丢失后已加密数据无法解密 |
| `config.yaml` | 全部部署参数 |

恢复时先还原数据库与密钥文件，再启动服务。

### 升级

1. 重新构建前端与二进制。
2. 替换 `bin/` 下的可执行文件。
3. 保留原 `config.yaml` 与 `data/signing_key.pem`。
4. 启动时自动迁移新增表与列。

### 清理 Redis

清空 `relay:*` 会使所有用户退出登录，丢失进行中的授权与绑定流程，不影响数据库数据。

## 故障排查

| 现象 | 排查方向 |
| --- | --- |
| 启动报缺少必需项 | 按报错补全 `config.yaml`，注意 `secret_key` 与 `email_domains` |
| `/healthz` 返回 503 | 检查 PostgreSQL 与 Redis 连接、账号权限、网络 |
| 登录全部 502 | 用 `casprobe` 验证上游；检查 DNS、出口网络、上游是否改版 |
| 登录提示上游要求验证码 | CAS 风控触发，relay 无法自动通过，需要用户到上游处理或更换出口 IP |
| 登录提示邮箱域名 | `email_domains` 前缀与门户 `MOBILE` 不匹配 |
| 授权返回 `invalid_scope` | 请求缺少 `openid`，或申请了应用未被允许的 scope |
| 授权返回 `invalid_request` | `redirect_uri` 不在允许列表，或公开客户端未带 PKCE |
| 授权返回 `access_denied` | 用户不在应用限定的用户组内 |
| 绑定回调 `bind_error=exchange` | 回调地址与平台登记值不一致，或授权码已过期 |
| 验证码不显示 | 浏览器无法访问 `static.geetest.com`，或 `captcha.enabled` 为 false；也检查响应头 `Content-Security-Policy` 是否放行了极验域名 |
| 管理后台 403 | 当前账号不是管理员，用 `relayctl user promote` 提升 |
| 下游校验令牌失败 | 检查 issuer 是否为最新 `base_url`，JWKS 是否指向本服务，签名密钥是否被更换 |
