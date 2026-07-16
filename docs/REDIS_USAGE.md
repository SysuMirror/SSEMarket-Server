# SSE Market 后端 Redis 使用文档

> 面向面试梳理：我们在哪些模块用了 Redis、用了哪种数据结构、怎么存、为什么这么设计。

---

## 一、整体架构

### 1.1 客户端与连接

| 项 | 说明 |
|---|---|
| 库 | `github.com/redis/go-redis/v9` |
| 全局实例 | `core.MyRedis`（`core/redis.go` 初始化） |
| DB | 固定 `DB 0` |
| 连接池 | `pool.size=100`，`min_idle=10`（`config/pool.go`） |
| 超时 | dial 500ms / read 300ms / write 300ms |
| 健康检查 | 启动 `Ping`；`/ready` 探针；`HotDataService` 每 5s Ping |

### 1.2 缓存分层（L1 + L2）

```
请求 → L1 BigCache（进程内）→ L2 Redis → MySQL
```

- **L1**：`core/local_cache.go`，BigCache，5min 物理窗口，512MB 上限
- **L2**：Redis String，统一走 `cache/manager.go` 的 **Cache-Aside**
- **治理**：击穿（singleflight + Redis 互斥锁预留多实例）、穿透（布隆过滤器，**内存**非 Redis）、雪崩（TTL 随机抖动 + 逻辑过期）

面试话术：**Redis 在我们项目里是 L2 分布式缓存 + 计数器/排行榜/实时状态存储，不是唯一数据源。**

---

## 二、Redis 数据结构总览

| 数据结构 | 使用场景 | 典型命令 |
|---|---|---|
| **String** | 业务缓存、Session、验证码、浏览数、聊天已读/在线、Feed 世代号、回源互斥锁 | GET / SET / SETNX / INCR / DEL / MGET |
| **Sorted Set** | 帖子热度排行榜 | ZADD / ZREVRANGE / DEL |
| **Set** | 聊天「已打开会话」「未读 key」索引 | SADD / SREM / SMEMBERS |
| **Pipeline** | 启动预热批量写浏览数 | Pipeline + SetNX |
| **Lua Script** | 互斥锁安全释放（校验 token 再 DEL） | EVAL |

---

## 三、按业务模块详解

### 3.1 统一二级缓存（Cache-Aside）

**代码位置**：`cache/manager.go`、`cache/value.go`、`cache/config.go`

**Redis 结构**：`String`（JSON 字节）

**存储格式**：

```json
{
  "data": { /* 业务对象 */ },
  "expire_at": "2026-06-14T12:00:00Z",
  "version": 1718366400000000000
}
```

**读取流程**：

1. 查 L1 → 命中且逻辑未过期 → 返回
2. 查 Redis `GET key` → 回填 L1 → 返回
3. 未命中：`singleflight` 合并回源；`lock:{key}` Redis 互斥锁（**多实例**时跨进程互斥，当前单实例主靠 singleflight）→ 查 DB → `SET key` 写 L2 + L1

**写入**：`SET key value EX (L2TTL + 随机 0~300s)`

**逻辑过期**（`LogicalExpire=true`）：物理 TTL 未到但 `expire_at` 已过 → **仍返回旧值**，后台 goroutine 异步刷新。用于热点数据避免击穿 MySQL。

#### 3.1.1 各缓存 Key 与策略

| Key 模式 | 业务 | 代码入口 | L1 TTL | L2 TTL | 逻辑过期 | 失效时机 |
|---|---|---|---|---|---|---|
| `post:detail:{postID}` | 帖子详情 DTO | `post_service.GetDetail` | 5min | 30min | ✅ | 改帖/评论/举报 |
| `comment:list:{postID}:{postType}` | 评论树 | `comment_service.ListForPost` | 3min | 15min | ❌ | 增删评 |
| `user:info:{userID}` | 用户信息 | `user_service.GetByID` | 10min | 1h | ❌ | 改资料 |
| `post:hot:list` | 热帖 Top100 JSON | 启动预热 `warmup_tasks` | 5min | 30min | ❌ | 仅预热写入，**运行时 API 直接读 ZSet** |
| `post:browse:public:g{n}:...` | 公共 Feed 列表 | 已**停用**（见 3.7） | 1min | 5min | ✅ | INCR 世代号 |

**Key 生成**：`util/cache_key.go`

---

### 3.2 帖子浏览数（Counter + 异步落库）

**代码位置**：`service/hot_data_service.go`、`util/cache_key.go`

**Redis 结构**：`String`（整数，无 TTL）

**Key**：`post:browse:count:{postID}`

**核心命令**：

