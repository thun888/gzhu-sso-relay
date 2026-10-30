# GZHU SSO Relay

1.  使用统一SSO登录 → 绑定gitea账号
2.  兼容现有gitea → 引入单独的路由，访问该路由请求时，中转返回对应的git用户的消息，而不是SSO的
3.  正常可以使用SSO端点，后台请求官方SSO各个接口，凑出数据返回

**技术栈要求**

后端：

1.  Gin
2.  gorm + 外部PgSQL
3.  Redis

前端：

1.  React
2.  shadcn/ui


**OIDC + OAuth**
需要创建单独的表
    1.  程序表
    2.  用户信任哪些程序
    3. ...

**登录流程：**
    1.  点击登录，跳转到 `sso-proxy`
    2.  输入账密（已登录则跳过）
    3.  跳至用户中心，显示已登录用户信息以供确认
    4.  继续授权（可跳过），可对接多个外部应用
    5.  检查当前应用有无强制外部应用绑定要求，以及用户组能否访问该应用
    6.  返回授权信息

补充说明：
- 自动探测目标应用连通性，不可用时才从 `profile_data` 中直接返回

1. user 表
    1.  `id` 学号（主键AK）
    2.  `name` 名称
    3.  `password` 密码（加密）
    4.  `email`（自动生成）
    5.  `type` 用户类型
        - 0：普通用户
        - 1：管理员

2. oauth_accounts 表
    1.  `id` 主键AK（UUID）
    2.  `user_id` 学号
    3.  `provider` 提供商
    4.  `provider_user_id` 提供商的用户ID
        （标注：两组联合索引）
    5.  `access_token` 访问令牌
    6.  `refresh_token` 刷新令牌
        （标注：令牌加密存储，配置文件留存密钥）
    7.  `profile_data` 原始信息，可直接返回
    8.  `created_at` 绑定时间
    9.  `updated_at` 更新时间

3. user_groups：中间表
    1.  `id` 主键AK
    2.  `user_id`
    3.  `group_id`
    4.  `created_at`
    5.  `updated_at`

4. groups：用户组
    1.  `id` 主键AK
    2.  `name` 组名
    3.  `code` 标识/slug
    4.  `parent_id` 父组ID
    5.  `created_at`
    6.  `updated_at`

5. oauth_clients：第三方应用表
    1.  `id` 主键AK
    2.  `client_id` 客户端标识
    3.  `client_secret` 客户端密钥
    4.  `name` 应用名称
    5.  `redirect_urls` 允许回调列表
    6.  `is_public` 是否公开
    7.  `created_at`
    8.  `updated_at`



6. group_oauth_clients：多对多关联表，控制哪些用户组有权访问哪些OAuth应用
    1.  `id`
    2.  `group_id`
    3.  `client_id`
    4.  `created_at`
    5.  `updated_at`


7. scopes 表
    1.  `id`（scope标识）
    2.  `name` → 命名规则：动作+资源，示例：`read:profile`、`read:e-card`、`read:face`
    3.  `display_name` 显示名称
    4.  `description` 描述
    5.  `is_default` 是否默认授权

8. client_allowed_scopes：中间表，应用允许请求的scope关联表