# 论坛置顶功能：生产级方案对比与落地设计报告

> 项目：SSE Market（软工集市）  
> 范围：主页帖子置顶队列  
> 版本：基于 2026-08 已上线实现 + 生产级方案横向对比  
> 目的：为后续演进提供可审计的决策依据（约束驱动，非主观偏好）

---

## 1. 原始业务问题与量化约束

### 1.1 原始业务问题（事实陈述）

| ID | 问题描述 | 用户可见现象 | 根因类别 |
|----|----------|--------------|----------|
| P1 | 置顶过期后状态与展示不一致 | 角标仍显示「置顶」，但帖子已不在顶部 | 读时过滤 vs 写时未回写 `is_pinned` |
| P2 | 置顶帖与普通帖排序混杂 | 过期后「被新帖压下去」且语义混乱 | 双通道架构下主列表按 `post_time` 排序 |
| P3 | 置顶顺序不符合群友预期 | 后置顶反而更靠前（LIFO） | `pinned_at DESC` 排序 |
| P4 | 管理员/发帖置顶入口不完整 | 勾选置顶无效果或进不了置顶区 | 缺 `pinned` 入参、缺 `pinned_expires_at` |
| P5 | 全站名额未在入队时强约束 | 超过 5 条时仅展示截断，库内仍有多条 `is_pinned=1` | 仅 `LIMIT 5` 展示，无入队校验 |

### 1.2 量化约束（决策底层依据）

#### 功能约束（硬规则）

| 约束项 | 数值/规则 | 来源 |
|--------|-----------|------|
| 置顶展示面 | 仅 `searchsort=home` + `partition=主页` | 产品 + `ListPinned` SQL |
| 全站队列容量 | `MaxPinnedQueueSize = 5` | 前端文案 + `domain/post.go` |
| 单用户并发置顶 | 1 条未过期 | `CountPinnedByUser >= 1` 拒绝 |
| 用户置顶时长 | 1 小时 | `PinPost`: `now + 1h` |
| 管理员优质置顶时长 | 30 天 | `AdminPinPost` |
| 队列排序 | FIFO：`pinned_at ASC, postID ASC` | 群友建议 + 当前实现 |
| 私密帖 | 仅作者自己在置顶区可见 | `is_private OR userID=viewer` |
| 非主页分区 | 不可用户置顶 | `ErrPinnedWrongPartition` |

#### 性能与 SLA（可观测目标）

| 指标 | 当前目标 | 依据 |
|------|----------|------|
| 首页 Browse P99 增量 | < +15ms（置顶查询相对无置顶） | 置顶仅 1 次额外 SQL，≤5 行 |
| 置顶入队 P99 | < 50ms | 单帖 UPDATE + 2 次 COUNT |
| 过期清理延迟 | ≤ 60s | Cron `@every 1m` |
| 首页缓存失效 | 入队/出队后立即失效世代号 | `InvalidateHomeBrowse()` |
| 读放大 | 每首页请求 +1 查询（`ListPinned`） | 与分页查询分离 |

#### 资源与团队（现状）

| 项 | 现状 |
|----|------|
| 存储 | MySQL `posts` 表字段：`is_pinned`, `pinned_at`, `pinned_expires_at` |
| 缓存 | 公共 Feed 世代号缓存；置顶列表未单独 Redis 缓存 |
| 计算 | 单 Go 服务 + 每分钟 Cron，无独立置顶微服务 |
| 团队 | 后端 1 人可维护；无专职 SRE |
| 工期（已完项） | Bug 修复 + FIFO 队列约 2–3 人日（含部署验证） |

#### 合规与安全

| 约束 | 说明 |
|------|------|
| 权限 | 仅帖作者可 `pinPost` / `unpinPost`；管理员独立接口 |
| 匿名帖 | 置顶区仍走 `anonymizeAuthor` 脱敏 |
| 审计 | 无独立置顶审计表；依赖 `pinned_at` 与业务日志 |
| 数据保留 | 出队即清空三字段，不保留历史置顶记录 |

---

## 2. 通用业务模型抽象

### 2.1 从 SSE Market 抽象的领域模型

