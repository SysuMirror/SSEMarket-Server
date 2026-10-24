-- 2026-10-24 用户自定义皮肤（桌面/手机双图）+ 皮肤库 + 分享添加。
-- 代码 AutoMigrate 已包含；这里作为手动兜底。

CREATE TABLE IF NOT EXISTS `user_skins` (
  `skin_id`      int NOT NULL AUTO_INCREMENT,
  `owner_id`     int NOT NULL,
  `name`         varchar(50) NOT NULL,
  `description`  varchar(255) NOT NULL DEFAULT '',
  `desktop_url`  varchar(1024) NOT NULL DEFAULT '',
  `mobile_url`   varchar(1024) NOT NULL DEFAULT '',
  `is_public`    tinyint(1) NOT NULL DEFAULT 0,
  `share_token`  varchar(64) NOT NULL,
  `created_at`   datetime DEFAULT NULL,
  `updated_at`   datetime DEFAULT NULL,
  PRIMARY KEY (`skin_id`),
  UNIQUE KEY `uk_user_skin_share_token` (`share_token`),
  KEY `idx_user_skin_owner` (`owner_id`),
  KEY `idx_user_skin_public` (`is_public`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS `user_skin_library` (
  `id`       int NOT NULL AUTO_INCREMENT,
  `user_id`  int NOT NULL,
  `skin_id`  int NOT NULL,
  `added_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_skin_library` (`user_id`, `skin_id`),
  KEY `idx_user_skin_library_user` (`user_id`),
  KEY `idx_user_skin_library_skin` (`skin_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE `user_theme_settings`
  ADD COLUMN `custom_skin_id` int NOT NULL DEFAULT 0 AFTER `custom_background_url`;
