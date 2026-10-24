-- 2026-10-24 币体系改为两位小数 + 修正程序员节发放金额 + 补发帖子作者投币分成。
--
-- 背景：
--   1. 币余额/流水原来是 int，装不下 10.24；这里改为 DECIMAL(12,2)。
--   2. 本次程序员节福利应为每人 10.24 币（此前按整数发了 10 币，补 +0.24）。
--   3. 帖子被投币时，作者应获得「0.1 × 投币数」的币分成；历史真实投币
--      （coin_throw.coin_num，不含投票折算）统一补发，投票转硬币的那部分不动。
--
-- 幂等：每一步都只处理「尚未修正」的行；重复执行不会重复加钱。
-- 执行：mysql --default-character-set=utf8mb4 ...

START TRANSACTION;

-- 1. 币余额 / 币流水改为两位小数 -----------------------------------------
ALTER TABLE users
  MODIFY COLUMN coins DECIMAL(12,2) NOT NULL DEFAULT 0.00;

ALTER TABLE coin_ledger
  MODIFY COLUMN amount DECIMAL(12,2) NOT NULL DEFAULT 0.00;

-- 2. 程序员节福利 10.00 -> 10.24（仅补差额，重复执行无副作用） ---------------
CREATE TEMPORARY TABLE tmp_festival_1024 (
  user_id int NOT NULL PRIMARY KEY
) ENGINE=MEMORY;

INSERT INTO tmp_festival_1024 (user_id)
SELECT user_id
FROM coin_ledger
WHERE event_type = 'programmers_day_2026'
  AND amount = 10.00;

UPDATE coin_ledger
SET amount = 10.24
WHERE event_type = 'programmers_day_2026'
  AND amount = 10.00;

UPDATE users u
JOIN tmp_festival_1024 t ON t.user_id = u.userID
SET u.coins = u.coins + 0.24;

DROP TEMPORARY TABLE tmp_festival_1024;

-- 3. 帖子作者投币分成：历史真实投币补发 0.1 × coin_num ---------------------
--    coin_throw.coin_num 只记真币（可撤回、扣过余额的），不含旧投票折算；
--    作者 = 帖子作者，排除自己给自己投币的异常行。
CREATE TEMPORARY TABLE tmp_post_tip_income (
  throw_id  int NOT NULL PRIMARY KEY,
  author_id int NOT NULL,
  target_id int NOT NULL,
  amount    decimal(12,2) NOT NULL
) ENGINE=MEMORY;

INSERT INTO tmp_post_tip_income (throw_id, author_id, target_id, amount)
SELECT
  ct.throw_id,
  p.userID,
  ct.target_id,
  ROUND(ct.coin_num * 0.10, 2)
FROM coin_throw ct
JOIN posts p ON p.postID = ct.target_id
WHERE ct.target_type = 'post'
  AND ct.coin_num > 0
  AND p.userID > 0
  AND p.userID <> ct.user_id
  AND NOT EXISTS (
    SELECT 1 FROM coin_ledger l
    WHERE l.dedup_key = CONCAT('post_tip_income:backfill:', ct.throw_id)
  );

INSERT INTO coin_ledger (user_id, target_type, target_id, event_type, amount, settled_value, dedup_key, created_at)
SELECT
  t.author_id,
  'post',
  t.target_id,
  'post_tip_income',
  t.amount,
  0,
  CONCAT('post_tip_income:backfill:', t.throw_id),
  NOW()
FROM tmp_post_tip_income t;

UPDATE users u
JOIN (
  SELECT author_id, SUM(amount) AS amount
  FROM tmp_post_tip_income
  GROUP BY author_id
) t ON t.author_id = u.userID
SET u.coins = u.coins + t.amount;

DROP TEMPORARY TABLE tmp_post_tip_income;

COMMIT;

-- 验证 ---------------------------------------------------------------------
-- SELECT event_type, COUNT(*), SUM(amount) FROM coin_ledger
--   WHERE event_type IN ('programmers_day_2026','post_tip_income') GROUP BY event_type;
-- SELECT COUNT(*) FROM users;