| 操作 | 命令 | 说明 |
|---|---|---|
| 浏览 +1 | `INCR` | 多实例并发安全，Redis 为跨副本唯一真相 |
| 冷启动 seed | `SETNX` | key 不存在时用 MySQL `browse_num` 初始化，避免从 0 累加分叉 |
| 读当前值 | `GET` | 展示浏览数 |
| MySQL 更新后对齐 | `SET` | `SyncBrowseRedisFloor` 保证 Redis ≥ DB |
| 启动预热 | `Pipeline` + `SETNX` | 最近 500 帖批量 seed（`warmup_tasks.taskBrowseCountBatch`） |

**为什么这样设计**：

- 帖子详情是**高频读**，每次浏览都 `UPDATE MySQL` 会打爆 DB
- Redis INCR O(1)，多 Pod 共享计数
- 后台 goroutine 每 **5s**（`hot_data.db_sync_interval`）把 dirty 帖子批量 `UPDATE` 回 MySQL（**最终一致**）
- Redis 挂了降级：直接读 DB，停止 INCR

**调用链**：`post_service.GetDetail` → `IncrBrowse` → `HotData.IncrBrowseCount`

---

### 3.3 帖子热度排行榜（Sorted Set）

**代码位置**：`heat/cron.go`、`service/post_service.go`（CalculateHeat）、`cache/warmup_tasks.go`

**Redis 结构**：`Sorted Set`

**Key**：`hot_posts_zset`

**Member**：JSON 字符串

```json
{"postID": 123, "title": "标题", "browseNum": 456.0}
```

**Score**：热度公式（`calculateHeat`）

```
score = log10(browseNum + commentNum/10 + likeNum/2) - daysSincePost
```

**核心命令**：

| 操作 | 命令 |
|---|---|
| 每小时全量刷新 | `DEL` → 循环 `ZADD` |
| 取 Top N | `ZREVRANGE` / `ZREVRANGE WITHSCORES` |
| API 展示 Top10 | `ZREVRANGE 0 19` 再查 DB 补全 |
| 预热详情 | 从 ZSet 取 Top100 postID 批量预热 `post:detail:*` |

**为什么用 ZSet**：

- 需要按 score 排序取 Top N，ZSet 天然 O(log N) 插入 + O(log N + M) 范围查询
- Member 存 JSON 避免二次查 Redis；展示时再查 MySQL 拿最新标题/状态

**定时任务**：`@hourly` 执行 `RefreshHeat()`；服务启动预热也会调用一次。

---

### 3.4 公共 Feed 缓存失效（Generation 计数器）

**代码位置**：`cache/public_feed.go`、`util/cache_key.go`

**Redis 结构**：`String`（整数）

**Key**：`post:browse:public:gen`

**命令**：`GET` 读世代号；`INCR` 批量失效

**列表 Key 格式**（含世代，当前**未启用读路径**）：

```
post:browse:public:g{gen}:home:part:default:ord:default:off:0:lim:15
```

**设计意图**：

- 发帖/删帖后若 `SCAN + DEL` 所有 Feed key 成本高且阻塞
- **世代号模式**：失效时 `INCR gen`，新请求用 `g{n+1}` 新 key，旧 key 靠 TTL 自然过期
- 无需 scan，适合多维度分页缓存

**当前状态**：`browse_feed.go` 写明列表**直查 MySQL**，公共 Feed L1/L2 读路径已停用；但 `InvalidateHomeBrowse()` 仍在发帖/删帖/抽奖等写路径调用，为后续启用预留。

---

### 3.5 用户 Session 快照

**代码位置**：`service/session_cache.go`、`middleware/user_session.go`

**Redis 结构**：`String`（JSON）

**Key**：`user:session:{userID}`

**Value**：完整 `domain.User` JSON

**TTL**：5 分钟

**命令**：`GET` 读；`SET EX 300` 写；`DEL` 失效

**使用场景**：

- JWT 鉴权后 middleware 加载用户，避免每次请求查 DB
- 登录/注册成功 `CacheUserAfterAuth` 预热
- 改密/注销 `InvalidateUserSession`

**与 `user:info:{id}` 区别**：

| | Session | UserInfo 缓存 |
|---|---|---|
| 用途 | 鉴权链路快速取当前用户 | 业务读用户资料 |
| 走 CacheManager | ❌ 直连 Redis | ✅ L1+L2 |
| TTL | 5min 固定 | L1 10min / L2 1h |

---

### 3.6 邮箱验证码

**代码位置**：`api/mailSender.go`、`service/user_service.go`

**Redis 结构**：`String`

| Key | Value | TTL | 说明 |
|---|---|---|---|
| `vcode:{email}` | 6 位验证码 | 5min | 防重复发送（存在则拒绝） |
| `vcode_fail:{email}` | 失败次数 | 5min | 防暴力破解 |

**流程**：

