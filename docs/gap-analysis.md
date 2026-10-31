# 能力盘点与同类产品对比

分析对象：`gzhu-sso-relay`（HEAD `cc72d47`，51 次提交）
对比范围：Keycloak、Apereo CAS、Shibboleth IdP/CARSI、Casdoor、Logto、Authentik、Ory Hydra、Dex、Authelia、Auth0/Entra ID、GitLab/Gitea。

## 0. 结论摘要

| 维度 | 判断 |
| --- | --- |
| 定位 | 单校身份中继：上游 CAS + 融合门户 → 本地用户库 → OIDC Provider。不是通用 IAM，也不该做成通用 IAM |
| 协议 | OIDC 授权码流程可用，覆盖约 60% 的 OIDC 核心（缺刷新令牌、撤销、自省、RP 发起登出） |
| 生态兼容 | 只能接"现代 Web 应用"。SAML SP 与 CAS 老系统一律接不进来，而这恰恰是高校存量大头 |
| 应用治理 | 只有管理员建应用 + 一次性 secret。缺状态机、按类型建模、自助申请、撤销、用量统计 |
| 合规 | 无审计表、无个人撤回同意入口。等保 2.0 与《个人信息保护法》都卡这两条 |
| 工程质量 | 0 个测试，无版本化迁移，单签名密钥。作为内部工具可接受，对外承诺 SLA 前需补 |

一句话：**作为"给新应用发 OIDC 凭证"的工具已经够用；作为"全校统一认证出口"还差一层。**

## 1. 现状能力清单

| 已实现 | 位置 |
| --- | --- |
| OIDC discovery / JWKS / authorize / token / userinfo | `internal/oidc/*` |
| 授权码流程、PKCE（S256 + plain）、公开客户端强制 PKCE | `authorize.go:103-116` |
| RS256 单密钥，首次启动生成 | `oidc/key.go` |
| 本地 Argon2id 校验 → 上游 CAS 兜底 → 回写哈希 | `auth/service.go:77-113` |
| Redis 会话 12h、登录限流 5 次/15min、极验按 IP | `session/`、`auth/throttle.go`、`captcha/` |
| 应用 CRUD、secret 轮换、图标、scope 分配 | `httpapi/admin_clients.go` |
| 用户组（含上游同步组）、组/用户白名单 | `repo/client.go:77-115`、`auth/service.go:196` |
| 强制第三方绑定校验（如必须先绑 Gitea） | `authorize.go:69-81` |
| 按 scope 裁剪 claims、consent 记录 | `provider.go:100-115`、`model/consent.go` |
| 首次授权邮件通知 | `notify/notify.go` |
| 内嵌 SPA：登录 / 用户中心 / 授权确认 / 管理后台 | `web/src/pages/*` |

## 2. 认证协议与流程

