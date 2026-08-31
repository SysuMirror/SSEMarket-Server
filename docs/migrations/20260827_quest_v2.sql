-- Quest v2: 金额类型、截止时间、图片附件、任务单
ALTER TABLE posts
  ADD COLUMN price_type VARCHAR(16) NOT NULL DEFAULT 'negotiable' COMMENT 'fixed|range|negotiable' AFTER quote_price,
  ADD COLUMN quote_price_min DECIMAL(10,2) NULL AFTER price_type,
  ADD COLUMN quote_price_max DECIMAL(10,2) NULL AFTER quote_price_min,
  ADD COLUMN quest_deadline DATETIME NULL COMMENT '任务截止时间' AFTER quest_status,
  ADD COLUMN quest_attachments JSON NULL COMMENT 'Quest 图片附件 JSON' AFTER quest_deadline;

CREATE TABLE IF NOT EXISTS quest_orders (
  order_id BIGINT PRIMARY KEY AUTO_INCREMENT,
  post_id BIGINT NOT NULL,
  publisher_id INT NOT NULL,
  worker_id INT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'accepted',
  accepted_at DATETIME NOT NULL,
  submitted_at DATETIME NULL,
  completed_at DATETIME NULL,
  cancelled_at DATETIME NULL,
  submit_note VARCHAR(500) NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE KEY uk_quest_order_post (post_id),
  INDEX idx_quest_order_worker (worker_id)
);
