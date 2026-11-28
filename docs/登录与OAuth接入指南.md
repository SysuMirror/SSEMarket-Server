# SSE Market 登录与 OAuth 接入指南

> 适用站点：`https://ssemarket.cn`（前端在 `/new/`，后端 API 在 `/api/`）
>
> 适用读者：需要接入集市账号体系的其他平台开发者 / 编码 Agent
>
> 文档基线：2026-10-25 的 `SSE_market_server` 与 `sse_market_new_client/newSSE` 代码。
> 文中的路径、字段、状态码均以这两个仓库的实现为准；改代码后请同步更新本文。

**目录**

- [0. 先看结论：你应该选哪条接入路径](#0-先看结论你应该选哪条接入路径)
- [1. 公共约定](#1-公共约定)
- [2. 方案 A：OAuth2 接入（第三方平台推荐）](#2-方案-aoauth2-接入第三方平台推荐)
- [3. 方案 B：直接使用集市账号密码登录（仅限可信第一方 / 内部工具）](#3-方案-b直接使用集市账号密码登录仅限可信第一方--内部工具)
- [4. 方案 C：集市内置的 UniSSO 单点登录](#4-方案-c集市内置的-unisso-单点登录)
- [5. 方案 D：PAT（个人访问令牌，供智能体 / MCP / 服务端程序）](#5-方案-dpat个人访问令牌供智能体--mcp--服务端程序)
- [6. 数据库 DDL 汇总](#6-数据库-ddl-汇总)
- [7. 接入验收清单](#7-接入验收清单)
- [8. 常见坑位与 FAQ](#8-常见坑位与-faq)
- [9. 参考实现（集市内已有接入）](#9-参考实现集市内已有接入)
- [10. 一页速查](#10-一页速查)

---

## 0. 先看结论：你应该选哪条接入路径

| 你的目标 | 用哪个方案 | 入口 |
|---|---|---|
| **第三方平台用「集市账号」登录自己的用户** | **方案 A：OAuth2（集市作为 IdP）** | `https://ssemarket.cn/new/connect` |
| 自己的服务需要直接验证集市用户的邮箱/密码 | 方案 B：账号密码登录 API | `POST /api/auth/login` |
| 集市本站用户想用 UniSSO（`sso.ssemarket.cn`）登录集市 | 方案 C：集市内置 SSO | `GET /api/sso/login` |
| 智能体 / MCP / 服务端程序以某个集市用户身份调用集市 API | 方案 D：PAT（个人访问令牌） | `POST /api/auth/token/verify` |

**最重要的三条结论：**

1. **第三方平台接「用集市账号登录」请只走 OAuth2（方案 A）。** 不要把用户的集市密码存到自己库里，也不要把集市返回的 `token`（7 天用户 JWT）长期保存或当成本平台会话。
2. **OAuth2 的 `access_token` 只有 5 分钟，没有 refresh token，也没有 OIDC `id_token`。** 它只用于「服务端立刻换一次用户信息」。换到用户信息后，请在自己的平台落用户、发自己的会话。
3. **OAuth2 不会替用户注册集市账号。** 用户在授权页发现未登录时，会被引导到集市登录/注册页；没有集市账号的人需要先完成集市注册（需要邮箱验证码 + CDKey）。

### 0.1 站点当前有哪几种「登录方式」

| 方式 | 用户入口 | 说明 |
|---|---|---|
| 邮箱 + 密码 | `https://ssemarket.cn/new/auth/login` | 主登录方式；密码由前端 AES 加密后提交 |
| UniSSO 单点登录 | 登录页「使用 UniSSO 登录」按钮 | 后端 `GET /api/sso/login`；**2026-10-25 生产实测为 503，需运维开启** |
| 第三方 OAuth2 | 不从集市登录页进入 | 是「别的平台用集市账号登录」的出口，不是集市用户登录入口 |
| PAT | 设置页创建 | 给智能体/脚本用的长期令牌，不是交互式登录 |

### 0.2 站点当前有哪几种「注册 / 建号」方式

| 方式 | 接口 | 说明 |
|---|---|---|
| 标准用户注册 | `POST /api/auth/validateEmail` + `POST /api/auth/register` | 邮箱验证码 + CDKey；普通用户/第三方测试用，详见 3.5 |
| OAuth 应用注册 | `POST /api/auth/admin/oauth/apps` | 注册的是「接入方应用」，不是用户，详见 2.2 |
| 平台中转建号 | `POST /api/open/users` | 开发者平台 SM2 验签；免验证码/CDKey，需平台开通，详见 3.5.11 |
| Mirror 内部建号 | `POST /api/sso/mirror/provision` | 仅 Mirror 服务内部使用，第三方不要用，详见 3.5.12 |

**域名与路径固定值：**

| 名称 | 值 |
|---|---|
| 前端站点根 | `https://ssemarket.cn/new/` |
| OAuth 授权（同意）页 | `https://ssemarket.cn/new/connect` |
| 后端 API 根 | `https://ssemarket.cn/api` |
| 集市 SSO 回调（注册到 UniSSO 的值） | `https://ssemarket.cn/api/sso/callback` |
| SSO 结果页（集市前端内部用） | `https://ssemarket.cn/new/auth/sso/result` |

> 如果集市换域名或部署到测试环境，把上表中的 `https://ssemarket.cn` 整体替换即可；
> 但 OAuth `redirect_uri` 必须与 `oauth2_apps.redirect_uri` **逐字符一致**。

---

## 1. 公共约定

### 1.1 响应包格式

绝大多数业务接口返回：

```json
{
  "code": 200,
  "data": { },
  "msg": "成功"
}
```

- `code=200` 表示业务成功；业务失败时 `code` 可能是 400/401/404/409/422，同时 **HTTP 状态码也可能同步为 4xx**。
- 少数中间件直接返回：`{"code":401,"msg":"权限不足"}`（没有 `data` 字段）。
- 登录限流直接返回 HTTP 429 + `{"message":"操作过于频繁，请稍后再试"}`。
- 管理端部分接口使用 `response.Fail`：**HTTP 200，但 body 的 `code=400`**。调用管理端时请判断 body 的 `code`，不要只看 HTTP 状态。
- 后端统一返回 `Access-Control-Allow-Origin: *`。不要因为「浏览器能调」就把含 `app_secret` 的 token 交换放进前端代码。

### 1.2 用户身份请求头

除明确标注 noauth 的接口外，用户态接口都需要：

```http
Authorization: Bearer <用户的集市 access token>
Content-Type: application/json
```

WebSocket 例外：`StrictAuthMiddleware` 也接受 `?token=<token>` 查询参数（浏览器 WebSocket 不能自定义 Header）。

### 1.3 编码与大小写

- 邮箱建议统一 `trim + lowercase` 后再提交；集市注册/登录按数据库排序规则（`utf8mb4_0900_ai_ci`）大小写不敏感，但不要依赖这一点。
- 所有 JSON 字段名区分大小写，按下文字段原样发送。
- 时间字段为 MySQL `datetime`，序列化后通常是 ISO/RFC3339 字符串。

---

## 2. 方案 A：OAuth2 接入（第三方平台推荐）

### 2.1 能力边界（先确认不踩坑）

| 项 | 当前能力 |
|---|---|
| 授权模式 | `authorization_code`（无 PKCE、无 implicit） |
| 客户端类型 | **机密客户端**：必须有 `app_secret`，且只能放在服务端 |
| scope | **只支持 `read_basic`**，传其他值会被拒绝 |
| 返回用户字段 | `user_id`、`name`、`email`、`avatar_url`、`intro`、`identity` |
| access_token 有效期 | **300 秒（5 分钟）** |
| refresh_token | **不支持**（`oauth2_tokens.refresh_token` 永远为空） |
| id_token / OIDC | **不支持** |
| 用户注册 | **OAuth 不负责注册**；用户必须事先有集市账号 |
| 授权码有效期 | 5 分钟，一次性使用 |
| 令牌撤销 | 没有单 token 撤销接口；停用整个应用（`is_active=0`）或删除用户后，旧 token 会在下次校验时失效 |

### 2.2 第一步：注册 OAuth 应用

集市**没有对外开放的应用自助注册接口**：`/api/auth/admin/oauth/apps` 属于管理员路由。第三方平台自己没有权限直接调用，**推荐让集市管理员**在管理端创建应用，或由有权限的运维直接写 `oauth2_apps`。

#### 方式 1：管理端 API（推荐）

先获取管理员 token：

```http
POST /api/auth/adminLogin
Content-Type: application/json

{"account":"<管理员账号>","password":"<管理员密码>"}
```

返回：

```json
{"code":200,"data":{"token":"<admin-jwt>"},"msg":"登录成功"}
```

然后创建应用：

```http
POST /api/auth/admin/oauth/apps
Authorization: Bearer <admin-jwt>
Content-Type: application/json

{
  "app_id": "your-app-prod",
  "app_name": "你的应用名",
  "redirect_uri": "https://your-app.example.com/auth/market/callback",
  "app_icon": "https://your-app.example.com/icon.png",
  "description": "接入说明（可选）"
}
```

- `app_id` 可省略；省略时服务端生成 `sse-` + 16 位十六进制随机串。
- `app_secret` 由服务端生成，**只在创建响应里明文返回一次**，请立刻写入密钥管理系统。
- 创建成功响应：

```json
{
  "code": 200,
  "data": {
    "app_id": "your-app-prod",
    "app_name": "你的应用名",
    "app_secret": "一次性明文密钥",
    "redirect_uri": "https://your-app.example.com/auth/market/callback",
    "description": "接入说明",
    "is_active": true
  },
  "msg": "已写入 oauth2_apps，请立刻保存 app_secret（只显示一次）"
}
```

更新应用（改名、换回调、停用、重置密钥）：

```http
PUT /api/auth/admin/oauth/apps/:appId
Authorization: Bearer <admin-jwt>
Content-Type: application/json

{
  "app_name": "新名字",
  "redirect_uri": "https://your-app.example.com/auth/market/callback",
  "description": "新说明",
  "app_icon": "https://...",
  "is_active": true,
  "regenerate_secret": true
}
```

- `regenerate_secret=true` 时响应会多返回一个新的 `app_secret`（同样只显示一次）。
- 管理端列表 `GET /api/auth/admin/oauth/apps` 只返回打码后的 secret。
- `DELETE /api/auth/admin/oauth/apps/:appId` 会**删除该 `app_id` 下的所有行**（多回调场景注意）。

#### 方式 2：直接写数据库（仅限有库权限的运维）

```sql
INSERT INTO oauth2_apps
  (app_id, app_name, app_secret, app_icon,
   redirect_uri, description, is_active, created_at, updated_at)
VALUES
  ('your-app-prod',
   '你的应用名',
   '<32位随机十六进制密钥>',
   'https://your-app.example.com/icon.png',
   'https://your-app.example.com/auth/market/callback',
   '接入说明',
   1, NOW(), NOW());
```

生成密钥：`openssl rand -hex 16`（得到 32 个十六进制字符，与集市管理端生成规则一致）。

> **手工插入中文时务必指定 utf8mb4！**
> 容器里的 `mysql` 客户端在未指定字符集时可能默认 `latin1`，会把 UTF-8 中文写成
> `UniSSO ç»Ÿä¸€èº«ä»½è®¤è¯` 这类「二次编码」乱码。执行 SQL 时请使用：
>
> ```bash
> docker exec -i -e MYSQL_PWD=... sse_market_db \
>   mysql -uroot ssemarket --default-character-set=utf8mb4 < insert.sql
> ```
>
> 或在 SQL 开头加 `SET NAMES utf8mb4;`。管理端 API 与 Go 程序写库不受此影响。
> 已经写坏的排查/修复见 6.11。

**重要说明：**

- `app_secret` 在数据库中是**明文**，不要在日志、前端 bundle、URL 中出现。
- 唯一键是 `(app_id, redirect_uri)`：同一个 `app_id` 可以有多条不同回调地址的记录。生产/测试回调建议用不同 `app_id`，避免误用。
- 库里还有 `alt_redirect_uri`、`alt_app_secret` 两列，但**当前代码完全不读取它们**，不要使用。
- `is_active=0` 会让授权页与 token 交换同时失效（`FindActiveOAuth2App*` 都带 `is_active=1`）。

### 2.3 浏览器授权流程（全链路）

```
你的平台浏览器                   集市后端 / 前端                         用户
    │                                  │                                  │
    │ ① 生成 state，存服务端会话        │                                  │
    │ ② 302 → /new/connect?...         │                                  │
    │─────────────────────────────────▶│                                  │
    │                                  │ ③ 未登录 → 集市登录/注册页        │
    │                                  │ ④ 登录后回到 /new/connect         │
    │                                  │ ⑤ 展示应用名/图标，用户点「允许」 │
    │                                  │ ⑥ POST /api/auth/connect/authorize│
    │                                  │    写 oauth2_codes + 授权记录      │
    │                                  │ ⑦ window.location = redirect_url  │
    │◀──── ⑧ 浏览器带着 code+state 回到你的 redirect_uri ─────────────────│
    │                                                                     │
    │ ⑨ 服务端 POST /api/auth/connect/token（code + app_id + app_secret）  │
    │ ⑩ 服务端 POST /api/auth/connect/userinfo（Bearer access_token）      │
    │ ⑪ 按 user_id upsert 本地用户，创建你自己的会话                      │
```

#### ① 生成 state

- 16~48 字节随机数，推荐 48 位十六进制（如 `crypto.randomBytes(24).toString('hex')`）。
- 存在你平台的服务端会话 / 加密 Cookie 中，设置 10 分钟过期。
- **必须一次性消费**：回调验证成功后立刻删除。
- 不要只用 URL 里的 `state` 做校验（攻击者可以自己造 state）。

#### ② 跳转授权页

```
https://ssemarket.cn/new/connect
  ?appid=<APP_ID>
  &redirect_uri=<URL-encoded REDIRECT_URI>
  &response_type=code
  &scope=read_basic
  &state=<STATE>
```

注意：

- 授权页读取的查询参数名是 **`appid`**（不是 `app_id`）。后端授权接口内部字段才叫 `app_id`。
- `redirect_uri` 必须与数据库中的记录完全一致。
- 授权页要求用户已登录集市；未登录会自动跳集市登录页，登录后回到授权页。
- scope 只接受 `read_basic`，其他值页面会直接显示「暂不支持」。

#### ③ 集市内部的同意与授权

授权页由集市前端 `ConnectView` 处理，它会在用户点击「允许」后调用：

```http
POST /api/auth/connect/authorize
Authorization: Bearer <当前集市用户 JWT>
Content-Type: application/json

{
  "app_id": "your-app-prod",
  "redirect_uri": "https://your-app.example.com/auth/market/callback",
  "response_type": "code",
  "scope": "read_basic",
  "state": "<原样带回>"
}
```

服务端行为：

1. 校验 `response_type == "code"`；
2. 校验 `scope == "read_basic"`；
3. 按 `(app_id, redirect_uri, is_active=1)` 查应用；
4. 生成 32 位十六进制授权码（5 分钟过期）；
5. 写 `oauth2_codes` 与 `oauth2_auth_records`；
6. 返回：

服务端写入的 SQL（等价语句）：

```sql
INSERT INTO oauth2_codes
  (code, app_id, user_id, scope, redirect_uri, state,
   expires_at, is_used, created_at)
VALUES
  (?, ?, ?, 'read_basic', ?, ?, NOW() + INTERVAL 5 MINUTE, 0, NOW());

INSERT INTO oauth2_auth_records
  (user_id, app_id, app_name, scope, ip_address, user_agent,
   status, authorized_at, expires_at, created_at, updated_at)
VALUES
  (?, ?, ?, 'read_basic', ?, ?, 'authorized',
   NOW(), NOW() + INTERVAL 5 MINUTE, NOW(), NOW());
```

返回：

```json
{
  "code": 200,
  "data": {
    "redirect_url": "https://your-app.example.com/auth/market/callback?code=<CODE>&state=<STATE>"
  },
  "msg": "授权成功"
}
```

前端随后执行 `window.location.href = redirect_url`，浏览器就回到你的回调地址。

> **重要限制**：`redirect_url` 是字符串直接拼接 `"<redirect_uri>?code=...&state=..."`。
> 因此你的 `redirect_uri` **不要带 `?` 查询串或 `#` 片段**；需要额外参数请放在 `state` 里自行传递。
> `state` 也建议只用十六进制/字母数字，避免特殊字符拼接问题。

#### ④ 你的回调地址收到 code

```
GET https://your-app.example.com/auth/market/callback?code=<CODE>&state=<STATE>
```

你要做的：

1. 检查 `state` 与会话中保存的值是否一致（建议常量时间比较）；
2. 立即从会话中删除 state（防重放）；
3. 如果 `state` 不匹配，直接拒绝，不要兑换 code；
4. 在服务端用 code 换 token（下一步）。

#### ⑤ 服务端换 access_token

```http
POST https://ssemarket.cn/api/auth/connect/token
Content-Type: application/x-www-form-urlencoded

code=<CODE>&app_id=<APP_ID>&app_secret=<APP_SECRET>
```

- 参数名严格为 `code`、`app_id`、`app_secret`（不是 `appid`）。
- 必须服务端调用；`app_secret` 不得进浏览器。
- 成功响应：

```json
{
  "code": 200,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 300,
    "scope": "read_basic"
  },
  "msg": "获取访问令牌成功"
}
```

服务端此时执行的数据库操作：

```sql
-- 1. 校验应用密钥
SELECT * FROM oauth2_apps
WHERE app_id = ? AND app_secret = ? AND is_active = 1
LIMIT 1;

-- 2. 取未使用的授权码
SELECT * FROM oauth2_codes
WHERE code = ? AND app_id = ? AND is_used = 0
LIMIT 1;

-- 3. 标记授权码已使用（先标记，再发令牌）
UPDATE oauth2_codes SET is_used = 1 WHERE code_id = ?;

-- 4. 记录令牌（审计用；中间件校验时不查此表）
INSERT INTO oauth2_tokens
  (access_token, refresh_token, app_id, user_id, scope,
   expires_at, is_active, created_at, updated_at)
VALUES
  (?, '', ?, ?, 'read_basic', NOW() + INTERVAL 5 MINUTE, 1, NOW(), NOW());
```

失败时的行为：

| 场景 | HTTP | body |
|---|---|---|
| 缺 `code`/`app_id`/`app_secret` | 400 | `{"code":400,"data":null,"msg":"缺少必要参数"}` |
| code 过期（参数齐全） | 400 | `{"code":400,"data":null,"msg":"授权码已过期"}` |
| code 不存在 / 已使用 | 404 | `{"code":404,"data":null,"msg":"授权码不存在或已使用"}` |
| `app_secret` 错误 / 应用停用 | 401 | `{"code":401,"data":null,"msg":"应用密钥错误"}` |

> 兑换是一次性的：即使你的服务端在收到 token 后崩溃，也不能用同一个 code 重试。
> 网络超时后重试通常得到 404，正确做法是让用户重新发起授权。
> 也不要并发兑换同一个 code：代码先查后改，不是原子占位。

#### ⑥ 服务端取用户信息

```http
POST https://ssemarket.cn/api/auth/connect/userinfo
Authorization: Bearer <access_token>
Content-Type: application/json
```

- `POST`，没有请求体。
- 成功响应：

```json
{
  "code": 200,
  "data": {
    "user_id": 12345,
    "name": "用户名",
    "email": "user@example.com",
    "avatar_url": "https://...",
    "intro": "个人简介",
    "identity": "student"
  },
  "msg": "获取用户信息成功"
}
```

- `user_id` 是集市内部自增用户 ID，**这是唯一稳定身份键**。
- `email` 只用于展示/联系，不要用邮箱做账号关联主键（邮箱可能变更，且集市明确禁止按邮箱自动关联）。
- `identity` 当前可能是 `student` / `teacher`（数据库也允许 `organization`），请按字符串透传，不要写死枚举。
- 失败：
  - 401：token 缺失/无效/过期/用户不存在/应用停用；
  - 403：token 的 scope 不是 `read_basic`（正常流程不会出现）。

#### ⑦ 在你自己平台落用户与会话

推荐逻辑：

1. 用 `market_user_id = String(user_id)` 查本地映射表；
2. 存在则更新展示字段（name/email/avatar_url/identity），并更新 `last_login_at`；
3. 不存在则插入新用户；
4. 写入你自己的 session（或签发你自己的 JWT/Cookie）；
5. 丢弃集市 `access_token`，不要持久化；
6. 跳回业务页面。

### 2.4 完整代码示例（Node.js / Express）

```js
// npm i express express-session
import express from 'express'
import session from 'express-session'
import crypto from 'node:crypto'

const MARKET_WEB = process.env.MARKET_WEB_BASE || 'https://ssemarket.cn/new'
const MARKET_API = process.env.MARKET_API_BASE || 'https://ssemarket.cn/api'
const APP_ID = process.env.MARKET_OAUTH_APP_ID
const APP_SECRET = process.env.MARKET_OAUTH_APP_SECRET
const REDIRECT_URI = process.env.MARKET_OAUTH_REDIRECT_URI
  || 'https://your-app.example.com/auth/market/callback'

const app = express()
app.use(session({
  secret: process.env.SESSION_SECRET,
  resave: false,
  saveUninitialized: false,
  cookie: { httpOnly: true, sameSite: 'lax', maxAge: 10 * 60 * 1000 },
}))

// 1) 发起登录
app.get('/auth/market', (req, res) => {
  const state = crypto.randomBytes(24).toString('hex')
  req.session.marketOAuthState = state

  const params = new URLSearchParams({
    appid: APP_ID,
    redirect_uri: REDIRECT_URI,
    response_type: 'code',
    scope: 'read_basic',
    state,
  })
  res.redirect(`${MARKET_WEB}/connect?${params.toString()}`)
})

// 2) 回调
app.get('/auth/market/callback', async (req, res) => {
  const { code, state } = req.query
  const expected = req.session.marketOAuthState
  delete req.session.marketOAuthState

  if (!code || !state || !expected
      || Buffer.byteLength(String(state)) !== Buffer.byteLength(expected)
      || !crypto.timingSafeEqual(Buffer.from(String(state)), Buffer.from(expected))) {
    return res.status(400).send('state 校验失败，请重新登录')
  }

  // 3) code 换 token（服务端）
  const tokenRes = await fetch(`${MARKET_API}/auth/connect/token`, {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({
      code: String(code),
      app_id: APP_ID,
      app_secret: APP_SECRET,
    }),
  })
  if (!tokenRes.ok) {
    console.error('market token exchange failed', tokenRes.status, await tokenRes.text())
    return res.status(502).send('集市登录失败，请重试')
  }
  const tokenBody = await tokenRes.json()
  const accessToken = tokenBody?.data?.access_token
  if (!accessToken) return res.status(502).send('集市未返回 access_token')

  // 4) 换用户信息（服务端；5 分钟内完成）
  const infoRes = await fetch(`${MARKET_API}/auth/connect/userinfo`, {
    method: 'POST',
    headers: { authorization: `Bearer ${accessToken}` },
  })
  if (!infoRes.ok) {
    console.error('market userinfo failed', infoRes.status, await infoRes.text())
    return res.status(502).send('获取集市用户信息失败')
  }
  const info = (await infoRes.json())?.data
  if (!info?.user_id) return res.status(502).send('集市用户信息不完整')

  // 5) 你的业务库：按 user_id upsert（示例函数）
  const user = await upsertMarketUser({
    marketUserId: String(info.user_id),
    name: info.name || '',
    email: info.email || '',
    avatarUrl: info.avatar_url || '',
    identity: info.identity || '',
  })

  // 6) 发你自己的会话，不要保存 accessToken
  req.session.userId = user.id
  res.redirect('/')
})
```

`upsertMarketUser` 对应的 MySQL：

```sql
INSERT INTO market_users
  (market_user_id, name, email, avatar_url, identity,
   first_login_at, last_login_at)
VALUES
  (?, ?, ?, ?, ?, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  email = VALUES(email),
  avatar_url = VALUES(avatar_url),
  identity = VALUES(identity),
  last_login_at = NOW();
```

> 如果你的 MySQL 版本较新，可用 `INSERT ... AS new ON DUPLICATE KEY UPDATE name = new.name` 写法。
> 并发首次登录时靠 `UNIQUE KEY(market_user_id)` 保证只建一条；遇到唯一键冲突请回读再更新。

### 2.5 完整代码示例（Python / requests）

```python
import secrets
import requests
from flask import Flask, session, redirect, request, abort

MARKET_WEB = "https://ssemarket.cn/new"
MARKET_API = "https://ssemarket.cn/api"
APP_ID = "your-app-prod"
APP_SECRET = "your-app-secret"
REDIRECT_URI = "https://your-app.example.com/auth/market/callback"

app = Flask(__name__)
app.secret_key = "your-session-secret"


@app.get("/auth/market")
def market_login():
    state = secrets.token_hex(24)
    session["market_oauth_state"] = state
    params = {
        "appid": APP_ID,
        "redirect_uri": REDIRECT_URI,
        "response_type": "code",
        "scope": "read_basic",
        "state": state,
    }
    from urllib.parse import urlencode
    return redirect(f"{MARKET_WEB}/connect?{urlencode(params)}")


@app.get("/auth/market/callback")
def market_callback():
    code = request.args.get("code", "")
    state = request.args.get("state", "")
    expected = session.pop("market_oauth_state", "")
    if not code or not expected or not secrets.compare_digest(state, expected):
        abort(400, "state 校验失败")

    r = requests.post(
        f"{MARKET_API}/auth/connect/token",
        data={"code": code, "app_id": APP_ID, "app_secret": APP_SECRET},
        timeout=10,
    )
    if r.status_code != 200:
        abort(502, "集市 token 交换失败")
    access_token = (r.json().get("data") or {}).get("access_token")
    if not access_token:
        abort(502, "集市未返回 access_token")

    r = requests.post(
        f"{MARKET_API}/auth/connect/userinfo",
        headers={"Authorization": f"Bearer {access_token}"},
        timeout=10,
    )
    if r.status_code != 200:
        abort(502, "集市用户信息获取失败")
    info = r.json()["data"]

    user = upsert_market_user(  # 你自己的业务函数
        market_user_id=str(info["user_id"]),
        name=info.get("name", ""),
        email=info.get("email", ""),
        avatar_url=info.get("avatar_url", ""),
        identity=info.get("identity", ""),
    )
    session["user_id"] = user.id
    return redirect("/")
```

### 2.6 你自己平台的数据库操作

#### 2.6.1 用户映射表（必需）

```sql
CREATE TABLE `market_users` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `market_user_id` VARCHAR(32)     NOT NULL COMMENT '集市 users.userID，唯一身份键',
  `name`           VARCHAR(100)    NOT NULL DEFAULT '',
  `email`          VARCHAR(255)    NOT NULL DEFAULT '',
  `avatar_url`     VARCHAR(1024)   NOT NULL DEFAULT '',
  `identity`       VARCHAR(32)     NOT NULL DEFAULT '',
  `first_login_at` DATETIME        NOT NULL,
  `last_login_at`  DATETIME        NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_market_user_id` (`market_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**为什么不能用 email 做唯一键：**

- 集市明确禁止按 email 自动关联（邮箱可被改、可重复使用）；
- `user_id` 才是集市的稳定主键；
- 同一个人可能换邮箱，但 `user_id` 不变。

#### 2.6.2 登录会话表（按你平台现有体系即可）

OAuth 只负责「证明这个人是集市用户 X」，之后你需要签发**自己平台**的会话。示例：

```sql
CREATE TABLE `sessions` (
  `session_id` CHAR(64)     NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME     NOT NULL,
  `expires_at` DATETIME     NOT NULL,
  PRIMARY KEY (`session_id`),
  KEY `idx_sessions_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

不要把集市的 `access_token` 写进这张表——它 5 分钟就过期，而且集市没有 refresh token。

#### 2.6.3 可选：登录审计表

```sql
CREATE TABLE `market_login_logs` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `market_user_id` VARCHAR(32)     NOT NULL,
  `ip`             VARCHAR(45)     NOT NULL DEFAULT '',
  `user_agent`     VARCHAR(500)    NOT NULL DEFAULT '',
  `created_at`     DATETIME        NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_market_user_id` (`market_user_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

#### 2.6.4 推荐事务顺序

集市数据库在你的平台不可达，不要尝试跨库事务。回调成功后的本地事务建议：

```sql
START TRANSACTION;

-- 1. 幂等 upsert 用户
INSERT INTO market_users
  (market_user_id, name, email, avatar_url, identity,
   first_login_at, last_login_at)
VALUES (?, ?, ?, ?, ?, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  name = VALUES(name),
  email = VALUES(email),
  avatar_url = VALUES(avatar_url),
  identity = VALUES(identity),
  last_login_at = NOW();

-- 2. 取本地用户主键（或用 LAST_INSERT_ID 技巧）
SELECT id FROM market_users WHERE market_user_id = ?;

-- 3. 建本地会话
INSERT INTO sessions (session_id, user_id, created_at, expires_at)
VALUES (?, ?, NOW(), NOW() + INTERVAL 7 DAY);

COMMIT;
```

如果第 1 步遇到唯一键冲突（并发首次登录），回滚后重新查询再更新即可。

### 2.7 OAuth 错误处理矩阵

| 阶段 | 现象 | 原因 | 你的处理 |
|---|---|---|---|
| 授权页 | 页面提示缺少参数 | 少传 `appid`/`redirect_uri`/`scope`/`response_type` | 检查 URL 参数名 |
| 授权页 | 「暂不支持」 | scope 不是 `read_basic` | 固定用 `read_basic` |
| 授权页 | 「获取应用信息失败」 | `app_id` 不存在或 `is_active=0` | 核对应用状态 |
| 授权页 | 自动跳登录页 | 用户未登录集市 | 正常流程，登录后回来 |
| authorize | 400 `不支持的response_type` | 不是 `code` | 固定 `code` |
| authorize | 400 `不支持的scope` | 不是 `read_basic` | 固定 `read_basic` |
| authorize | 400 `redirect_uri不匹配` | 与 DB 记录不一致（含斜杠、大小写、端口、查询串） | 逐字符核对 |
| authorize | 404 `应用不存在或未激活` | app_id 错 / 被停用 | 核对 `oauth2_apps` |
| callback | 没有收到 code | 用户取消 / 集市报错 | 提示用户重新登录 |
| token | 400 `授权码已过期` | code 超过 5 分钟 | 重新发起授权 |
| token | 404 `授权码不存在或已使用` | code 已兑换 / 被并发兑换 | 不要重试，重新授权 |
| token | 401 `应用密钥错误` | secret 错 / 应用停用 | 检查 secret 与 `is_active` |
| userinfo | 401 | token 过期/伪造/用户被删/应用停用 | 重新授权；检查是否超过 5 分钟 |
| userinfo | 403 `权限不足` | token scope 不是 `read_basic` | 正常流程不应出现，检查应用配置 |

### 2.8 OAuth 安全清单

必须做到：

- [ ] `app_secret` 只存在服务端环境变量/密钥管理系统，不进前端、不进 Git、不进日志。
- [ ] 授权 URL 的 `state` 由服务端生成、服务端保存、回调一次性校验。
- [ ] 回调只接受 `GET`，且校验 `state` 后才兑换 code。
- [ ] token 交换和 userinfo 都在服务端完成，使用 HTTPS。
- [ ] 不把集市 `access_token` 写进浏览器 localStorage、Cookie、URL、日志。
- [ ] 不把集市 `access_token` 当成本平台会话；换完 userinfo 立即丢弃。
- [ ] 用 `user_id` 做身份键，不用 email。
- [ ] 对 `redirect_uri` 做白名单校验（只接受自己配置的回调地址）。
- [ ] 记录登录审计（时间、IP、UA、market_user_id）。

**当前实现的安全边界（接入方必须知道）：**

1. `oauth2_tokens.access_token` 在数据库中是明文存储，数据库权限要收紧。
2. 集市没有「按 token 撤销」接口；停用应用或删除用户后，旧 token 会在下一次校验时被拒。
3. **OAuth2 access_token 与集市普通用户 JWT 使用同一个签名密钥 `crypto.jwtKey`，且 `/api/auth/*` 的用户鉴权只校验签名与 `UserID`。** 因此这枚 token 在 5 分钟内也可能通过集市普通用户接口的鉴权。接入方不得把它交给前端或第三方，也不得把它当作用户会话长期使用。建议集市维护方后续为 OAuth2 token 增加独立签名密钥或 `aud`/`sub` 校验。
4. `authorize` 接口不校验 `state` 是否与用户会话绑定（只原样写入 code 表并回传）；CSRF 防护由接入方自己完成。

---

## 3. 方案 B：直接使用集市账号密码登录（仅限可信第一方 / 内部工具）

> 不建议第三方平台使用。用户需要把集市密码交给你的服务端，风险远高于 OAuth。
> 如果只是「用集市账号登录」，请回到方案 A。

### 3.1 密码的 AES 加密规则（前端同款）

集市登录/注册接口的 `password`、`password2` 字段不是明文，而是 **AES-128-CBC + PKCS7 + Base64**：

| 项 | 值 |
|---|---|
| 算法 | AES-128-CBC |
| Key（UTF-8 字节） | `16bit secret key`（正好 16 字节） |
| IV（ASCII 字节） | `sha256(key)` 的十六进制字符串的前 16 个字符 = `ba79295cdabd3a86` |
| 填充 | PKCS7 |
| 输出 | 标准 Base64（无 `Salted__` 头、无换行） |

**测试向量（可直接用于验证你的实现）：**

```text
明文：secret123
密文：ZQwiKcmlFoNOeBdvHoUOGQ==
```

用集市后端解密该密文会得到 `"secret123"`；生产 `/api/auth/login` 实测该密文能通过解密校验（返回「用户不存在」而不是密码格式错误）。

JavaScript（与前端 `setPassword` 等价）：

```js
import AES from 'crypto-js/aes'
import Utf8 from 'crypto-js/enc-utf8'
import SHA256 from 'crypto-js/sha256'

const key = Utf8.parse('16bit secret key')
const iv = Utf8.parse(SHA256('16bit secret key').toString()) // 64 位十六进制，AES 只用前 16 字节
const encrypted = AES.encrypt('secret123', key, { iv }).toString()
// encrypted === 'ZQwiKcmlFoNOeBdvHoUOGQ=='
```

Node.js 原生 crypto：

```js
import crypto from 'node:crypto'

const key = Buffer.from('16bit secret key', 'utf8')            // 16 bytes
const iv = Buffer.from(
  crypto.createHash('sha256').update(key).digest('hex').slice(0, 16),
  'ascii',
)                                                              // 'ba79295cdabd3a86'
const cipher = crypto.createCipheriv('aes-128-cbc', key, iv)
const out = Buffer.concat([cipher.update('secret123', 'utf8'), cipher.final()])
console.log(out.toString('base64')) // ZQwiKcmlFoNOeBdvHoUOGQ==
```

Python：

```python
import base64, hashlib
from Crypto.Cipher import AES
from Crypto.Util.Padding import pad

key = b'16bit secret key'
iv = hashlib.sha256(key).hexdigest()[:16].encode('ascii')
cipher = AES.new(key, AES.MODE_CBC, iv)
ct = cipher.encrypt(pad(b'secret123', AES.block_size))
print(base64.b64encode(ct).decode())  # ZQwiKcmlFoNOeBdvHoUOGQ==
```

**注意：**

- 这把 key 写在前端 bundle 里，属于「传输混淆」，不是真正机密；真正保护依赖 HTTPS。
- 如果传入的密文无法 Base64 解码/长度不对，后端会解密为空字符串，表现为「密码不能少于6位」。
- 别把明文密码或密文写进日志。

### 3.2 登录

```http
POST https://ssemarket.cn/api/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "<AES 加密后的 Base64>"
}
```

成功：

```json
{
  "code": 200,
  "data": {
    "token": "<access jwt>",
    "refresh_token": "<refresh jwt>"
  },
  "msg": "登录成功"
}
```

**令牌规格：**

| 令牌 | 有效期 | 说明 |
|---|---|---|
| `token`（access） | **7 天** | JWT HS256，`iss=loginTest`，claims 含 `UserID` |
| `refresh_token` | **30 天** | 同时写入 `refresh_tokens` 表，**一次性使用** |

**错误：**

| HTTP | body msg | 原因 |
|---|---|---|
| 422 | 用户不存在 | 邮箱未注册 |
| 400 | 密码错误 | bcrypt 不匹配 |
| 422 | 密码不能少于6位 | 密码短于 6 位或密文非法 |
| 422 | 邮箱号为空！ | email 为空 |
| 429 | `{"message":"操作过于频繁，请稍后再试"}` | 登录限流 |

登录限流默认：同一 IP + 设备指纹 60 秒内 500 次触发，封禁 180 秒；可用环境变量 `LOGIN_LIMIT_MAX` / `LOGIN_LIMIT_WINDOW` / `LOGIN_LIMIT_BLOCK` 调整。可选请求头 `X-Device-ID`（≥8 字符）用于区分设备。

### 3.3 刷新令牌

```http
POST https://ssemarket.cn/api/auth/refresh
Content-Type: application/json

{"refresh_token":"<refresh jwt>"}
```

成功：

```json
{
  "code": 200,
  "data": {
    "token": "<新的 access jwt>",
    "refresh_token": "<新的 refresh jwt>"
  },
  "msg": "刷新token成功"
}
```

服务端数据库操作：

```sql
SELECT * FROM refresh_tokens
WHERE token = ? AND user_id = ? AND is_used = 0
LIMIT 1;

UPDATE refresh_tokens SET is_used = 1 WHERE id = ?;

INSERT INTO refresh_tokens
  (user_id, token, expires_at, is_used, created_at)
VALUES (?, ?, NOW() + INTERVAL 30 DAY, 0, NOW());
```

规则：

- 旧 refresh token 用过一次就标记 `is_used=1`，再次使用返回 401。
- 并发刷新请在你自己的服务端串行化；否则可能只有第一个成功。
- refresh 过期返回 401 `refresh token 已过期`。
- 刷新会同时换发新的 refresh token，请把两个都替换掉。

### 3.4 获取当前用户信息

```http
GET https://ssemarket.cn/api/auth/info
Authorization: Bearer <access jwt>
```

```json
{
  "code": 200,
  "data": {
    "user": {
      "userID": 12345,
      "name": "用户名",
      "phone": "13800000000",
      "avatarURL": "https://...",
      "email": "user@example.com",
      "identity": "student",
      "coins": 12.34
    }
  }
}
```

### 3.5 用户注册（邮箱验证码 + CDKey）——完整接口

> **先分清两类「注册」：**
>
> - **用户注册**：本节中的 `POST /api/auth/register`，作用是**创建集市账号**。
> - **OAuth 应用注册**：2.2 节中的 `oauth2_apps` / 管理端接口，作用是**让别的平台接入集市登录**，不创建用户。
>
> 另外还有两条受限建号通道：`POST /api/open/users`（开发者平台 SM2 中转）和
> `POST /api/sso/mirror/provision`（Mirror 服务内部使用）。第三方平台默认只能用本节的标准注册接口。

#### 3.5.1 注册前必须准备什么

| 前置条件 | 说明 |
|---|---|
| 一个未注册的邮箱 | 接口只校验包含 `@`；建议客户端做完整邮箱格式校验 |
| 邮箱验证码 | 6 位数字，5 分钟内有效，通过 `POST /api/auth/validateEmail` 发送 |
| 密码 | 明文 ≥ 6 位；提交时必须 AES 加密（见 3.1） |
| CDKey（邀请码） | 一次性，`cd_keys.used=0`；管理端生成的通常是 8 位十六进制 |
| 手机号（可选） | 可传 `""` 或省略；为空时后端随机生成 11 位数字 |
| Redis | 验证码签码/校验依赖 Redis，不可用则注册失败 |
| SMTP | 服务端需配置好发信账号，否则验证码发不出去 |

浏览器端注册页：`https://ssemarket.cn/new/auth/register`。

**SMTP 配置（只列键名，不记录任何真实账号/密码）：**

```ini
SMTP_EMAIL_USERNAME=<发信邮箱>
SMTP_EMAIL_PASSWORD=<发信邮箱授权码，不是登录密码>
SMTP_ADDR=<SMTP 地址，例如 smtp.example.com:465>
SMTP_HOST=<SMTP 主机名>
```

- 读取位置：`SSE_market_server/config/emailConst.go` 的 `InitEmailConfig()`，使用 `godotenv.Load(".env")` 加载服务端工作目录下的 `.env`。
- 发信人模板：`软工集市团队 <SMTP_EMAIL_USERNAME>`；验证邮件标题：`软工集市访问验证`。
- 另一组 `SMTP_NOTICE_*` 只用于系统报警/通知，**不用于注册验证码**。
- **SSO 模式下走邀请码注册时，集市这四项同样必填**：UniSSO 注册页在收到邀请码后会服务端调用 `POST /api/auth/validateEmail` 代发「集市验证码」。缺任意一项时发件人会退化成 `软工集市团队 <>`，接口报 `mail: invalid string`，用户在 SSO 注册页看到的就是这句话，注册直接中断。
- 这些值是服务端机密，禁止写进前端、文档、日志或 Git。

#### 3.5.2 完整流程

```
客户端                          集市后端                       邮件/Redis
  │ POST /api/auth/validateEmail   │                              │
  │───────────────────────────────▶│ 校验 mode + 邮箱是否已注册    │
  │                                │ 生成 6 位验证码               │
  │                                │ 写 Redis vcode:<email> 5min   │
  │                                │──── SMTP 发送验证码 ─────────▶│
  │◀── 200 邮箱发送成功 ───────────│                              │
  │                                                               │
  │ 用户查收邮件，拿到 6 位验证码                                  │
  │                                                               │
  │ POST /api/auth/register        │                              │
  │  {name,email,password(AES),    │                              │
  │   password2(AES),phone,        │                              │
  │   valiCode,CDKey}              │                              │
  │───────────────────────────────▶│ 解密密码 → 校验字段           │
  │                                │ 校验 Redis 验证码             │
  │                                │ 标记 CDKey 已用               │
  │                                │ bcrypt 哈希 → INSERT users    │
  │                                │ INSERT refresh_tokens         │
  │                                │ 清理验证码 Redis key          │
  │◀── 200 {token, refresh_token} ─│                              │
```

#### 3.5.3 第一步：发送邮箱验证码

```http
POST https://ssemarket.cn/api/auth/validateEmail
Content-Type: application/json

{
  "email": "new@example.com",
  "mode": 0
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `email` | string | 是 | 会先 `trim + lowercase` |
| `mode` | int | 是 | `0` = 要求邮箱**未注册**（注册场景）；`1` = 要求邮箱**已注册**（找回密码/改密场景） |

**成功响应（注意 `data` 是嵌套的）：**

```json
{
  "code": 200,
  "data": { "data": "new@example.com" },
  "msg": "邮箱发送成功"
}
```

**失败响应：**

| HTTP | body 示例 | 条件 |
|---|---|---|
| 400 | `{"code":400,"data":{"data":"a@b.com","mode":0},"msg":"邮箱已注册"}` | `mode=0` 且邮箱已存在 |
| 400 | `{"code":400,"data":{"data":"a@b.com","mode":1},"msg":"该邮箱未注册，请先完成注册操作"}` | `mode=1` 且邮箱不存在 |
| 400 | `{"code":400,"data":{"data":"a@b.com"},"msg":"邮箱获取错误"}` | email 为空 |
| 400 | `{"code":400,"data":null,"msg":"验证码已发送，请5分钟后再试"}` | 5 分钟内重复发送 |
| 400 | `{"code":400,"data":null,"msg":"验证码服务不可用"}` | Redis 不可用 |
| 400 | `{"code":400,"data":null,"msg":"<SMTP 错误>"}` | SMTP 配置错误/发信失败 |

**验证码行为（重要）：**

- 6 位随机数字，写入 Redis `vcode:<小写邮箱>`，TTL **5 分钟**。
- 失败次数写入 `vcode_fail:<小写邮箱>`，TTL 5 分钟；**累计 5 次错误后删除验证码**，必须重新发送。
- 验证码有效期内重复调用 `validateEmail` 会被拒（`验证码已发送，请5分钟后再试`）。
- 重新发送成功时会重置失败次数。
- 注册成功后服务端会删除 `vcode:*` 与 `vcode_fail:*`。

cURL：

```bash
curl -sS -X POST https://ssemarket.cn/api/auth/validateEmail \
  -H 'Content-Type: application/json' \
  -d '{"email":"new@example.com","mode":0}'
```

#### 3.5.4 第二步：提交注册

```http
POST https://ssemarket.cn/api/auth/register
Content-Type: application/json
```

**请求字段（字段名以 Go 结构体为准）：**

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 否 | 昵称；为空时后端用随机英文名填充 |
| `email` | string | 是 | 必须包含 `@`；不能已被占用 |
| `password` | string | 是 | **AES 加密后的 Base64**，明文 ≥ 6 位 |
| `password2` | string | 是 | 同上，且解密后必须与 `password` 相同 |
| `phone` | string | 否 | `""`/省略 = 随机生成 11 位；传则必须正好 11 个字符 |
| `valiCode` | string | 是 | 6 位邮箱验证码；后端字段名为 `ValiCode`，JSON 大小写不敏感 |
| `CDKey` | string | 是 | 一次性邀请码；**不要写成 `cd_key`**（下划线不匹配） |

**完整请求示例：**

```json
{
  "name": "新用户",
  "email": "new@example.com",
  "password": "ZQwiKcmlFoNOeBdvHoUOGQ==",
  "password2": "ZQwiKcmlFoNOeBdvHoUOGQ==",
  "phone": "",
  "valiCode": "123456",
  "CDKey": "1a2b3c4d"
}
```

> 上面的 `ZQwiKcmlFoNOeBdvHoUOGQ==` 是明文 `secret123` 的 AES 密文（见 3.1 测试向量），
> 仅用于演示格式，不要拿它当真实密码。

**字段细节：**

- `name`：API 不校验长度/字符；建议 1~50 字符。为空时随机生成。
- `email`：后端只检查 `strings.Contains(email, "@")` 且未被占用；不校验完整 RFC 格式。
  服务端只 `trim`，不强制小写；数据库排序规则大小写不敏感，建议客户端主动小写。
- `phone`：为空时随机 11 位数字；传值时后端只检查 `len(phone) == 11`（不会补位/去空格），
  建议固定传 11 位数字或直接传空。
- `password` / `password2`：先 AES 解密；`password2` 明文 < 6 位直接 422；
  两次不一致直接 422。bcrypt 对超过 72 字节的密码会报错，建议 ≤ 72 字节。
- `valiCode`：大小写不敏感匹配；必须与 Redis 中的 6 位验证码完全一致。
- `CDKey`：必须存在且 `used=0`；成功后置 `used=1`、写 `usedtime`。

cURL（把邮箱/验证码/CDKey 换成自己的）：

```bash
curl -sS -X POST https://ssemarket.cn/api/auth/register \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"新用户",
    "email":"new@example.com",
    "password":"ZQwiKcmlFoNOeBdvHoUOGQ==",
    "password2":"ZQwiKcmlFoNOeBdvHoUOGQ==",
    "phone":"",
    "valiCode":"123456",
    "CDKey":"1a2b3c4d"
  }'
```

#### 3.5.5 成功响应与自动登录

```json
{
  "code": 200,
  "data": {
    "token": "<access jwt，7 天>",
    "refresh_token": "<refresh jwt，30 天，一次性>"
  },
  "msg": "注册成功"
}
```

注册成功即等同于登录成功：

- `token` / `refresh_token` 与 `/api/auth/login` 返回结构完全一致；
- 服务端已写入 `refresh_tokens`；
- 前端拿到后直接 `setToken` / `setRefreshToken`，再调 `GET /api/auth/info` 取用户资料；
- 不需要再调一次登录接口。

#### 3.5.6 服务端校验顺序（决定你收到哪条错误）

`service.Users.Register` 按以下顺序执行，**前面的失败会遮蔽后面的原因**：

1. `phone` 为空 → 随机生成 11 位，直到不重复；
2. `len(phone) != 11` → `ErrBadRequest`；
3. `len(password) < 6` → `ErrBadRequest`；
4. `name` 为空 → 随机英文名；
5. 手机号已存在 → `ErrConflict`；
6. 邮箱不含 `@` → `ErrBadRequest`；
7. 邮箱已存在 → `ErrConflict`；
8. 校验邮箱验证码（Redis）→ 失败 `ErrUnauthorized`；
9. 校验并消费 CDKey → 失败 `ErrBadRequest`；
10. bcrypt 哈希 → `INSERT users`；
11. 签发 access/refresh → `INSERT refresh_tokens` → 缓存用户；
12. 删除 Redis 验证码与失败计数。

> **关键：服务端内部错误码不等于最终文案。**
> 控制器对 `ErrBadRequest` 只按「邮箱是否合法」和「请求里 CDKey 是否非空」两个条件选文案，
> 因此手机号长度、密码长度、CDKey 无效都可能被显示成同一条错误。详见 3.5.7。

#### 3.5.7 错误响应矩阵（按最终返回文案）

先记住控制器的映射规则：

```text
ErrConflict      → 422，文案看「请求里的 phone 是否非空」
ErrUnauthorized  → 400 验证码错误
ErrBadRequest    → 若 email 为空/不含 @        → 422 邮箱不符合要求
                   否则若 CDKey 非空           → 400 激活码错误
                   否则                        → 422 手机号必须为11位!!!
其他              → 500 注册失败
```

因此最终文案如下：

| HTTP | body `msg` | 真实触发条件（可能被遮蔽） |
|---|---|---|
| 422 | 密码不能少于6位 | 控制器预检：`password2` 解密后 < 6 位（在所有服务端校验之前） |
| 422 | 两次密码不相同，请重新输入 | 控制器预检：两次 AES 解密结果不一致 |
| 422 | 该手机号已存在 | `ErrConflict` 且请求 `phone` 非空；**即使实际冲突的是邮箱**也会提示手机号 |
| 422 | 该邮箱已存在 | `ErrConflict` 且请求 `phone` 为空/省略 |
| 422 | 邮箱不符合要求 | `ErrBadRequest` 且 email 为空或不含 `@`；可能遮蔽手机号/密码错误 |
| 400 | 激活码错误 | `ErrBadRequest`、email 合法、且请求 `CDKey` **非空**；真实原因可能是：手机号长度不对、服务层密码 < 6 位、CDKey 不存在/已使用 |
| 422 | 手机号必须为11位!!! | `ErrBadRequest`、email 合法、且请求 `CDKey` **为空**；真实原因可能是：CDKey 没传、手机号长度不对、服务层密码 < 6 位 |
| 400 | 验证码错误 | 验证码不匹配/已过期/已因 5 次失败被删除 |
| 500 | 注册失败 | 数据库/Redis/内部错误 |

**接入建议：**

- 永远传非空且格式正确的 `CDKey`，并保证 `phone` 为空或正好 11 位；
- 不要让用户根据 `msg` 猜字段，客户端自己先做本地校验；
- 遇到 400「激活码错误」时，不要只检查 CDKey，也要检查 phone/password；
- 所有注册错误都带 `"data": null`。

#### 3.5.8 服务端数据库 / Redis 操作（按顺序）

```sql
-- 1. 手机号/邮箱占用检查
SELECT COUNT(*) FROM users WHERE phone = ?;
SELECT COUNT(*) FROM users WHERE email = ?;

-- 2. 校验验证码（Redis，不是 MySQL）
--    GET vcode:<lower(email)>        → 与 valiCode 比较
--    GET vcode_fail:<lower(email)>   → 错误次数 +1，>=5 时 DEL vcode 键
--    SET vcode_fail:<email> <n> EX 300
--    SET vcode:<email> <code> EX 300（发送时）

-- 3. 消费 CDKey（一次性）
SELECT * FROM cd_keys WHERE content = ? AND used = 0 LIMIT 1;
UPDATE cd_keys SET used = 1, usedtime = NOW() WHERE cdkeyID = ?;

-- 4. 写入用户（password 为 bcrypt 哈希，不是明文/AES 密文）
INSERT INTO users
  (name, phone, email, password, banend, registered_at)
VALUES
  (?, ?, ?, '<bcrypt hash>', NOW(), NOW());

-- 5. 写入 refresh token
INSERT INTO refresh_tokens
  (user_id, token, expires_at, is_used, created_at)
VALUES
  (?, '<refresh jwt>', NOW() + INTERVAL 30 DAY, 0, NOW());

-- 6. 成功后清理 Redis
--    DEL vcode:<email> vcode_fail:<email>
```

写入 `users` 时的默认值：

| 列 | 值 |
|---|---|
| `userID` | 自增 |
| `password` | bcrypt hash（`golang.org/x/crypto/bcrypt`，默认 cost） |
| `registered_at` | `NOW()` |
| `banend` | `NOW()`（不是封禁；解封/注册时间字段的历史用法） |
| `identity` | 数据库默认 `student` |
| `user_type` | 数据库默认 `market` |
| `coins` / `experience` / `novel_points` | 默认 0 |

**并发注意：**

- `users.email` 在现网 DDL 中**没有唯一索引**，服务层是「先查后插」；
  同一邮箱并发注册存在极小的重复风险。接入方不要并发提交同一邮箱。
- `UseCDKey` 也是「先查后改」，没有 `WHERE used=0` 的原子更新；
  并发使用同一个 CDKey 可能双花。接入方不要并发提交同一个 CDKey。

#### 3.5.9 CDKey（邀请码）：获取与测试

**生产环境：**

- 由管理员/运维发放；管理员接口（需 admin token）：
  - `GET /api/auth/admin/keys?used=0`：列出未使用邀请码；
  - `POST /api/auth/addKey`：`{"Key":"..."}` 指定内容，或 `{}` 自动生成 8 位十六进制；
  - `POST /api/auth/admin/keys/batch`：`{"Num":10}` 批量生成（最多 100 个）。
- 第三方开发者不要自己往生产库插 CDKey。

**测试环境**（仅限你有测试库权限时）：

```sql
-- 先确认不重复；cd_keys.content 是 char(9)，没有唯一索引
SELECT COUNT(*) FROM cd_keys WHERE content = 'TEST0001';

INSERT INTO cd_keys (content, used, createdtime, usedtime)
VALUES ('TEST0001', 0, NOW(), NOW());
```

生成规则参考：管理端自动生成的是 `util.GenerateRandomString(8)` → 8 位十六进制；
数据库字段允许 9 个字符，手工值不要超过 9 位。

#### 3.5.10 验证码收不到 / 发送失败排查

按顺序检查：

1. `POST /api/auth/validateEmail` 是否返回 200；失败时 body 的 `msg` 就是具体原因。
2. 服务端 `.env`（或环境变量）是否提供 `SMTP_EMAIL_USERNAME` / `SMTP_EMAIL_PASSWORD` / `SMTP_ADDR` / `SMTP_HOST`；
   发信密码是邮箱「授权码」，不是邮箱登录密码。
3. Redis 是否可用：
   - `GET vcode:<小写邮箱>` 是否能看到 6 位码；
   - `TTL vcode:<小写邮箱>` 是否在 0~300 秒。
4. 是否在 5 分钟内重复点击「发送验证码」→ 会被拒，等 TTL 过期或换邮箱。
5. 是否连续输错 5 次 → 验证码键被删除，需要重新发送。
6. 查收垃圾邮件/拦截箱；确认发信域名/邮箱未被 SMTP 服务商限流。
7. 服务端日志只看错误类型，不要打印验证码、SMTP 密码、密码密文。

**不要在文档、工单、聊天里贴真实 SMTP 授权码。** 需要排障时让运维在服务器上检查环境变量。

#### 3.5.11 平台中转建号：`POST /api/open/users`

这是给**集市开发者平台已登记模块**用的建号通道，不面向普通第三方网站：

- 不走邮箱验证码、不走 CDKey；
- 由开发者平台中转，使用平台 SM2 私钥签发的 `x-model-authorization`（`slug=__platform__`）；
- 签名串与验签规则见 `docs/身份验证说明.md`；
- 直接调用本接口必须持有平台签名，普通模块/第三方不要尝试自行伪造。

请求：

```http
POST https://ssemarket.cn/api/open/users
x-model-authorization: base64url(__platform__).base64url(ts).base64url(nonce).base64url(sign)
x-model-query-time: <毫秒时间戳，可选但建议>
Content-Type: application/json

{
  "name": "昵称（可选，空则随机）",
  "email": "user@example.com",
  "password": "<明文，≥6 位>",
  "phone": "（可选，空则随机 11 位）"
}
```

成功：

```json
{
  "code": 200,
  "data": {
    "user": {
      "userID": 12345,
      "name": "昵称",
      "email": "user@example.com",
      "phone": "13800000000",
      "accessToken": "<7 天 access jwt>",
      "refreshToken": "<30 天 refresh jwt>"
    }
  },
  "msg": "用户已创建"
}
```

校验/错误：

- `email` 必须包含 `@`，且未被占用 → 400「email 不合法」/「该邮箱已存在」；
- `password` ≥ 6 位 → 400「password 不能少于 6 位」；
- `phone` 非空时按 `trim` 后必须 11 位且不重复 → 400「phone 必须为 11 位数字」/「该手机号已存在」；
- 内部错误 → 500「创建失败」。
- 该通道**没有邮箱验证码和 CDKey**，因此只应授予可信模块；平台侧需自行做好实名/风控。

服务端最终仍走 `PlatformCreateUser`：查重 → bcrypt → `INSERT users` → 签发 tokens → `INSERT refresh_tokens`。
返回的 `accessToken` / `refreshToken` 可以直接作为集市用户态使用。

#### 3.5.12 Mirror 内部建号：`POST /api/sso/mirror/provision`

内部通道，**第三方不要使用**：

```http
POST /api/sso/mirror/provision
X-Unisso-Trust: <SSO_CLIENT_SECRET>
Content-Type: application/json

{"name":"...","email":"...","password":"<明文，≥6 位>"}
```

- 认证凭据是 SSO 的 `client_secret`；
- 邮箱已存在则直接复用并签发 tokens，不存在则创建；
- 不校验邮箱验证码/CDKey；
- 返回 `data.user` + `data.token` + `data.refresh_token`；
- 仅限 Mirror 服务在 UniSSO 验证后调用。

#### 3.5.13 注册后的账号状态

- 新用户 `identity=student`、`user_type=market`、`coins=0`、`experience=0`、`novel_points=0`；
- `registered_at` 写入当前时间；`banend` 也被写成当前时间（历史字段用法，不代表封禁）；
- 头像为空，前端按默认头像兜底；
- 不需要再调用登录接口，直接用注册返回的 `token` 访问 `/api/auth/info`；
- 如果注册后立即调用第三方 OAuth，仍需先在集市侧完成一次登录态（注册接口已自动给 token）。

#### 3.5.14 注册接入安全建议

- 注册接口是 `noauth`，不要在客户端之外暴露内部错误细节；
- 邮箱验证码接口没有额外业务限流，生产部署应在网关/nginx 层对同一 IP 做频率限制；
- 不要让第三方平台替用户保管集市密码；第三方接入优先用 OAuth2（方案 A）；
- CDKey 一次性且当前没有数据库唯一索引/原子占用，禁止并发复用；
- 邮箱占用状态会通过 `mode=0` 暴露（「邮箱已注册」），这是产品取舍，接入方不要用它做账号枚举；
- 任何日志都不要打印明文密码、AES 密文、邮箱验证码、SMTP 授权码。

### 3.6 退出登录

集市**没有服务端 logout 接口**。退出 = 客户端丢弃 `token` 与 `refresh_token`。

若要强制让 refresh token 失效，需要后台/运维把 `refresh_tokens.is_used` 置 1；access token 本身是无状态 JWT，到期（7 天）前无法按 token 撤销。

### 3.7 数据库操作汇总（方案 B）

| 操作 | 表 | 读/写 |
|---|---|---|
| 登录 | `users` | 读（按 email 取 bcrypt hash） |
| 登录成功 | `refresh_tokens` | 写（插入 refresh token 记录） |
| 刷新 | `refresh_tokens` | 读旧记录 + 更新 `is_used=1` + 插入新记录 |
| 注册 | `users`、`cd_keys`、`refresh_tokens` | 读重 + 写用户 + 标记 CDKey + 写 refresh |
| 取用户信息 | `users` | 读 |
| 退出 | 无 | 无服务端操作 |

---

## 4. 方案 C：集市内置的 UniSSO 单点登录

> **先分清用途：**
> - `/api/sso/*` 是「**集市自己**用 UniSSO 登录集市」的通道。它最终签发的是**集市 JWT**，并把浏览器送到**集市前端** `/new/auth/sso/result`。
> - 第三方平台想「用集市账号登录自己」不能用这套接口，请用**方案 A**。
> - 第三方平台如果想直接接 UniSSO（`sso.ssemarket.cn`）登录自己，需要单独向 UniSSO 注册自己的 client，不能复用集市的 `client_id`/`client_secret`；见 4.8。

### 4.1 配置（集市后端）

配置文件：`SSE_market_server/config/application.yml` 的 `sso` 段；生产建议用环境变量覆盖：

```ini
SSO_ENABLED=true
SSO_CLIENT_ID=<UniSSO 分配的 client_id>
SSO_CLIENT_SECRET=<UniSSO 分配的 client_secret>
SSO_AUTHORIZE_URL=https://sso.ssemarket.cn/authorize
SSO_TOKEN_URL=https://sso.ssemarket.cn/api/oauth/token
SSO_USERINFO_URL=https://sso.ssemarket.cn/api/oauth/userinfo
SSO_REDIRECT_URI=https://ssemarket.cn/api/sso/callback
```

约束：

- `SSO_CLIENT_SECRET` 只放后端环境变量，禁止进 Git/前端/日志。
- `SSO_REDIRECT_URI` 必须与 UniSSO 应用注册的回调地址完全一致。
- 未启用时 `/api/sso/login` 与 `/api/sso/bind/start` 返回 503。
- **现状提示（2026-10-25 实测）**：生产 `https://ssemarket.cn/api/sso/login` 返回 `503 {"code":503,"data":null,"msg":"SSO 登录未启用或暂时不可用"}`，说明当前部署未开启 SSO（或配置缺失）。使用前先让运维确认 `SSO_ENABLED` 与四项 URL/凭据均已注入。

### 4.2 路由一览

| 方法 | 路径 | 鉴权 | 用途 |
|---|---|---|---|
| GET | `/api/sso/login` | 无 | 未登录发起 SSO，302 到 UniSSO |
| GET | `/api/sso/callback` | 无 | UniSSO 回调，302 到集市前端结果页 |
| POST | `/api/sso/exchange` | 无 | ticket → 集市令牌 / bind_needed |
| POST | `/api/sso/bind` | 无 | bind_needed ticket + 邮箱密码 → 写绑定 + 发令牌 |
| POST | `/api/sso/bind/start` | 用户 JWT | 已登录用户主动绑定，返回 `authorize_url` |
| GET | `/api/sso/status` | 用户 JWT | 查询当前账号绑定状态 |
| POST | `/api/sso/unbind` | 用户 JWT | 验证密码后解绑 |
| POST | `/api/sso/mirror/provision` | `X-Unisso-Trust` | Mirror 服务内部建号，**第三方不要用** |

### 4.3 流程 1：未登录用户用 SSO 登录

```
浏览器                    集市后端                        UniSSO
  │ GET /api/sso/login       │                              │
  │─────────────────────────▶│ 生成 state + PKCE verifier   │
  │                          │ 存 Redis（10 分钟）           │
  │◀── 302 authorize URL ────│                              │
  │──────────────────── 登录/授权 ─────────────────────────▶│
  │◀──── 302 /api/sso/callback?code&state ─────────────────│
  │─────────────────────────▶│ GETDEL state 校验             │
  │                          │ POST token（PKCE + secret）  │
  │                          │ GET userinfo（Bearer）        │
  │                          │ 查 sso_bindings               │
  │◀─ 302 /new/auth/sso/result?ticket=xxx                   │
  │ POST /api/sso/exchange {ticket}                         │
  │─────────────────────────▶│ GETDEL ticket → 发集市 JWT    │
  │◀── {type:"login", token, refresh_token}                 │
```

关键点：

- `state` 与 PKCE `verifier` 存 Redis：
  - key：`sso:state:<state>`
  - value：JSON `{verifier, bind_uid}`
  - TTL：10 分钟
  - 读取用 `GETDEL`，一次性消费，天然防重放。
- 回调成功后写 ticket：
  - key：`sso:ticket:<ticket>`
  - value：JSON `{type, uid?, sub?, sso_username?, sso_email?}`
  - TTL：2 分钟
  - `type ∈ {login, bind_needed, bound}`。
- 登录/绑定成功的 ticket 在 exchange 时 `GETDEL` 消费；`bind_needed` 的 ticket 在 exchange 时只读不删，直到 `/api/sso/bind` 成功时消费（所以 2 分钟内可重复读取，但最终只能绑定一次）。
- 集市不保存 UniSSO 的 access_token，只在回调当次用它取 userinfo。
- 回调失败时 302 到：
  - `/new/auth/sso/result?error=state`（state 缺失/过期/重放）
  - `?error=exchange`（UniSSO 换 token 或取 userinfo 失败）
  - `?error=conflict`（该 SSO 已绑定其他账号 / 当前账号已有绑定）

### 4.4 流程 2：绑定已有集市账号（首次 SSO）

当 UniSSO 的 `sub` 没有绑定任何集市账号，且本次不是已登录用户主动绑定时：

1. 回调写入 `ticket{type="bind_needed", sub, sso_username, sso_email}`；
2. 前端跳到 `/new/auth/sso/bind?ticket=...`；
3. 用户输入**集市邮箱 + 集市密码**（密码 AES 加密）；
4. 前端调用：

```http
POST /api/sso/bind
Content-Type: application/json

{
  "ticket": "<ticket>",
  "email": "user@example.com",
  "password": "<AES 加密后的集市密码>"
}
```

5. 后端校验邮箱存在 + bcrypt 密码正确 + 该账号未绑定 + 该 sub 未绑定；
6. 写 `sso_bindings`；
7. 发集市 `token` + `refresh_token`（与密码登录结构一致）。

错误：

| HTTP | msg | 原因 |
|---|---|---|
| 422 | 该邮箱未注册集市账号，请先注册 | `users` 无此邮箱 |
| 401 | 邮箱或密码错误 | bcrypt 不匹配 |
| 409 | 该账号或 SSO 已存在绑定 | 双向唯一键冲突 |
| 400 | 请求参数错误 | ticket/email 缺失或密码太短 |

**铁律：绝不会按 email 自动关联。** email 相同只用于页面提示，必须通过集市密码验证。

### 4.5 流程 3：已登录用户主动绑定

1. 前端带用户 JWT 调用：

```http
POST /api/sso/bind/start
Authorization: Bearer <集市用户 JWT>
```

2. 后端检查当前账号未绑定，生成 `state`（带 `bind_uid`）并返回：

```json
{"code":200,"data":{"authorize_url":"https://sso.ssemarket.cn/authorize?..."},"msg":"获取绑定地址成功"}
```

3. 浏览器整页跳转 `authorize_url`；
4. 回调时如果 sub 未绑定 → 直接写 `sso_bindings` → ticket `{type="bound"}`；
5. 前端 exchange 得到集市 token + refresh token，并提示「SSO 绑定成功」。
6. 若该 sub 已绑其他账号，或当前账号已有绑定 → `error=conflict` / 409。

### 4.6 流程 4：查询状态与解绑

查询：

```http
GET /api/sso/status
Authorization: Bearer <集市用户 JWT>
```

```json
{
  "code": 200,
  "data": {
    "bound": true,
    "sso_username": "unisso-name",
    "bound_at": "2026-08-26T12:00:00+08:00"
  },
  "msg": "成功"
}
```

解绑：

```http
POST /api/sso/unbind
Authorization: Bearer <集市用户 JWT>
Content-Type: application/json

{"password":"<AES 加密后的集市密码>"}
```

- 必须验证当前用户密码（防止会话被劫持后恶意解绑）；
- 成功删除 `sso_bindings` 对应行；
- 密码错误 401；未绑定 409。

### 4.7 SSO 数据库与 Redis 操作

**MySQL：`sso_bindings`（唯一持久化数据）**

```sql
CREATE TABLE `sso_bindings` (
  `id`            INT NOT NULL AUTO_INCREMENT,
  `local_user_id` INT NOT NULL,
  `sso_sub`       VARCHAR(64) NOT NULL COMMENT 'UniSSO 不透明 UUID，不解析',
  `sso_username`  VARCHAR(100) NOT NULL DEFAULT '',
  `created_at`    DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sso_sub` (`sso_sub`),
  UNIQUE KEY `uk_local_user_id` (`local_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- 一个 SSO 身份只能绑一个集市账号；
- 一个集市账号只能绑一个 SSO 身份；
- 绑定：`INSERT`；解绑：`DELETE`；查询：按 `sso_sub` 或 `local_user_id`。
- 不存 UniSSO 的 access_token/refresh_token。

**Redis：**

| key | value | TTL | 读取方式 |
|---|---|---|---|
| `sso:state:<state>` | `{"verifier":"...","bind_uid":0}` | 10 分钟 | `GETDEL` |
| `sso:ticket:<ticket>` | `{"type":"login","uid":123}` 等 | 2 分钟 | 登录/绑定 `GETDEL`，bind_needed 先 `GET` |

Redis 不可用时 SSO 流程会失败（state/ticket 无法保存）；密码登录与 OAuth 不受影响。

### 4.8 如果你的平台想直接接 UniSSO

不要复用集市的 `client_id`/`client_secret`。你需要：

1. 在 UniSSO 侧注册自己的应用（client），登记你的回调地址；
2. 使用授权码 + PKCE S256；
3. 端点（以 UniSSO 当前生产为准，建议先读 discovery）：
   - discovery：`https://sso.ssemarket.cn/.well-known/openid-configuration`
   - authorize：`GET https://sso.ssemarket.cn/authorize`
   - token：`POST https://sso.ssemarket.cn/api/oauth/token`
   - userinfo：`GET https://sso.ssemarket.cn/api/oauth/userinfo`
4. scope 当前为 `openid profile email`（UniSSO 会把粗粒度 profile 展开成 `profile:*`）；
5. userinfo 实测字段：`sub`、`preferred_username`、`email`、`email_verified`；
6. `sub` 视为不透明 UUID，不要解析；
7. 不要验证 UniSSO 的 `id_token` 签名（其 `iss` 可能不一致，且历史上有默认 HS256 密钥问题）；以 userinfo 的 `sub` 为准。

> 具体 client 注册流程、consent 页 CSRF 等属于 UniSSO 侧协议，请以 UniSSO 管理方文档为准；本文不展开。

---

## 5. 方案 D：PAT（个人访问令牌，供智能体 / MCP / 服务端程序）

PAT 不是第三方登录方案，而是**某个集市用户主动签发**给智能体使用的长期令牌。

### 5.1 创建

```http
POST /api/auth/token/create
Authorization: Bearer <该用户的集市 access token>
Content-Type: application/json

{
  "name": "Claude Desktop",
  "scopes": ["agent:chat", "forum:read"]
}
```

可用 scope：

| scope | 含义 |
|---|---|
| `agent:chat` | 小秋通用对话 |
| `forum:read` | 论坛检索：搜索帖子、查看评论 |
| `forum:write` | 以该用户身份发帖 |

成功响应：

```json
{
  "code": 200,
  "data": {
    "token_id": 1,
    "token": "sk_xq_<64位十六进制>",
    "prefix": "sk_xq_ab",
    "name": "Claude Desktop",
    "scopes": "agent:chat,forum:read",
    "is_active": true,
    "created_at": "2026-10-25T12:00:00+08:00"
  },
  "msg": "创建成功，请立即复制保存，明文 token 仅此一次返回"
}
```

- 明文只返回一次；DB 只存 `SHA-256(明文)` 的小写十六进制。
- 每个用户最多 10 个活跃 token（超出 429）。
- 列表 `GET /api/auth/token/list?page=1&page_size=20`（不含明文）。
- 吊销 `POST /api/auth/token/revoke {"token_id":1}`（软删，`is_active=0`）。
- 删除 `POST /api/auth/token/delete {"token_id":1}`（硬删，必须先吊销）。

### 5.2 验证

```http
POST /api/auth/token/verify
Authorization: Bearer sk_xq_<...>
```

```json
{
  "code": 200,
  "data": {
    "user_id": 12345,
    "scopes": ["agent:chat", "forum:read"],
    "is_active": true
  },
  "msg": "ok"
}
```

验证逻辑（供自建网关参考）：

```sql
SELECT * FROM user_api_tokens
WHERE token_hash = SHA2('<完整明文 token>', 256)
  AND is_active = 1
LIMIT 1;
```

> 表中 `token` 列实际叫 `token_hash`；哈希是完整明文（含 `sk_xq_` 前缀）的 SHA-256 小写十六进制。
> 验证成功后可异步回写 `last_used_at`。

**PAT 不是用户 JWT**：它不能直接作为 `Authorization: Bearer` 访问 `/api/auth/*`（那些接口只认集市用户 JWT）。
PAT 只用于明确支持 PAT 的服务（如 MCP 网关），或先调 `/api/auth/token/verify` 换取 `(user_id, scopes)` 再自行鉴权。

---

## 6. 数据库 DDL 汇总

> 数据库名：`ssemarket`（以部署配置为准）。以下为与登录/OAuth 直接相关的表。
> **除 `oauth2_apps` 的运维注册外，不建议第三方直接写任何集市表；应走 API。**

### 6.1 `oauth2_apps`（OAuth 应用）

```sql
CREATE TABLE `oauth2_apps` (
  `id` int NOT NULL AUTO_INCREMENT,
  `app_id` varchar(100) DEFAULT NULL,
  `app_name` varchar(100) DEFAULT NULL,
  `app_secret` varchar(255) DEFAULT NULL,
  `app_icon` varchar(255) DEFAULT NULL,
  `redirect_uri` varchar(255) DEFAULT NULL,
  `alt_redirect_uri` varchar(255) DEFAULT NULL,   -- 当前代码不使用
  `alt_app_secret` varchar(255) DEFAULT NULL,     -- 当前代码不使用
  `description` varchar(500) DEFAULT NULL,
  `is_active` tinyint(1) DEFAULT '1',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_oauth2_app_redirect` (`app_id`,`redirect_uri`),
  KEY `idx_oauth2_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 6.2 `oauth2_codes`（授权码，5 分钟）

```sql
CREATE TABLE `oauth2_codes` (
  `code_id` int NOT NULL AUTO_INCREMENT,
  `code` varchar(100) DEFAULT NULL,
  `app_id` varchar(100) DEFAULT NULL,
  `user_id` bigint DEFAULT NULL,
  `scope` varchar(100) DEFAULT NULL,
  `redirect_uri` varchar(255) DEFAULT NULL,
  `state` varchar(100) DEFAULT NULL,
  `expires_at` datetime DEFAULT NULL,
  `is_used` tinyint(1) DEFAULT '0',
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`code_id`),
  UNIQUE KEY `uni_oauth2_codes_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 6.3 `oauth2_tokens`（访问令牌，审计用）

```sql
CREATE TABLE `oauth2_tokens` (
  `token_id` int NOT NULL AUTO_INCREMENT,
  `app_id` varchar(100) DEFAULT NULL,
  `user_id` int DEFAULT NULL,
  `scope` varchar(100) DEFAULT NULL,
  `expires_at` datetime DEFAULT NULL,
  `is_active` tinyint(1) DEFAULT '1',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  `access_token` text,
  `refresh_token` text,
  PRIMARY KEY (`token_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

> `access_token` 是明文 JWT；`refresh_token` 当前恒为空字符串。
> 令牌校验走 JWT 签名 + `oauth2_apps.is_active` + 用户存在性，不查询本表的 `is_active`。
> 因此**直接改 `oauth2_tokens.is_active=0` 不能撤销令牌**；要立即失效只能停用应用或删除用户。

### 6.4 `oauth2_auth_records`（授权记录）

```sql
CREATE TABLE `oauth2_auth_records` (
  `record_id` int NOT NULL AUTO_INCREMENT,
  `user_id` bigint DEFAULT NULL,
  `app_id` varchar(100) DEFAULT NULL,
  `app_name` varchar(100) DEFAULT NULL,
  `scope` varchar(100) DEFAULT NULL,
  `ip_address` varchar(45) DEFAULT NULL,
  `user_agent` varchar(500) DEFAULT NULL,
  `status` enum('authorized','expired') DEFAULT 'authorized',
  `authorized_at` datetime DEFAULT NULL,
  `expires_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`record_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

> 当前没有定时任务把过期记录改成 `expired`；它主要用于用户端「授权记录」展示（`GET /api/auth/connect/records`）。

### 6.5 `sso_bindings`

见 4.7。

### 6.6 `refresh_tokens`

```sql
CREATE TABLE `refresh_tokens` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` int NOT NULL,
  `token` varchar(255) NOT NULL,
  `expires_at` datetime NOT NULL,
  `is_used` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `token` (`token`),
  KEY `idx_refresh_tokens_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 6.7 `users`（只列登录相关列）

```sql
CREATE TABLE `users` (
  `userID` int NOT NULL AUTO_INCREMENT,
  `phone` char(20) DEFAULT NULL,
  `email` varchar(255) DEFAULT NULL,
  `password` varchar(255) DEFAULT NULL,   -- bcrypt hash
  `name` varchar(50) DEFAULT NULL,
  `intro` varchar(255) DEFAULT NULL,
  `banend` datetime DEFAULT NULL,
  `registered_at` datetime DEFAULT NULL,
  `avatar_url` varchar(255) DEFAULT NULL,
  `identity` enum('student','teacher','organization') DEFAULT 'student',
  `user_type` varchar(16) DEFAULT 'market',
  PRIMARY KEY (`userID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

> 只读参考。注册必须走 `/api/auth/register`（邮箱验证码 + CDKey + bcrypt），不要直插 users。

### 6.8 `cd_keys`

```sql
CREATE TABLE `cd_keys` (
  `cdkeyID` int NOT NULL AUTO_INCREMENT,
  `content` char(9) DEFAULT NULL,
  `used` tinyint(1) DEFAULT NULL,
  `createdtime` datetime DEFAULT NULL,
  `usedtime` datetime DEFAULT NULL,
  PRIMARY KEY (`cdkeyID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 6.9 `user_api_tokens`（PAT）

```sql
CREATE TABLE `user_api_tokens` (
  `token_id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `token_hash` varchar(64) NOT NULL,
  `token_prefix` varchar(16) NOT NULL,
  `name` varchar(64) NOT NULL,
  `scopes` varchar(255) NOT NULL,
  `last_used_at` datetime DEFAULT NULL,
  `is_active` tinyint(1) NOT NULL DEFAULT '1',
  `created_at` datetime NOT NULL,
  `updated_at` datetime NOT NULL,
  PRIMARY KEY (`token_id`),
  UNIQUE KEY `idx_user_api_tokens_token_hash` (`token_hash`),
  KEY `idx_user_api_tokens_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 6.10 运维清理建议（非自动）

```sql
-- 过期授权码：保留 1 天便于排查，之后可删
DELETE FROM oauth2_codes
WHERE expires_at < NOW() - INTERVAL 1 DAY;

-- 过期 OAuth2 令牌：标记失效（本表不参与鉴权，仅审计）
UPDATE oauth2_tokens
SET is_active = 0
WHERE expires_at < NOW() AND is_active = 1;

-- 过期授权记录：按留存策略归档/删除
DELETE FROM oauth2_auth_records
WHERE authorized_at < NOW() - INTERVAL 180 DAY;

-- 已使用/过期的 refresh token：按留存策略清理
DELETE FROM refresh_tokens
WHERE expires_at < NOW() - INTERVAL 7 DAY;
```

### 6.11 OAuth 应用中文乱码：排查与修复

**典型现象：** 授权页应用名/描述显示成 `UniSSO ç»Ÿä¸€èº«ä»½è®¤è¯`、`iWiki æµ‹è¯•ç«™` 这类字符。

**根因：** 用 `mysql` CLI 手工写库时连接字符集是 `latin1`，UTF-8 中文被二次编码。
管理端 API / Go 程序写入不会出现这个问题。

**先预览（不要直接全表 UPDATE）：**

```sql
SELECT id, app_id, app_name,
       CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4) AS fixed_name,
       description,
       CONVERT(BINARY(CONVERT(description USING latin1)) USING utf8mb4) AS fixed_desc
FROM oauth2_apps
WHERE app_name <> CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4)
   OR description <> CONVERT(BINARY(CONVERT(description USING latin1)) USING utf8mb4);
```

确认 `fixed_*` 是可读中文后，按明确的主键修复：

```sql
UPDATE oauth2_apps
SET app_name   = CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4),
    description = CONVERT(BINARY(CONVERT(description USING latin1)) USING utf8mb4)
WHERE id IN (12, 21, 28);   -- 换成预览出的 id
```

`oauth2_auth_records.app_name` 是授权时快照，也要一起修：

```sql
SELECT DISTINCT app_name,
       CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4) AS fixed_name
FROM oauth2_auth_records
WHERE app_name <> CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4);

UPDATE oauth2_auth_records
SET app_name = CONVERT(BINARY(CONVERT(app_name USING latin1)) USING utf8mb4)
WHERE app_name = '<预览出的乱码原文>';
```

**更稳妥的修复方式**（不依赖连接字符集）：把正确文本先转成 UTF-8 十六进制，再写入：

```sql
UPDATE oauth2_apps
SET app_name = CONVERT(UNHEX('556E6953534F20E7BB9FE4B880E8BAABE4BBBDE8AEA4E8AF81') USING utf8mb4)
WHERE id = 28;
```

**预防：**

- 手工执行 SQL 时始终加 `--default-character-set=utf8mb4`；
- 或在会话开头执行 `SET NAMES utf8mb4;`；
- 插入后立即用 `SELECT id, app_name, HEX(app_name)` 检查，正常中文的十六进制应以 `E4~E9` 开头。

---

## 7. 接入验收清单

### 7.1 OAuth2 接入（方案 A）

- [ ] `oauth2_apps` 中存在 `(app_id, redirect_uri)` 且 `is_active=1`。
- [ ] 授权页 URL 使用 `appid`、`redirect_uri`、`response_type=code`、`scope=read_basic`、`state`。
- [ ] 未登录用户访问授权页 → 跳集市登录 → 登录后回到授权页。
- [ ] 授权页展示正确的应用名/图标/描述。
- [ ] 点击「允许」后，浏览器回到 `redirect_uri`，URL 带 `code` 与 `state`。
- [ ] 回调校验 `state`，且 state 一次性消费。
- [ ] 服务端 `POST /api/auth/connect/token` 成功拿到 300 秒 token。
- [ ] 服务端 `POST /api/auth/connect/userinfo` 拿到 `user_id`/`name`/`email`。
- [ ] 本地按 `market_user_id` upsert，不重复建用户。
- [ ] 二次登录同一个集市用户，本地 `market_user_id` 不变。
- [ ] 重复使用同一个 code → 404；等待 5 分钟后 → 400。
- [ ] 错误 `app_secret` → 401；`is_active=0` → 401。
- [ ] 并发首次登录不会产生两条本地用户（唯一键 + 冲突回读）。
- [ ] 前端/日志/URL 中找不到 `app_secret` 与集市 `access_token`。
- [ ] 用户从你的平台退出后，重新登录仍映射到同一个 `market_user_id`。

### 7.2 密码登录（方案 B）

- [ ] AES 测试向量通过：`secret123` → `ZQwiKcmlFoNOeBdvHoUOGQ==`。
- [ ] 登录成功拿到 `token` + `refresh_token`。
- [ ] `Authorization: Bearer <token>` 可访问 `/api/auth/info`。
- [ ] refresh 成功换到新 token + 新 refresh，旧 refresh 再用返回 401。
- [ ] 错误密码返回 400；不存在邮箱返回 422；短密码返回 422。
- [ ] 客户端退出后清除两个 token。

### 7.3 SSO（方案 C）

- [ ] `SSO_ENABLED=true` 且六个 SSO 环境变量齐全。
- [ ] UniSSO 侧注册的回调是 `https://ssemarket.cn/api/sso/callback`。
- [ ] 未绑定用户走完 UniSSO → 绑定页 → 输入集市邮箱密码 → 登录成功。
- [ ] 已绑定用户再次 SSO → 直接登录。
- [ ] 已登录用户在设置页发起绑定 → 成功后 status 显示 bound。
- [ ] 解绑必须输入正确密码；错误密码 401。
- [ ] 同一 SSO 绑第二个账号 → 409。
- [ ] 同一集市账号绑第二个 SSO → 409。
- [ ] Redis 中的 state/ticket 消费后立即消失（GETDEL）。

### 7.4 用户注册（`/api/auth/register`）

- [ ] `POST /api/auth/validateEmail` 对未注册邮箱返回 200，邮件能在 5 分钟内收到 6 位验证码。
- [ ] 同一邮箱 5 分钟内重复发送被拒（400「验证码已发送，请5分钟后再试」）。
- [ ] 错误验证码返回 400「验证码错误」；连续错 5 次后必须重新发送。
- [ ] 正确填写 name/email/password/password2/valiCode/CDKey 后返回 200 + token + refresh_token。
- [ ] 用注册返回的 token 调 `GET /api/auth/info` 能拿到 `userID`，且 `identity=student`。
- [ ] 同一邮箱再次注册被拒（422）。
- [ ] 同一 CDKey 第二次注册被拒（400 激活码错误）。
- [ ] `phone=""` 时注册成功，数据库里是随机 11 位手机号。
- [ ] `phone` 传非 11 位时被拒（注意文案可能显示为「激活码错误」，见 3.5.7）。
- [ ] 注册后 `refresh_tokens` 有对应记录，`/api/auth/refresh` 能刷新。
- [ ] 新用户 `registered_at` 已写入，`coins=0`。
- [ ] 不对同一邮箱或同一 CDKey 做并发提交（当前没有唯一索引/原子占用）。

---

## 8. 常见坑位与 FAQ

### Q1：授权页参数到底叫 `appid` 还是 `app_id`？

- **授权页 URL**：`appid`
- **后端 API（authorize/check）**：JSON 字段 `app_id`
- **token 交换表单**：`app_id`

这是历史命名不一致，按上表发送即可。

### Q2：为什么 `POST /api/auth/connect/check` 直接调用返回 401？

它在 `auth` 路由组下，要求**当前集市用户**的 `Authorization: Bearer`。它只供授权页展示应用信息，不是公开的应用查询接口。第三方后端不要匿名调用它。

### Q3：redirect_uri 明明一样，为什么还提示不匹配？

逐字符检查：

- 协议 `https` vs `http`
- 域名大小写、端口
- 末尾 `/`
- 是否带了 `?` 查询串（**不要带**，后端是字符串拼接 `?code=`）
- URL 编码前后是否一致（数据库存的是解码后的值）

### Q4：OAuth 的 access_token 能不能存起来下次用？

不能。只有 5 分钟且没有 refresh。正确做法是每次登录都重新走授权，换到 `user_id` 后建你自己的长期会话。

### Q5：能不能用同一个 code 重试？

不能。code 一次性；网络超时后重试通常 404。让用户重新授权。

### Q6：为什么 token 兑换成功，但 userinfo 401？

- 超过 5 分钟；
- token 被截断/多了空格；
- 应用被停用；
- 用户被删除；
- 用了集市用户 JWT 而不是 OAuth2 access_token（用户 JWT 没有 `AppID`，OAuth 中间件会拒绝）。

### Q7：能不能按 email 自动关联已有本地账号？

不要。OAuth 返回的 email 只用于展示/联系；用 `user_id` 做唯一键。集市自己的 SSO 绑定也明确禁止按 email 自动关联。

### Q8：一个 app_id 可以配置多个回调地址吗？

可以。`oauth2_apps` 的唯一键是 `(app_id, redirect_uri)`，一个 `app_id` 可以有多行。但要注意：

- token 兑换只按 `app_id + app_secret` 查，**不按 code 的 redirect_uri 再校验 secret**；
- 如果多行共用同一个 `app_id` 但不同 secret，任意一行的 secret 都可能通过；
- 生产/测试建议用不同 `app_id`，避免密钥与回调串用；
- 管理端删除按 `app_id` 会删掉全部行。

### Q9：能不能只停用某个 token，不影响其他用户？

没有按 token 撤销的接口。可以：

- 停用整个应用：`UPDATE oauth2_apps SET is_active=0 WHERE app_id=?`（该应用所有新老 token 都失效）；
- 删除用户：该用户所有 token 失效；
- 等 5 分钟自然过期。

### Q10：`alt_redirect_uri` / `alt_app_secret` 是什么？

历史遗留列，当前代码不读取。不要依赖。

### Q11：OAuth2 支持 PKCE 吗？

不支持。它是机密客户端模式，必须有 `app_secret`。不要把 secret 放进移动 App/SPA 的公开代码里；移动端/SPA 请用「后端换 token」的模式，或不要把集市 OAuth 直接暴露给不可信客户端。

### Q12：SSO 能替没有集市账号的人自动注册吗？

不能。当前 `/api/sso/*` 的设计是「先有集市账号，再用 SSO 绑定/登录」，不会免 CDKey 注册。未绑定用户会被引导到绑定页，要求输入已有集市账号的邮箱密码。

### Q13：响应里 HTTP 200 但 code 400 是什么情况？

部分管理端接口使用 `response.Fail`，HTTP 状态固定 200，业务码在 body 的 `code`。统一以 body 的 `code` 为准。

### Q14：登录接口为什么返回「密码不能少于6位」，我明明传了正确密码？

99% 是 AES 密文不对（Base64 非法、IV 错、模式/填充不对，或者直接传了明文）。用 3.1 的测试向量先验证加密实现。

### Q15：SSO 的 `sub` 能不能解析出学号/用户名？

不能。`sub` 是不透明 UUID；需要展示名时用 `preferred_username`/`email`，且仅作展示。

---

## 9. 参考实现（集市内已有接入）

iWiki 已经按方案 A 接入，可作为代码参考：

| 文件 | 作用 |
|---|---|
| `iWiki/code/iwiki/apps/iwiki-api/src/config/oauth.ts` | 读取 `OAUTH_*` 环境变量，区分主域/子域回调与 secret |
| `.../services/oauth-client.ts` | 换 token、取 userinfo、check 应用 |
| `.../services/oauth-params.ts` | 拼接授权 URL、校验允许的回调地址 |
| `.../services/oauth-session.ts` | state 生成与 HMAC 校验（一次性） |
| `.../services/oauth-user.ts` | 按 `oauth_id` upsert 本地用户 |
| `.../routes/auth.ts` | `/oauth/login-url`、`/oauth/callback`、`/oauth/token`、`/oauth/userinfo` |
| `.../db/init.ts` | `users.oauth_id` 列与唯一索引迁移 |
| `.../apps/iwiki-web/src/views/OAuthCallbackView.vue` | 前端回调页 |

**注意参考实现里的两个细节，不要照抄错：**

1. 集市 `/api/auth/connect/check` 需要用户 JWT；iWiki 后端 `checkOAuthApp()` 在未带 token 时调用会 401。授权页自身会带用户 token 调用，第三方后端不要把它当公开接口。
2. iWiki 的 token 表单同时发了 `appid` 和 `app_id`；集市只读 `app_id`。新接入直接发 `app_id` 即可。

### 9.1 编码 Agent 落地清单（可直接照做）

**要新增的配置（服务端环境变量）：**

```ini
MARKET_WEB_BASE=https://ssemarket.cn/new
MARKET_API_BASE=https://ssemarket.cn/api
MARKET_OAUTH_APP_ID=<集市管理员提供的 app_id>
MARKET_OAUTH_APP_SECRET=<集市管理员提供的一次性 secret>
MARKET_OAUTH_REDIRECT_URI=https://your-app.example.com/auth/market/callback
SESSION_SECRET=<你自己的会话密钥>
```

**要实现的文件/模块：**

1. `config/oauth.*`：读取上述环境变量，导出 `buildAuthorizeUrl(state)`；
2. `routes/auth.*`：
   - `GET /auth/market`：生成 state → 302 到 `${MARKET_WEB_BASE}/connect?...`；
   - `GET /auth/market/callback`：校验 state → 服务端换 token → 取 userinfo → upsert 本地用户 → 建会话 → 302 到业务首页；
3. `services/market-oauth.*`：封装 `exchangeCode(code)` 与 `fetchUserInfo(accessToken)`，统一处理 4xx/5xx 与 body `code`；
4. 数据库迁移：建 `market_users`，`market_user_id` 唯一；
5. 前端登录页：加「使用 SSE Market 登录」按钮，跳 `/auth/market`；
6. 日志：记录 `market_user_id`、登录时间、IP、UA；**禁止**记录 `app_secret` 和 `access_token`。

**必须补的测试：**

- state 正确 + code 有效 → 登录成功，本地用户按 `market_user_id` 幂等；
- state 错误/复用 → 拒绝，且不兑换 code；
- code 二次兑换 → 404；
- `app_secret` 错误 → 401；
- `market_user_id` 相同、name/email 变化 → 更新展示字段，不新建用户；
- 并发首次登录 → 只有一条本地用户；
- 应用停用（`is_active=0`）→ 授权页/token 交换失败。

**禁止事项：**

- 禁止把 `app_secret` 放进前端 bundle、移动端包、URL、日志；
- 禁止把集市 `access_token` 存进 localStorage / 长期数据库；
- 禁止按 email 自动合并账号；
- 禁止用 `POST /api/auth/connect/check` 做服务端应用校验（它需要用户登录态）；
- 禁止把 `/api/sso/*` 当成第三方登录接口使用。

---

## 10. 一页速查

```text
【第三方登录：OAuth2】
1. 管理员建应用 → app_id / app_secret / redirect_uri
2. 浏览器 → https://ssemarket.cn/new/connect?appid=...&redirect_uri=...&response_type=code&scope=read_basic&state=...
3. 回调收到 ?code=...&state=...（校验 state）
4. 服务端 POST https://ssemarket.cn/api/auth/connect/token
       form: code, app_id, app_secret
5. 服务端 POST https://ssemarket.cn/api/auth/connect/userinfo
       header: Authorization: Bearer <access_token>
6. 用 data.user_id upsert 本地用户 → 发自己的会话
7. access_token 5 分钟、无 refresh；用完即弃

【集市用户注册】
1. POST /api/auth/validateEmail
   {"email":"...","mode":0}        // mode 0=未注册；1=已注册
   → 邮箱收到 6 位验证码，5 分钟有效
2. POST /api/auth/register
   {"name":"...","email":"...",
    "password":"<AES base64>","password2":"<AES base64>",
    "phone":"","valiCode":"123456","CDKey":"1a2b3c4d"}
   → data.token (7d) + data.refresh_token (30d, 一次性)
   // CDKey 必填；phone 可空；密码明文 ≥6 位
   // 另：平台中转建号 POST /api/open/users（SM2 验签，需平台开通）

【集市本站账号密码登录】
POST /api/auth/login
{"email":"...","password":"<AES-128-CBC base64>"}
→ data.token (7d) + data.refresh_token (30d, 一次性)
刷新：POST /api/auth/refresh {"refresh_token":"..."}

【集市本站 SSO】
GET /api/sso/login（整页跳转）→ UniSSO → /api/sso/callback
→ /new/auth/sso/result?ticket=...
POST /api/sso/exchange {"ticket":"..."}
绑定：POST /api/sso/bind {"ticket","email","password(AES)"}

【数据库】
集市：oauth2_apps / oauth2_codes / oauth2_tokens / oauth2_auth_records
      sso_bindings / refresh_tokens / user_api_tokens
你自己的平台：market_users(market_user_id UNIQUE) + 你自己的 sessions
```

---

*最后更新：2026-10-25。若代码有变更，请同步更新本文的端点、字段与 DDL。*
