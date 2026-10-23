-- =============================================================================
-- 全站皮肤 / 外观设置 migration（2026-10-23，2026 程序员节皮肤）
-- 目标库: ssemarket
-- 背景:
--   建表以 common/database.go 的 AutoMigrate 为准（已加入 theme_events 与 user_theme_settings）。
--   本文件只是人工参考/应急脚本：AutoMigrate 因权限等原因未生效时手动执行一次。
-- 说明:
--   1. 全部为新增表，不动既有数据；可重复执行（IF NOT EXISTS）。
--   2. 活动时间窗在 theme_events 表里，一行一期活动，由管理员直接 INSERT；
--      代码不会自动插入任何活动，表为空时前端按「无活动」处理。
--   3. 没有行的用户 = 全默认：auto 跟随活动、开场画面开、背景开、皮肤自带背景。
--   4. starts_at / ends_at 一律按「北京时间墙钟」填写（DATETIME），与服务器时区无关。
-- =============================================================================

USE ssemarket;

CREATE TABLE IF NOT EXISTS `theme_events` (
  `event_id` INT NOT NULL AUTO_INCREMENT,
  `theme_id` VARCHAR(64) NOT NULL COMMENT '皮肤 ID，如 programmers-day-2026',
  `name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '展示名，如 2026 程序员节',
  `starts_at` DATETIME NOT NULL COMMENT '开始（北京时间墙钟，含）',
  `ends_at` DATETIME NOT NULL COMMENT '结束（北京时间墙钟，不含）',
  `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '0=临时停用',
  `created_at` DATETIME NULL,
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`event_id`),
  UNIQUE KEY `uk_theme_event_window` (`theme_id`, `starts_at`),
  KEY `idx_theme_event_starts` (`starts_at`),
  KEY `idx_theme_event_ends` (`ends_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='节日皮肤活动时间窗（一行一期）';

-- 增量（2026-10-24）：帖子卡片透明度。新库已包含在上面的 CREATE TABLE 里；
-- 已建过表的库执行下面一行即可（AutoMigrate 启动时也会自动加列，二选一）。
-- ALTER TABLE `user_theme_settings`
--   ADD COLUMN `post_card_opacity` TINYINT UNSIGNED NOT NULL DEFAULT 70
--   COMMENT '帖子卡片不透明度 30-100，仅开启全站背景时生效';

-- 示例：需要时手动执行（代码不会自动插入）
-- INSERT INTO `theme_events` (`theme_id`, `name`, `starts_at`, `ends_at`, `enabled`)
-- VALUES ('programmers-day-2026', '2026 程序员节',
--         '2026-10-24 00:00:00', '2026-10-25 00:00:00', 1);

CREATE TABLE IF NOT EXISTS `user_theme_settings` (
  `user_id` INT NOT NULL,
  `theme_mode` VARCHAR(64) NOT NULL DEFAULT 'auto'
    COMMENT 'auto/default/<skin-id>，如 programmers-day-2026',
  `splash_enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `background_enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `background_source` VARCHAR(16) NOT NULL DEFAULT 'default'
    COMMENT 'default=皮肤自带背景 / custom=图床上传图片',
  `custom_background_url` VARCHAR(2048) NOT NULL DEFAULT ''
    COMMENT '图床返回的 http(s) 绝对地址；不接受 public 相对路径',
  `post_card_opacity` TINYINT UNSIGNED NOT NULL DEFAULT 70
    COMMENT '帖子卡片不透明度 30-100，仅开启全站背景时生效',
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户全站皮肤/外观设置';
