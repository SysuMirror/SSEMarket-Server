# 工作台应用（Workbench App）接口文档

- **日期**: 2026-08-31
- **状态**: 已实现（平台中转接口部分）；管理端 CRUD 为待开发
- **关联前端**: `sse_market_new_client/newSSE/src/views/WorkbenchView.vue`
- **鉴权规范**: 参见 `docs/身份验证说明.md`（x-model-Authorization / SM2）

## 1. 背景

工作台页面（`/workbench`）目前是纯静态入口页：

- "常用"区应用来自前端 `commonEntries` 硬编码数组（排行榜 / 小秋 / 打分 / 简历 / 商城 / iWiki / 课程专区 / 学人汇）
- "全部分区"来自前端常量 `CONTENT_PARTITIONS`

本接口让集市开发者平台上的模块（经平台中转）把自家站点注册进工作台；也支持管理端后续扩展 CRUD。

## 2. 已实现：平台中转接口（/api/open/*）

### 2.1 接口列表

| # | 方法 | 路径 | 鉴权 | 说明 |
| - | ---- | ---- | ---- | ---- |
| 1 | POST | `/api/open/workbench/apps` | 平台验签 | 添加工作台站点 |
| 2 | GET  | `/api/open/workbench/apps` | 平台验签 | 查询可见站点列表 |
| 3 | POST | `/api/open/users` | 平台验签 | 注册用户（平台建号，免验证码/CDKey） |

### 2.1.1 POST /api/open/users —— 平台建号

鉴权与工作台接口**完全一致**（`x-model-authorization` + `x-model-query-time`，见 §2.2）。

**请求体**（JSON）：

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| `email` | string | ✅ | 用户邮箱，全库唯一 |
| `password` | string | ✅ | 明文密码，≥6 位（经平台内网中转） |
| `name` | string | ❌ | 昵称，缺省随机英文昵称 |
| `phone` | string | ❌ | 11 位手机号，缺省随机生成 |

**响应**（200）：

```json
{
  "code": 200,
  "data": {
    "user": {
      "userID": 1305,
      "name": "zhangsan",
      "email": "zhangsan@example.com",
      "phone": "13800138000",
      "accessToken": "eyJhbGciOi...",
      "refreshToken": "eyJhbGciOi..."
    }
  },
  "msg": "用户已创建"
}
```

**错误**：

| HTTP | 场景 |
| ---- | ---- |
| 400 | `email 不合法` / `password 不能少于 6 位` / `phone 必须为 11 位数字` / `该邮箱已存在` / `该手机号已存在` |
| 401 | 平台验签失败（同 §2.2 各失败原因） |
| 500 | 数据库异常等系统错误 |

**平台侧登记**：在开发者平台「模块管理 → 对外 API」登记 `POST /api/open/users`（上游地址 `http://server:8080/api/open/users`，方法 POST），经 `/api/open/invoke/:apiId` 中转调用即可。

### 2.2 鉴权方式（平台中转验签）

模块**不直接**调本服务。模块先调平台 `/api/open/invoke/:apiId`，平台转发到本服务时附带：

| 请求头 | 含义 |
|--------|------|
| `x-model-authorization` | 平台 SM2 私钥签发的 token：`base64url(slug).base64url(ts).base64url(nonce).base64url(sign)`，slug 固定 `__platform__`，签名串 `"{slug}\n{ts}\n{nonce}\n{queryTime}"`（SM3 摘要，非 DER，64 字节 r\|\|s），queryTime 取 `x-model-query-time` 头 |
| `x-model-query-time` | 平台签发调用时间（毫秒时间戳），与签名串第 4 段一致（±5 分钟窗口）；缺失时回退旧三段的兼容校验 |
| `x-invoker-module` | 真实调用方模块 slug（明文，落库 `invoker_slug` 字段） |

服务端中间件 `middleware.PlatformAuthMiddleware()` 校验：