| 能力 | relay | 同类做法 | 判断 |
| --- | --- | --- | --- |
| 授权码流程 | 有 | 全都有 | 达标 |
| PKCE | S256 + plain，公开客户端强制 | Keycloak/Auth0/Logto 强制 S256，不推荐 plain | 建议禁 plain |
| refresh_token | **无** | Keycloak/Logto/Casdoor/Hydra/Auth0 全有；Dex 用 `offline_access` | **最大缺口**。1 小时后必须回浏览器重新授权，移动端与长会话 SPA 无法接受 |
| offline_access scope | 无 | 同上 | 同上 |
| token revocation（RFC 7009） | 无 | Hydra/Keycloak/Logto/Casdoor 均有 | 应用侧无法主动吊销泄露令牌 |
| introspection（RFC 7662） | 无 | Hydra/Keycloak/Casdoor 有 | 资源服务只能离线验签，无法感知禁用 |
| RP 发起登出（`end_session`） | 无 | Keycloak/Auth0/Logto/Casdoor 均支持，支持 `id_token_hint` + `post_logout_redirect_uri` | 用户从门户退出，应用里仍在线 |
| 后端通道登出 | 无 | Keycloak、Authentik、Entra 支持 Back-Channel Logout 1.0 | 与上一条配套，缺了就是"假单点退出" |
| `prompt=none/login/consent` | 无 | 全都有 | RP 无法做静默探测登录态、无法强制重新认证 |
| `login_hint` | 无 | 全都有 | 多账号场景无法预填 |
| `max_age` / `acr_values` | 无 | Keycloak（ACR/LOA）、Entra（认证强度） | 高敏应用（财务、成绩）无法要求二次认证 |
| `groups` claim | **无**（只在 `/api/me` 返回） | Keycloak/Authentik/Entra 可把组写进 token | 应用无法自己做细粒度授权，只能靠门户粗粒度拦截 |
| 自定义 scope 产生 claim | **无**（`claimsFor` 只认 profile/email） | Keycloak client scope + mapper、Casdoor 属性映射 | 后台能勾 `read:e-card` 这类 scope，但请求它什么也不返回，等于装饰 |
| userinfo 方法 | 仅 GET | 规范要求 GET + POST | 少数客户端库用 POST 会失败 |
| 动态客户端注册（RFC 7591） | 无 | Keycloak、Hydra、Authentik 支持 | 校内可控场景可不要 |
| PAR / JAR / DPoP / mTLS | 无 | Hydra、Keycloak 较新版本支持 | 不必做 |
| device_code（RFC 8628） | 无 | Keycloak/Entra/Auth0 支持 | 只有接 CLI/大屏时才需要 |
| client_credentials | 无 | 全都有 | 校内微服务间调用需要 |
| token exchange（RFC 8693） | 无 | Keycloak 支持 | 微服务网关场景才需要 |
| **SAML 2.0 SP** | **无** | Keycloak、CAS、Shibboleth、Casdoor、Authentik | **硬缺口**。知网、超星、CARSI 联邦、大量教务系统只认 SAML |
| **CAS 1/2/3 服务端** | **无**（只做客户端） | CAS、Casdoor 支持 | 改造量最小、收益最大：上游本来就是 CAS，协议栈现成，老系统零改动迁移 |
| LDAP / RADIUS 出口 | 无 | Keycloak、Authelia、CAS | 网络设备、老 B/S 系统可能需要 |
| SCIM 2.0 用户同步 | 无 | Keycloak、Zitadel、Entra | 应用侧自动开号销号，避免离职账号残留 |

**协议侧建议优先级**：refresh_token → end_session + back-channel → prompt/login_hint → groups claim → CAS 服务端 → SAML。前四项是"接进去能不能用"的问题，CAS/SAML 是"能不能全覆盖"的问题。

## 3. 令牌与会话生命周期

| 能力 | relay | 同类 | 判断 |
| --- | --- | --- | --- |
| access token 形式 | 自包含 JWT，1h，全局固定 | Keycloak/Logto 支持按 client 配 TTL；Hydra 支持 opaque | 应按应用配 TTL；离线验签无法感知禁用 |
| 令牌受众 | `aud=[client_id]` | 支持多受众 / `resource` 参数 | 多个微服务共用一个 token 时无法限定 |
| 签名密钥轮换 | 单密钥，换 key 即全量失效 | Keycloak 多 key 并存（active/passive），Auth0 自动轮换 | 平滑轮换能力缺失，是一次性运维事故点 |
| 上下文会话 | Redis 单 token，固定 12h | Keycloak 区分 idle 与 max，支持 remember-me | 无空闲超时 |
| 用户级会话索引 | **无**（只按 token 反查用户） | Keycloak 会话列表、Entra 撤销会话 | 无法"下线该用户全部设备"，禁用后旧会话仍活 12h |
| 令牌撤销传播 | 无 | 撤销会话即失效 refresh token，back-channel 通知 RP | 与审计、合规直接相关 |
| 组变更生效 | 授权时检查，已发令牌不回收 | Keycloak 每次 token 请求重新算 claim | 跨组调岗后最长 1h 残留权限 |

