# 勋章系统说明

## 1. 设计目标

- **可复用**：勋章目录、发放记录、展示偏好三张表；以后加新勋章只需要
  「上传图床 → INSERT badges → 批量 INSERT user_badges」，不改代码。
- **对外挂 1 枚**：帖子/评论等作者昵称旁展示用户在个人主页选择的 1 枚勋章。
- **主页多选/不展示**：个人主页勋章墙默认展示全部已获得勋章，用户可以多选
  或明确选择「不展示」。
- **图片全部在图床**：`badges.image_url` 存图床返回的绝对 http(s) 地址。

## 2. 表结构

| 表 | 作用 |
| --- | --- |
| `badges` | 勋章目录（一行一枚）：`code` 稳定标识、名称/描述、图床地址、分类、排序、启用 |
| `user_badges` | 发放记录：`(user_id, badge_id)` 与 `(user_id, award_key)` 双重唯一，批量 INSERT 天然幂等 |
| `user_badge_prefs` | 展示偏好：`worn_badge_id`（0=不挂）、`profile_badge_ids`（空串=默认全部，`[]`=不展示） |

建表兜底 SQL：`docs/migrations/20261024_badges.sql`。代码启动时也会 AutoMigrate。

## 3. 新增一枚勋章（管理员流程）

1. 把设计好的 PNG/SVG 上传到图床（主站配置 `imagebed.*`，与后台上传头像同一个服务）：

   ```bash
   TOKEN='<IMAGEBED_API_TOKEN>'
   curl -s https://image.ssemarket.cn/api/upload \
     -H "Authorization: Bearer $TOKEN" \
     -F "file=@2027-programmers-day.png"
   ```

   返回形如：

   ```json
   {"code":0,"message":"ok","data":[{"url":"https://image.ssemarket.cn/i/images/2026/10/24/xxxx.png",
     "thumb_url":"https://image.ssemarket.cn/i/thumbs/2026/10/24/xxxx.jpg", ...}]}
   ```

   **取 `data[0].url` 原样入库**。URL 里的 `/i/images/YYYY/MM/DD/` 是图床按上传
   日期生成的目录（不是我们拼的），不要自己改日期、不要用 `thumb_url`
   （勋章需要透明背景和清晰边缘）。入库前 `curl -I` 确认 200 且
   `Content-Type: image/png`。

2. 插入目录：

   ```sql
   INSERT INTO badges (code, name, description, image_url, category, sort_order, enabled, created_at, updated_at)
   VALUES ('programmers-day-2027', '2027 程序员节', '2027 程序员节限定',
           'https://image.ssemarket.cn/i/images/2026/10/24/xxxx.png', 'festival', 100, 1, NOW(), NOW());
   ```

   - `code` 全局唯一，发放/幂等键都以它为准；改名不影响已发出的记录。
   - `category`：`anniversary`（注册周年）/ `festival`（节日）/ `special`（其它）。
   - `sort_order` 越大越靠前；`enabled=0` 可临时下架，用户已获得的记录保留。

3. 批量发放（幂等，可重复执行）：

   ```sql
   INSERT IGNORE INTO user_badges (user_id, badge_id, award_key, source, note, awarded_at, created_at)
   SELECT u.userID, b.badge_id, 'programmers-day-2027', 'auto', '2027 程序员节全站发放', NOW(), NOW()
   FROM users u
   JOIN badges b ON b.code = 'programmers-day-2027'
   WHERE u.userID > 0;
   ```

   `award_key` 建议用「事件:期次」的可读字符串，例如
   `programmers-day-2027`、`anniversary:2023:3`。同一个用户同一件事重复插入
   会被 `uk_user_badge_award` 挡住。

## 4. 注册周年发放规则

**注册时间以 `users.registered_at` 为准，不再使用 `banend`。**
历史账号的 `banend` 会被封禁/解封改写（`now+天数` / `now-1h`），曾导致
部分老用户漏发周年勋章。新注册账号在 `Create` 时写入 `registered_at`；
历史账号已按「数据库备份里的原始 banend → 当前 banend → 最早活跃时间」
取最早值回填（见 `docs/migrations/20261024_registered_at_backfill.sql`）。

自动发放由服务启动时执行一次，之后每天 00:10（容器 `TZ=Asia/Shanghai`）
扫描：

```sql
TIMESTAMPDIFF(YEAR, u.registered_at, NOW()) >= n
```

给已满 n 年的用户发放 `anniversary-n`，`awarded_at` 取「注册日 + n 年」。
当前目录有 1/2/3 周年；以后加 `anniversary-4` 等只需 INSERT `badges`，
cron 会自动覆盖（SQL 最多枚举到 10 周年）。

## 5. 前端展示语义

- `user_badge_prefs.worn_badge_id`：对外只挂 1 枚。为 0 时昵称旁不显示。
  帖子/评论组件通过 `GET /api/badges/worn?ids=1,2,3` 批量取，前端一次合并请求。
- `user_badge_prefs.profile_badge_ids`：
  - 空字符串 / 接口返回 `null`：默认展示全部已获得勋章；
  - `[]`：明确不展示；
  - `[1,4]`：只展示这两枚。
- 个人主页勋章墙：他人看到的是服务端按对方选择过滤后的列表；本人看到同样
  过滤结果 + 「管理」入口，避免「设置后自己看到的和访客不同」。
- 匿名帖作者 `userID=0`，不会请求/展示勋章，避免泄露身份。

## 6. 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth/badges` | 我的全部勋章 + 展示偏好（登录） |
| POST | `/api/auth/badges/prefs` | 保存 `{wornBadgeId, profileBadgeIds}`；`profileBadgeIds=null` 表示全部 |
| GET | `/api/badges/user/:userID` | 他人主页勋章墙（按对方偏好过滤，公开） |
| GET | `/api/badges/worn?ids=1,2,3` | 批量取对外挂载勋章（最多 100 个用户，公开） |

返回的勋章对象：

```json
{
  "badgeID": 4,
  "code": "programmers-day-2026",
  "name": "2026 程序员节",
  "description": "1024，码上相见",
  "imageURL": "https://image.ssemarket.cn/i/images/2026/10/24/xxxx.png",
  "category": "festival",
  "awardedAt": "2026-10-24T12:00:00+08:00"
}
```

## 7. 币：程序员节 10.24 与帖子投币分成

- `users.coins`、`coin_ledger.amount` 已改为 `DECIMAL(12,2)`，支持小数。
- 2026 程序员节全站发放 **10.24 币**，流水事件 `programmers_day_2026`，
  用户流水里显示为「程序员节发放」。幂等键：
  `programmers-day-2026:coin:<userID>`。
- 帖子被投币时，作者获得 **0.1 × 投币数** 的币（事件 `post_tip_income`，
  显示「帖子获投币分成」），与投币同一事务。仅真实投币，旧投票折算不参与。
- 历史修正脚本：`docs/migrations/20261024_coin_decimal_tip_fix.sql`
  （10.00 → 10.24 补差额、按 `coin_throw.coin_num` 补发作者分成）。
