# Scheme B 改造草案（待确认）

> 状态：**仅新增 migration + 核心代码草案，尚未接线 Browse/PinPost/Cron，未部署。**  
> 请确认表结构与 API 形状后再进行「下一步：切换读写路径」。

## 1. 表结构要点

### `pin_queue`（当前有效队列）

| 字段 | 含义 |
|------|------|
| `scope` | 队列作用域，默认 `home`（预留多版面） |
| `post_id` | 帖子 ID |
| `owner_user_id` | 置顶发起人 |
| `source` | `user` / `admin` |
| `enqueue_at` | 入队时间 = **FIFO 排序键** |
| `expire_at` | 过期时间；到期出队 |

约束：`UNIQUE(scope, post_id)`；全站上限仍用应用层 `MaxPinnedQueueSize=5`。

### `pin_events`（审计）

记录 `enqueue / dequeue / expire / reject`，解决方案 A「难做审计」问题。

### 与 `posts` 的关系

- **不删除** `posts.is_pinned / pinned_at / pinned_expires_at`（回滚窗口）
- migration 会把当前未过期的 Scheme A 数据导入 `pin_queue`
- 确认切换后：写路径只写 `pin_queue`；读路径只读 `pin_queue`
- 可选后续 migration：停止回写 posts 置顶字段（字段可留空）

## 2. 新增文件

| 路径 | 作用 |
|------|------|
| `docs/migrations/20260824_pin_queue_scheme_b.sql` | DDL + 存量导入 |
| `model/pin_queue.go` | GORM model |
| `domain/pin.go` | 领域常量/结构 |
| `repository/pin_queue.go` | Repo 接口 |
| `repository/gorm/pin_queue.go` | Repo 实现 |
| `service/pin_queue_service.go` | 业务服务草案 |

## 3. 下一步接线计划（确认后再做）

1. `common/database.go`：`AutoMigrate(&model.PinQueue{}, &model.PinEvent{})`
2. `service/registry.go`：注入 `PinQueue` 全局服务
3. `PostService.Browse`：`ListPinned` 改为 `pinQueue.ListHomePinnedPostIDs` → `FindByIDs` 保序
4. `PinPost / UnpinPost / AdminPin / ClearExpiredPins / 删帖`：改写 `pin_queue`
5. `EffectiveIsPinned`：改为查 `pin_queue`（或 Browse 时批量标记）
6. Cron：改为 `PinQueue.ClearExpired`
7. 双写过渡（可选 1–2 天）：同时写 posts 字段，便于回滚

## 4. 需要你确认的点

1. **审计表 `pin_events`**：保留还是先不要？
2. **重复置顶策略**：拒绝（当前服务草案）还是更新 `enqueue_at` 排到队尾？
3. **管理员置顶 TTL**：继续 30 天？
4. **删帖联动**：删帖时是否强制 `DequeueByPostID`？（建议强制）
5. **过渡期**：是否需要短时间「双写 posts + pin_queue」？

确认后我再继续完整接线与部署。
