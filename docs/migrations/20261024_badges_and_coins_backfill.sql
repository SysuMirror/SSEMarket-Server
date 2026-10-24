-- 2026-10-24 首次勋章发放 + 程序员节 10.24 币福利（可重复执行，幂等）。
--
-- 内容：
--   1. 写入 4 枚勋章目录（图片已上传图床，URL 为 data[0].url 原样保存）；
--   2. 全站每位用户发放「2026 程序员节」勋章；
--   3. 按 users.banend 作为注册时间，补发已满 1/2/3 周年的纪念勋章；
--   4. 全站每位用户 +10.24 币（users.coins 为 DECIMAL(12,2)），
--      并写 coin_ledger 流水（dedup_key 保证重复执行不会多给）。
--
-- 注意：本项目历史表结构没有 users.created_at，注册时把时间写在 users.banend 里
-- （见 repository/gorm/user.go Create），因此这里把 banend 当作注册时间。
-- 执行：mysql --default-character-set=utf8mb4 ...

START TRANSACTION;

-- 1. 勋章目录 ---------------------------------------------------------------
INSERT INTO badges (code, name, description, image_url, category, sort_order, enabled, created_at, updated_at)
VALUES
  ('anniversary-1', '一周年纪念', '与集市相识满一年', 'https://image.ssemarket.cn/i/images/2026/10/24/99b407b0-43a1-4e96-863e-a39d7f36dd27.png', 'anniversary', 11, 1, NOW(), NOW()),
  ('anniversary-2', '二周年纪念', '与集市同行满两年', 'https://image.ssemarket.cn/i/images/2026/10/24/eb1b17cf-19f2-44d6-9d98-eb252378b60f.png', 'anniversary', 22, 1, NOW(), NOW()),
  ('anniversary-3', '三周年纪念', '与集市相伴满三年', 'https://image.ssemarket.cn/i/images/2026/10/24/ab3547a7-96d5-4cbc-ae85-1492f2f1154a.png', 'anniversary', 33, 1, NOW(), NOW()),
  ('programmers-day-2026', '2026 程序员节', '1024，码上相见。2026 程序员节限定', 'https://image.ssemarket.cn/i/images/2026/10/24/b659762e-379a-4de1-b5dc-cf113a98fa47.png', 'festival', 100, 1, NOW(), NOW())
ON DUPLICATE KEY UPDATE
  name        = VALUES(name),
  description = VALUES(description),
  image_url   = VALUES(image_url),
  category    = VALUES(category),
  sort_order  = VALUES(sort_order),
  enabled     = VALUES(enabled),
  updated_at  = NOW();

-- 2. 2026 程序员节：全站发放 ------------------------------------------------
INSERT IGNORE INTO user_badges (user_id, badge_id, award_key, source, note, awarded_at, created_at)
SELECT u.userID, b.badge_id, 'programmers-day-2026', 'auto', '2026 程序员节全站发放', NOW(), NOW()
FROM users u
JOIN badges b ON b.code = 'programmers-day-2026'
WHERE u.userID > 0;

-- 3. 注册周年：按已满年限补发 1/2/3 周年 --------------------------------
--    例：2023 年注册的用户会补发 1、2、3 周年三枚；2026 年注册的暂无。
INSERT IGNORE INTO user_badges (user_id, badge_id, award_key, source, note, awarded_at, created_at)
SELECT
  u.userID,
  b.badge_id,
  CONCAT('anniversary:', DATE_FORMAT(u.banend, '%Y'), ':', y.n),
  'auto',
  CONCAT('注册周年补发：满 ', y.n, ' 年'),
  NOW(),
  NOW()
FROM users u
JOIN (SELECT 1 AS n UNION ALL SELECT 2 UNION ALL SELECT 3) y
JOIN badges b ON b.code = CONCAT('anniversary-', y.n)
WHERE u.userID > 0
  AND u.banend IS NOT NULL
  AND u.banend > '2000-01-01'
  AND TIMESTAMPDIFF(YEAR, u.banend, NOW()) >= y.n;

-- 4. 程序员节福利：每人 +10.24，写流水 + 加余额 --------------------------------
CREATE TEMPORARY TABLE tmp_festival_coin_users (
  user_id int NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT INTO tmp_festival_coin_users (user_id)
SELECT u.userID
FROM users u
WHERE u.userID > 0
  AND NOT EXISTS (
    SELECT 1 FROM coin_ledger l
    WHERE l.dedup_key = CONCAT('programmers-day-2026:coin:', u.userID)
  );

INSERT INTO coin_ledger (user_id, target_type, target_id, event_type, amount, settled_value, dedup_key, created_at)
SELECT
  t.user_id,
  '',
  0,
  'programmers_day_2026',
  10.24,
  0,
  CONCAT('programmers-day-2026:coin:', t.user_id),
  NOW()
FROM tmp_festival_coin_users t;

UPDATE users u
JOIN tmp_festival_coin_users t ON t.user_id = u.userID
SET u.coins = u.coins + 10.24;

DROP TEMPORARY TABLE tmp_festival_coin_users;

COMMIT;

-- 验证 ---------------------------------------------------------------------
-- SELECT code, COUNT(*) FROM user_badges ub JOIN badges b ON b.badge_id = ub.badge_id GROUP BY code;
-- SELECT COUNT(*) AS coin_users, SUM(amount) AS total_coins FROM coin_ledger
--   WHERE dedup_key LIKE 'programmers-day-2026:coin:%';
