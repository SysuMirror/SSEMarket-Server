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
  SELECT 'novel_ratings.comment_id' UNION ALL
  SELECT 'novel_comments.is_rating' UNION ALL
  SELECT 'coin_ledger'            UNION ALL
  SELECT 'coin_throw'             UNION ALL
  SELECT 'coin_throw.thrown_total'
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

-- 评分自动附带的书评：novel_ratings 挂它的 comment_id；
-- novel_comments 用 is_rating 标记这类评论（前端特殊渲染）
ALTER TABLE `novel_ratings`
  ADD COLUMN `comment_id` INT NOT NULL DEFAULT 0 COMMENT '评分自动附带的书评ID(0=未生成)';

ALTER TABLE `novel_comments`
  ADD COLUMN `is_rating` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '评分自动附带的书评';

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
  `comment_id` INT NOT NULL DEFAULT 0 COMMENT '评分自动附带的书评ID(0=未生成)',
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
  `coin_num`     INT NOT NULL DEFAULT 0 COMMENT '当前投着的币数(0-2)，取消投币会清零',
  `thrown_total` INT NOT NULL DEFAULT 0 COMMENT '历史累计投过的总枚数(取消不减)，投币流水幂等键序号',
  `created_at`  DATETIME NULL,
  `updated_at`  DATETIME NULL,
  PRIMARY KEY (`throw_id`),
  UNIQUE KEY `uk_coin_throw` (`user_id`, `target_type`, `target_id`),
  KEY `idx_coin_throw_user` (`user_id`),
  KEY `idx_coin_throw_target` (`target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='投币记录(每人每内容上限2币)';

-- coin_throw 补 thrown_total 列（2026-10-12 下午新增：取消投币后再投的流水幂等键）。
-- 老数据回填 thrown_total = coin_num：旧版流水幂等键序号 = 投完后的累计枚数，
-- 已用掉的最大序号就是当时的 coin_num，回填后新序号从这之后继续，不会撞旧键。
ALTER TABLE `coin_throw`
  ADD COLUMN `thrown_total` INT NOT NULL DEFAULT 0 COMMENT '历史累计投过的总枚数(取消不减)';
UPDATE `coin_throw` SET `thrown_total` = `coin_num` WHERE `thrown_total` = 0 AND `coin_num` > 0;

-- ---------------------------------------------------------------------------
-- 5.1) 取消投币上线后的数据修正（2026-10-12 晚，可重复执行）
-- 线上 bug：取消投币后再投，被误报「已经给这个内容投满 2 币了」。
-- 根因有两处，代码侧已修（见 repository/gorm/coin.go），这里修数据：
--   (a) 投币流水幂等键换格式：旧键 coin_throw:post:12:34:2 的序号取自「投完后的
--       累计枚数」，新键 coin_throw:<throw_id>:<序号> 的序号取自只增的 thrown_total。
--       老库 thrown_total 没回填（AutoMigrate 只加列、默认 0）时，取消后重投会算出
--       同一个序号 → 撞 uk_coin_dedup → 被当成「已投满」。新键带 throw_id，
--       与历史键彻底分开，不回填也不会撞；下面的回填只是把序号顺手对齐。
--   (b) 旧代码把「历史投票折算」（1 票 = 1 币，没扣过余额）写进了 coin_num，
--       于是取消投币会连折算票一起退成真币、经验重算也会多发经验。
--       coin_num 现在只记真币，折算票读取时由 pvotes 现算，所以按流水重算这一列。
-- 全部以 coin_ledger 为准（流水是唯一不会说谎的地方），因此可重复执行。
-- ---------------------------------------------------------------------------

-- (1) 有投币/退款流水的记录：coin_num = 投币支出 + 退款收入（当前真投着的币）
UPDATE `coin_throw` ct
JOIN (
  SELECT `user_id`, `target_type`, `target_id`,
         -SUM(`amount`) AS real_coins
  FROM `coin_ledger`
  WHERE `event_type` IN ('coin_throw', 'coin_refund')
  GROUP BY `user_id`, `target_type`, `target_id`
) l ON l.`user_id` = ct.`user_id`
   AND l.`target_type` = ct.`target_type`
   AND l.`target_id`   = ct.`target_id`
SET ct.`coin_num` = LEAST(GREATEST(l.`real_coins`, 0), 2);

-- (2) 没有任何投币/退款流水的记录：从没真投过币（旧折算票造成的非零值清零）
UPDATE `coin_throw` ct
LEFT JOIN `coin_ledger` l
       ON l.`user_id` = ct.`user_id`
      AND l.`target_type` = ct.`target_type`
      AND l.`target_id` = ct.`target_id`
      AND l.`event_type` IN ('coin_throw', 'coin_refund')
SET ct.`coin_num` = 0
WHERE l.`ledger_id` IS NULL AND ct.`coin_num` > 0;

-- (3) thrown_total 对齐到「历史投出去过的总枚数」（只增不减，取流水累计与现值较大者）
UPDATE `coin_throw` ct
JOIN (
  SELECT `user_id`, `target_type`, `target_id`, -SUM(`amount`) AS thrown
  FROM `coin_ledger`
  WHERE `event_type` = 'coin_throw'
  GROUP BY `user_id`, `target_type`, `target_id`
) l ON l.`user_id` = ct.`user_id`
   AND l.`target_type` = ct.`target_type`
   AND l.`target_id`   = ct.`target_id`
SET ct.`thrown_total` = GREATEST(ct.`thrown_total`, l.`thrown`);

-- (4) 自查：coin_num 与流水对不上的记录（应为 0 行）
SELECT ct.`throw_id`, ct.`user_id`, ct.`target_type`, ct.`target_id`, ct.`coin_num`,
       COALESCE(-SUM(l.`amount`), 0) AS ledger_coins
FROM `coin_throw` ct
LEFT JOIN `coin_ledger` l
       ON l.`user_id` = ct.`user_id`
      AND l.`target_type` = ct.`target_type`
      AND l.`target_id` = ct.`target_id`
      AND l.`event_type` IN ('coin_throw', 'coin_refund')
GROUP BY ct.`throw_id`, ct.`user_id`, ct.`target_type`, ct.`target_id`, ct.`coin_num`
HAVING ct.`coin_num` <> LEAST(GREATEST(COALESCE(-SUM(l.`amount`), 0), 0), 2);

-- =============================================================================
-- 回滚（按需执行；additive 变更回滚即删列/删表，会丢币数据，慎执行）
-- ALTER TABLE users  DROP COLUMN coins, DROP COLUMN experience, DROP COLUMN novel_points;
-- ALTER TABLE posts  DROP COLUMN coin_num;
-- ALTER TABLE novels DROP COLUMN coin_num, DROP COLUMN rating_sum, DROP COLUMN rating_count;
-- ALTER TABLE novel_comments DROP COLUMN is_rating;
-- DROP TABLE IF EXISTS coin_throw, coin_ledger, novel_ratings, novel_ledger;
-- =============================================================================
