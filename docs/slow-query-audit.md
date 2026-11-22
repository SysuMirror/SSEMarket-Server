# SSEMarket 后端慢查询与索引审计报告

> 需求单：**SK-3E5511AB**（缺陷 / 平台 market-server / normal）
> 分支：`fix/yz-sk-3e5511ab`
> 审计对象：`/home/cloud/repos/market-server`（Go + Gin + GORM v1.31 / MySQL 8.0）
> 产出性质：**只读审计**。本文只新增文档，不改任何业务逻辑、不执行 DDL。

---

## 0. 结论摘要（先看这里）

生产上最值得先处理的不是单条 SQL 写错，而是三类系统性问题：

1. **以 `LIKE '%关键词%'` 为主的搜索**：帖子、用户、小说、评分、Quest、音乐都走前后通配，
   B-Tree 索引完全用不上，数据量一上来必然是全表扫描。
2. **请求路径上的 N+1 / 无界查询**：评论树、通知列表、用户统计、用户搜索、聊天记录、
   榜单等存在“先查一批 ID，再逐条查明细/作者”的写法；其中若干查询没有 `LIMIT`。
3. **按表达式排序 / `OR` 可见性条件导致索引失效**：首页自定义排序、投币榜、
   小说列表 `COALESCE(...)` 排序、`(is_private = 0 OR userID = ?)` 等，
   只能全表扫 + filesort。

按影响面排序的 Top 问题：

| 编号 | 优先级 | 受影响接口 | 根因 | 建议动作 |
| --- | --- | --- | --- | --- |
| SQ-01 | P0 | `POST /api/auth/browse`、`POST /api/auth/getPostNum` | 首页按 `like_num/comment_num/browse_num` 排序无索引；可见性 `OR`；深分页 OFFSET | 建排序复合索引；评估拆分 `OR`；改 keyset 分页 |
| SQ-02 | P0 | `browse`、`search/users`、`novel/list`、`rating/post/list`、`quest/list` | 前后通配 `LIKE '%kw%'` 全表扫描 | MySQL 8 `FULLTEXT ... WITH PARSER ngram` 或外部搜索；短期限制候选集 |
| SQ-03 | P0 | `POST /api/auth/browse`（首页热门评论） | 一次取 15 个帖子的**全部**一级评论再在 DB 排序 | 窗口函数取每帖 Top1 + `(ptargetID, like_num)` 索引 |
| SQ-04 | P0 | `POST /api/auth/showPcomments` | 评论树冷缓存时 `1 + N` 次查询；评论无分页 | 批量 `ctargetID IN (...)`；分页/上限 |
| SQ-05 | P0 | `GET /api/auth/getNotice` | 每条通知再查发送者、评论链；每页 20 条 → 40+ SQL | 批量作者/目标；复合索引 |
| SQ-06 | P0 | `POST /api/auth/statistics` | 逐条评论查 `like_num`；聊天记录全量拉取；`SELECT *` 大字段 | SQL 聚合 + `COUNT(DISTINCT)` |
| SQ-07 | P1 | `GET /api/auth/getChatHistory`、`GET /api/auth/quest/chatPeers` | `sender OR target` + 无 `LIMIT`；Preload 额外查询；索引顺序不匹配 | 双索引 + keyset 分页 + 批量用户 |
| SQ-08 | P1 | `GET /api/auth/postRanking`、`GET /api/auth/userRanking` | 表达式排序全表扫；分组聚合全表扫；Top100 再逐条查用户 | 生成列 + 索引；批量 `IN` 查用户 |
| SQ-09 | P1 | 每小时定时任务 `CalculateAndSaveScores` | 按用户 × 帖子 × 评论逐条查询，O(用户+帖子+评论) 次往返 | 改成几条 `GROUP BY` 聚合 |
| SQ-10 | P1 | `POST /api/auth/search/users` | 候选集已全表 LIKE，展示结果又每人 2 次查询 | 批量查帖子预览 + 计数 |
| SQ-11 | P1 | `POST /api/auth/novel/list`、`novel/shelf/list` | `COALESCE(...)` 排序无法走索引；List 与 Count 同条件查两遍 | 生成列 + 排序索引；去掉重复 Count |
| SQ-12 | P1 | `POST /api/auth/novel/chapter`、`novel/comment/list` | 段评统计按 `chapter_id` 过滤但无索引；评论 List+Count 重复 | 复合索引；去掉重复 Count |

更完整的清单、SQL、EXPLAIN 验证方法与 DDL 见下文。

---

## 1. 审计范围与方法

### 1.1 覆盖范围

- 数据访问实现：`repository/gorm/*.go`（29 个文件，约 8.6k 行）
- 服务层调用路径：`service/post_service.go`、`browse_feed.go`、`comment_service.go`、
  `user_service.go`、`notice_service.go`、`chat_service.go`、`novel_service.go`、
  `recommendation_service.go`、`user_search_service.go`、`domain_services.go`
- 表结构与索引：`model/*.go` 的 GORM tag、`common/database.go` 的 `AutoMigrate` /
  `ensure*Index`、`docs/migrations/*.sql`
- 路由映射：`route/routes.go`

### 1.2 方法

1. 按“路由 → controller → service → repository”逐条还原 GORM 链式调用；
2. 根据 GORM 的 `Model` / `Select` / `Where` / `Order` / `Limit` 还原实际 SQL；
3. 结合 model tag 与迁移脚本推断当前索引；
4. 对可疑 SQL 给出 **EXPLAIN / EXPLAIN ANALYZE 验证脚本** 与期望执行计划。

### 1.3 限制（重要）

- 本次**没有连接生产库**，未实际执行 EXPLAIN，也没有采样 slow log。
  文中“当前执行计划”是基于索引与 SQL 形态的推断；最终以 DBA 在只读副本上的
  `EXPLAIN ANALYZE` 为准。
- GORM 的索引 tag 会由 `AutoMigrate` 建索引，但历史库可能存在手工索引或缺失。
  **执行任何 DDL 前必须先 `SHOW INDEX FROM <table>` 核对**，避免重复建索引。
- 项目 README 写明使用 MySQL 8.0，因此文中使用 8.0 能力
  （降序索引、生成列、窗口函数、`EXPLAIN ANALYZE`、`INVISIBLE INDEX`）。
  若实际是 5.7，`DESC` 索引与窗口函数需要改写法。

### 1.4 优先级定义

- **P0**：高频读接口，数据量增长后必然劣化，且已影响主流程（首页/详情/评论/通知）。
- **P1**：高频或定时任务，单次可能不慢，但会持续占用连接/CPU，或存在明确 N+1。
- **P2**：低频后台/管理端、当前数据量小、或主要影响写放大/可维护性。

---

## 2. 现有索引基线（推断）

> 以 `SHOW INDEX` 为准确认；下表用于避免重复建索引。

