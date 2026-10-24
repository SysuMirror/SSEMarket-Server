-- 2026-10-24 勋章系统建表（代码 side 已由 AutoMigrate 兜底，这里用于手动迁移）。
--
-- 三张表：
--   badges             勋章目录（管理员上传图床后 INSERT 一行）
--   user_badges        发放记录（批量 INSERT ... SELECT，唯一索引保证幂等）
--   user_badge_prefs   展示偏好（对外挂载 1 枚 / 主页多选或全不显示）
--
-- 执行：mysql --default-character-set=utf8mb4 ...

CREATE TABLE IF NOT EXISTS `badges` (
  `badge_id`    int NOT NULL AUTO_INCREMENT,
  `code`        varchar(64) NOT NULL,
  `name`        varchar(64) NOT NULL DEFAULT '',
  `description` varchar(255) NOT NULL DEFAULT '',
  `image_url`   varchar(1024) NOT NULL DEFAULT '',
  `category`    varchar(32) NOT NULL DEFAULT '',
  `sort_order`  int NOT NULL DEFAULT 0,
  `enabled`     tinyint(1) NOT NULL DEFAULT 1,
  `created_at`  datetime DEFAULT NULL,
  `updated_at`  datetime DEFAULT NULL,
  PRIMARY KEY (`badge_id`),
  UNIQUE KEY `uk_badge_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_badges` (
  `id`         int NOT NULL AUTO_INCREMENT,
  `user_id`    int NOT NULL,
  `badge_id`   int NOT NULL,
  `award_key`  varchar(128) NOT NULL,
  `source`     varchar(16) NOT NULL DEFAULT 'auto',
  `note`       varchar(255) NOT NULL DEFAULT '',
  `awarded_at` datetime NOT NULL,
  `created_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_badge` (`user_id`, `badge_id`),
  UNIQUE KEY `uk_user_badge_award` (`user_id`, `award_key`),
  KEY `idx_user_badge_user` (`user_id`),
  KEY `idx_user_badge_badge` (`badge_id`),
  KEY `idx_user_badge_time` (`awarded_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_badge_prefs` (
  `user_id`          int NOT NULL,
  `worn_badge_id`    int NOT NULL DEFAULT 0,
  `profile_badge_ids` text,
  `updated_at`       datetime DEFAULT NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
