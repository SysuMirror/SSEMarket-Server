# SSO 上线清单（Task 11）

> 适用范围：`ssemarket.cn`（前端 `https://ssemarket.cn/new/`，后端 `https://ssemarket.cn/api/*`）接入 UniSSO（`sso.ssemarket.cn`）。
> 执行顺序：① 生产库建表 → ② 注入环境变量 → ③ 后端镜像构建部署 → ④ 前端构建部署 → ⑤ 生产 e2e → ⑥ 回归抽查。

---

## 1. 生产库建表 SQL

在生产 MySQL（`ssemarket` 库，**生产实例 ≠ 175.178.90.215:3506 远程调试库**）执行：

```sql
USE ssemarket;

CREATE TABLE IF NOT EXISTS `sso_bindings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `local_user_id` INT NOT NULL,
  `sso_sub` VARCHAR(64) NOT NULL,
  `sso_username` VARCHAR(100) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_sso_sub` (`sso_sub`),
  UNIQUE KEY `uniq_local_user_id` (`local_user_id`),
  KEY `idx_sso_bindings_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**字段说明**：
- `sso_sub`：UniSSO userinfo 返回的 sub（不透明 UUID，64 字符足够）。
- `local_user_id`：集市 `users.id`。
- 双向唯一索引：确保一个 SSO 身份只能绑定一个集市账号，一个集市账号只能绑定一个 SSO 身份。
- `sso_username`：UniSSO userinfo 返回的 username（冗余字段，便于在设置页直接展示，不必再调 UniSSO）。

**执行方式**：在服务器上 `docker exec -it <sse_market_db_容器名> mysql -uroot -p ssemarket`，粘上述 SQL。

**回滚 SQL**：`DROP TABLE IF EXISTS sso_bindings;`

---

## 2. 后端环境变量（注入到生产容器/compose）

```
SSO_ENABLED=true
SSO_CLIENT_ID=client_uKTL00HcD4VAXyKI2TkBzA
SSO_CLIENT_SECRET=TvPaKBlMtbQvOUCeHINOCK8meA6ttW5VpSkLPRIkwwk
# 其余（URL、redirect_uri、scope、agent）走 application.yml 默认值即可。
# application.yml 默认值见下方对照表。
```

**application.yml 默认值对照**（仅当生产 URL 与默认不同时才需注入覆盖）：

| 变量名 | 默认值 | 是否需覆盖 |
| --- | --- | --- |
| `SSO_ISSUER_URL` | `https://sso.ssemarket.cn` | 否 |
| `SSO_REDIRECT_URI` | `https://ssemarket.cn/api/sso/callback` | 否 |
| `SSO_SCOPE` | `openid profile email` | 否 |
| `SSO_AUTHORIZE_PATH` | `/authorize` | 否 |
| `SSO_TOKEN_PATH` | `/api/oauth/token` | 否 |
| `SSO_USERINFO_PATH` | `/api/oauth/userinfo` | 否 |
| `SSO_STATE_TTL_SEC` | `600`（10 分钟） | 否 |
| `SSO_TICKET_TTL_SEC` | `120`（2 分钟） | 否 |
| `SSO_BIND_TIMEOUT_SEC` | `600` | 否 |
| `SSO_HTTP_AGENT` | （空，使用默认 Go client） | 否 |

> **安全约束**：`SSO_CLIENT_SECRET` 仅注入到后端容器环境变量，禁止入 git、禁止入前端 bundle、禁止打印日志。生产建议用 docker secret 或 .env 文件 + `chmod 600`，不写进 compose.yml 明文。

---

## 3. 前端构建

```bash
cd sse_market_new_client/newSSE
VITE_API_BASE_URL=/api npm run build
# 产物 dist/ 部署到前端静态目录
```

前端无运行时环境变量；`/api` 由 nginx 反代到后端容器。

---

## 4. 生产 e2e 验证（浏览器无痕窗口）