| 表 | 已存在的主要索引（推断） | 明显缺口 |
| --- | --- | --- |
| `users` | PK `userID`；`idx_users_email(email)` | `phone`、`name`、`score` 无索引 |
| `posts` | PK；`postuser(userID)`；`idx_posts_post_time(post_time)`；`idx_posts_post_kind_time(post_kind, post_time)` | 排序字段（like/comment/browse/coin）、分区、生成列均无索引 |
| `pcomments` | PK；`pcommentuser(userID)`；`pcommenttarget(ptargetID)` | `(ptargetID, like_num, pcommentID)`、`(userID, time)` |
| `ccomments` | PK；`ccommentuser(userID)`；`ccommenttarget(ctargetID)` | 暂无（批量 IN 可命中 `ctargetID`） |
| `plikes` | PK；`pliketarget`；`plikeuser`；`uk_plikes_user_target(userID,ptargetID)` | `(userID, time)` |
| `psaves` | PK；`psavetarget(ptargetID)`；`userID` 普通索引；**无 (userID,ptargetID) 唯一键** | `(userID, psaveID)` |
| `pbrowses` | PK；`pbrowsetarget`；`pbrowseuser`；`uk_pbrowses_user_target(userID,ptargetID)` | `(userID, time)` |
| `post_watches` | PK；`uk_post_watch_user_post(userID, postID)` | **`postID` 单列索引** |
| `notices` | PK；`noticereceiver(receiver)`；`noticesender(sender)` | `(receiver, read, noticeID)` |
| `chat_msgs` | PK；`idx_target_sender(targetUserID, senderUserID)`（复合） | 单列 sender、`(user,createdAt)`、`(target,sender,thread,createdAt)` |
| `comment_reactions` | PK；`uk_comment_reactions_user_target(userID,targetType,targetID)` | `(userID,targetType,reaction,targetID)` |
| `coin_ledger` | PK；`idx_coin_user_time(userID,createdAt)`；`idx_coin_target(targetType,targetID)`；`idx_coin_event`；`uk_coin_dedup` | `(userID,targetType,targetID,eventType)`、`(userID,eventType,createdAt)` |
| `coin_throw` | PK；`uk_coin_throw(userID,targetType,targetID)`；`idx_coin_throw_user`；`idx_coin_throw_target(targetID)` | `(targetType,targetID)` |
| `novels` | PK；`idx_novel_user`；`idx_novel_category`；`idx_novel_status` | `COALESCE` 排序生成列 |
| `novel_comments` | PK；`idx_novel_comment_novel(novel_id)`；`idx_novel_comment_user(user_id)` | `(chapter_id,target_type,paragraph_index)`、`(novel_id,target_type,chapter_id)`、点赞排序 |
| `novel_comment_likes` | PK；`uk_novel_comment_like(comment_id,user_id)`；`idx_novel_comment_like(comment_id)` | `(user_id, comment_id)` |
| `rating_posts` | PK；`rating_post_category`；`rating_post_user` | 搜索/排序复合索引 |
| `rating_comments` | PK；`rating_comment_post`；`rating_comment_user` | 暂无 |
| `videos` | PK；`idx_videos_admin`；`idx_videos_status_published_at(status)`；`deleted_at` | 索引名虽含 published_at，实际只有 `status` 单列 |

---

## 3. 问题详单

### SQ-01 首页列表：排序字段无索引 + 可见性 `OR`

**优先级：P0**

**接口**：`POST /api/auth/browse`（`searchsort=home` 且 `orderBy` 为
`like_desc/like_asc/comment_desc/comment_asc/browse_desc/browse_asc`）；
`POST /api/auth/getPostNum`（同筛选条件的 Count）。

**代码**：

- `repository/gorm/post.go:157-164`（`ListBrowse`）
- `repository/gorm/post.go:356-375`（`applyBrowseOrder`）
- `repository/gorm/post.go:520-522`（`applyVisibility`）
- `repository/gorm/post.go:470-518`（`buildBrowseQuery`，Count 复用同一套条件）

**还原 SQL（示例：按点赞排序）**：

```sql
SELECT postID, userID, `partition`, title, ptext, comment_num, like_num, vote_num,
       coin_num, browse_num, post_time, heat, photos, tag, post_kind, quote_price,
       quest_status, is_private, is_anonymous, is_pinned, pinned_at, pinned_expires_at
FROM posts
WHERE (is_private = 0 OR userID = ?)
ORDER BY like_num DESC, postID DESC
LIMIT 15 OFFSET 0;
```

Count 版本：

```sql
SELECT count(*) FROM posts WHERE (is_private = 0 OR userID = ?);
```

**问题**：

1. `like_num` / `comment_num` / `browse_num` 都没有索引，
   `ORDER BY 指标 DESC, postID DESC` 只能全表扫描后 filesort；
   数据量越大、翻页越深越慢。
2. `(is_private = 0 OR userID = ?)` 是跨列 `OR`。MySQL 可能做 index merge，
   也可能直接用 `idx_posts_post_time` 扫全表再过滤；在“按点赞排序”场景下
   连 `post_time` 索引也无法消除 filesort。
3. `getPostNum` 每次轮询都会对同一批条件做一次 `count(*)`；首页无筛选时
   等价于统计“公开帖 + 自己私密帖”，是全表级计数。
4. 深分页 `OFFSET`：页码越大，需要跳过的行越多。

**EXPLAIN 验证建议**：

```sql
EXPLAIN ANALYZE
SELECT postID, like_num, postID
FROM posts
WHERE (is_private = 0 OR userID = 1)
ORDER BY like_num DESC, postID DESC
LIMIT 15 OFFSET 0;
-- 关注 type=ALL / key=NULL / Extra=Using filesort / actual rows 是否接近全表
```

**建议**：

- 若产品确实需要按点赞/评论/浏览排序，给对应字段建**带排序方向的复合索引**，
  见 §4 DDL `idx_posts_private_like` 等；先只给线上实际使用的排序建，避免写放大。
- `OR` 可评估改写为 `UNION ALL`（公开帖分支 + 本人私密帖分支）或加 `is_private`
  生成列/覆盖索引，由 service 层组合；**本次不改逻辑**，建议后续单独排期。
- Count 与 List 合并：列表接口不必每次返回精确 `total`，可缓存总数或返回
  “是否还有下一页”；`getPostNum` 建议加短 TTL 缓存（Redis 已在用）。
- 深分页改 keyset：`WHERE (like_num, postID) < (?, ?)`。

---

### SQ-02 搜索：`LIKE '%关键词%'` 全表扫描

**优先级：P0**

**接口 / 代码**：

| 接口 | 代码位置 | SQL 形态 |
| --- | --- | --- |
| `POST /api/auth/browse`（`searchinfo`） | `repository/gorm/post.go:177-191`、`500-514` | `title LIKE ? OR ptext LIKE ? [OR tag LIKE ?]` |
| `POST /api/auth/search/users` | `repository/gorm/user.go:826-861` | `name LIKE ? ESCAPE '!' OR intro LIKE ? ESCAPE '!'` |
| `POST /api/auth/novel/list` | `repository/gorm/novel.go:135-150` | `novels.title LIKE ? OR novels.intro LIKE ? OR novels.author_name LIKE ?` |
| `POST /api/auth/novel/comment/list` | `repository/gorm/novel.go:513-515` | `content LIKE ?` |
| `POST /api/auth/rating/post/list` | `repository/gorm/rating.go:130-145` | `title LIKE ? OR content LIKE ?`；还会先做分类名 LIKE 子查询 |
| `POST /api/auth/quest/list` | `repository/gorm/quest.go:50-53` | `title LIKE ? OR ptext LIKE ?` |
| 管理员用户筛选 | `repository/gorm/admin.go:140-148` | `name LIKE ?`（前缀）+ `phone =` + **无 LIMIT** |
| 简历/商店/音乐 | `repository/gorm/resume.go:66`、`shop.go:37`、`controller/musicController.go:401-403` | 同类 `LIKE '%kw%'` |

**问题**：

- 前后通配符无法使用 B-Tree；`ptext` 是 `varchar(10000)`，`posts` 上的
  `title LIKE OR ptext LIKE` 会读取大量非索引列，I/O 与 CPU 都高。
- 搜索与 Count 通常成对执行（`applyBrowseFilter` 被 List/Count 复用），
  慢查询会被放大一倍。
- `SearchCandidateUsers` 还会对候选集 `ORDER BY score DESC, userID ASC`，
  在无 `(score)` 索引时叠加 filesort。

**EXPLAIN 验证建议**：

```sql
EXPLAIN ANALYZE
SELECT postID, title FROM posts
WHERE (title LIKE '%关键词%' OR ptext LIKE '%关键词%')
ORDER BY post_time DESC LIMIT 15;
-- 预期：type=ALL, key=NULL, rows≈posts 总行数
```