## 4. 应用创建与治理

这是需求里特别提到的角度。

| 能力 | relay | 同类做法 | 判断 |
| --- | --- | --- | --- |
| 创建入口 | 仅管理员后台 / `relayctl` | Auth0/Azure/GitLab/Gitea/Casdoor：登录用户自助创建；Keycloak：仅管理员 | 校内可控，管理员建可接受；但缺"申请—审批"流 |
| 按应用类型建模 | 仅 `is_public` 布尔 | Auth0/Azure 分 SPA / Native / Web / M2M，自动决定是否发 secret、是否强制 PKCE、能否用 refresh_token | 建议引入 `client_type`，把策略挂在类型上 |
| 应用状态机 | 无（只能删） | Keycloak `enabled` 开关、Auth0 应用可禁用、Azure 有发布状态 | 下线维护只能删了重建，历史授权全丢 |
| 级联清理 | 删除不清 consent，不吊销令牌（文档已承认） | 删除客户端即失效全部令牌与该应用会话 | 应级联清 consent + 记审计 |
| 密钥管理 | 一次性 secret，轮换即断旧 | 双 secret 并存过渡期、带过期时间、私有密钥 JWT | 轮换是硬切换，应用方必须同时改，风险高 |
| 回调地址规则 | 全等匹配（大小写不敏感），仅 http/https | 支持 loopback 任意端口（RFC 8252）、自定义 scheme（`myapp://`）、部分支持通配子域 | **桌面/移动端应用注册不了**，`checkRedirects` 直接拒 |
| 是否跳过同意 | 无开关，任何应用首次必弹同意页 | Keycloak `consent required`、GitLab `trusted` 应用 | 零权限的内部应用也要用户点一次，纯摩擦 |
| 应用元数据 | 名称、主页、描述、图标 | 隐私政策 URL、服务条款、技术支持联系方式、负责人、所属部门 | 授权页无法给出合规信息 |
| 用量统计 | 无 | Auth0/Keycloak/Logto 提供按应用统计 | 无法判断哪些应用可下线、哪些异常 |
| 品牌定制 | 全局登录页文案 | Keycloak realm/client 主题、Auth0 按应用 logo+文案 | 用户会怀疑"跳到这个页面安全吗" |

## 5. 信任与授权决策

| 能力 | relay | 同类 | 判断 |
| --- | --- | --- | --- |
| 组限制 | 直接成员匹配 | Keycloak/Authentik 支持组层级递归 | **`groups.parent_id` 存了但授权不递归**，子组用户拿不到父组权限 |
| 用户白名单 | 有（`is_private` + `client_user_grants`） | 全都有 | 达标 |
| 属性级策略 | 无 | Keycloak authorization services、CAS attribute release policy、Entra conditional access | 只到"能不能进"，到不了"进去看到什么" |
| 同意记录 | 按 scope 集合，永久有效 | Keycloak 同意可设过期、可撤销 | 无过期、无用户自助撤销 |
| 取消授权 | 界面点"取消"只跳 `/`，**不通知应用** | 全部实现回传 `error=access_denied&state=` | **BUG**：应用侧会一直挂着等回调 |
| 强制绑定 | 有，配置驱动 | 少见（Keycloak identity provider + required action 组合实现） | 这是本项目的差异化能力，保留 |

## 6. 身份源与认证方式

