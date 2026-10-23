-- =============================================================================
-- 全站皮肤 / 外观设置 migration（2026-10-23，2026 程序员节皮肤）
-- 目标库: ssemarket
-- 背景:
--   建表以 common/database.go 的 AutoMigrate 为准（已加入 user_theme_settings）。
--   本文件只是人工参考/应急脚本：AutoMigrate 因权限等原因未生效时手动执行一次。
-- 说明:
--   1. 全部为新增表，不动既有数据；可重复执行（IF NOT EXISTS）。
--   2. 活动时间窗不在数据库里，写死在 Go 的 domain/theme.go 注册表，
--      活动结束后代码无需迁移，前端会自动回到默认皮肤。
--   3. 没有行的用户 = 全默认：auto 跟随活动、开场画面开、背景开、皮肤自带背景。
-- =============================================================================

USE ssemarket;

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
  `updated_at` DATETIME NULL,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户全站皮肤/外观设置';