**建议**：

1. **中期方案**：MySQL 8 原生全文索引（中文需 ngram 解析器）：

   ```sql
   ALTER TABLE posts
     ADD FULLTEXT INDEX ft_posts_title_ptext (title, ptext) WITH PARSER ngram;
   ```

   查询改为 `MATCH(title, ptext) AGAINST (? IN BOOLEAN MODE)`。
   注意：分词/相关度语义与 `LIKE` 不同，需要产品确认；不能直接原地替换。
   用户搜索同理可对 `users(name, intro)`、`novels(title, intro, author_name)` 建全文索引。
2. **短期方案**：限制搜索范围（只搜标题、只搜最近 N 天/前 N 条），
   或把搜索请求限流并加短缓存；搜索结果不返回精确 total。
3. 管理端 `name LIKE 'x%'` 是前缀匹配，可建 `idx_users_name(name)`；
   但 `phone` 目前无索引，`showUsers` 按手机号筛选会全表扫。

---

### SQ-03 首页热门评论：一次取回全部评论再排序

**优先级：P0**

**接口**：`POST /api/auth/browse`（`attachPersonalBrowseState`，含置顶帖）。

**代码**：

- `repository/gorm/post.go:325-346`（`FindHotPcommentsByPostIDs`）
- `service/browse_feed.go:55`（每页 15 帖调用一次）

**还原 SQL**：

```sql
SELECT *
FROM pcomments
WHERE ptargetID IN (?, ?, ... 15 个 ?)
ORDER BY ptargetID, like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC;
```

**问题**：

- 目标只是“每个帖子取 1 条热门评论”，却把 15 个帖子的**所有**一级评论
  都拉到应用层，再由 Go 循环取第一条。
- 热门帖有几百条评论时，单次首页请求会传输数百行、在 DB 侧做表达式排序。
- `pcomments` 只有 `pcommenttarget(ptargetID)`，`like_num` 排序无索引。

**EXPLAIN 验证建议**：

```sql
EXPLAIN ANALYZE
SELECT * FROM pcomments
WHERE ptargetID IN (1,2,3)
ORDER BY ptargetID, like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC;
```

**建议**：

- MySQL 8 用窗口函数，只返回每帖一行：

  ```sql
  SELECT *
  FROM (
    SELECT pc.*,
           ROW_NUMBER() OVER (
             PARTITION BY ptargetID
             ORDER BY like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC
           ) AS rn
    FROM pcomments pc
    WHERE ptargetID IN (?, ?, ...)
  ) t
  WHERE t.rn = 1;
  ```

- 增加索引 `(ptargetID, like_num DESC, pcommentID)`，降低窗口排序成本。
- 首页结果可缓存（帖子列表已有缓存基础设施），或把“热门评论 ID”在发评论/点赞时维护。

---

### SQ-04 评论树：冷缓存时的 N+1 与无界查询

**优先级：P0**

**接口**：`POST /api/auth/showPcomments`（GET 语义但走 POST）。

**代码**：

- `service/comment_service.go:91-105`（`loadCommentTree`）
- `repository/gorm/comment.go:29-40`（`ListPcommentsByPostID`，无 ORDER BY / LIMIT）
- `repository/gorm/comment.go:80-91`（`ListCcommentsByPcommentID`，无 ORDER BY / LIMIT）

**执行过程**：

```sql
-- 1 次
SELECT * FROM pcomments WHERE ptargetID = ?;

-- 对每条一级评论各 1 次（N 次）
SELECT * FROM ccomments WHERE ctargetID = ?;
```

**问题**：

- 冷缓存（或缓存失效后首个请求）产生 `1 + N` 次数据库往返；
  热门帖 N 可能上百，接口尾延迟直接飙高。
- 两张表都没有分页/上限；一级评论和楼中楼全量返回，序列化与网络传输同样重。
- 评论反应（点赞/反对）还需额外 4 次批量查询（`PcommentLikedSet` 等），
  这些已经是批量写法，不是主要问题。

**EXPLAIN / 现象验证**：

- 打开 GORM 日志或 slow log，观察一次 `showPcomments` 的 SQL 条数 ≈ 一级评论数 + 5。
- 用 `SHOW SESSION STATUS LIKE 'Questions'` 前后差值统计。

**建议**：

- 把 `ListCcommentsByPcommentID` 的逐条调用改成一次
  `WHERE ctargetID IN (...) ORDER BY ctargetID, ccommentID`，在 Go 侧分组。
- 一级评论加 `ORDER BY pcommentID` + 分页（`LIMIT/OFFSET` 或 keyset），
  楼中楼默认只取前 N 条并返回 `hasMore`。
- 评论写路径已正确失效缓存；这里主要是冷启动/热点帖成本。

---

### SQ-05 通知列表：每条通知 2~3 次附加查询

**优先级：P0**

**接口**：`GET /api/auth/getNotice`、`GET /api/auth/getNoticeNum`。

**代码**：

- `service/notice_service.go:71-120`（`buildNoticeItem`）
- `repository/gorm/notice.go:43-62`（`ListPage`）
- `repository/gorm/notice.go:24-41`（Count 方法）

**还原 SQL**：

```sql
SELECT * FROM notices
WHERE receiver = ? AND `read` = ?
ORDER BY noticeID DESC
LIMIT ?;

-- 每条通知：
SELECT * FROM users WHERE userID = ?;                    -- 发送者
SELECT * FROM pcomments WHERE pcommentID = ?;            -- 评论类目标（若为 ccomment 还要再查）
SELECT * FROM ccomments WHERE ccommentID = ?;
```

**问题**：

- 每页 20 条通知 → 约 40~60 条 SQL（1 条列表 + 2~3 条/条）。
- `notices` 只有 `noticereceiver(receiver)`；查询是
  `receiver = ? AND read = ? ORDER BY noticeID DESC`，
  缺少 `(receiver, read, noticeID)` 复合索引，过滤 + 排序会退化为回表/filesort。
- `getNoticeNum` 每次做两次 `count(*)`：`receiver` 和 `receiver+read`。

**建议**：

- 批量取发送者：先收集 `sender` 集合，`WHERE userID IN (...)` 一次查回。
- 评论类通知的目标链也批量取（按类型分组 `IN` 查询）。
- 建 ``idx_notices_receiver_read_id (receiver, `read`, noticeID DESC)``；
  之后 `noticereceiver(receiver)` 成为前缀冗余索引，可在确认无其他用途后删除。
- 未读数可放 Redis 计数（已有 Redis），避免每次 `count(*)`。

---

### SQ-06 用户统计接口：逐条查点赞 + 全量聊天记录

**优先级：P0**

**接口**：`POST /api/auth/statistics`。

**代码**：

- `service/user_service.go:800-930`（`GetStatistics`）
- `repository/gorm/user.go:742-758`（`ListPostSummariesByUserID`）
- `repository/gorm/user.go:669-682`、`675-682`、`717-724`（逐条 like_num）
- `repository/gorm/user.go:766-797`（`ListChatMessagesForUser`）

**问题**：

1. **逐条查询**：

   ```sql
   SELECT pcommentID FROM pcomments WHERE userID = ?;
   -- 对每个 pcommentID：
   SELECT like_num FROM pcomments WHERE pcommentID = ?;
   ```

   `ccomment` 同理；用户评论越多，SQL 条数线性增长。
2. `ListPostSummariesByUserID` 是 `SELECT * FROM posts WHERE userID = ?`，
   把 `ptext`（最长 10000 字符）、`photos` 等大字段全部拉回，只为了统计几个计数。
3. `ListChatMessagesForUser` 拉取该用户所有聊天消息（无 `LIMIT`），
   再 `Preload("SenderUser")` / `Preload("TargetUser")` 生成 2 条额外查询，
   只为了统计“聊过天的用户数 / 消息数”。