| 能力 | relay | 同类 | 判断 |
| --- | --- | --- | --- |
| 密码认证 | 本地哈希优先、上游兜底 | Keycloak/CAS 委托 LDAP/AD | 合理，等于"上游可用时透明、上游不可用时降级" |
| MFA（TOTP/WebAuthn） | 无 | Keycloak 全套、Authelia、Casdoor、Logto | 管理员账号无二次认证，等保三级不通过 |
| Passkey | 无 | Keycloak、Logto、Authentik 已支持 | 可后置 |
| 社交/企业登录 | 反向：只做绑定，不做登录 | Keycloak identity brokering、Casdoor | 边界清晰，不建议扩大 |
| 多身份源 | 单校单上游（`upstream.cas_base_url`） | Keycloak 多 realm、Casdoor 多 organization | 若未来要并分校/附属医院，需多租户改造 |
| 密码自助 | 无（依赖上游） | 多数提供 portal 入口 | 至少应在用户中心给"去上游改密"的链接 |
| 账号状态 | active/disabled | 加锁定、过期、待激活、离职保留期 | 缺"毕业生保留 6 个月"这类校园特有能力 |

## 7. 安全与合规

| 项 | 现状 | 风险 |
| --- | --- | --- |
| CSRF | 无 token；cookie 未设 `SameSite`（`auth.go:173`） | 所有应用都在 `*.gzhu.edu.cn` 下，属同站，浏览器默认 Lax 拦不住。任一被攻陷的子域可冒充用户调用 `/api/admin/*` |
| 授权码消费顺序 | `token.go:48-55` 先 `GetDel` 再比对 client_id | 他人可用错配 client 试探，把合法授权码提前消耗，形成登录失败 DoS |
| token 端点限流 | 无 | `invalid_client` 走 Argon2id 校验，错误 secret 洪泛可直接打满 CPU |
| 安全响应头 | 无 CSP / HSTS / X-Frame-Options | 点击劫持、混合内容 |
| 审计日志 | 仅 stdout，无表、无留存策略 | 等保 2.0 三级要求审计留存 ≥6 个月，且管理员操作需可追溯 |
| 个人数据权利 | 无"已授权应用"列表，无法撤回同意 | 《个人信息保护法》第 15 条撤回同意权，硬合规项 |
| 凭据存储 | 本地存 Argon2id 哈希 | 与上游双份凭据，泄露面翻倍，需在文档中明示风险 |
| 迁移 | `AutoMigrate`，无版本表 | 复杂变更只能靠手写 backfill（已有 4 处），回滚困难 |

## 8. 运维与可观测

| 能力 | relay | 同类 |
| --- | --- | --- |
| 健康检查 | `/healthz` 合并 DB+Redis，两者挂任一即 503 | Keycloak 分离 liveness/readiness/started；Ory 有独立探针 |
| 指标 | 无 | Keycloak `/metrics`、Logto、Authentik、Hydra 均有 Prometheus |
| 链路追踪 | 无 | 多数支持 OTel |
| 审计检索 | 无（只能 grep stdout） | Keycloak admin events 可查可导 |
| 配置热更新 | 需重启 | Keycloak 部分支持 runtime 修改 |
| 高可用 | 签名密钥落本地盘，多实例需共享文件 | Keycloak 密钥存 DB，天然多实例 |

## 9. 用户侧体验

| 能力 | 现状 | 同类 |
| --- | --- | --- |
| 已授权应用管理 | 无 | Google/微信/飞书/Keycloak account console 都有，且可撤销 |
| 登录设备管理 | 无 | Keycloak sessions、Entra 我的登录 |
| 安全事件通知 | 仅首次授权邮件 | 新设备/异地登录、授权变更、密码修改提醒 |
| 多语言 | 中文硬编码 | Keycloak/Logto i18n |
| 错误页 | 取消后静默回首页，无解释 | 统一错误页 + 错误码 |
| 可访问性 | 未验证 | — |

## 10. 缺陷清单（建议立即修）

下列 1–9 项已在同批修改中修复，第 10 项属于设计缺口，留待权限模型调整时一并处理。