1. 头存在且为四段 base64url
2. slug 必须为 `__platform__`
3. `ts` 与服务器偏差 ≤ 5 分钟
4. 若带 `x-model-query-time`：其为毫秒时间戳、与服务器偏差 ≤ 5 分钟、且与 `ts` 偏差 ≤ 5 分钟；签名串按四段 `{slug}\n{ts}\n{nonce}\n{queryTime}` 验签
5. SM2 验签通过（平台公钥，`hash:true, der:false` 语义；同时兼容"纯 SM3 摘要"与"标准 ZA(uid)"两种签名形态；未带 queryTime 时回退旧三段 `{slug}\n{ts}\n{nonce}`）
6. nonce 非空（平台每次转发重新签发保证一次性）

验签通过后 `x-invoker-module` 才被信任并写入 `ctx`，控制器取出作为 `InvokerSlug` 落库（`source = platform`）。

**平台公钥**（服务端读取 `PLATFORM_SM2_PUBLIC_KEY` 环境变量，未配置时回退内置值）：

```
04f01e80d1f5218eb28dc223577f74725552ea30f38dc4d2e7bb03ecbabea509d33f9815ac63a5c930fc5cbd575684ea44067c249b85d51e4afe3df2af5fc09cf0
```

> 密钥对与开发者平台 `server/.env` 的 `PLATFORM_SM2_PRIVATE_KEY` / `PLATFORM_SM2_PUBLIC_KEY` 同源；平台轮换密钥后需同步更新本服务环境变量。

### 2.3 添加站点

```
POST /api/open/workbench/apps
x-model-authorization: <平台签发 token>
x-invoker-module: <调用方模块 slug>
Content-Type: application/json
```

**请求体**：

```json
{
  "name": "失物招领",
  "icon": "tabler:search",
  "targetType": "link",
  "target": "https://lostfound.example.ssemarket.cn/",
  "groupName": "common",
  "colorBg": "#ffe4e6",
  "colorFg": "#be123c",
  "sortOrder": 90,
  "isVisible": true
}
```

| 字段 | 类型 | 必填 | 说明 |
| ---- | ---- | ---- | ---- |
| name | string | 是 | 站点名，1~20 字符，全局唯一；重复返回 409 |
| icon | string | 是 | Iconify 名称（`集合:名称`），如 `tabler:search` |
| targetType | string | 是 | `route`（站内路由，以 `/` 开头）/ `link`（站外 http(s) URL） |
| target | string | 是 | 跳转地址，≤500 字符 |
| groupName | string | 否 | 分组，默认 `common` |
| colorBg | string | 否 | 图标底色 `#RRGGBB` |
| colorFg | string | 否 | 图标前景色 `#RRGGBB` |
| sortOrder | int | 否 | 排序；≤0 时自动取当前最大值 +10 |
| isVisible | bool | 否 | 默认 `true`；`false` 表示隐藏（暂不可见） |

**响应 200**（`response.Success` 包装）：

```json
{
  "code": 200,
  "data": {
    "app": {
      "appID": 9,
      "name": "失物招领",
      "icon": "tabler:search",
      "targetType": "link",
      "target": "https://lostfound.example.ssemarket.cn/",
      "groupName": "common",
      "colorBg": "#ffe4e6",
      "colorFg": "#be123c",
      "source": "platform",
      "invokerSlug": "my-service",
      "sortOrder": 90,
      "isVisible": true,
      "createdAt": "2026-08-31T16:40:00+08:00"
    }
  },
  "msg": "站点已添加"
}
```

**错误**：

| HTTP | code | 场景 |
| ---- | ---- | ---- |
| 400 | 400 | 参数错误（body 非法 / 校验失败，msg 附具体原因） |
| 401 | 401 | 缺少或未通过 `x-model-authorization` 验签 |
| 409 | 409 | 站点名称已存在 |
| 500 | 500 | 服务端异常 |

### 2.4 查询可见站点

```
GET /api/open/workbench/apps
x-model-authorization: <平台签发 token>
x-invoker-module: <调用方模块 slug>
```

