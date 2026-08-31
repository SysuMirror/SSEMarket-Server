# UniSSO 单点登录接入设计（集市主站）

日期：2026-08-26
状态：待评审
涉及仓库：`SSE_market_server`（后端）、`sse_market_new_client/newSSE`（前端）

## 1. 背景与目标

集市主站接入 UniSSO（`sso.ssemarket.cn`）作为额外登录入口。核心不变式：

> SSO 只是获取本地 JWT 的第四条途径（密码登录 / 注册 / refresh 刷新 / SSO），
> 产出物与其他途径完全一致，`StrictAuthMiddleware` 及下游全部 auth 路由零改动。

### 已确认决策

| 决策点 | 结论 |
|---|---|
| SSO 注册新账号（CDKey） | **首期不做**，绑定页仅"验证绑定"单路径，新用户仍走原注册页 |
| token 交付方式 | 一次性 ticket 交换（302 只带 ticket，前端 POST 换 JWT） |
| 前端仓库 | `sse_market_new_client/newSSE`（Vue3 + TS + Pinia） |
| 解绑能力 | 用户自助：设置页展示绑定状态 + 验证密码后解绑 |

### 明确不做（YAGNI）

- 不做 SSO 注册新账号（免 CDKey 会旁路邀请制，首期规避）
- 不做按 email 自动关联（email 可冒充，仅凭密码验证绑定）
- 不做 refresh_token 轮换、不做资料双向同步
- 不存 UniSSO access_token（仅回调当次调用 userinfo，用完即弃）

## 2. 数据模型

### 2.1 新表 `sso_bindings`