4. 最后 `chatCount := len(chatMsgs)`，完全可以在 SQL 里 `COUNT(*)`。

**建议（后续实现，不在本单改代码）**：

```sql
-- 发帖数 / 获赞 / 浏览等一次聚合
SELECT COUNT(*) AS post_cnt, COALESCE(SUM(like_num),0) AS like_sum,
       COALESCE(SUM(browse_num),0) AS browse_sum
FROM posts WHERE userID = ?;

-- 一级评论点赞一次聚合
SELECT COUNT(*) AS cnt, COALESCE(SUM(like_num),0) AS like_sum
FROM pcomments WHERE userID = ?;

-- 楼中楼同理
SELECT COUNT(*) AS cnt, COALESCE(SUM(like_num),0) AS like_sum
FROM ccomments WHERE userID = ?;

-- 聊天消息数 / 对端人数
SELECT COUNT(*) AS msg_cnt, COUNT(DISTINCT IF(senderUserID = ?, targetUserID, senderUserID)) AS peer_cnt
FROM chat_msgs WHERE senderUserID = ? OR targetUserID = ?;
```

需要 `posts(userID)`、`pcomments(userID)`、`ccomments(userID)` 以及
`chat_msgs` 的双向索引（见 §4）。

---

### SQ-07 聊天：双向 `OR` 查询、无上限、索引顺序不匹配

**优先级：P1**

**接口**：

- `GET /api/auth/getChatHistory` → `service/chat_service.go:19-25` →
  `repository/gorm/chat.go:27-44`（`ListHistory`）
- `GET /api/auth/quest/chatPeers` → `service/chat_service.go:60-...` →
  `repository/gorm/chat.go:92-115`（`ListMessagesForUser`）
- `POST /api/auth/statistics` 也会走 `ListChatMessagesForUser`
  （`repository/gorm/user.go:766-797`，与上者逻辑重复）

**还原 SQL**：

```sql
-- 会话历史：两个方向 OR，且无 LIMIT
SELECT * FROM chat_msgs
WHERE threadAnonymous = ?
  AND ((targetUserID = ? AND senderUserID = ?)
    OR (targetUserID = ? AND senderUserID = ?))
ORDER BY createdAt;

-- 会话列表：用户参与的所有消息，且无 LIMIT
SELECT * FROM chat_msgs
WHERE senderUserID = ? OR targetUserID = ?;
-- 然后 Preload 两个 User 关联，额外 2 条 SELECT ... WHERE userID IN (...)
```

**问题**：

- `chat_msgs` 现有复合索引 `idx_target_sender(targetUserID, senderUserID)`：
  - 只有 `targetUserID` 前缀可用；
  - `senderUserID = ? OR targetUserID = ?` 无法整体使用；
  - `ORDER BY createdAt` 需要 filesort。
- 两个查询都**没有 `LIMIT`**；用户聊天历史越长，响应越大、内存越高。
- `ListMessagesForUser` 的 `Preload` 虽然批量，但仍固定多出 2 条查询；
  会话列表只需要最近一条消息 + 对端信息，当前是“全量消息再在 Go 里归并”。

**建议**：

- 增加双索引（覆盖两种访问方向）：

  ```sql
  ALTER TABLE chat_msgs ADD INDEX idx_chat_sender_created (senderUserID, createdAt, chatMsgID);
  ALTER TABLE chat_msgs ADD INDEX idx_chat_target_created (targetUserID, createdAt, chatMsgID);
  ```

- 会话历史改 keyset：`AND chatMsgID < ? ORDER BY chatMsgID DESC LIMIT 50`，
  前端反向渲染；避免 `createdAt` 相同导致分页错位。
- 会话列表用“每个对端取最新一条”的窗口函数/子查询 + `LIMIT`，
  或维护 `chat_peers` 汇总表；不要拉全量消息。

---

### SQ-08 投币榜：表达式排序、全表分组、Top N 再逐条查用户

**优先级：P1**

**接口**：`GET /api/auth/postRanking`、`GET /api/auth/userRanking`。

**代码**：`repository/gorm/rank.go:78-111`、`114-175`。

**还原 SQL**：

```sql
-- 帖子榜
SELECT postID, title, `partition`, post_time, coin_num, vote_num
FROM posts
WHERE (is_private = 0 OR is_private IS NULL)
ORDER BY (coin_num + vote_num) DESC, postID DESC
LIMIT 100;

-- 作者榜
SELECT userID, COALESCE(SUM(coin_num + vote_num), 0) AS coins
FROM posts WHERE (is_private = 0 OR is_private IS NULL)
GROUP BY userID;

SELECT user_id, COALESCE(SUM(coin_num), 0) AS coins
FROM novels GROUP BY user_id;

-- 之后对 Top ≤100 每个用户：
SELECT userID, name, avatar_url FROM users WHERE userID = ?;
```

**问题**：

- `ORDER BY (coin_num + vote_num)` 是表达式，无法用 B-Tree 索引 → 全表扫 + filesort。
- 两个 `GROUP BY` 都是全表聚合，每次榜单请求都重新计算。
- 最后 100 条 `SELECT ... WHERE userID = ?` 是典型 N+1（最多 100 次往返）。
- `UserCoinRanking` 还会把全部用户分组结果加载进内存再排序。

**建议**：

- MySQL 8 生成列 + 索引：

  ```sql
  ALTER TABLE posts
    ADD COLUMN coin_rank_score INT
      GENERATED ALWAYS AS (coin_num + vote_num) STORED;
  ALTER TABLE posts
    ADD INDEX idx_posts_coin_rank (is_private, coin_rank_score DESC, postID DESC);
  ```

  查询改写为 `ORDER BY coin_rank_score DESC, postID DESC`（等价语义）。
- 作者榜改为定时任务写汇总表（如 `user_coin_rank`），或 Redis ZSET 维护；
  接口只读 Top N。
- Top N 用户信息一次 `WHERE userID IN (...)` 查回，避免 100 次点查。

---

### SQ-09 每小时评分任务：按用户逐条查询

**优先级：P1（定时任务，但会拖慢全库）**

**位置**：`service/user_service.go:717-798`，由 `main.go:89` 的
`@every 1h` cron 与启动任务触发。

**问题**：

- 外层遍历全部用户；内层对每个用户：
  - `ListPostIDsByUserID` → 每条帖子一次 `SELECT like_num FROM posts WHERE postID = ?`
  - `ListPcommentIDsByUserID` → 每条评论一次 `SELECT like_num ...`
  - `ListCcommentIDsByUserID` → 每条楼中楼一次
  - 另有 6 个按 `IN (...)` 的统计查询
- 复杂度约为 `O(用户数 × 人均内容数)` 次数据库往返；用户和内容增长后，
  每小时任务会长时间占用连接池，直接影响线上请求。
- `RebuildRankStats`（`repository/gorm/rank.go:63-77`，`main.go:179` 启动时同步执行）
  同样是“每个用户一次 `SUM(like_num)`”，会拖慢启动。

**建议**：

- 改为集合式 SQL，一次算出所有用户的分数：

  ```sql
  SELECT userID, COALESCE(SUM(like_num), 0) AS like_sum
  FROM posts GROUP BY userID;

  SELECT userID, COALESCE(SUM(like_num), 0) AS like_sum
  FROM pcomments GROUP BY userID;

  SELECT userID, COALESCE(SUM(like_num), 0) AS like_sum
  FROM ccomments GROUP BY userID;

  SELECT userID, COUNT(*) AS post_cnt FROM posts GROUP BY userID;
  SELECT userID, COUNT(*) AS pc_cnt FROM pcomments GROUP BY userID;
  SELECT userID, COUNT(*) AS cc_cnt FROM ccomments GROUP BY userID;
  ```

  在 Go 内存中按 `userID` 合并，只对分数有变化的用户 `UPDATE`。