| # | 问题 | 位置 | 影响 |
| --- | --- | --- | --- |
| 1 | 取消授权不回传 `access_denied` | `web/src/pages/Consent.tsx:140` | 应用侧无限等待 |
| 2 | 无 CSRF 防护、cookie 未设 SameSite | `httpapi/auth.go:173` | 同站子域可伪造管理操作 |
| 3 | 授权码先删后校验 client | `oidc/token.go:48` | 登录 DoS |
| 4 | 组层级不参与授权判定 | `repo/client.go:103` | 管理员配了子组，实际不生效 |
| 5 | token 端点无失败限流 + Argon2id | `oidc/token.go:38` | CPU 耗尽 |
| 6 | 无可停用状态，删除不级联 | `admin_clients.go:287` | 授权记录残留 |
| 7 | 无法强制下线用户会话 | `session/session.go` | 禁用后仍在线 12h |
| 8 | 回调地址不支持 loopback 与自定义 scheme | `admin_clients.go:405` | 桌面/移动端接不进 |
| 9 | 无安全响应头 | `cmd/relay/main.go:145` | 点击劫持 |
| 10 | 自定义 scope 不产生任何 claim | `oidc/provider.go:100` | 权限模型是装饰 |

## 11. 路线图建议

### P0 — 正确性与安全（约 3–5 人日）

1. 取消授权回传 `error=access_denied`，`Pending` 里已存 `AuthorizeURL`，可直接复用。
2. CSRF token + 显式 `SameSite=Lax`（或对 `/api/admin` 校验 `Origin`）。
3. 授权码改为"先读、校验、后删"。
4. 应用加 `enabled` 字段，停用即拒绝授权与令牌请求；删除级联清 consent。
5. token 端点加 IP+client 维度限流，`invalid_client` 直接短路（不改哈希也先挡）。
6. 组授权递归父链。
7. 安全响应头中间件。
8. 回调地址允许 `http://127.0.0.1:*` 与自定义 scheme。

### P1 — 生态兼容（约 15–20 人日）

1. refresh_token + rotation + 按 client 配 TTL。
2. `end_session_endpoint`、`id_token_hint`、back-channel logout。
3. `prompt` / `login_hint` / `max_age`。
4. `groups` claim + 自定义 scope → claim 映射表（把 `scopes` 表扩成"scope → claim 表达式"）。
5. 用户侧"已授权应用"列表与撤销。
6. 审计表（登录、授权、管理操作三类事件）+ 管理后台查询。
7. 多签名密钥并存与平滑轮换。
8. 双 secret 过渡期轮换。
9. 按 client 的"是否需要同意"开关。
10. OIDC 一致性测试（OpenID Foundation conformance suite）纳入 CI。

### P2 — 覆盖面与运营（约 30+ 人日）

1. CAS 1/2/3 服务端（`/cas/serviceValidate`），老系统零改动迁移。
2. SAML 2.0 IdP（先满足 CARSI / 知网 / 超星）。
3. 开发者自助注册 + 审批流 + 应用负责人。
4. 应用用量统计（授权人数、登录量、失败率、最后活跃）。
5. SCIM 2.0 或组织同步 API。
6. `/metrics` + OTel、liveness/readiness 拆分。
7. 迁移工具版本化（引入 goose/atlas）。
8. 登录设备管理 + 安全事件通知。
9. 按应用品牌定制登录页、i18n。
10. 管理员 MFA。

## 12. 明确不建议做

| 项 | 理由 |
| --- | --- |
| 自建社交登录 / 多租户 / 计费 | 这是 Casdoor、Logto 的赛道，本项目价值在上游对接与绑定策略 |
| 通用 authorization services（UMA） | 校内应用规模下，组 + claim 足够；引入后维护成本远超收益 |
| 动态客户端注册对外开放 | 校内应用需人工审核回调地址，开放注册等于开放钓鱼回调 |
| PAR / JAR / DPoP / mTLS | 无对应威胁模型，只会增加联调成本 |
| 自建密码体系（找回、策略、过期） | 上游才是权威，重复建设会制造"两边密码不一致"的支持工单 |
| 重写为一般 IAM 产品 | 现有 51 次提交的领域逻辑（邮箱生成规则、上游组同步、强制绑定）才是资产 |