**场景 A — 新用户绑定登录**：
1. 无痕窗口打开 `https://ssemarket.cn/new/auth/login`。
2. 点击「使用 UniSSO 登录」按钮。
3. 跳转到 UniSSO 登录页，输入 UniSSO 账号密码（同意授权）。
4. 回跳到 `https://ssemarket.cn/new/auth/sso/result?ticket=...`。
5. 因无绑定记录，前端自动跳到 `/auth/sso/bind`。
6. 输入已注册的集市账号邮箱密码，点「验证并绑定」。
7. 期望：跳回首页已登录状态（`setToken/setUserInfo` 完成）。
8. 刷新页面，仍保持登录。

**场景 B — 已绑定用户直接登录**：
1. 退出登录。
2. 再次点「使用 UniSSO 登录」，走完 UniSSO 授权。
3. 期望：直接跳回首页已登录（ticket 走 login 分支，直接发令牌）。

**场景 C — 已登录用户在设置页绑定/解绑**：
1. 用纯邮箱密码登录的账号访问「设置 → SSO 账号绑定」。
2. 期望：展示「未绑定」状态。
3. 点「绑定 UniSSO」按钮 → 跳转 UniSSO 授权 → 回跳 `/auth/sso/result`。
4. 期望：自动跳回首页，重新进设置页展示「已绑定」。
5. 输入登录密码 → 点「解绑 SSO」→ 期望：回到「未绑定」状态。

**场景 D — conflict 验证**：
1. 用第二个集市账号登录，访问设置页，点「绑定 UniSSO」。
2. 在 UniSSO 用**场景 A/C 已绑定的同一个 SSO 账号**完成授权。
3. 期望：回跳 `/auth/sso/result?error=conflict`，前端报「该 SSO 账号已绑定其他集市账号」。

---

## 5. 回归抽查（防止影响原有系统）

- 邮箱密码登录：`POST /api/user/login` 正常发 token + refresh。
- 登出：`POST /api/user/logout` 清掉 refresh_token。
- refresh：`POST /api/user/refreshToken` 用 refresh_token 换新 access + refresh。
- PAT：`/api/pat/list`、`/api/pat/create` 等接口正常。
- 帖子/评论/消息等业务接口正常（带 JWT 调用）。

---

## 6. 回滚预案

- 后端：切回上一版镜像（保持 `SSO_ENABLED=true` 也无影响，未触发 SSO 入口即可）。
- 前端：切回上一版 dist。
- 数据库：`DROP TABLE IF EXISTS sso_bindings;`（仅在确认无已绑定用户时；若有已绑定用户，建议保留表，仅关闭 SSO_ENABLED）。
- 关闭 SSO 入口：把 `SSO_ENABLED` 改回 `false` 或注释掉，重启后端。`/api/sso/login` 会回 503，前端按钮可保留但不影响登录主流程。

---

## 7. 预先准备（可在本次开发本机完成）

- 本地 `compose.yml` 启动一次带 `SSO_ENABLED=true` 的服务，curl 走查 `/api/sso/login` 应 302 到 `https://sso.ssemarket.cn/authorize?...`。
- 把生产建表 SQL 写入 `SSE_market_server/scripts/sso_create_table.sql`，方便服务器上直接执行。
- 把环境变量清单写入本文件，作为运维注入依据。

---

## 8. 检查清单（部署前对照）

- [ ] 后端 `go build .` 通过
- [ ] 后端 `go test ./service/ ./controller/ ./repository/gorm/ ./config/ ./util/` 全绿
- [ ] 前端 `npm run build` 通过
- [ ] 本地 docker compose 带 `SSO_ENABLED=true` 启动，`/api/sso/login` 302 到 authorize
- [ ] 生产库建表 SQL 已写入 `SSE_market_server/scripts/sso_create_table.sql`
- [ ] 生产环境变量清单已准备好（见第 2 节）
- [ ] 上一版后端镜像、前端 dist 已备份（用于回滚）
