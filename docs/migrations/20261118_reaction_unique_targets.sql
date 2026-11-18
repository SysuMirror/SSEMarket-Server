-- 2026-11-18 浏览与评论反应幂等：同一用户对同一目标只允许一条明细。
-- 执行顺序：先清理重复，再建唯一索引，最后按明细修正计数。
-- 建议低峰期执行；大表可先加临时复合索引，避免自连接去重过慢。

-- ========== 1. 浏览去重 ==========
DELETE b1 FROM pbrowses b1
JOIN pbrowses b2
  ON b1.userID = b2.userID
 AND b1.ptargetID = b2.ptargetID
 AND b1.pbrowseID > b2.pbrowseID;

ALTER TABLE pbrowses
  ADD UNIQUE KEY uk_pbrowses_user_target (userID, ptargetID);

-- ========== 2. 一级评论点赞/点踩去重 ==========
DELETE l1 FROM pclikes l1
JOIN pclikes l2
  ON l1.userID = l2.userID
 AND l1.pctargetID = l2.pctargetID
 AND l1.pclikeID > l2.pclikeID;

ALTER TABLE pclikes
  ADD UNIQUE KEY uk_pclikes_user_target (userID, pctargetID);

DELETE d1 FROM pcdenies d1
JOIN pcdenies d2
  ON d1.userID = d2.userID
 AND d1.pctargetID = d2.pctargetID
 AND d1.pcdenyID > d2.pcdenyID;

ALTER TABLE pcdenies
  ADD UNIQUE KEY uk_pcdenies_user_target (userID, pctargetID);

-- ========== 3. 楼中楼点赞/点踩去重 ==========
DELETE l1 FROM cclikes l1
JOIN cclikes l2
  ON l1.userID = l2.userID
 AND l1.cctargetID = l2.cctargetID
 AND l1.cclikeID > l2.cclikeID;

ALTER TABLE cclikes
  ADD UNIQUE KEY uk_cclikes_user_target (userID, cctargetID);

DELETE d1 FROM ccdenies d1
JOIN ccdenies d2
  ON d1.userID = d2.userID
 AND d1.cctargetID = d2.cctargetID
 AND d1.ccdenyID > d2.ccdenyID;

ALTER TABLE ccdenies
  ADD UNIQUE KEY uk_ccdenies_user_target (userID, cctargetID);

-- ========== 4. 按明细修正评论点赞/点踩数 ==========
UPDATE pcomments pc
LEFT JOIN (SELECT pctargetID, COUNT(*) cnt FROM pclikes GROUP BY pctargetID) l
  ON l.pctargetID = pc.pcommentID
LEFT JOIN (SELECT pctargetID, COUNT(*) cnt FROM pcdenies GROUP BY pctargetID) d
  ON d.pctargetID = pc.pcommentID
SET pc.like_num = COALESCE(l.cnt, 0),
    pc.deny_num = COALESCE(d.cnt, 0);

UPDATE ccomments cc
LEFT JOIN (SELECT cctargetID, COUNT(*) cnt FROM cclikes GROUP BY cctargetID) l
  ON l.cctargetID = cc.ccommentID
LEFT JOIN (SELECT cctargetID, COUNT(*) cnt FROM ccdenies GROUP BY cctargetID) d
  ON d.cctargetID = cc.ccommentID
SET cc.like_num = COALESCE(l.cnt, 0),
    cc.deny_num = COALESCE(d.cnt, 0);

-- ========== 5. 可选：按评论明细修正帖子评论数 ==========
-- 注意：如果 posts.comment_num 还统计了评分评论等其他来源，请不要执行这一段，
-- 否则会把评分帖的评论数覆盖成 0。
-- UPDATE posts p
-- LEFT JOIN (
--     SELECT pc.ptargetID AS postID,
--            COUNT(DISTINCT pc.pcommentID) + COUNT(cc.ccommentID) AS cnt
--     FROM pcomments pc
--     LEFT JOIN ccomments cc ON cc.ctargetID = pc.pcommentID
--     GROUP BY pc.ptargetID
-- ) c ON c.postID = p.postID
-- SET p.comment_num = COALESCE(c.cnt, 0);

-- ========== 6. 验证 ==========
-- 浏览重复，应返回 0 行
SELECT userID, ptargetID, COUNT(*) AS c
FROM pbrowses
GROUP BY userID, ptargetID
HAVING c > 1;

-- 一级评论反应重复，应返回 0 行
SELECT userID, pctargetID, COUNT(*) AS c
FROM pclikes
GROUP BY userID, pctargetID
HAVING c > 1;

SELECT userID, pctargetID, COUNT(*) AS c
FROM pcdenies
GROUP BY userID, pctargetID
HAVING c > 1;

-- 楼中楼反应重复，应返回 0 行
SELECT userID, cctargetID, COUNT(*) AS c
FROM cclikes
GROUP BY userID, cctargetID
HAVING c > 1;

SELECT userID, cctargetID, COUNT(*) AS c
FROM ccdenies
GROUP BY userID, cctargetID
HAVING c > 1;

-- 评论反应计数，应返回 0 行
SELECT pc.pcommentID, pc.like_num, COALESCE(l.cnt, 0) AS real_likes,
       pc.deny_num, COALESCE(d.cnt, 0) AS real_denies
FROM pcomments pc
LEFT JOIN (SELECT pctargetID, COUNT(*) cnt FROM pclikes GROUP BY pctargetID) l
  ON l.pctargetID = pc.pcommentID
LEFT JOIN (SELECT pctargetID, COUNT(*) cnt FROM pcdenies GROUP BY pctargetID) d
  ON d.pctargetID = pc.pcommentID
WHERE pc.like_num <> COALESCE(l.cnt, 0)
   OR pc.deny_num <> COALESCE(d.cnt, 0);

SELECT cc.ccommentID, cc.like_num, COALESCE(l.cnt, 0) AS real_likes,
       cc.deny_num, COALESCE(d.cnt, 0) AS real_denies
FROM ccomments cc
LEFT JOIN (SELECT cctargetID, COUNT(*) cnt FROM cclikes GROUP BY cctargetID) l
  ON l.cctargetID = cc.ccommentID
LEFT JOIN (SELECT cctargetID, COUNT(*) cnt FROM ccdenies GROUP BY cctargetID) d
  ON d.cctargetID = cc.ccommentID
WHERE cc.like_num <> COALESCE(l.cnt, 0)
   OR cc.deny_num <> COALESCE(d.cnt, 0);