1. 发送前 `GET vcode:{email}`，存在 → 返回「5 分钟后再试」
2. 发送后 `SET vcode:{email} code EX 300`，并重置 fail 计数
3. 校验失败 `INCR fail`（通过 GET+SET 实现计数）；≥5 次 `DEL vcode`
4. 校验成功 `DEL vcode + vcode_fail`

**为什么放 Redis**：验证码短生命周期、多实例共享、天然支持 TTL 自动过期。

---

### 3.7 即时聊天：在线状态 & 未读数

**代码位置**：`controller/chat.go`、`util/chatUtil.go`

#### 3.7.1 会话「正在查看」标记

**Redis 结构**：`String` + `Set`（索引）

| Key | 类型 | Value | TTL |
|---|---|---|---|
| `{senderID}To{targetID}IsExist_direct` | String | `1` | 24h |
| `{senderID}To{targetID}IsExist_anon` | String | `1` | 24h（匿名会话 lane=anon） |
| `chatViewIndex:{userID}` | Set | 上述 String key 集合 | 无 |

**逻辑**：

- 用户打开聊天页 → `SET` 标记 + `SADD` 索引
- 离开聊天页 → `DEL` + `SREM`
- 打开新会话前 → `SMEMBERS` 索引 → 批量 `DEL` 清理旧标记

**作用**：接收方正在看该会话时，发消息**不增加未读数**（`GET chatViewKey`，不存在才 INCR）。

#### 3.7.2 未读消息计数

**Redis 结构**：`String`（计数）+ `Set`（索引）

| Key | 类型 | 说明 |
|---|---|---|
| `{receiverID}RecvUnread{senderID}_direct` | String | 未读条数，无 TTL |
| `{receiverID}RecvUnread{senderID}_anon` | String | 匿名会话未读 |
| `chatUnreadIndex:{userID}` | Set | 该用户所有未读 key |

**命令**：

- 新消息且对方不在线/未打开会话：`INCR` + `SADD`
- 打开会话清零：`DEL` + `SREM`
- 会话列表展示：`GET` 每个会话未读
- 总未读角标：`SMEMBERS` → `MGET` 批量求和

**为什么用 Set 索引**：

- 用户可能有 N 个会话，不能 `KEYS *Unread*`
- Set 维护「该用户有哪些未读 key」，O(N) 批量 MGET

**为什么分 direct/anon lane**：

- 同一对用户可能存在实名私信 + 匿名树洞两条会话线，key 必须隔离。

---

### 3.8 回源互斥锁（Redis SET NX）

**代码位置**：`util/lock.go`，被 `cache/manager.go` 调用

**Redis 结构**：`String`

**Key**：`lock:{业务缓存key}`

**加锁**：`SET key uuid NX EX 5`

**解锁**：Lua 脚本 — 仅当 value 等于自己的 uuid 时才 `DEL`，防误删他人锁

**用途**：Cache-Aside 回源 DB 前，**多实例**部署时跨进程互斥，避免多 Pod 同时加载同一 key。

**与 singleflight 的分工**：

| 机制 | 作用范围 | 当前单实例下的实际角色 |
|------|---------|---------------------|
| `singleflight.Group` | **进程内** goroutine 合并 | **主手段**：同一 key 只回源 DB 一次 |
| Redis 互斥锁 | **跨进程**（多 Pod 共享 Redis） | **预留**：`compose.yml` 仅 1 个 server 容器，跨进程竞争几乎不发生 |

**部署事实**：`compose.yml` 中 `sse_market_server` 为单容器（`container_name: server`），无副本数配置。实现写法是 Redis 互斥锁的常见模式，但**只有水平扩展多实例后**才宜称「分布式锁」。

---

## 四、启动预热（Warmup）

**代码位置**：`cache/warmup.go`、`cache/warmup_tasks.go`、`main.go`

服务 `/ready` 前必须完成预热（`cache.IsReady()`）。

| 任务 | Redis 操作 | 目的 |
|---|---|---|
| `hot_posts_zset` | 调用 `heat.RefreshHeat()` 填充 ZSet | 排行榜就绪 |
| `hot_posts_top100_cache` | ZREVRANGE → SET `post:hot:list` | 热帖 JSON 缓存 |
| `browse_count_batch` | Pipeline SetNX 500 帖 | 浏览数 key 预 seed |
| `active_users_top50` | SET `user:info:{id}` | 活跃用户资料缓存 |

---

## 五、缓存失效矩阵（写路径）

| 操作 | 失效内容 |
|---|---|
| 发帖/删帖/改隐私/抽奖 | `InvalidatePublicFeed`（INCR gen） |
| 改帖内容 | `DEL post:detail:{id}` |
| 增删评论 | `DEL post:detail:{id}` + `DEL comment:list:{id}:post` |
| 改用户资料 | `DEL user:info:{id}` + `DEL user:session:{id}` |
| 登录/改密 | `DEL user:session:{id}` |

---

## 六、监控与运维