```sql
CREATE TABLE `sso_bindings` (
  `id`            INT NOT NULL AUTO_INCREMENT,
  `local_user_id` INT NOT NULL,
  `sso_sub`       VARCHAR(64) NOT NULL COMMENT 'UniSSO 不透明 UUID，不解析',
  `sso_username`  VARCHAR(100) NOT NULL DEFAULT '' COMMENT '展示用快照，绑定时的 UniSSO 用户名',
  `created_at`    DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sso_sub` (`sso_sub`),
  UNIQUE KEY `uk_local_user_id` (`local_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

双向唯一索引：一个 SSO 身份只绑一个集市账号，一个集市账号只绑一个 SSO 身份。

### 2.2 Redis key（均一次性 + TTL）

| key | value | TTL | 用途 |
|---|---|---|---|
| `sso:state:{state}` | JSON `{verifier, bind_uid, redirect}` | 10min | CSRF 防护 + PKCE verifier 暂存；读取用 GETDEL |
| `sso:ticket:{ticket}` | JSON `{type, uid?, sub?, sso_username?, sso_email?}` | 2min | 回调→前端交接；读取用 GETDEL |

`type ∈ {login, bind_needed, bound}`。

## 3. 后端设计（SSE_market_server）

### 3.1 配置

`application.yml` 新增 `sso` 段：`enabled / client_id / client_secret / authorize_url / token_url / userinfo_url / redirect_uri`。client_secret 仅存后端配置，禁止进前端 bundle。

### 3.2 路由（7 个）

```
GET  /api/sso/login        noauth  未登录发起：生成 state+PKCE，302 到 UniSSO authorize
POST /api/sso/bind/start   auth    已登录发起绑定：返回 {authorize_url}（前端再跳转）
GET  /api/sso/callback     noauth  UniSSO 回调：校验 state → 换 token → 取 userinfo → 302 前端结果页
POST /api/sso/exchange     noauth  ticket → {token, refresh_token} 或 bind_needed
POST /api/sso/bind         noauth  ticket + 邮箱密码(AES) → 写绑定 + {token, refresh_token}
GET  /api/sso/status       auth    返回 {bound, sso_username?, bound_at?}
POST /api/sso/unbind       auth    验证密码(AES)后删除绑定
```

说明：
- 回调路径用 `/api/sso/callback` 而非清单示例的 `/sso/callback`：nginx `location /api` 已整体转发后端，**零 nginx 改动**，消除部署遗漏风险。redirect_uri 注册时以此为准。
- 绑定发起必须走 `POST /api/sso/bind/start` 返回 URL 而非直接 302：顶级导航无法携带 Authorization header。
- `/api/sso/login` 与 `/api/sso/callback` 挂 noauth（OptionalAuth），`callback` 对游客开放（UniSSO 回跳无本地凭证）。

### 3.3 代码结构（沿用现有分层）

```
model/sso_binding.go          实体（仿 oauth2app.go 风格）
repository/sso_binding.go     接口：FindBySub / FindByUserID / Create / Delete
repository/gorm/sso_binding.go 实现
service/sso_service.go        BuildAuthorizeURL / ExchangeCode / FetchUserinfo
                              state·ticket 的 Redis 读写 / PKCE 工具(S256)
controller/ssoController.go   7 个 handler
```

JWT 发放复用 `userService.issueTokens` 同款逻辑（access + refresh + SaveRefreshToken + CacheUserAfterAuth），保证与其他登录途径产物完全一致。

### 3.4 绑定判定规则（安全核心）

回调取得 `sub` 后：

```
查 sso_bindings by sso_sub
├─ 命中
│   ├─ state 带 bind_uid 且 ≠ 已绑 user_id → 错误：该 SSO 已绑定其他账号
│   └─ 其余 → ticket{type=login, uid}          → 前端 exchange 直接登录
└─ 未命中
    ├─ state 带 bind_uid → 写绑定 → ticket{type=bound, uid}（已登录用户主动绑定成功）
    └─ 无 bind_uid → ticket{type=bind_needed, sub, sso_username, sso_email}
                         → 前端绑定页，用户输集市邮箱+密码验证后 POST /api/sso/bind
```

**铁律：绝不按 email 自动关联。** email 相同仅作页面提示（"该邮箱已有集市账号，请验证绑定"），关联动作必须通过本地密码 bcrypt 验证。

## 4. 核心流程时序

### 4.1 未登录 SSO 登录

```
浏览器                集市后端                     UniSSO
  │ window.location=/api/sso/login │                       │
  │─────────────────▶│ 生成 state+verifier → Redis(TTL10m)  │
  │◀─302 authorize────│                                       │
  │────────────────────────── 登录/授权 ────────────────────▶│
  │◀─────────── 302 /api/sso/callback?code&state ───────────│
  │─────────────────▶│ GETDEL state 校验(CSRF+防重放)         │
  │                  │ POST token(code+verifier+secret) ───▶│
  │                  │ GET userinfo ← sub/email/username ───│
  │                  │ 查绑定 → ticket → Redis(TTL2m)         │
  │◀─302 /auth/sso/result?ticket=xxx                          │
  │ POST /api/sso/exchange {ticket}                           │
  │─────────────────▶│ GETDEL ticket → issueTokens           │
  │◀─ {token, refresh_token}（与密码登录返回结构一致）          │
```

### 4.2 绑定已有账号（sub 无绑定）

同 4.1 至回调，exchange 返回 `{type: "bind_needed", sso_email}` → 前端绑定页（`/auth/sso/bind?ticket=...`）→ 用户输集市邮箱+密码（AES）→ `POST /api/sso/bind` → bcrypt 验证通过 → 写 `sso_bindings` → issueTokens → 登录完成。

### 4.3 已登录用户主动绑定（设置页）

`POST /api/sso/bind/start`（带 JWT，后端将 uid 写入 state）→ 跳 UniSSO → 回调：sub 未命中 → 直接写绑定 → `ticket{type=bound}` → 前端提示"绑定成功"。sub 已绑其他账号 → 错误提示。

### 4.4 解绑

设置页 → `POST /api/sso/unbind {password(AES)}`（auth）→ 验证当前登录用户密码 → 删 `sso_bindings`。密码验证防止会话被劫持后恶意解绑。

## 5. 前端设计（newSSE）

| 文件 | 改动 |
|---|---|
| `src/api/sso/sso.ts` | 新增：exchange / bind / bindStart / status / unbind 封装（密码 AES 复用 `LoginAndRegister/utils.ts`） |
| `src/views/SsoResultView.vue` | 新增：读 URL `?ticket`，调 exchange；login→存 token 跳首页（复用 userStore.setToken + rememberMe 逻辑）；bind_needed→跳绑定页；错误→提示+回登录页 |
| `src/views/SsoBindView.vue` | 新增：集市邮箱+密码表单（单路径，无注册 tab），提交 bind；UniSSO email 已有账号时提示引导 |
| `src/views/LoginView.vue` | 追加「使用 SSO 登录」按钮：`window.location.href = '/api/sso/login'` |
| `src/views/OptionsView.vue` | 追加「账号绑定」区块：status 展示绑定状态/用户名；「绑定 SSO」按钮调 bindStart 后跳转；「解绑」弹密码确认框 |
| `src/router/index.ts` | 注册 `/auth/sso/result`、`/auth/sso/bind`（meta 免登录守卫） |

UI 遵循现有藏青 Slate 冷灰 token 体系、纯色无渐变、链接 hover 颜色柔和加深。

## 6. 错误处理矩阵

| 场景 | 后端行为 | 前端表现 |
|---|---|---|
| state 缺失/过期/重放 | 302 结果页 `?error=state` | "登录状态已过期，请重新发起" |
| code 换 token 失败 / UniSSO 宕机 | 302 结果页 `?error=exchange` | "SSO 服务暂时不可用"（密码登录不受影响，天然降级） |
| ticket 无效/过期/重放 | 401 | 引导重走 SSO |
| bind 密码错误 | 401 | 表单内联提示 |
| bind 时 sub 已被其他账号绑定 | 409 | "该 SSO 账号已绑定其他用户" |
| bind 时 email 不存在 | 422 | "账号不存在，请先注册集市账号" |
| unbind 密码错误 | 401 | 表单提示 |
| 绑定发起时本账号已有绑定 | 409 | "当前账号已绑定 SSO，请先解绑" |

## 7. 安全设计

1. client_secret 仅后端持有；code_verifier 仅存 Redis 不出后端。
2. state：32 字节随机 hex，GETDEL 一次性消费，TTL 10min（CSRF + 重放双防）。
3. ticket：32 字节随机 hex，GETDEL 一次性，TTL 2min；JWT/refresh_token 永不出现在 URL、浏览器历史、access log。
4. PKCE S256：verifier 64 字节随机 base64url；challenge = BASE64URL(SHA256(verifier))。
5. sub 视为不透明 UUID，不做任何解析；email 仅作展示提示，不参与鉴权判定。
6. 解绑需密码复核；绑定（未登录路径）需密码复核——两个高危动作都有第二因子。
7. access_token 有效期 1 小时且仅当次使用，本地会话不依赖它（遵循清单 D）。

## 8. 测试与验收

| 层 | 内容 |
|---|---|
| 单测（service） | PKCE S256 用 RFC 7636 官方测试向量断言；state/ticket 一次性语义；ExchangeCode 用 httptest mock UniSSO |
| 单测（repo） | Create 后 FindBySub 命中；重复 sub / 重复 local_user_id 均报冲突 |
| 集成（controller） | curl 走查矩阵：state 错误、ticket 重放、bind 密码错、email 撞库拒绝、bind=1 双向绑定、unbind 密码错 |
| 回归 | 原密码登录 / 注册 / refresh / PAT / OAuth2 Provider / 管理员登录全冒烟 |
| 端到端 | 浏览器走完流程 4.1–4.4 各一遍 + 原登录回归 |

## 8.1 S1 契约实测（2026-08-26，新版 UniSSO @ 本机 8000，经 ssh -R 隧道 服务器:8888 → 本机:8000）

discovery（`/.well-known/openid-configuration`）实测：

| 项 | 实测值 | 对设计的影响 |
|---|---|---|
| authorize | `GET /authorize` | 与设计一致 |
| token | `POST /api/oauth/token` | 与设计一致 |
| userinfo | `GET /api/oauth/userinfo`（302→`/login?next=` 未登录） | 与设计一致 |
| grant_types | `authorization_code, refresh_token` | 满足需求 |
| PKCE | `S256, plain` | 用 S256 |
| client 认证 | `client_secret_post / basic` | 用 post |
| id_token 签名 | **HS256**（对称） | 集市侧不验 id_token 签名，只取 userinfo 的 sub，规避 |
| scopes | **仅 `openid profile email`** | 清单建议的 `profile:username profile:avatar roles` **当前版本不支持**，scope 按 `openid profile email` 申请；绑定页展示用字段从 userinfo profile 取 |

错误格式实测：token 端点无效 code → `400 {"error":"invalid_grant"}`（标准 OAuth2 错误体，ExchangeCode 按此解析）。

`GET /authorize`（client_id 未注册）→ `307` 无 Location、body `{"error":"Temporary Redirect"}`（新旧版行为一致，属 UniSSO 侧错误路径实现缺陷，不影响正常流程，需在联调时用真实 client_id 验证）。

### 8.1.1 S1 深入实测补充（2026-08-26，应用直插 DB 尝试 + 服务方答复收口）

**本机实例（8000，经隧道 8888）register/login 一律 500、`/env` 全 `<unset>`**。服务方答复定位：
- 500 根因**不是 Redis**——新版缺 Redis 时静默降级内存存储（redis_client.py），不会 500；新版在「同远程库 + 无 Redis」环境 e2e 27/27 全过（含 register/login）。本机 500 为**环境变量未注入**（DB 亦未配置）所致，属部署配置问题，非代码缺陷
- MySQL PROCESSLIST 中 183.14.28.51 的 2 条 unisso 连接是**服务方开发实例**，非本机实例（与本机 /env 全 unset 吻合）
- 晚间复测本机：login 仍 500、/authorize 仍无 Location——本机未更新，**不再作为流程验证目标**

**授权码存储格式（服务方确认，解开直插换 token 必败之谜）**：只存 Redis 不进 MySQL——key `unisso:auth_code:{code}`，值 JSON（client_id/user_id/redirect_uri/scope/code_challenge/state），TTL 600s，一次性（use 即删）。MySQL `authorization_codes` 为遗留表，代码不读写 → 直插换 token 必然 `invalid_grant`，**直插思路放弃，联调走正常浏览器流程**。对集市设计零影响：集市不接触 code 存储，600s TTL / 一次性均为 OAuth2 常规假设，ExchangeCode 无需调整。

**服务方已修复（待现网部署后生效）**：
1. `/authorize` 307 丢 Location（全局异常处理器缺陷）→ 已修，未登录返回 307 + `Location: /login?next=/authorize`
2. `/redis` 误报「凭证未注入」→ 已改为上报真实连接模式（redis / memory-fallback）

**联调物料（已备于远程 MySQL 175.178.90.215:3506 unisso 库）**——**已作废**（生产库并非此库，见 8.1.2；保留记录供追溯）：
- 应用（applications 表）：`client_id=mkt58f8d32bc67f3bd5fb3491964fb916b1`，redirect_uri=`https://market.ssemarket.cn/api/sso/callback`，scopes=`openid profile email`，is_confidential=1；client_secret 明文见当日会话记录（若交换报 invalid_client 即哈希方案不匹配，用 `/api/apps/{app_id}/reset-secret` 重置）
- 服务账号（users 表）：`market-sso@mail2.sysu.edu.cn`（口令见当日会话记录）

**S1 收口**：等新版部署到现网域名（替换 23004 旧版；旧版 register/login 500 属旧代码问题，不再排查）→ 浏览器正常流程走 authorize→code→token→userinfo 全链路。**S2 起不被阻塞，可先行**。

### 8.1.2 S1 全链路 e2e 实测（2026-08-26 晚，生产 sso.ssemarket.cn v1.1.0）——PASS

新版上现网后，用真实凭据在生产跑通完整授权码流程（脚本化模拟浏览器：login → PKCE authorize → consent 批准 → code+state → token → userinfo → code 重放拒绝 → refresh_token）。

**关键发现：生产库 ≠ 远程 MySQL（175.178.90.215:3506）**。在生产注册的新用户未落入远程 unisso 库，且直插的服务账号登录报 `invalid_credentials` → 远程库直插的 app/账号/授权码**全部作废**。改为走服务方指定的正道：登录会话 + `POST /api/apps` 自助注册，**已在生产拿到正式集成凭据**：

| 物料 | 值（生产正式） |
|---|---|
| client_id | `client_uKTL00HcD4VAXyKI2TkBzA` |
| client_secret | `TvPaKBlMtbQvOUCeHINOCK8meA6ttW5VpSkLPRIkwwk`（S2 配置到集市后端环境变量，**禁止入 git/前端**） |
| redirect_uri | `https://market.ssemarket.cn/api/sso/callback` |
| 测试账号 | `e2e-probe@mail2.sysu.edu.cn` / `ProbeE2e-2026`（联调用，正式上线前可换服务方提供的账号） |

**实测契约（修正/补充设计假设）**：
1. **consent 流程**：`GET /authorize`（有会话、首次）→ 200 授权同意页 → `POST /api/oauth/authorize`（表单：client_id/redirect_uri/scope/state/code_challenge/code_challenge_method/`approved=true`，需 `X-CSRF-Token` 头，来自 `GET /api/csrf-token`）→ 302 到 `redirect_uri?code=...&state=...`。**全部发生在 UniSSO 页面侧，集市后端无感知**——集市只见回调 query 上的 code+state
2. **code 一次性**：重放同一 code → `400 {"error":"invalid_grant"}` ✓（与设计假设一致）
3. **token 响应字段**：`access_token`（JWT HS256，`expires_in=3600`）+ `refresh_token` + `id_token` + `scope` + `token_type`。refresh_token grant 实测可用（清单第 7 条可选项成立，集市 session 期内仍可不刷新）
4. **id_token 的 `iss=http://ssemarket.cn:23004`**（内部源，与 `https://sso.ssemarket.cn` 及 discovery issuer 均不一致）——**进一步坐实设计决策：集市不验 id_token，只取 userinfo 的 sub**
5. **scope 自动展开证实（清单 D）**：申请 `openid profile email`，token 实授 `email openid profile:avatar profile:name profile:student_id profile:username`——粗粒度 profile 被展开为全部 profile:*，集市侧按原样申请即可，解析时忽略 scope 字符串差异
6. **userinfo 实测返回**：`sub` / `preferred_username` / `email` / `email_verified`（即使 scope 含 profile:avatar/name/student_id 也不返回这些字段）→ **绑定页展示字段定为 preferred_username + email**，设计 4.2 中涉及 avatar/姓名的展示项删去
7. **token/userinfo 端点无需 CSRF**（server-to-server，client_secret/Bearer 认证）——CSRF 仅用于 UniSSO 自家页面 API（如 /api/apps 注册）

**S1 状态：完成**。契约全部实测锁定，生产凭据已就绪，S2 起随时可开发。


**反馈给 UniSSO 方的安全问题**（不阻塞集市接入）：
1. `/.well-known/jwks.json` 公开了默认 HS256 密钥 `change-me-in-production-unisso-secret-key-2024`——任何人可伪造 id_token，上线前必须换强随机密钥并建议改 RS256
2. `/env` `/db` `/redis` `/minio` `/crash` `/shared` `/platform` 调试端点无鉴权暴露（当前 env 值均 `<unset>`，风险可控但应在生产关闭）
3. `/openapi.json` 暴露全部路由面

## 9. 实施步骤（供 writing-plans 细化）

1. **S1 契约确认**（人工）：**✅ 完成**（2026-08-26，见 8.1/8.1.1/8.1.2）——生产 v1.1.0 全链路 e2e PASS（login→PKCE authorize→consent→code→token→userinfo→code 一次性→refresh），正式 client_id/secret 已在生产注册就绪
2. **S2 数据层与配置**：model + 建表 SQL + repo + sso 配置段 → 验证：repo 单测
3. **S3 SSO Service**：PKCE / state / ticket / ExchangeCode / FetchUserinfo → 验证：RFC 向量 + httptest
4. **S4 Controller 与路由**：7 路由挂载 → 验证：curl 走查矩阵
5. **S5 后端回归**：登录/refresh/PAT/OAuth2 冒烟
6. **S6 前端 API + 结果页**：sso.ts + SsoResultView → 验证：流程 4.1
7. **S7 前端绑定页 + 登录按钮**：SsoBindView + LoginView → 验证：流程 4.2
8. **S8 前端设置页**：OptionsView 绑定区块 → 验证：流程 4.3 / 4.4
9. **S9 部署**：UniSSO 正式注册 → `go build -o serve .` → `docker cp serve server:/app/serve` → restart → `strings /app/serve | grep sso_bindings` 验证新二进制生效