- 或者建离线汇总表，由定时任务增量更新。
- `RebuildRankStats` 同样用一条 `SELECT userID, SUM(like_num) FROM posts GROUP BY userID`
  替代逐用户查询。

---

### SQ-10 用户搜索：候选集全表 LIKE + 每个结果 2 次查询

**优先级**：P1

**接口**：`POST /api/auth/search/users`。

**代码**：`service/user_search_service.go:26-110`；
`repository/gorm/user.go:826-861`（候选）、`863-904`（帖子预览/计数）。

**问题**：

- 候选查询：

  ```sql
  SELECT * FROM users
  WHERE (name LIKE '%kw%' ESCAPE '!' OR intro LIKE '%kw%' ESCAPE '!')
  ORDER BY score DESC, userID ASC
  LIMIT 60;
  ```

  `%kw%` 无法走索引；`ORDER BY score` 也无索引。
- 截断到最终 `limit` 个结果后，对**每个**用户各查一次
  `ListPublicPostCards(userID, 3)` 和 `CountPublicPosts(userID)`：
  最多 2 × limit 次额外 SQL。service 注释已意识到 N+1，但只做了“先截断”优化。

**建议**：

- 用窗口函数一次取每个用户的最近 3 条帖子：

  ```sql
  SELECT * FROM (
    SELECT p.*, ROW_NUMBER() OVER (PARTITION BY userID ORDER BY post_time DESC, postID DESC) AS rn
    FROM posts
    WHERE userID IN (?, ?, ...) AND is_private = 0 AND is_anonymous = 0
  ) t WHERE t.rn <= 3;
  ```

- 计数一次 `SELECT userID, COUNT(*) FROM posts WHERE userID IN (...) AND ... GROUP BY userID`。
- 建索引 `(userID, is_private, is_anonymous, post_time DESC)`。
- 候选集搜索按 SQ-02 的全文索引方案处理。

---

### SQ-11 小说列表：`COALESCE` 排序、List/Count 重复、搜索全表

**优先级**：P1

**接口**：`POST /api/auth/novel/list`、`POST /api/auth/novel/mine`、
`POST /api/auth/novel/shelf/list`。

**代码**：`repository/gorm/novel.go:161-190`；
`controller/novel_controller.go:394-406`（List 后再 Count）。

**还原 SQL**：

```sql
SELECT * FROM novels
WHERE ...
ORDER BY COALESCE(novels.latest_chapter_at, novels.created_at) DESC, novels.novel_id DESC
LIMIT 15 OFFSET 0;

SELECT count(*) FROM novels WHERE ...;  -- 同条件再查一遍
```

书架列表则是：

```sql
SELECT novels.*
FROM novels
JOIN novel_bookshelf nb ON nb.novel_id = novels.novel_id AND nb.user_id = ?
LEFT JOIN novel_progress np ON np.novel_id = novels.novel_id AND np.user_id = ?
ORDER BY COALESCE(np.updated_at, nb.created_at) DESC, novels.novel_id DESC
LIMIT 15 OFFSET 0;
```

**问题**：

- `ORDER BY COALESCE(...)` 是表达式排序，`idx_novel_user/category/status` 都无法消除 filesort；
  `WHERE category/status/author` 过滤后仍要排序全量结果。
- 每次列表请求都执行 List + Count 两条同条件查询（Count 还会再扫一遍）。
- `keyword` 走三列 `LIKE '%kw%'`，无法用索引。

**建议**：

- 增加持久化生成列并建索引：

  ```sql
  ALTER TABLE novels
    ADD COLUMN sort_at DATETIME
      GENERATED ALWAYS AS (COALESCE(latest_chapter_at, created_at)) STORED;

  ALTER TABLE novels ADD INDEX idx_novels_sort_at (sort_at DESC, novel_id DESC);
  ALTER TABLE novels ADD INDEX idx_novels_category_sort (category, sort_at DESC, novel_id DESC);
  ALTER TABLE novels ADD INDEX idx_novels_status_sort (status, sort_at DESC, novel_id DESC);
  ALTER TABLE novels ADD INDEX idx_novels_user_sort (user_id, sort_at DESC, novel_id DESC);
  ```

  查询改为 `ORDER BY sort_at DESC, novel_id DESC`。
- 列表接口返回精确 total 的成本较高：可缓存 total 或采用“是否还有下一页”；
  若产品必须显示总数，把 Count 与 List 分到不同接口/低频刷新。
- 搜索按 SQ-02 处理。

---

### SQ-12 小说评论 / 段评：缺复合索引 + 重复 Count

**优先级**：P1

**接口**：`POST /api/auth/novel/chapter`（阅读页段落气泡）、
`POST /api/auth/novel/comment/list`。

**代码**：

- `repository/gorm/novel.go:519-544`（`ListComments` / `CountComments`）
- `repository/gorm/novel.go:547-565`（`ParagraphCommentCounts`）
- `service/novel_service.go:365-395`（List + Count 各一次）
- `service/novel_service.go:252`（章节详情调用段评统计）

**还原 SQL**：

```sql
-- 段评气泡统计
SELECT paragraph_index, COUNT(*) AS cnt
FROM novel_comments
WHERE chapter_id = ? AND target_type = 'paragraph'
GROUP BY paragraph_index;

-- 评论列表（章节/段落）
SELECT * FROM novel_comments
WHERE novel_id = ? AND target_type = ? AND chapter_id = ? AND paragraph_index = ?
ORDER BY comment_id DESC
LIMIT 15 OFFSET 0;

SELECT count(*) FROM novel_comments
WHERE novel_id = ? AND target_type = ? AND chapter_id = ? AND paragraph_index = ?;
```

**问题**：

- `novel_comments` 只有 `novel_id` 和 `user_id` 单列索引；
  `chapter_id`、`target_type`、`paragraph_index` 均无索引。
  段评统计会扫描该表全部段落评论；阅读页打开每一章都会执行。
- 评论列表用 `novel_id` 索引能过滤到整本书，但章节/段落条件只能回表过滤；
  热门书评论多时较慢。
- List 与 Count 重复执行同一套条件。

**建议**：

```sql
ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_chapter
    (chapter_id, target_type, paragraph_index, comment_id);

ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_novel_target
    (novel_id, target_type, chapter_id, comment_id);

-- 按点赞排序时（Order == likes）使用
ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_novel_like
    (novel_id, like_num DESC, comment_id DESC);
```

- 段评统计走 `idx_novel_comments_chapter` 的覆盖索引，不再全表扫。
- 列表接口的 total 可缓存，或只在第一页返回。

---

### SQ-13 帖子详情：多次单行点查可合并

**优先级**：P2（SQL 本身都走主键，QPS 高时值得优化）

**接口**：`POST /api/auth/showDetails`。

**代码**：`service/post_service.go:159-200`。

**执行过程**：

```sql
SELECT * FROM posts WHERE postID = ?;              -- 详情缓存 miss 时
SELECT * FROM users WHERE userID = ?;              -- 详情缓存 miss 时
SELECT * FROM plikes WHERE userID = ? AND ptargetID = ? LIMIT 1;
SELECT * FROM psaves WHERE userID = ? AND ptargetID = ? LIMIT 1;
SELECT * FROM post_watches WHERE userID = ? AND postID = ? LIMIT 1;
-- 投币数：coin_throw + coin_ledger 共 2 条
```

**问题**：

- 4 条单行查询都是主键/唯一索引点查，但可合并为 1 条
  `LEFT JOIN` 或 `UNION ALL` 常量查询，减少 3 次往返。
- 详情命中缓存后仍会做 3 次用户态点查；高 QPS 下连接数压力明显。

**建议**：