**响应 200**：`WorkbenchAppResponse` JSON 数组（仅 `is_visible=true`，按 `sortOrder` 升序），字段同 2.3。

### 2.5 平台登记方式

在本服务对外登记为"对外提供 API"时：

- 上游地址指向本服务：`POST /api/open/workbench/apps` / `GET /api/open/workbench/apps`
- 平台转发时自动重新签发（`__platform__`），模块侧无需自己实现 SM2 签名
- 多个模块可共用同一登记接口（授权 grants 控制谁能调）；每个模块的站点都会记录 `invokerSlug`，便于审计与后续按模块管理

### 2.6 实现位置（代码索引）

| 文件 | 内容 |
| ---- | ---- |
| `model/workbench_app.go` | GORM 模型，表 `workbench_apps`（`common/database.go` 已 AutoMigrate） |
| `domain/workbench.go` | 实体与入参 |
| `repository/workbench_app.go` + `repository/gorm/workbench_app.go` | 持久化 |
| `service/workbench_service.go` | 校验（名称/图标/targetType/target）、唯一性、默认值 |
| `middleware/platform_auth.go` | 平台 SM2 验签中间件 + token 解析 |
| `controller/workbench_controller.go` | `OpenWorkbenchAppCreate` / `OpenWorkbenchAppList` |
| `route/routes.go` | `openPlatform` 路由组（仅挂 `PlatformAuthMiddleware`，不走用户/管理员 JWT） |
| `_testtools/platform_auth_verify/` | 验签逻辑端到端自测（签名/验签/防篡改，`go run ./platform_auth_verify` 全部 PASS） |

## 3. 数据表

表名 `workbench_apps`（AutoMigrate 自动建表）：

| 列 | 类型 | 说明 |
| ---- | ---- | ---- |
| app_id | INT PK AUTO_INCREMENT | 主键 |
| name | VARCHAR(50) UNIQUE | 站点名 |
| icon | VARCHAR(100) | Iconify 图标 |
| target_type | VARCHAR(10) | route / link |
| target | VARCHAR(500) | 跳转地址 |
| group_name | VARCHAR(20) | 分组，默认 common |
| color_bg / color_fg | VARCHAR(20) | 图标配色 |
| source | VARCHAR(20) | `platform`（平台中转）/ `local`（站内创建） |
| invoker_slug | VARCHAR(100) | 平台中转时的真实调用方模块 slug |
| sort_order | INT | 排序，升序 |
| is_visible | TINYINT(1) | 是否可见 |
| created_at / updated_at | DATETIME | 时间戳 |

## 4. 待实现（管理端 CRUD，沿用 adminAuth 分组）

- `GET /api/auth/workbench/apps` —— 游客获取可见列表（前端工作台页用，noauth）
- `GET/POST/PUT/DELETE /api/auth/admin/workbench/apps(/:appId)` —— 管理端管理
- `PUT /api/auth/admin/workbench/apps/sort` —— 批量排序
- 前端 `WorkbenchView.vue` 改为拉接口渲染（`targetType=link` 新窗口外链，`route` 站内路由），接口异常回退内置默认列表
- 校验规则、响应包装与 2.3 一致（camelCase、`response.Success/Fail`）

## 5. 初始化种子数据（建议）

把前端现有 8 个硬编码应用写入 `workbench_apps`（`source=local`，`sortOrder` 按 10 递增）：

| name | icon | targetType | target |
| ---- | ---- | ---------- | ------ |
| 排行榜 | tabler:trophy | route | /ranking |
| 小秋 | tabler:sparkles | route | /smart-xiaoqiu |
| 打分 | tabler:star | route | /rating |
| 简历 | tabler:file-cv | route | /resume |
| 商城 | tabler:shopping-bag | route | /shop |
| iWiki | tabler:book-2 | link | https://ssemarket.cn/iwiki/ |
| 课程专区 | tabler:book | route | /course |
| 学人汇 | tabler:external-link | link | https://scholarhub.top/ |