```
Feed（信息流）
  ├── NormalStream（普通帖流）：按时间/热度分页
  └── PinnedQueue（置顶队列）：容量有限、有时效、独立排序

PinOperation（置顶操作）
  ├── enqueue(post_id, enqueue_at, expire_at)
  ├── dequeue(post_id)  // 手动取消
  └── expire(post_id)   // 定时或读时过滤

Constraints（约束）
  ├── global_capacity = 5
  ├── per_user_capacity = 1
  ├── ttl_user = 1h
  └── scope = single_feed_surface（仅主页）
```

### 2.2 行业同类通用场景

| 场景 | 共性 |
|------|------|
| 社区论坛首页 | 少量全局置顶 + 主列表分页 |
| 电商店铺公告 | 固定条数置顶商品/公告 |
| 企业内部 BBS | 管理员置顶 + 用户短期置顶 |
| 内容平台 Feed | 「运营位」与「自然排序」双轨 |

共性特征：**有界队列（Bounded Queue）+ TTL + 与主 Feed 解耦展示**。

---

## 3. 业界主流方案 A / B / C

### 方案 A：帖子表内联字段（Inline Flags）— **当前选用基准**

**设计思想**：在 `posts` 表用 `is_pinned`、`pinned_at`、`pinned_expires_at` 表达置顶；读时单独查询置顶子集，主列表 `NOT IN` 排除。

| 维度 | 说明 |
|------|------|
| 适用 | 置顶量极小（≤10）、单库、团队小、延迟可接受 1 分钟级过期 |
| 优点 | 零额外存储组件；事务简单；与现有 GORM 栈一致 |
| 缺点 | 与帖子生命周期耦合；难做置顶审计；`COUNT`/`ORDER` 随全表扫描依赖索引 |
| 代表 | 大量中小型论坛、Discourse 部分实现、自建 BBS |

### 方案 B：独立置顶队列表（Dedicated Pin Table / Redis ZSET）

**设计思想**：`pin_queue(post_id, enqueue_at, expire_at, rank)` 或 Redis `ZSET pin:home`；帖子表不存置顶态。

| 维度 | 说明 |
|------|------|
| 适用 | 置顶频繁变更、需精确排序、多版面多队列、毫秒级过期 |
| 优点 | 队列语义清晰；易扩展多 Feed；Redis ZSET 天然有序 |
| 缺点 | 双写一致性（帖删除 vs 队列）；多一层运维；冷启动需同步 |
| 代表 | 微博/头条运营位、大型社区运营后台 |

### 方案 C：主查询统一排序（`ORDER BY is_pinned DESC, post_time DESC`）

**设计思想**：不拆 `pinned` 数组，单 SQL 排序；置顶作为排序权重。

| 维度 | 说明 |
|------|------|
| 适用 | 置顶即「永远最高优先级」、无复杂 FIFO、分页简单 |
| 优点 | 实现最少；前端无需拼接 |
| 缺点 | 分页边界复杂（置顶挤占每页槽位）；多置顶顺序难表达；与「仅首页展示」耦合差 |
| 代表 | 早期 WordPress 插件、简单 CMS |

---

## 4. 统一评估维度与横向对比

| 维度 | 权重（本业务） | 方案 A 内联 | 方案 B 独立队列 | 方案 C 统一排序 |
|------|----------------|-------------|-----------------|-----------------|
| 实现成本 | 高 | ★★★★★ | ★★ | ★★★★ |
| 运维复杂度 | 高 | ★★★★★ | ★★★ | ★★★★ |
| 队列语义（FIFO） | 高 | ★★★★（`pinned_at`） | ★★★★★ | ★★ |
| 过期一致性 | 高 | ★★★（+Cron） | ★★★★ | ★★★ |
| 扩展多版面 | 中 | ★★★ | ★★★★★ | ★★ |
| 分页正确性 | 高 | ★★★★★（排除 ID） | ★★★★★ | ★★ |
| 10x 流量扩容 | 中 | ★★★（索引+缓存） | ★★★★ | ★★★ |
| 审计/运营 | 低 | ★★ | ★★★★ | ★★ |

**阶段适配判断**

| 阶段 | 推荐 | 排除 |
|------|------|------|
| 短期（当前 DAU、5 条置顶） | **方案 A** | B（过度工程）、C（分页与 FIFO 差） |
| 中期（多分区置顶、运营后台） | A 演进或 B  hybrid | C |
| 长期（多租户、毫秒过期、A/B 运营位） | B 或 A+B 缓存层 | 纯 C |

