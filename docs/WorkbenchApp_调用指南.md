# 工作台站点接口调用指南

> 接口规格详见 `docs/WorkbenchApp_API.md`，鉴权机制详见 `docs/身份验证说明.md`。
> 本文档只讲"怎么调"，按三种场景给出可复制的示例。

## 1. 调用链路

```
┌─────────────┐  ① x-model-Authorization(模块私钥签名)   ┌──────────────────┐
│ 模块服务     │ ──────────────────────────────────────→ │ 开发者平台         │
│ (my-service) │   POST /api/open/invoke/:apiId          │ (校验模块身份/授权) │
└─────────────┘                                          └────────┬─────────┘
                                                                  │ ② 平台用平台私钥
                                                                  │    重新签发并转发
                                                                  ▼
                                                       ┌──────────────────────┐
                                                       │ SSE_market_server     │
                                                       │ POST /api/open/       │
                                                       │      workbench/apps   │
                                                       │ (平台公钥验签 → 落库)  │
                                                       └──────────────────────┘
```

- 模块**不直接**调用本服务，统一经平台代理转发
- 本服务只认平台签发的 `x-model-authorization`（slug 固定 `__platform__`）
- 真实调用方通过 `x-invoker-module` 传递，会落库到 `invoker_slug` 字段

## 2. 前置条件（平台侧一次性配置）

1. 模块已在开发者平台注册（slug 如 `my-service`）
2. 管理员把本服务接口登记为「对外提供 API」：
   - 上游地址：`https://ssemarket.cn/api/open/workbench/apps`（本地联调用 `http://127.0.0.1:8080/api/open/workbench/apps`）
   - 方法：`POST`（添加）/ `GET`（查询可见列表）
3. 在 API 广场为调用模块授予该 API 的 scope（grants），否则平台第 6 步校验返回 403

## 3. 场景 A：模块经平台代理调用（标准方式）

### 3.1 Node.js 完整示例

```javascript
const sm2 = require('sm-crypto').sm2

const SLUG = 'my-service'              // 你的模块 slug
const PRIVATE_KEY = '<你的模块私钥>'     // 模块 API 页获取，仅存服务端
const PLATFORM = 'https://developer.ssemarket.cn'

function buildModelAuthorization() {
  const ts = String(Date.now())
  const nonce = Math.random().toString(36).slice(2, 12)
  const payload = `${SLUG}\n${ts}\n${nonce}`
  const sign = sm2.doSignature(payload, PRIVATE_KEY, { hash: true, der: false })
  const b64u = (s) => Buffer.from(s, 'utf8').toString('base64url')
  return `${b64u(SLUG)}.${b64u(ts)}.${b64u(nonce)}.${b64u(sign)}`
}

// 添加工作台站点（apiId 换成平台登记后生成的 ID）
async function addWorkbenchApp() {
  const res = await fetch(`${PLATFORM}/api/open/invoke/42`, {
    method: 'POST',
    headers: {
      'x-model-Authorization': buildModelAuthorization(),
      'content-type': 'application/json',
    },
    body: JSON.stringify({
      name: '失物招领',
      icon: 'tabler:search',
      targetType: 'link',
      target: 'https://lostfound.example.ssemarket.cn/',
      groupName: 'common',
      colorBg: '#ffe4e6',
      colorFg: '#be123c',
      isVisible: true,
    }),
  })
  console.log(res.status, await res.json())
}

addWorkbenchApp()
```

> 注意请求头大小写：模块→平台用 `x-model-Authorization`；平台转发到本服务时用小写 `x-model-authorization`（HTTP 头不区分大小写，两种都能被服务端识别）。nonce 每次必须重新生成。

### 3.2 查询可见站点

同上，把 URL 换成 `GET /api/open/invoke/<查询API的apiId>`，GET 请求无需 body。

## 4. 场景 B：本地联调（直接对本服务发请求）

跳过平台代理，直接用**平台私钥**签发 token 调本服务（适用于联调/自测）。平台私钥在开发者平台 `server/.env` 的 `PLATFORM_SM2_PRIVATE_KEY`。

```bash
# 1. 生成平台签名 token（需要 PLATFORM_SM2_PRIVATE_KEY）
TOKEN=$(node -e "
const sm2 = require('sm-crypto').sm2
const PRIV = process.env.PLATFORM_SM2_PRIVATE_KEY
const ts = String(Date.now())
const nonce = Math.random().toString(36).slice(2, 12)
const payload = '__platform__\n' + ts + '\n' + nonce
const sign = sm2.doSignature(payload, PRIV, { hash: true, der: false })
const b64u = (s) => Buffer.from(s, 'utf8').toString('base64url')
console.log([b64u('__platform__'), b64u(ts), b64u(nonce), b64u(sign)].join('.'))
")

# 2. 添加站点
curl -X POST http://127.0.0.1:8080/api/open/workbench/apps \
  -H "x-model-authorization: $TOKEN" \
  -H "x-invoker-module: my-service" \
  -H "content-type: application/json" \
  -d '{
    "name": "失物招领",
    "icon": "tabler:search",
    "targetType": "link",
    "target": "https://lostfound.example.ssemarket.cn/",
    "colorBg": "#ffe4e6",
    "colorFg": "#be123c"
  }'

# 3. 查询可见站点
curl http://127.0.0.1:8080/api/open/workbench/apps \
  -H "x-model-authorization: $TOKEN" \
  -H "x-invoker-module: my-service"
```

**成功响应**（HTTP 200）：

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

> 直接调本服务（绕过平台）时，`x-invoker-module` 头是必需的——正常转发链路里由平台自动附加；联调时手动带上以标识调用方。

## 5. 场景 C：只验验签逻辑（不启动服务、不连库）

```bash
cd /root/SSE_Market/SSE_market_server/_testtools
go run ./platform_auth_verify
```

覆盖 4 个用例：sm-crypto 两种签名形态验签、伪造 slug 拒绝、超时时间戳拒绝。输出 `ALL_PASS` 即验签逻辑正确。

## 6. 错误对照

| HTTP | msg | 原因 | 处理 |
| ---- | ---- | ---- | ---- |
| 401 | `缺少 x-model-authorization 请求头` | 平台未转发鉴权头 | 确认平台已登记该 API 为中转调用 |
| 401 | `平台验签失败（格式/时间窗/签名）` | token 非平台签发 / 公钥不匹配 / 时钟漂移 | 核对 `PLATFORM_SM2_PUBLIC_KEY` 与平台 `PLATFORM_SM2_PRIVATE_KEY` 配对；校时 NTP |
| 409 | `该站点名称已存在` | name 全局唯一冲突 | 换名或先删除旧站点 |
| 400 | `参数错误：…` | 校验失败，msg 带具体原因 | 见 WorkbenchApp_API.md 2.3 字段表 |
| 500 | `创建失败` | 数据库异常 | 查服务日志 |

## 7. 联调检查清单

- [ ] 平台侧已生成「生成认证令牌」并确认模块 grants 含该 API scope
- [ ] 本服务环境变量 `PLATFORM_SM2_PUBLIC_KEY` 已配置（或确认使用内置默认值且平台未轮换密钥）
- [ ] 服务启动后 `workbench_apps` 表已自动创建（AutoMigrate）
- [ ] 用场景 B 的 curl 跑通添加 + 查询，数据库可见 `source='platform'`、`invoker_slug` 正确
- [ ] 用场景 A 走平台代理全链路跑通