| 项 | 位置 |
|---|---|
| Redis Ping | `controller/health.go` `/ready` |
| 连接池指标 | `observability/collector.go` → Prometheus |
| 缓存命中率 | L1/L2 hit/miss（`observability/metrics.go`） |
| 浏览数同步延迟 | `BrowseSyncLag` gauge |
| 优雅关闭 | `main.go` → `HotData.Shutdown()` flush 浏览数到 MySQL |

---

## 七、未使用 / 已废弃的 Key（面试防踩坑）

| Key | 状态 |
|---|---|
| `post:browse:{userID}:...`（`GetPostBrowseKey`） | 已定义，**当前无读写** |
| `product:list:{userID}:...`（`GetProductListKey`） | 已定义，**当前无读写** |
| `post:browse:home:page1` | 旧首页缓存，失效时兼容删除 |
| 公共 Feed 读缓存 | **已停用**，列表直查 MySQL |
| 布隆过滤器 | **进程内存**（`common/bloom.go`），**不在 Redis** |

---

## 八、面试高频问答

### Q1：为什么浏览数用 Redis 而不是直接更新 MySQL？

**答**：详情页 QPS 高，MySQL 行锁 + 磁盘 IO 扛不住。Redis INCR 原子、内存、多实例共享；5s 批量异步落库，接受秒级最终一致；Redis 故障降级读 DB。

### Q2：Cache-Aside 怎么防击穿/穿透/雪崩？

**答**：

- **击穿**：**singleflight** 合并进程内回源；Redis 互斥锁供多实例扩展（当前单实例主靠前者）
- **穿透**：布隆过滤器拦截不存在 postID（内存实现）
- **雪崩**：L2 TTL 加随机 0~300s；逻辑过期返回旧值异步刷新

### Q3：为什么热度榜用 ZSet 而不是 List？

**答**：热度 score 动态变化，需按分数排序取 Top N。ZSet 支持按 score 范围查询；List 只能按插入顺序，不适合排行榜。

### Q4：Feed 缓存为什么用世代号而不是 DEL 所有 key？

**答**：Feed key 维度多（分区/排序/分页），scan 删除慢且阻塞。INCR 世代号 O(1) 失效整类缓存，旧 key 靠 TTL 回收，适合写多读多的列表场景。

### Q5：聊天未读为什么 String + Set 而不是 Hash？

**答**：每个会话独立计数 key，`INCR` 简单；Set 做用户级索引支持 `MGET` 批量聚合总未读。Hash 也可行，但当前模型按会话隔离更清晰，且匿名/实名 lane 分开。

### Q6：L1 和 L2 都要吗？

**答**：L1 降本延迟（微秒级）；L2 跨 Pod 共享。帖子详情等热点 L1 命中率高；浏览数/未读数等写多读少场景直连 Redis 不经 CacheManager。

### Q7：Redis 挂了怎么办？

**答**：

- 启动：`Ping` 失败 panic（强依赖）
- 运行时：`HotDataService` 检测健康，浏览数降级读 DB；Session/验证码等 GET 失败回源 DB 或返回错误
- `/ready` 探针失败，K8s 不切流量

---

## 九、Key 命名规范速查

```
post:detail:{id}              # 帖子详情缓存
post:browse:count:{id}        # 浏览计数
post:hot:list                 # 热帖列表 JSON（预热）
post:browse:public:gen        # Feed 世代号
hot_posts_zset                # 热度 ZSet

user:info:{id}                # 用户信息缓存
user:session:{id}             # JWT 用户快照

comment:list:{postID}:{type}  # 评论树

vcode:{email}                 # 邮箱验证码
vcode_fail:{email}            # 验证失败次数

{uid}To{tid}IsExist_{lane}    # 聊天正在查看
{uid}RecvUnread{tid}_{lane}   # 未读计数
chatViewIndex:{uid}           # 查看索引 Set
chatUnreadIndex:{uid}         # 未读索引 Set

lock:{cacheKey}               # 回源互斥锁（多实例时跨进程）
```

---

## 十、相关源码索引

| 模块 | 文件 |
|---|---|
| Redis 初始化 | `core/redis.go` |
| 连接池配置 | `config/pool.go` |
| Key 规范 | `util/cache_key.go` |
| 二级缓存 | `cache/manager.go`, `cache/value.go`, `cache/config.go` |
| 回源互斥锁 | `util/lock.go` |
| 浏览数 | `service/hot_data_service.go` |
| 热度榜 | `heat/cron.go` |
| Session | `service/session_cache.go` |
| 验证码 | `api/mailSender.go`, `service/user_service.go` |
| 聊天状态 | `controller/chat.go`, `util/chatUtil.go` |
| 预热 | `cache/warmup_tasks.go`, `main.go` |
| 健康检查 | `controller/health.go` |