---

## 5. 选定基准与定制改造（相对通用方案 A）

### 5.1 基准

**方案 A：帖子表内联 + API 双数组 `{ pinned, posts }`**（与 Reddit/现代论坛 API 分段返回类似）。

### 5.2 定制改动清单

| 通用 A 的不足 | 我们的约束 | 改动 | 收益 | 代价 |
|---------------|------------|------|------|------|
| `is_pinned` 直接返回 | P1 假置顶 | `EffectiveIsPinned()` 读时计算 | 角标与队列一致 | 每次序列化多一次时间比较 |
| 仅读时过滤过期 | DB 脏数据 | `ClearExpiredPins` Cron + 启动清理 | `COUNT` 准确；个人页不误判 | 最多 60s 延迟；额外 UPDATE |
| `pinned_at DESC` | 群友 FIFO | 改为 `ASC, postID ASC` | 顺序可预期 | 后置顶不再「抢第一」 |
| 无全站入队上限 | 5 条产品规则 | `CountActivePinned` 入队前校验 | 库内不会出现第 6 条有效置顶 | 满员时用户需等待 |
| 管理员无 `expires` | 进不了 `ListPinned` | 写入 `pinned_expires_at` | 管理员置顶可见 | 需定义管理员 TTL（30d） |
| 发帖 `pinned` 未绑定 | 前端勾选无效 | `PostMsg.Pinned` + 失败可感知 | 行为与 UI 一致 | API 契约变更（已兼容） |
| 非主页可置顶 | 置顶区不展示 | `partition=主页` 校验 | 避免无效置顶 | 其他分区帖不能置顶 |

---

## 6. 完整落地设计（绑定约束）

### 6.1 架构

```
Client (HomeView)
  → POST /api/auth/browse  →  PostService.Browse
       ├─ ListPinned (FIFO, ≤5)     → result.Pinned
       ├─ ListBrowse (exclude IDs)  → result.Posts
       └─ attachPersonalBrowseState (点赞/收藏/热评)

Pin 写路径
  → POST /api/auth/pinPost | unpinPost | post( pinned=true )
       → PostService.PinPost / UnpinPost
       → posts 表 UPDATE
       → InvalidateHomeBrowse()

过期出队
  → pin/cron.go @every 1m → ClearExpiredPins → InvalidateHomeBrowse (if n>0)
```

**为何这样设计**：置顶量 ≤5，独立微服务或 Redis 队列的固定成本高于收益；双数组 API 使前端分页与置顶解耦，满足约束「不与普通帖乱序」。

### 6.2 存储

| 字段 | 类型 | 语义 | 索引建议 |
|------|------|------|----------|
| `is_pinned` | tinyint | 是否在队（出队清 0） | 复合索引候选 |
| `pinned_at` | datetime | 入队时刻（FIFO 键） | `(is_pinned, pinned_expires_at, partition, pinned_at)` |
| `pinned_expires_at` | datetime | 出队条件之一 | 同上 |

**ListPinned SQL 语义**：

```sql
WHERE is_pinned=1 AND pinned_expires_at > NOW() AND partition='主页'
  AND (is_private=0 OR userID=:viewer)
ORDER BY pinned_at ASC, postID ASC
LIMIT 5
```

**为何不新增 `pin_order` 整型**：`pinned_at` 在单队列、无手动调序场景下等价 FIFO 序号；减少迁移与重排逻辑。

### 6.3 接口

| 接口 | 行为 | 关键约束绑定 |
|------|------|--------------|
| `POST /api/auth/browse` | 返回 `{ pinned, posts, total }` | 仅 home；pinned FIFO |
| `POST /api/auth/pinPost` | 入队 | 主页/1人1条/全站5条 |
| `POST /api/auth/unpinPost` | 出队 | 作者本人 |
| `POST /api/auth/post` + `pinned:true` | 发帖并入队 | 失败返回「发帖成功，但置顶失败」 |
| 管理端 `markHQPost` | 管理员入队 | 30d TTL + 全站5条 |

### 6.4 流程

**入队**

