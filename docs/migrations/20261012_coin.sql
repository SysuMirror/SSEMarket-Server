-- =============================================================================
-- 投币体系 + 小说评分 migration（2026-10-12）
-- 目标库: ssemarket
-- 背景:
--   代码里 common/database.go 的 AutoMigrate 本应自动建这些列/表，但线上未生效
--   （visit 报 Error 1054: Unknown column 'coins'，推测 DB 账号无 DDL 权限或
--   迁移失败被旧版代码静默吞掉——现已改为打日志，见 [db] automigrate ... failed）。
--   本文件是手动兜底：AutoMigrate 不生效时，用有 DDL 权限的账号执行一次。
-- 说明:
--   1. 全部 additive，不动既有数据；历史投票折算（1 票 = 1 币）是读取时计算的
--      （repository/gorm/coin.go legacyVoteCoins 读 pvotes），不需要数据回填。
--   2. 建表用 IF NOT EXISTS，可重复执行；ALTER 若列已存在会报 1060/1061，
--      忽略该条继续执行下一条即可（MySQL 8 无 ADD COLUMN IF NOT EXISTS）。
--   3. 执行前可先跑「第 0 节」自查缺了哪些；执行后重启服务并清 Redis browse
--      feed 缓存（旧缓存里 PostID=0，见 ef0c6e1）。
-- =============================================================================

USE ssemarket;

-- ---------------------------------------------------------------------------
-- 0) 自查：列出缺失的列与表（只读，先跑这个）
-- ---------------------------------------------------------------------------
SELECT t.name AS need, IF(c.table_name IS NOT NULL, 'OK', 'MISSING') AS status
FROM (
  SELECT 'users.coins'            AS name UNION ALL
  SELECT 'users.experience'       UNION ALL
  SELECT 'users.novel_points'     UNION ALL
  SELECT 'posts.coin_num'         UNION ALL
  SELECT 'novels.coin_num'        UNION ALL
  SELECT 'novels.rating_sum'      UNION ALL
  SELECT 'novels.rating_count'    UNION ALL
  SELECT 'novel_ledger'           UNION ALL
  SELECT 'novel_ratings'          UNION ALL
  SELECT 'coin_ledger'            UNION ALL
  SELECT 'coin_throw'
) t
LEFT JOIN information_schema.columns c
  ON c.table_schema = DATABASE()
 AND c.table_name = SUBSTRING_INDEX(t.name, '.', 1)
 AND (INSTR(t.name, '.') = 0 OR c.column_name = SUBSTRING_INDEX(t.name, '.', -1))
GROUP BY t.name, c.table_name
ORDER BY t.name;

-- ---------------------------------------------------------------------------
-- 1) 既有表加列（已存在则报 1060 Duplicate column，忽略继续）
-- ---------------------------------------------------------------------------
-- 站内币余额（赚币/投币口径见 domain/coin.go）
ALTER TABLE `users`
  ADD COLUMN `coins` INT NOT NULL DEFAULT 0 COMMENT '站内币余额';

-- 小说经验/积分（20261011_novel.sql 之后的积分/经验体系，若线上也缺一并补）
ALTER TABLE `users`
  ADD COLUMN `experience`    INT NOT NULL DEFAULT 0 COMMENT '读者经验',
  ADD COLUMN `novel_points`  INT NOT NULL DEFAULT 0 COMMENT '作者积分，单位分(1积分=10分)';

-- 被投币总数（展示用计数；历史票数 vote_num 读取时折算，不改写）
ALTER TABLE `posts`
  ADD COLUMN `coin_num` INT NOT NULL DEFAULT 0 COMMENT '被投币总数(展示用)';

ALTER TABLE `novels`
  ADD COLUMN `coin_num`     INT NOT NULL DEFAULT 0 COMMENT '被投币总数(展示用)',
  ADD COLUMN `rating_sum`   INT NOT NULL DEFAULT 0 COMMENT '评分总和(平均分=rating_sum/rating_count)',
  ADD COLUMN `rating_count` INT NOT NULL DEFAULT 0 COMMENT '评分人数';

