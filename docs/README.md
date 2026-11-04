# GZHU SSO Relay 文档

relay 把学校统一身份认证包装成标准 OIDC Provider。用户仍然使用学号或教工号与原密码登录，也可以用微信扫码登录，下游应用只面对 OIDC。服务在本地维护用户、用户组、第三方账号绑定与授权记录。

## 文档索引

| 文档 | 读者 | 内容 |
| --- | --- | --- |
| [configuration.md](configuration.md) | 部署与运维 | 全部配置项、默认值、校验规则、变更影响 |
| [usage.md](usage.md) | 运维与管理员 | 登录、用户中心、绑定、管理后台、命令行与日常运维 |
| [integration.md](integration.md) | 接入方开发 | OIDC 端点、scope、claims、错误码与示例 |

## 组件

| 命令 | 用途 |
| --- | --- |
| `relay` | HTTP 服务，提供 OIDC、账号 API 与内嵌前端 |
| `relayctl` | 命令行管理：创建应用、提升管理员 |
| `casprobe` | 探测上游 CAS 与融合门户，输出用户信息快照 |

## 依赖

| 依赖 | 说明 |
| --- | --- |
| Go | 1.27.3，见 `go.mod` |
| Node.js | 构建前端，Vite 6 |
| PostgreSQL | 主存储，启动时自动迁移表结构 |
| Redis | 会话、授权码、待确认授权、扫码登录状态、通行密钥挑战、限流与验证码标记 |
| 极验 | 可选，第四代行为验证 |

## 快速开始

### 1. 构建前端

```bash
npm --prefix web ci
npm --prefix web run build
```

仓库不包含 `web/dist`。缺少产物时 `build.sh` 会直接退出。

### 2. 准备配置

```bash
cp config.example.yaml config.yaml
```

至少填写 `server.base_url`、`secret_key`、`database.dsn`、`server.email_domains`，并确认 `redis.addr` 可连接。各字段含义见 [configuration.md](configuration.md)。

### 3. 编译

```bash
./build.sh
```

产物位于 `bin/`：`relay`、`relayctl`、`casprobe`。交叉编译示例：

```bash
./build.sh -t linux/amd64 -o dist
```

### 4. 启动

```bash
./bin/relay -config config.yaml
```

首次启动会建表、写入默认 scope，并按 `server.signing_key_path` 生成 RS256 签名密钥。服务收到 SIGINT 或 SIGTERM 后停止接收新请求，最多等待 10 秒完成退出。

### 5. 首次登录与管理员

浏览器打开 `server.base_url`，使用学校账号登录。用户首次登录时自动入库，邮箱按 `server.email_domains` 生成。

提升管理员：

```bash
./bin/relayctl -config config.yaml user promote -id <学号>
```

目标用户必须先登录过一次。刷新用户中心后出现「管理后台」入口。

## 部署要点

- `server.base_url` 必须与外部访问地址完全一致，它同时是 OIDC issuer，会写入 `id_token` 的 `iss` 与 discovery 文档。
- 对外使用 HTTPS。`base_url` 以 `https://` 开头时，会话 cookie 才带 `Secure`。
- `config.yaml` 与 `data/signing_key.pem` 含敏感信息，不要提交版本库。`.gitignore` 已忽略二者。
- 反向代理不在本机时，把代理地址加入 `server.trusted_proxies`，否则限流和验证码会按代理 IP 计数。
- 备份范围：PostgreSQL、`server.signing_key_path`、`secret_key`。更换签名密钥会使所有已签发令牌失效。

## 健康检查

```http
GET /healthz
```

正常时返回 200：

```json
{"status":"ok","database":"ok","redis":"ok"}
```

数据库或 Redis 不可用时返回 503，`status` 为 `degraded`，并标明不可用的依赖。
