-- Quest MVP：仅 additive，可回滚。现有帖与用户数据不受影响。
-- 执行前建议备份；未执行时旧服务仍可运行（新字段由 AutoMigrate 可选补齐）。

ALTER TABLE posts
  ADD COLUMN post_kind VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'quest: bounty|inquiry，空=普通帖' AFTER tag,
  ADD COLUMN quote_price DECIMAL(10,2) NULL DEFAULT NULL COMMENT '悬赏理想金额，求助须为 NULL' AFTER post_kind;

ALTER TABLE users
  ADD COLUMN user_type VARCHAR(16) NOT NULL DEFAULT 'market' COMMENT 'market|external，Quest 外院发帖' AFTER hide_profile_posts;

CREATE INDEX idx_posts_post_kind_time ON posts (post_kind, post_time DESC);

-- 回滚见 docs/quest-mvp-rollback.md
