# SSE Market 十大热门接口与请求示例

本文档汇总日常访问与压测中最常用的 **10 个 API**，含路径、鉴权、请求体与 `curl` 示例。

---

## 0. 通用约定

### 入口地址

| 环境 | Base URL | 说明 |
|------|----------|------|
| **生产** | `https://ssemarket.cn/api` | 经 nginx，路径前缀 `/api` |
| **压测** | `https://ssemarket.cn/loadtest/api` 或 `http://<服务器IP>:8888/api` | 独立栈，数据与生产隔离 |

下文 `{BASE}` 表示上述 Base URL（**不含**末尾 `/`）。

### 鉴权

除登录、注册等 `noauth` 接口外，需在 Header 携带 JWT：

```http
Authorization: Bearer <access_token>
Content-Type: application/json
```

登录成功后从 `data.token` 取值（见接口 1）。

### 响应格式（两类）

| 类型 | 接口 | 成功时 Body |
|------|------|-------------|
| **包装 JSON** | 登录、发帖、点赞等 | `{"code":200,"data":{...},"msg":"..."}` |
| **直出 JSON** | browse、showDetails、showPcomments | HTTP 200，body 为**数组或对象**，无 `code/data/msg` 包装 |

### 密码加密（登录 / 注册）

前端使用 AES 加密后再 Base64 传输，服务端 `util.Decrypt` 解密。  
测试密码 `88888888` 的密文示例：

```
hAiGv5/B92BEtQVqtetxEw==
```

---

## 1. 登录 Login

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/login` |
| 鉴权 | 否 |

**请求 Body**

```json
{
  "email": "2215020897@qq.com",
  "password": "hAiGv5/B92BEtQVqtetxEw=="
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"2215020897@qq.com","password":"hAiGv5/B92BEtQVqtetxEw=="}'
```

**成功响应示例**

```json
{
  "code": 200,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
  },
  "msg": "登录成功"
}
```

---

## 2. 帖子列表 Browse（首页 Feed）

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/browse` |
| 鉴权 | 是 |

**请求 Body**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `searchsort` | string | 否 | `home`（默认）/ `save` / `history` |
| `limit` | int | 否 | 每页条数，默认 15 |
| `offset` | int | 否 | 分页偏移 |
| `userTelephone` | string | 建议 | 当前用户手机号 |
| `partition` | string | 否 | 分区，空或 `主页` |
| `searchinfo` | string | 否 | 搜索关键词 |
| `orderBy` | string | 否 | `time_desc` 等 |

**curl**

```bash
export TOKEN="<登录得到的 token>"

curl -s -X POST "{BASE}/auth/browse" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "searchsort": "home",
    "limit": 15,
    "offset": 0,
    "userTelephone": "07799520231"
  }'
```

**成功响应**：JSON **数组**（每项含 `PostID`、`Title`、`Content` 摘要、`IsLiked`、`IsSaved` 等）。

---

## 3. 帖子详情 ShowDetails

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/showDetails` |
| 鉴权 | 是 |

**请求 Body**

```json
{
  "postID": 5056,
  "postType": "post",
  "userTelephone": "07799520231"
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/showDetails" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "postID": 5056,
    "postType": "post",
    "userTelephone": "07799520231"
  }'
```

**成功响应**：JSON **对象**（含完整 `Content`、`Browse` 实时浏览数、`IsLiked`、`IsLottery` 等）。  
每次成功访问会 Redis 递增浏览数。

---

## 4. 评论列表 showPcomments

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/showPcomments` |
| 鉴权 | 是 |

> 路由注册为 `api/auth/showPcomments`（与 `/auth/showPcomments` 等价，nginx 统一走 `/api/auth/...`）。

**请求 Body**

```json
{
  "postID": 5056,
  "postType": "post",
  "userTelephone": "07799520231"
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/showPcomments" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"postID":5056,"postType":"post","userTelephone":"07799520231"}'
```

**成功响应**：JSON **数组**（主评论 + `SubComments` 子评论树）。

---

