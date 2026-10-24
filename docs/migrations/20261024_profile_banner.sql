-- 2026-10-24 个人主页顶部栏板背景（手动兜底；代码 AutoMigrate 也会加列）。
--
-- profile_banner_url：图床返回的绝对 http(s) 地址，空 = 默认藏青渐变；
-- profile_banner_opacity：0-100，主页主人设置，所有访问者共用。

ALTER TABLE users
  ADD COLUMN `profile_banner_url` varchar(1024) NOT NULL DEFAULT '' AFTER `avatar_url`,
  ADD COLUMN `profile_banner_opacity` tinyint unsigned NOT NULL DEFAULT 100 AFTER `profile_banner_url`;
