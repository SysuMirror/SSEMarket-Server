-- =============================================================================
-- Scheme B: 独立置顶队列表 migration
-- 目标库: ssemarket
-- 说明:
--   1. 新建 pin_queue（当前有效置顶队列）+ pin_events（审计流水，可选但推荐）
--   2. 从 posts 内联字段回填当前未过期置顶（兼容 Scheme A 存量）
--   3. 不删除 posts.is_pinned / pinned_at / pinned_expires_at（回滚窗口保留）
--   4. 应用切换到读 pin_queue 后，再另开 migration 停止回写 posts 置顶字段
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1) 主页置顶队列（有界 FIFO）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pin_queue` (
  `pin_id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '队列主键',
  `scope` VARCHAR(32) NOT NULL DEFAULT 'home' COMMENT '队列作用域；当前仅 home，预留多版面',
  `post_id` INT NOT NULL COMMENT '帖子 ID（posts.postID）',
  `owner_user_id` INT NOT NULL DEFAULT 0 COMMENT '置顶发起人；用户置顶=作者，管理员置顶=管理员 ID',
  `source` VARCHAR(16) NOT NULL DEFAULT 'user' COMMENT '来源: user | admin',
  `enqueue_at` DATETIME(3) NOT NULL COMMENT '入队时间；FIFO 排序键',
  `expire_at` DATETIME(3) NOT NULL COMMENT '过期时间；到期后出队',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`pin_id`),
  -- 同一 scope 下同一帖只能在队一次
  UNIQUE KEY `uk_pin_scope_post` (`scope`, `post_id`),
  -- 列表查询: scope + 未过期 + FIFO
  KEY `idx_pin_scope_expire_enqueue` (`scope`, `expire_at`, `enqueue_at`, `post_id`),
  -- 每人上限: scope + owner + 未过期
  KEY `idx_pin_scope_owner_expire` (`scope`, `owner_user_id`, `expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='主页置顶队列(Scheme B)';

-- ---------------------------------------------------------------------------
-- 2) 置顶审计流水（生产级：可追溯入队/出队/过期）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `pin_events` (
  `event_id` BIGINT NOT NULL AUTO_INCREMENT,
  `scope` VARCHAR(32) NOT NULL DEFAULT 'home',
  `post_id` INT NOT NULL,
  `actor_user_id` INT NOT NULL DEFAULT 0 COMMENT '操作者；系统过期清理可为 0',
  `action` VARCHAR(16) NOT NULL COMMENT 'enqueue | dequeue | expire | reject',
  `source` VARCHAR(16) NOT NULL DEFAULT 'user' COMMENT 'user | admin | system',
  `detail` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '拒绝原因/附加说明',
  `enqueue_at` DATETIME(3) NULL COMMENT '关联入队时间（若有）',
  `expire_at` DATETIME(3) NULL COMMENT '关联过期时间（若有）',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`event_id`),
  KEY `idx_pin_events_post_time` (`post_id`, `created_at`),
  KEY `idx_pin_events_scope_time` (`scope`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='置顶操作审计流水';

-- ---------------------------------------------------------------------------
-- 3) 存量迁移：从 Scheme A 内联字段导入当前未过期置顶
--    仅导入 partition=主页 且 pinned_expires_at > NOW() 的记录
--    按 pinned_at ASC 保持原 FIFO 语义
-- ---------------------------------------------------------------------------
INSERT INTO `pin_queue` (`scope`, `post_id`, `owner_user_id`, `source`, `enqueue_at`, `expire_at`)
SELECT
  'home' AS scope,
  p.`postID` AS post_id,
  p.`userID` AS owner_user_id,
  'user' AS source,
  COALESCE(p.`pinned_at`, p.`post_time`) AS enqueue_at,
  p.`pinned_expires_at` AS expire_at
FROM `posts` p
WHERE p.`is_pinned` = 1
  AND p.`pinned_expires_at` IS NOT NULL
  AND p.`pinned_expires_at` > NOW()
  AND p.`partition` = '主页'
ORDER BY COALESCE(p.`pinned_at`, p.`post_time`) ASC, p.`postID` ASC
ON DUPLICATE KEY UPDATE
  `owner_user_id` = VALUES(`owner_user_id`),
  `enqueue_at` = VALUES(`enqueue_at`),
  `expire_at` = VALUES(`expire_at`),
  `updated_at` = CURRENT_TIMESTAMP(3);

-- 审计：记录一次 migrate_from_scheme_a
INSERT INTO `pin_events` (`scope`, `post_id`, `actor_user_id`, `action`, `source`, `detail`, `enqueue_at`, `expire_at`)
SELECT
  'home',
  q.`post_id`,
  q.`owner_user_id`,
  'enqueue',
  'system',
  'migrate_from_scheme_a',
  q.`enqueue_at`,
  q.`expire_at`
FROM `pin_queue` q
WHERE q.`scope` = 'home';

-- ---------------------------------------------------------------------------
-- 4) 回滚脚本（单独执行，勿与 forward 同跑）
-- ---------------------------------------------------------------------------
-- DROP TABLE IF EXISTS `pin_events`;
-- DROP TABLE IF EXISTS `pin_queue`;
-- -- posts.is_pinned 等字段保留，可回退到 Scheme A 读路径