-- ---------------------------------------------------------------------------
-- 2) 小说积分/经验流水（若 20261011 之后线上也没建过这张表）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novel_ledger` (
  `ledger_id`     INT NOT NULL AUTO_INCREMENT,
  `user_id`       INT NOT NULL,
  `novel_id`      INT NOT NULL DEFAULT 0,
  `event_type`    VARCHAR(32) NOT NULL COMMENT '事件类型，见 domain/novel_points.go',
  `amount`        INT NOT NULL DEFAULT 0 COMMENT '发放量；积分以分记账',
  `settled_value` INT NOT NULL DEFAULT 0 COMMENT '里程碑事件原始量游标(阅读秒/写作字)',
  `dedup_key`     VARCHAR(128) NOT NULL COMMENT '幂等键，唯一',
  `created_at`    DATETIME NULL,
  PRIMARY KEY (`ledger_id`),
  UNIQUE KEY `uk_ledger_dedup` (`dedup_key`),
  KEY `idx_ledger_user_time` (`user_id`, `created_at`),
  KEY `idx_ledger_novel` (`novel_id`),
  KEY `idx_ledger_event` (`event_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说积分/经验流水';

-- ---------------------------------------------------------------------------
-- 3) 小说评分（1–5 星，每人每本一条可改；作者首次收到该读者评分 +1 币）
--    注意列名是 score（不是 rating）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `novel_ratings` (
  `rating_id`  INT NOT NULL AUTO_INCREMENT,
  `novel_id`   INT NOT NULL,
  `user_id`    INT NOT NULL,
  `score`      INT NOT NULL DEFAULT 0 COMMENT '1–5 星',
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`rating_id`),
  UNIQUE KEY `uk_novel_rating` (`novel_id`, `user_id`),
  KEY `idx_novel_rating_novel` (`novel_id`),
  KEY `idx_novel_rating_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='小说评分(每人每本一条)';

-- ---------------------------------------------------------------------------
-- 4) 币流水（dedup_key 唯一索引保证幂等；Amount 正=发币，负=投币支出）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `coin_ledger` (
  `ledger_id`     INT NOT NULL AUTO_INCREMENT,
  `user_id`       INT NOT NULL,
  `target_type`   VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'post|novel；每日任务为空',
  `target_id`     INT NOT NULL DEFAULT 0,
  `event_type`    VARCHAR(32) NOT NULL COMMENT 'visit_daily|post_daily|read_daily|write_daily|rated|coin_throw|acc_read|acc_write',
  `amount`        INT NOT NULL DEFAULT 0,
  `settled_value` INT NOT NULL DEFAULT 0 COMMENT '纯计数事件(当日阅读秒/写作字)游标',
  `dedup_key`     VARCHAR(128) NOT NULL,
  `created_at`    DATETIME NULL,
  PRIMARY KEY (`ledger_id`),
  UNIQUE KEY `uk_coin_dedup` (`dedup_key`),
  KEY `idx_coin_user_time` (`user_id`, `created_at`),
  KEY `idx_coin_target` (`target_type`, `target_id`),
  KEY `idx_coin_event` (`event_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='站内币流水';

-- ---------------------------------------------------------------------------
-- 5) 投币记录（表名单数 coin_throw；唯一索引兜住「每人每内容最多 2 币」）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `coin_throw` (
  `throw_id`    INT NOT NULL AUTO_INCREMENT,
  `user_id`     INT NOT NULL,
  `target_type` VARCHAR(16) NOT NULL COMMENT 'post|novel',
  `target_id`   INT NOT NULL,
  `coin_num`    INT NOT NULL DEFAULT 0 COMMENT '累计投币数，上限 2',
  `created_at`  DATETIME NULL,
  `updated_at`  DATETIME NULL,
  PRIMARY KEY (`throw_id`),
  UNIQUE KEY `uk_coin_throw` (`user_id`, `target_type`, `target_id`),
  KEY `idx_coin_throw_user` (`user_id`),
  KEY `idx_coin_throw_target` (`target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='投币记录(每人每内容上限2币)';

-- =============================================================================
-- 回滚（按需执行；additive 变更回滚即删列/删表，会丢币数据，慎执行）
-- ALTER TABLE users  DROP COLUMN coins, DROP COLUMN experience, DROP COLUMN novel_points;
-- ALTER TABLE posts  DROP COLUMN coin_num;
-- ALTER TABLE novels DROP COLUMN coin_num, DROP COLUMN rating_sum, DROP COLUMN rating_count;
-- DROP TABLE IF EXISTS coin_throw, coin_ledger, novel_ratings, novel_ledger;
-- =============================================================================