1. 校验分区、归属、未重复置顶、用户 1 条、全站 <5  
2. `UPDATE posts SET is_pinned=1, pinned_at=NOW(), pinned_expires_at=?`  
3. `InvalidateHomeBrowse()`

**出队（手动）**

1. `UPDATE` 清空三字段  
2. 失效缓存

**出队（过期）**

1. Cron 扫描 `is_pinned=1 AND (expires IS NULL OR expires <= NOW())`  
2. 批量清空 + 条件失效缓存

**读首页**

1. `ListPinned` → 置顶队列  
2. `ExcludePostIDs` → 主列表去重  
3. 前端 `[...pinned, ...posts]`

---

## 7. 风险、容灾、回滚、运维、扩容

### 7.1 风险矩阵

| 风险 | 概率 | 影响 | 兜底 |
|------|------|------|------|
| Cron 停跑导致脏 `is_pinned` | 低 | 中 | 读时 `EffectiveIsPinned` + `ListPinned` 过滤；监控 Cron 日志 |
| 全站 5 名额打满 | 中 | 低 | 明确错误码 `ErrGlobalPinnedLimitReached` |
| 用户长驻页面置顶不刷新 | 中 | 低 | 可选前端 `refreshPinned` 定时器（未强制） |
| `pinned_at` 时钟漂移 | 低 | 低 | 单 MySQL 实例 `NOW()` 统一 |
| 管理员置顶非主页帖 | 低 | 中 | 运营规范；可后续加 partition 校验 |

### 7.2 回滚策略

| 层级 | 操作 |
|------|------|
| 代码回滚 | 回滚 `server` 镜像；DB 字段可保留（向后兼容） |
| 排序回滚 | 恢复 `ORDER BY pinned_at DESC`（仅改一行，注释已保留） |
| 数据修复 | `UPDATE posts SET is_pinned=0, pinned_at=NULL, pinned_expires_at=NULL WHERE ...` |

### 7.3 运维检查项

- 日志：`[pin] ClearExpiredPins 已清理 N 条`  
- 指标建议：`pin_active_count`、`pin_enqueue_total`、`pin_clear_total`、`browse_pinned_query_ms`  
- 告警：Cron 连续 3 次失败；`active_pins > 5`（不应出现，出现即 bug）

### 7.4 扩容路径

| 流量级 | 动作 |
|--------|------|
| 当前 | 复合索引 + 世代号缓存失效 |
| 10x Browse QPS | 缓存 `ListPinned` 结果 30–60s（接受轻微过期延迟） |
| 多版面置顶 | 引入 `pin_queue` 表或 Redis ZSET（演进至方案 B） |
| 精确毫秒过期 | Redis TTL + key 过期事件，或缩短 Cron |

---

## 8. 与「生产级」方案的差距与演进建议

| 生产级能力 | 当前状态 | 建议优先级 |
|------------|----------|------------|
| 置顶审计日志 | 无 | P2：独立 `pin_events` 表 |
| 运营手动调序 | 仅 FIFO | P3：管理员 `pin_rank` 覆盖 |
| 读路径缓存 | 每次 SQL | P2：首页 pinned 短缓存 |
| 精确过期 | ≤60s | P3：可接受则保持 |
| 多 Feed 置顶 | 仅主页 | 按产品扩展时迁 B |
| 置顶与删帖联动 | 删帖未显式出队 | P1：删帖时 `Unpin` 或依赖 FK 清理 |

---

## 9. 结论（决策闭环）

1. **问题本质**：有界、有时效的置顶队列与主 Feed 必须**双轨存储与展示**；单表 `ORDER BY is_pinned` 无法满足 FIFO + 分页 + 过期一致性。  
2. **约束决定选型**：5 条、单库、小团队 → **方案 A 内联字段** 为当前最优，而非 Redis 队列。  
3. **已落地定制**：FIFO、`EffectiveIsPinned`、Cron 出队、全站入队上限、管理员 TTL、发帖 `pinned` 绑定。  
4. **生产级下一步**（若业务增长）：复合索引 → pinned 短缓存 → 审计表 → 必要时 **方案 B hybrid**。

---

*文档与代码锚点：`domain/post.go`、`repository/gorm/post.go`（`ListPinned`）、`service/post_service.go`（`PinPost`）、`pin/cron.go`、`controller/postController.go`（`browse` / `EffectiveIsPinned`）。*