- 用一条 SQL 或 pipeline 批量取 `liked/saved/watched`：

  ```sql
  SELECT
    EXISTS(SELECT 1 FROM plikes WHERE userID=? AND ptargetID=?) AS liked,
    EXISTS(SELECT 1 FROM psaves WHERE userID=? AND ptargetID=?) AS saved,
    EXISTS(SELECT 1 FROM post_watches WHERE userID=? AND postID=?) AS watched;
  ```

- `MyThrows` 当前是 `coin_throw` + `pvotes` 两条查询；历史投票已下线，
  可评估只查 `coin_throw` 或缓存。

---

### SQ-14 收藏列表 / 浏览历史：JOIN 后排序与深分页

**优先级**：P2

**代码**：`repository/gorm/post.go:125-155`（保存列表）、`165-169`（历史）。

```sql
SELECT psaves.*
FROM psaves
JOIN posts ON psaves.ptargetID = posts.postID
WHERE psaves.userID = ? AND posts.postID != 0
  AND (posts.is_private = 0 OR posts.userID = ?)
ORDER BY psaves.psaveID DESC
LIMIT 15 OFFSET ?;
```

**问题**：

- `psaves` 只有 `userID` 单列索引，`ORDER BY psaveID DESC` 需要 filesort。
- 之后再 `SELECT ... FROM posts WHERE postID IN (...)` 回表一次；
  这是两段式查询，但第二段命中主键，可接受。
- `psaves` **没有 `(userID, ptargetID)` 唯一索引**，`CreateSave` 是
  “先查再插”，并发下可能重复（不是慢查询，但顺带记录）。

**建议**：

```sql
ALTER TABLE psaves ADD INDEX idx_psaves_user_save (userID, psaveID DESC);
-- 评估清理重复后：
ALTER TABLE psaves ADD UNIQUE INDEX uk_psaves_user_target (userID, ptargetID);
```

---

### SQ-15 投币 / 积分流水：缺少组合索引

**优先级**：P2

**代码**：`repository/gorm/coin.go:496-511`（`realCoinsFromLedger`）、
`540-555`（`ThrownForTarget`）、`597-622`（`List`）；
`repository/gorm/ledger.go:430-447`、`477-517`。

```sql
-- 投币/取消时按流水算净额
SELECT COALESCE(SUM(amount), 0) FROM coin_ledger
WHERE user_id = ? AND target_type = ? AND target_id = ?
  AND event_type IN ('throw','refund');

-- 内容被投币总数
SELECT COALESCE(SUM(coin_num), 0) FROM coin_throw
WHERE target_type = ? AND target_id = ?;

-- 我的流水
SELECT * FROM coin_ledger
WHERE user_id = ? AND event_type IN (...)
ORDER BY created_at DESC, ledger_id DESC
LIMIT 30 OFFSET ?;
```

**问题**：

- `coin_ledger` 现有 `(user_id, created_at)` 与 `(target_type, target_id)`；
  第一条查询需要四列组合，MySQL 只能选其中一个索引再回表过滤。
- `coin_throw` 现有 `(user_id,target_type,target_id)` 与 `(target_id)`；
  `WHERE target_type AND target_id` 用 `target_id` 索引后回表过滤类型，
  选择性一般。
- 流水列表 `event_type IN (...)` 是低选择性过滤，主要靠 `(user_id, created_at)`；
  若用户流水很多可考虑 `(user_id, event_type, created_at)`。

**建议**：

```sql
ALTER TABLE coin_ledger
  ADD INDEX idx_coin_user_target_event
    (user_id, target_type, target_id, event_type);

ALTER TABLE coin_ledger
  ADD INDEX idx_coin_user_event_time
    (user_id, event_type, created_at DESC, ledger_id DESC);

ALTER TABLE coin_throw
  ADD INDEX idx_coin_throw_type_target (target_type, target_id);
```

---

### SQ-16 通知/聊天/评论反应的索引顺序问题（补充）

**优先级**：P2

1. **`post_watches` 缺 `postID` 前缀**（`repository/gorm/post.go:456-468`）：

   ```sql
   SELECT userID FROM post_watches WHERE postID = ?;
   ```

   现有唯一索引 `(userID, postID)` 不能用于 `postID` 单列过滤。
   每次发评论/回复都会调用一次，是全表扫描的潜在热点。

   ```sql
   ALTER TABLE post_watches ADD INDEX idx_post_watches_post (postID);
   ```

2. **评论反应批量位图**（`repository/gorm/comment.go:126-161`）：

   ```sql
   SELECT * FROM comment_reactions
   WHERE userID = ? AND targetType = ? AND reaction = ? AND targetID IN (...);
   ```

   唯一索引是 `(userID,targetType,targetID)`，`reaction` 只能回表过滤。
   增加非唯一索引即可（不改变唯一语义）：

   ```sql
   ALTER TABLE comment_reactions
     ADD INDEX idx_comment_reactions_user_type_reaction_target
       (userID, targetType, reaction, targetID);
   ```

3. **小说评论点赞位图**（`repository/gorm/novel.go:634-647`）：

   ```sql
   SELECT * FROM novel_comment_likes WHERE user_id = ? AND comment_id IN (...);
   ```

   唯一索引是 `(comment_id,user_id)`，`user_id` 不是前缀。

   ```sql
   ALTER TABLE novel_comment_likes
     ADD INDEX idx_novel_comment_likes_user (user_id, comment_id);
   ```

---

### SQ-17 推荐索引构建与交互查询

**优先级**：P2（后台任务，但会周期性扫全表）

**代码**：`repository/gorm/recommendation.go:136-165`（`ListPostMeta`）、
`167-192`（`ListEmbeddings`）、`194-281`（待嵌入/待计数）、
`283-438`（用户交互）、`service/recommendation_service.go:184-231`。

**问题**：

- `loadIndex` 每次刷新都执行：

  ```sql
  SELECT postID, ... FROM posts;                       -- 全表
  SELECT post_id, kind, ..., embedding FROM post_embeddings;  -- 全表 + 大 BLOB
  ```

  运行在后台 goroutine，但会周期性占用大量 I/O、网络与内存；
  帖子/向量越多，刷新越慢，可能超过刷新间隔。
- `CountPending`：

  ```sql
  SELECT count(*) FROM posts
  LEFT JOIN post_embeddings e ON e.post_id = posts.postID AND e.kind = 'text'
  WHERE e.id IS NULL AND posts.is_private = 0;

  -- 图片待处理版本还多一个 ptext LIKE '%![%' 全表条件
  ```

  `posts.is_private` 无索引；`ptext LIKE '%![%'` 无法走索引。
- 用户交互列表：

  ```sql
  SELECT * FROM pbrowses WHERE userID = ? ORDER BY time DESC LIMIT 300;
  SELECT * FROM plikes   WHERE userID = ? ORDER BY time DESC LIMIT 200;
  SELECT * FROM pcomments WHERE userID = ? ORDER BY time DESC LIMIT 200;
  ```

  前两者只有 `(userID, ptargetID)` 唯一索引 / `userID` 单列索引，
  `ORDER BY time DESC` 需要 filesort。

**建议**：

- 增量刷新索引：记录 `updated_at`/`postID` 水位，只取变化部分；
  向量表只取 `content_hash` 变化的行。
- 待处理统计加 `(is_private)` 或维护 `embedding_status` 标记列；
  图片待处理避免 `%![%`，在写入时打标记。
- 交互列表按需加：

  ```sql
  ALTER TABLE pbrowses ADD INDEX idx_pbrowses_user_time (userID, time DESC);
  ALTER TABLE plikes   ADD INDEX idx_plikes_user_time   (userID, time DESC);
  ALTER TABLE pcomments ADD INDEX idx_pcomments_user_time (userID, time DESC);
  ```

---

### SQ-18 管理端与低频接口（批量记录）

**优先级**：P2

