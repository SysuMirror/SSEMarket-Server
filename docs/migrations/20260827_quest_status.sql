-- Quest 帖子状态（additive）
ALTER TABLE posts
  ADD COLUMN quest_status VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'open|in_progress|completed|cancelled' AFTER quote_price;

UPDATE posts SET quest_status = 'in_progress' WHERE post_kind = 'bounty' AND quest_status = '';
UPDATE posts SET quest_status = 'open' WHERE post_kind = 'inquiry' AND quest_status = '';
