-- =============================================================================
-- 表情包体系 migration（2026-10-13，同日修订：收藏表 → 个人表情组）
-- 目标库: ssemarket
-- 背景:
--   建表由 common/database.go 的 AutoMigrate 负责（model.EmojiPack / Emoji /
--   UserEmojiPack），内置表情由 seedBuiltinEmoji 幂等种子写入。
--   本文件只是人工参考 / AutoMigrate 不生效时的应急兜底，正常部署不需要执行。
-- 说明:
--   1. 全部 additive，不动既有数据；建表 IF NOT EXISTS 可重复执行。
--   2. 内置表情 URL 存裸文件名（乖巧.png），前端从 public/emoji/ 解析资源；
--      上传表情 URL 存图床绝对地址（内置组批量上图床的方案见 docs/emoji_builtin.md）。
--   3. 删除全部是软删除（isDeleted），历史聊天正文里的 [emoji:<id>] token
--      之后仍能渲染，所以不要物理删行。
--   4. 「我的表情」不再是收藏表，而是每人一个的个人表情组
--      （emoji_packs.isPersonal = 1，ownerUserID = 用户ID，默认空、首次放表情才建）。
--   5. 若之前建过旧表 user_emojis（收藏设计，已废弃），可以不管它；
--      想清理请在确认无数据后手动 DROP TABLE user_emojis。
-- =============================================================================

USE ssemarket;

-- ---------------------------------------------------------------------------
-- 1. 表
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS emoji_packs (
  packID INT NOT NULL AUTO_INCREMENT,
  title VARCHAR(100) NOT NULL,
  ownerUserID INT NOT NULL DEFAULT 0,
  isPersonal TINYINT(1) NOT NULL DEFAULT 0,
  isPublic TINYINT(1) NOT NULL DEFAULT 0,
  coverEmojiID INT NOT NULL DEFAULT 0,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  createdAt DATETIME NULL,
  PRIMARY KEY (packID),
  KEY idx_emoji_pack_owner (ownerUserID)
);

CREATE TABLE IF NOT EXISTS emojis (
  emojiID INT NOT NULL AUTO_INCREMENT,
  packID INT NOT NULL DEFAULT 0,
  name VARCHAR(100) NOT NULL,
  url VARCHAR(500) NOT NULL,
  isBuiltin TINYINT(1) NOT NULL DEFAULT 0,
  ownerUserID INT NOT NULL DEFAULT 0,
  isDeleted TINYINT(1) NOT NULL DEFAULT 0,
  createdAt DATETIME NULL,
  PRIMARY KEY (emojiID),
  KEY idx_emoji_pack (packID),
  KEY idx_emoji_builtin (isBuiltin),
  KEY idx_emoji_owner (ownerUserID)
);

CREATE TABLE IF NOT EXISTS user_emoji_packs (
  id INT NOT NULL AUTO_INCREMENT,
  userID INT NOT NULL,
  packID INT NOT NULL,
  createdAt DATETIME NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_user_emoji_pack (userID, packID)
);

-- 老库补列（AutoMigrate 会加，手动兜底用；新库建表已含该列，执行会报重复列、忽略即可）：
-- ALTER TABLE emoji_packs ADD COLUMN isPersonal TINYINT(1) NOT NULL DEFAULT 0;

-- ---------------------------------------------------------------------------
-- 2. 内置组「小汐（SSE娘）」+ 内置表情种子（与 domain.BuiltinEmojiFiles 对应）
--    AutoMigrate + seedBuiltinEmoji 会自动做；这里给手动兜底。
--    注意：内置表情只插「名字不存在」的，重复执行安全。
--    2026-10-13 起内置组标题为「小汐（SSE娘）」；老库里若还是「默认表情」，
--    启动种子会自动改名，也可手动执行下面的 UPDATE。
-- ---------------------------------------------------------------------------

INSERT INTO emoji_packs (title, ownerUserID, isPublic, createdAt)
SELECT '小汐（SSE娘）', 0, 1, NOW()
WHERE NOT EXISTS (SELECT 1 FROM emoji_packs WHERE ownerUserID = 0 AND isDeleted = 0);

UPDATE emoji_packs SET title = '小汐（SSE娘）'
WHERE ownerUserID = 0 AND isDeleted = 0 AND title = '默认表情';

-- 需要手动补内置表情时，按前端 public/emoji/ 的文件名逐条执行，例如：
-- INSERT INTO emojis (packID, name, url, isBuiltin, ownerUserID, createdAt)
-- SELECT packID, '乖巧', '乖巧.png', 1, 0, NOW() FROM emoji_packs
-- WHERE ownerUserID = 0 AND isDeleted = 0
--   AND NOT EXISTS (SELECT 1 FROM emojis WHERE isBuiltin = 1 AND name = '乖巧')
-- LIMIT 1;
