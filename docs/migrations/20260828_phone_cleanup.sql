-- Phone 字段残留清理（2026-08-28）
-- 目标：消除空 phone / phone='0' 等可被 OR 查询误命中的脏数据。
-- 执行前请备份 users 表。

-- 1. 空 phone 回填为 legacy_{userID}（唯一占位，不参与业务查询）
UPDATE users
SET phone = CONCAT('legacy_', userID)
WHERE phone IS NULL OR TRIM(phone) = '';

-- 2. 注销用户 phone='0' 改为 deleted_{userID}
UPDATE users
SET phone = CONCAT('deleted_', userID)
WHERE phone = '0';

-- 3. 校验：不应再存在可误查的 phone
-- SELECT userID, name, phone FROM users WHERE phone IN ('', '0') OR phone IS NULL;