## 5. 点赞 / 取消点赞 UpdateLike

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/updateLike` |
| 鉴权 | 是 |

**请求 Body**

```json
{
  "UserTelephone": "07799520231",
  "PostID": 5056
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/updateLike" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"UserTelephone":"07799520231","PostID":5056}'
```

**说明**：toggle 语义，已赞则取消。字段名首字母大写与前端历史约定一致。

---

## 6. 收藏 / 取消收藏 UpdateSave

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/updateSave` |
| 鉴权 | 是 |

**请求 Body**

```json
{
  "UserTelephone": "07799520231",
  "PostID": 5056
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/updateSave" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"UserTelephone":"07799520231","PostID":5056}'
```

---

## 7. 发帖 Post

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/post` |
| 鉴权 | 是 |

**请求 Body**

```json
{
  "UserTelephone": "07799520231",
  "Title": "测试标题",
  "Content": "帖子正文内容",
  "Partition": "日常",
  "Photos": "",
  "TagList": "标签1|标签2",
  "isPrivate": false
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/post" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "UserTelephone": "07799520231",
    "Title": "测试标题",
    "Content": "帖子正文内容",
    "Partition": "日常",
    "Photos": "",
    "TagList": "",
    "isPrivate": false
  }'
```

**成功响应**

```json
{
  "code": 200,
  "data": null,
  "msg": "发帖成功"
}
```

---

## 8. 发表评论 PostPcomment

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/postPcomment` |
| 鉴权 | 是 |

**请求 Body**

```json
{
  "UserTelephone": "07799520231",
  "PostID": 5056,
  "Content": "这是一条主评论"
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/postPcomment" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "UserTelephone": "07799520231",
    "PostID": 5056,
    "Content": "这是一条主评论"
  }'
```

---

## 9. 用户信息 GetInfo

| 项 | 值 |
|----|-----|
| 方法 | `POST` |
| 路径 | `/auth/getInfo` |
| 鉴权 | 是 |

**请求 Body**（`phone` 与 `userID` 二选一或同时传）

```json
{
  "phone": "07799520231",
  "userID": 278
}
```

**curl**

```bash
curl -s -X POST "{BASE}/auth/getInfo" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"phone":"07799520231","userID":278}'
```

**成功响应**：直出 JSON 对象（`userID`、`name`、`score`、`avatarURL` 等；查看本人时含 `email`）。

**相关接口**：`GET /auth/info` — 返回当前登录用户（包装格式 `code/data/user`）。

---

## 10. 通知列表 GetNotice

| 项 | 值 |
|----|-----|
| 方法 | `GET` |
| 路径 | `/auth/getNotice` |
| 鉴权 | 是 |

**Query 参数**

| 参数 | 说明 |
|------|------|
| `pageSize` | 每页条数 |
| `requireID` | 分页游标（上一页最后一条 notice ID） |
| `read` | `0` 未读 / `1` 已读 等 |

**curl**

```bash
curl -s -G "{BASE}/auth/getNotice" \
  -H "Authorization: Bearer $TOKEN" \
  --data-urlencode "pageSize=20" \
  --data-urlencode "requireID=0" \
  --data-urlencode "read=0"
```

**成功响应**：JSON 对象，含 `NoticeList`、`TotalNum`、`UnreadTotalNum` 等。

**相关接口**：`GET /auth/getNoticeNum` — 仅取未读数量。

---

## 附录 A：压测推荐组合

压测时优先覆盖读多写少路径（与线上一致）：

```text
login → browse(home, offset=0) → showDetails(HOT_POST_IDS) → showPcomments
```

- 使用 `HOT_POST_IDS` 避免随机 postID 404（见 `SSE_market_server_test/scripts/export_hot_post_ids.sh`）。
- browse / showDetails / showPcomments 为**直出 JSON**，判定成功用 HTTP 200 + JSON  parse，勿按 `code==200` 包装格式检查。

## 附录 B：完整调用示例（生产）

```bash
BASE="https://ssemarket.cn/api"

# 1. 登录
TOKEN=$(curl -s -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"2215020897@qq.com","password":"hAiGv5/B92BEtQVqtetxEw=="}' \
  | jq -r '.data.token')

# 2. 首页列表
curl -s -X POST "$BASE/auth/browse" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"searchsort":"home","limit":15,"offset":0,"userTelephone":"07799520231"}' | jq 'length'

# 3. 帖子详情
curl -s -X POST "$BASE/auth/showDetails" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"postID":5056,"postType":"post","userTelephone":"07799520231"}' | jq '.Title'
```

## 附录 C：接口热度说明

| 排序 | 接口 | 典型场景 |
|------|------|----------|
| 1 | Login | 会话建立 |
| 2 | Browse | 首页/收藏/历史列表 |
| 3 | ShowDetails | 帖子详情页 |
| 4 | showPcomments | 详情页评论树 |
| 5 | UpdateLike | 点赞交互 |
| 6 | UpdateSave | 收藏交互 |
| 7 | Post | 发帖 |
| 8 | PostPcomment | 评论 |
| 9 | GetInfo | 个人主页/他人资料 |
| 10 | GetNotice | 消息通知 |

压测第十八轮单接口 QPS 参考（200 连接、2min）：showDetails ≈ 875 > showPcomments ≈ 494 > browse ≈ 370。
