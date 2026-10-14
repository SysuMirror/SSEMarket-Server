-- =============================================================================
-- 职位账号（开发人员匿名 @）migration
-- 目标库: ssemarket
-- 说明:
--   1. 建表由 GORM AutoMigrate 负责（common/database.go），本文件只是
--      「人工参考 / 应急兜底」脚本，与 docs/migrations/ 其它文件同一约定。
--   2. 管理员日常操作只有第 2 节的 INSERT：插一行 → 重启（或不重启，首次被
--      使用时自动生效）。职位小号（users 表的派生行）由系统自动建，勿手插。
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1) 表结构（AutoMigrate 已建，这里仅对照）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `staff_aliases` (
  `id` INT NOT NULL AUTO_INCREMENT,
  `alias` VARCHAR(50) NOT NULL COMMENT '被 @ 的对外名字，也是职位主页昵称',
  `title` VARCHAR(50) NOT NULL COMMENT '职称（主页称号）',
  `staff_user_id` INT NOT NULL COMMENT '真实开发者 userID：只用于收通知与回复鉴权，绝不外露',
  `alias_user_id` INT NOT NULL DEFAULT 0 COMMENT '职位小号 userID（系统自动派生，管理员留 0）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_staff_alias_alias` (`alias`),
  KEY `idx_staff_alias_staff` (`staff_user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ---------------------------------------------------------------------------
-- 2) 管理员插入一个职位（唯一需要做的事）
--    staff_user_id 用真实开发者的 userID（管理后台 showUsers 可查）。
-- ---------------------------------------------------------------------------
-- INSERT INTO staff_aliases (alias, title, staff_user_id)
-- VALUES ('iwiki开发人员', 'iwiki开发人员', 123);

-- ---------------------------------------------------------------------------
-- 3) 常用运维
-- ---------------------------------------------------------------------------
-- 列出全部职位与派生小号：
--   SELECT id, alias, title, staff_user_id, alias_user_id FROM staff_aliases;
-- 改职称（重启后同步到小号称号；运行期由懒同步兜底）：
--   UPDATE staff_aliases SET title = 'iwiki核心开发' WHERE alias = 'iwiki开发人员';
-- 下线一个职位（软做法：管理员直接 DELETE 该行；小号 users 行会留下，
--   如需彻底清理再手动 DELETE users 里对应 email = 'staff-<id>@staff.internal' 的行）：
--   DELETE FROM staff_aliases WHERE alias = 'iwiki开发人员';
-- 查某职位的小号（对外身份）：
--   SELECT u.* FROM users u JOIN staff_aliases s ON u.userID = s.alias_user_id
--   WHERE s.alias = 'iwiki开发人员';