| 接口/代码 | SQL 形态 | 风险 |
| --- | --- | --- |
| `POST /api/auth/showUsers`（`repository/gorm/admin.go:137-160`） | `phone = ?` / `name LIKE 'x%'` / `idPass = ?`，**无 LIMIT** | `phone`/`name` 无索引；结果无上限，用户量大时回传巨大 |
| `POST /api/auth/getfeedback`（`repository/gorm/post.go:725-735`） | `SELECT * FROM feedbacks` | 无分页 |
| `ListAllTags`（`repository/gorm/post.go:672-682`） | `SELECT * FROM tags` | 标签表小，可接受 |
| `repository/gorm/sue.go:31` | `WHERE finish = 0` | `finish` 无索引，举报量大时全表扫 |
| `repository/gorm/lottery.go:185/204/212` | 按 `lotteryID` 取全部参与/奖品 | 单次抽奖可接受，但参与量大时内存高 |
| `repository/gorm/rating.go:308` | `WHERE rating_post_id = ? ORDER BY comment_id DESC` 无 LIMIT | 评分帖评论全量 |
| `controller/musicController.go:396-403` | `WHERE is_published=1 AND is_delete=0` + `ORDER BY post_time DESC` + 三列 LIKE | 无对应索引；数据量小，暂列 P2 |
| `controller/musicController.go:561` | 清理草稿 `post_time < ?` | 定时任务，建议 `(is_published,is_delete,post_time)` |
| `repository/gorm/user.go:799-819`（`ListTopActive`） | `ORDER BY received_like_num + (SELECT SUM(...) FROM posts WHERE userID=users.userID) + (SELECT SUM(...) FROM novels WHERE user_id=users.userID)` | 全用户扫 + 每用户两个关联子查询；仅缓存预热/启动时调用 |

---

## 4. 建议新增索引 DDL

> **执行前必读**
>
> 1. 先用 `SHOW INDEX FROM <table>;` 确认索引不存在；
> 2. 大表（`posts`、`pcomments`、`ccomments`、`plikes`、`pbrowses`、`chat_msgs`、
>    `coin_ledger`、`users`）使用 `ALGORITHM=INPLACE, LOCK=NONE` 或
>    `pt-online-schema-change` / `gh-ost`，单条 DDL 单独执行并观察；
> 3. 新增索引会增加写放大与磁盘占用，**不要一次性全加**；按 P0 → P1 → P2 分批，
>    每批用 `EXPLAIN ANALYZE` 对比前后计划；
> 4. MySQL 8 可用 `ALTER TABLE ... ADD INDEX ... INVISIBLE` 先建后观察，
>    确认无误再 `ALTER TABLE ... ALTER INDEX ... VISIBLE`。
> 5. 生成的 `sort_at` / `coin_rank_score` 列属于 schema 变更，需评估 ORM 是否
>    需要同步 model（本次不改代码，落地时由后续单处理）。

### 4.1 第一批（P0 相关）

```sql
-- 首页热门评论：每帖 Top-N
ALTER TABLE pcomments
  ADD INDEX idx_pcomments_target_like
    (ptargetID, like_num DESC, pcommentID);

-- 通知列表
ALTER TABLE notices
  ADD INDEX idx_notices_receiver_read_id
    (receiver, `read`, noticeID DESC);

-- 评论/回复通知的 watcher 反查
ALTER TABLE post_watches
  ADD INDEX idx_post_watches_post (postID);
```

### 4.2 第二批（P1 相关）

```sql
-- 首页按指标排序（只建线上实际使用的排序；三选一或按需）
ALTER TABLE posts
  ADD INDEX idx_posts_private_like (is_private, like_num DESC, postID DESC);
ALTER TABLE posts
  ADD INDEX idx_posts_private_comment (is_private, comment_num DESC, postID DESC);
ALTER TABLE posts
  ADD INDEX idx_posts_private_browse (is_private, browse_num DESC, postID DESC);

-- 分区页 / 个人主页
ALTER TABLE posts
  ADD INDEX idx_posts_partition_time (`partition`, post_time DESC, postID DESC);
ALTER TABLE posts
  ADD INDEX idx_posts_user_private_time
    (userID, is_private, is_anonymous, post_time DESC);

-- 投币榜
ALTER TABLE posts
  ADD COLUMN coin_rank_score INT
    GENERATED ALWAYS AS (coin_num + vote_num) STORED;
ALTER TABLE posts
  ADD INDEX idx_posts_coin_rank (is_private, coin_rank_score DESC, postID DESC);

-- 小说列表
ALTER TABLE novels
  ADD COLUMN sort_at DATETIME
    GENERATED ALWAYS AS (COALESCE(latest_chapter_at, created_at)) STORED;
ALTER TABLE novels
  ADD INDEX idx_novels_sort_at (sort_at DESC, novel_id DESC);
ALTER TABLE novels
  ADD INDEX idx_novels_category_sort (category, sort_at DESC, novel_id DESC);
ALTER TABLE novels
  ADD INDEX idx_novels_status_sort (status, sort_at DESC, novel_id DESC);
ALTER TABLE novels
  ADD INDEX idx_novels_user_sort (user_id, sort_at DESC, novel_id DESC);

-- 小说评论 / 段评
ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_chapter
    (chapter_id, target_type, paragraph_index, comment_id);
ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_novel_target
    (novel_id, target_type, chapter_id, comment_id);
ALTER TABLE novel_comments
  ADD INDEX idx_novel_comments_novel_like
    (novel_id, like_num DESC, comment_id DESC);

-- 小说评论点赞位图
ALTER TABLE novel_comment_likes
  ADD INDEX idx_novel_comment_likes_user (user_id, comment_id);

-- 聊天
ALTER TABLE chat_msgs
  ADD INDEX idx_chat_sender_created (senderUserID, createdAt, chatMsgID);
ALTER TABLE chat_msgs
  ADD INDEX idx_chat_target_created (targetUserID, createdAt, chatMsgID);

-- 评论反应位图
ALTER TABLE comment_reactions
  ADD INDEX idx_comment_reactions_user_type_reaction_target
    (userID, targetType, reaction, targetID);

-- 收藏列表排序
ALTER TABLE psaves
  ADD INDEX idx_psaves_user_save (userID, psaveID DESC);
```

### 4.3 第三批（P2 / 按需）

```sql
-- 投币与积分流水
ALTER TABLE coin_ledger
  ADD INDEX idx_coin_user_target_event
    (user_id, target_type, target_id, event_type);
ALTER TABLE coin_ledger
  ADD INDEX idx_coin_user_event_time
    (user_id, event_type, created_at DESC, ledger_id DESC);
ALTER TABLE coin_throw
  ADD INDEX idx_coin_throw_type_target (target_type, target_id);

-- 推荐交互时间线
ALTER TABLE pbrowses
  ADD INDEX idx_pbrowses_user_time (userID, time DESC);
ALTER TABLE plikes
  ADD INDEX idx_plikes_user_time (userID, time DESC);
ALTER TABLE pcomments
  ADD INDEX idx_pcomments_user_time (userID, time DESC);

-- 用户搜索 / 管理端筛选
ALTER TABLE users
  ADD INDEX idx_users_phone (phone);
ALTER TABLE users
  ADD INDEX idx_users_name (name);
ALTER TABLE users
  ADD INDEX idx_users_score (score DESC);

-- 举报 / 评分等低频
ALTER TABLE sues
  ADD INDEX idx_sues_finish (finish);
ALTER TABLE rating_posts
  ADD INDEX idx_rating_posts_category_id (category_id, rating_post_id DESC);
```

> `users(name)` 只能帮 `LIKE '前缀%'`；`%关键词%` 仍需全文索引。
> `sues`、`rating_posts` 若数据量小可暂缓。

### 4.4 全文索引（需产品确认语义后单独排期）

