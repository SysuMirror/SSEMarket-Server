-- Additive schema for the native PAT MCP endpoint; alternatively AutoMigrate.
-- Do not drop these tables on rollback: action keys prevent duplicate writes.
CREATE TABLE IF NOT EXISTS forum_agent_actions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    action_key VARCHAR(64) NOT NULL,
    payload_hash CHAR(64) NOT NULL,
    result LONGTEXT NULL,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    UNIQUE KEY uq_forum_agent_action (user_id, action_key)
);

CREATE TABLE IF NOT EXISTS forum_agent_media (
    media_ref CHAR(36) NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    url TEXT NOT NULL,
    width BIGINT NULL,
    height BIGINT NULL,
    created_at DATETIME(3) NULL,
    KEY idx_forum_agent_media_user_id (user_id)
);
