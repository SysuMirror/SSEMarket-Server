-- SSO 单点登录（UniSSO）建表 SQL
-- 在生产 MySQL ssemarket 库执行
-- 详见 docs/superpowers/plans/2026-08-26-sso-login-deploy.md

USE ssemarket;

CREATE TABLE IF NOT EXISTS `sso_bindings` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `local_user_id` INT NOT NULL,
  `sso_sub` VARCHAR(64) NOT NULL,
  `sso_username` VARCHAR(100) NOT NULL DEFAULT '',
  `created_at` DATETIME NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_sso_sub` (`sso_sub`),
  UNIQUE KEY `uniq_local_user_id` (`local_user_id`),
  KEY `idx_sso_bindings_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 回滚：DROP TABLE IF EXISTS sso_bindings;