```sql
-- 中文需 ngram 解析器；建索引期间会消耗较多 I/O
ALTER TABLE posts
  ADD FULLTEXT INDEX ft_posts_title_ptext (title, ptext) WITH PARSER ngram;

ALTER TABLE users
  ADD FULLTEXT INDEX ft_users_name_intro (name, intro) WITH PARSER ngram;

ALTER TABLE novels
  ADD FULLTEXT INDEX ft_novels_search (title, intro, author_name) WITH PARSER ngram;
```

查询替换为：

```sql
SELECT ... FROM posts
WHERE MATCH(title, ptext) AGAINST (? IN BOOLEAN MODE)
ORDER BY post_time DESC
LIMIT 15;
```

注意：

- `MATCH` 的相关度排序与 `LIKE` 的“包含”语义不同，短词/停用词行为也不同；
- 需要先在小流量环境验证中文分词效果；
- 若不做全文索引，至少应限制搜索范围并加缓存，避免任意关键词触发全表扫描。

---

## 5. EXPLAIN 验证清单

建议 DBA 在只读副本上按以下顺序采集（用真实选择度参数替换 `?`）：

```sql
-- 0. 会话级工具
SHOW INDEX FROM posts;
SHOW INDEX FROM pcomments;
SHOW INDEX FROM notices;
SHOW INDEX FROM chat_msgs;

-- 1. 首页默认时间流
EXPLAIN ANALYZE
SELECT postID, like_num, comment_num, browse_num, post_time
FROM posts
WHERE (is_private = 0 OR userID = 1)
ORDER BY post_time DESC
LIMIT 15 OFFSET 0;

-- 2. 首页按点赞排序
EXPLAIN ANALYZE
SELECT postID, like_num
FROM posts
WHERE (is_private = 0 OR userID = 1)
ORDER BY like_num DESC, postID DESC
LIMIT 15 OFFSET 0;

-- 3. 首页热门评论
EXPLAIN ANALYZE
SELECT * FROM pcomments
WHERE ptargetID IN (1, 2, 3)
ORDER BY ptargetID, like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC;

-- 4. 通知列表
EXPLAIN ANALYZE
SELECT * FROM notices
WHERE receiver = 1 AND `read` = 0
ORDER BY noticeID DESC
LIMIT 20;

-- 5. 会话历史
EXPLAIN ANALYZE
SELECT * FROM chat_msgs
WHERE threadAnonymous = 0
  AND ((targetUserID = 1 AND senderUserID = 2)
    OR (targetUserID = 2 AND senderUserID = 1))
ORDER BY createdAt;

-- 6. 投币榜
EXPLAIN ANALYZE
SELECT postID, title, `partition`, post_time, coin_num, vote_num
FROM posts
WHERE (is_private = 0 OR is_private IS NULL)
ORDER BY (coin_num + vote_num) DESC, postID DESC
LIMIT 100;

-- 7. 作者榜分组
EXPLAIN ANALYZE
SELECT userID, COALESCE(SUM(coin_num + vote_num), 0) AS coins
FROM posts
WHERE (is_private = 0 OR is_private IS NULL)
GROUP BY userID;

-- 8. 小说列表
EXPLAIN ANALYZE
SELECT * FROM novels
WHERE category = '玄幻'
ORDER BY COALESCE(latest_chapter_at, created_at) DESC, novel_id DESC
LIMIT 15;

-- 9. 段评统计
EXPLAIN ANALYZE
SELECT paragraph_index, COUNT(*) AS cnt
FROM novel_comments
WHERE chapter_id = 1 AND target_type = 'paragraph'
GROUP BY paragraph_index;

-- 10. 用户搜索候选
EXPLAIN ANALYZE
SELECT userID, name, intro, score
FROM users
WHERE (name LIKE '%关键词%' ESCAPE '!' OR intro LIKE '%关键词%' ESCAPE '!')
ORDER BY score DESC, userID ASC
LIMIT 60;
```

关注列：

- `type`：出现 `ALL` 说明全表；
- `key`：`NULL` 说明未用索引；
- `rows` / `actual rows`：是否接近表总行数；
- `Extra`：`Using filesort`、`Using temporary`、`Using join buffer` 都是优化信号。

同时建议临时打开慢查询采样（由运维执行，不写死在代码里）：

```sql
SET GLOBAL slow_query_log = ON;
SET GLOBAL long_query_time = 0.5;
SET GLOBAL log_queries_not_using_indexes = ON;  -- 仅短时间开启，日志量会很大
```

长期看，`performance_schema.events_statements_summary_by_digest` 比逐条慢日志
更适合定位“总耗时最高”的 SQL：

```sql
SELECT DIGEST_TEXT, COUNT_STAR, SUM_TIMER_WAIT/1e12 AS total_sec,
       AVG_TIMER_WAIT/1e9 AS avg_ms, SUM_ROWS_EXAMINED
FROM performance_schema.events_statements_summary_by_digest
ORDER BY SUM_TIMER_WAIT DESC
LIMIT 20;
```

---

## 6. 风险与注意事项

1. **本报告不改业务逻辑**。所有“合并查询 / 去掉 Count / 分页 / 窗口函数”建议
   都需要后续单独需求单实现并回归接口语义。
2. **索引不是越多越好**：
   - `posts` 是写热点（点赞、浏览、评论都会更新计数），给它加多个排序索引会放大写成本；
   - 建议先只加线上确实在用的排序，观察 `Innodb_row_lock_time` 与写延迟。
3. **生成列**（`coin_rank_score`、`sort_at`）属于表结构变更：
   - 需要同步确认 GORM model 是否需要加字段，避免 `AutoMigrate` 与手工列冲突；
   - 大表 `ADD COLUMN ... STORED` 会重建表，必须用 online DDL 工具。
4. **全文索引**会改变搜索语义；中文分词、停用词、相关度排序都需要产品验收，
   不能视为 `LIKE` 的等价替换。
5. **唯一索引**（如 `psaves(userID,ptargetID)`）在历史脏数据上可能创建失败；
   需先按 `docs/migrations/20261118_*` 的模式清理重复并确认，不在启动路径强建。
6. **`OR` 改写**（可见性条件）需要谨慎：`UNION ALL` 会在应用层合并排序/分页，
   容易引入分页错误；建议先用 `EXPLAIN` 确认 MySQL 是否已经使用 index merge，
   再决定是否值得改。
7. **连接池**：当前 `max_open=150`（`config/pool.go`）。N+1（尤其是评分任务、
   通知列表）会长时间占用连接，索引优化与查询合并应同时推进。

---

## 7. 附录：审计覆盖的代码文件

| 模块 | 文件 |
| --- | --- |
| 帖子 | `repository/gorm/post.go`、`post_like.go`、`post_browse.go`、`repository/gorm/comment_stats.go` |
| 评论 | `repository/gorm/comment.go`、`comment_reaction.go` |
| 用户 | `repository/gorm/user.go`、`admin.go`、`staff.go` |
| 通知/聊天 | `repository/gorm/notice.go`、`chat.go` |
| 榜单 | `repository/gorm/rank.go` |
| 小说 | `repository/gorm/novel.go` |
| 投币/积分 | `repository/gorm/coin.go`、`ledger.go` |
| 推荐 | `repository/gorm/recommendation.go` |
| 评分/其他 | `repository/gorm/rating.go`、`lottery.go`、`badge.go`、`shop.go`、`sue.go`、`resume.go` |
| 服务层 | `service/post_service.go`、`browse_feed.go`、`comment_service.go`、`user_service.go`、`user_search_service.go`、`notice_service.go`、`chat_service.go`、`novel_service.go`、`recommendation_service.go` |
| 表结构/迁移 | `model/*.go`、`common/database.go`、`common/plike_migrate.go`、`docs/migrations/*.sql` |
| 路由 | `route/routes.go` |

---

_报告结束。_
