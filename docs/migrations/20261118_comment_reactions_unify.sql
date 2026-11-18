-- 2026-11-18 评论反应合并：pclikes/pcdenies/cclikes/ccdenies -> comment_reactions。
-- 新表用 (userID, targetType, targetID) 唯一键保证一个用户对一条评论只有一种反应。
-- 旧表先保留，确认新表数据无误后再手动删除。

CREATE TABLE IF NOT EXISTS comment_reactions (
    reactionID INT NOT NULL AUTO_INCREMENT,
    userID INT NOT NULL,
    targetType VARCHAR(16) NOT NULL,
    targetID INT NOT NULL,
    reaction VARCHAR(8) NOT NULL,
    PRIMARY KEY (reactionID),
    UNIQUE KEY uk_comment_reactions_user_target (userID, targetType, targetID)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 回填：先赞后踩。如果同一用户/目标在两张旧表都有记录，INSERT IGNORE 保留先插入的赞。
INSERT IGNORE INTO comment_reactions (userID, targetType, targetID, reaction)
SELECT userID, 'pcomment', pctargetID, 'like' FROM pclikes;

INSERT IGNORE INTO comment_reactions (userID, targetType, targetID, reaction)
SELECT userID, 'pcomment', pctargetID, 'deny' FROM pcdenies;

INSERT IGNORE INTO comment_reactions (userID, targetType, targetID, reaction)
SELECT userID, 'ccomment', cctargetID, 'like' FROM cclikes;

INSERT IGNORE INTO comment_reactions (userID, targetType, targetID, reaction)
SELECT userID, 'ccomment', cctargetID, 'deny' FROM ccdenies;

-- 按新表修正评论点赞/点踩数。
UPDATE pcomments pc
LEFT JOIN (
    SELECT targetID, COUNT(*) cnt
    FROM comment_reactions
    WHERE targetType = 'pcomment' AND reaction = 'like'
    GROUP BY targetID
) l ON l.targetID = pc.pcommentID
LEFT JOIN (
    SELECT targetID, COUNT(*) cnt
    FROM comment_reactions
    WHERE targetType = 'pcomment' AND reaction = 'deny'
    GROUP BY targetID
) d ON d.targetID = pc.pcommentID
SET pc.like_num = COALESCE(l.cnt, 0),
    pc.deny_num = COALESCE(d.cnt, 0);

UPDATE ccomments cc
LEFT JOIN (
    SELECT targetID, COUNT(*) cnt
    FROM comment_reactions
    WHERE targetType = 'ccomment' AND reaction = 'like'
    GROUP BY targetID
) l ON l.targetID = cc.ccommentID
LEFT JOIN (
    SELECT targetID, COUNT(*) cnt
    FROM comment_reactions
    WHERE targetType = 'ccomment' AND reaction = 'deny'
    GROUP BY targetID
) d ON d.targetID = cc.ccommentID
SET cc.like_num = COALESCE(l.cnt, 0),
    cc.deny_num = COALESCE(d.cnt, 0);

-- 验证：应返回 0 行
SELECT userID, targetType, targetID, COUNT(*) AS c
FROM comment_reactions
GROUP BY userID, targetType, targetID
HAVING c > 1;

-- 新旧行数对比（可选）
SELECT 'pclikes' AS src, COUNT(*) AS cnt FROM pclikes
UNION ALL SELECT 'pcdenies', COUNT(*) FROM pcdenies
UNION ALL SELECT 'cclikes', COUNT(*) FROM cclikes
UNION ALL SELECT 'ccdenies', COUNT(*) FROM ccdenies
UNION ALL SELECT 'comment_reactions', COUNT(*) FROM comment_reactions;

-- 确认新表数据无误后再执行（会删旧数据，先备份）：
-- DROP TABLE pclikes;
-- DROP TABLE pcdenies;
-- DROP TABLE cclikes;
-- DROP TABLE ccdenies;
