-- 2026-11-18 点赞幂等：同一用户对同一帖子只允许一条点赞明细。
-- 执行顺序：先清理重复，再建唯一索引，最后按明细修正 posts.like_num。
-- 建议低峰期执行；plikes 很大时先加临时复合索引，避免自连接去重过慢。

-- 1. 可选：大表先加临时索引，加速下面的自连接去重
-- ALTER TABLE plikes ADD INDEX idx_plikes_user_target_tmp (userID, ptargetID);

-- 2. 执行前先确认重复规模
SELECT userID, ptargetID, COUNT(*) AS c
FROM plikes
GROUP BY userID, ptargetID
HAVING c > 1;

-- 3. 每组只保留最早的 plikeID，删除并发漏洞产生的重复行
DELETE p1 FROM plikes p1
JOIN plikes p2
  ON p1.userID = p2.userID
 AND p1.ptargetID = p2.ptargetID
 AND p1.plikeID > p2.plikeID;

-- 4. 建唯一索引，作为后续并发点赞的最终裁判
ALTER TABLE plikes
  ADD UNIQUE KEY uk_plikes_user_target (userID, ptargetID);

-- 5. 按明细表修正点赞数（大表建议按 postID 分批执行）
UPDATE posts p
LEFT JOIN (
    SELECT ptargetID, COUNT(*) AS cnt
    FROM plikes
    GROUP BY ptargetID
) l ON l.ptargetID = p.postID
SET p.like_num = COALESCE(l.cnt, 0);

-- 6. 可选：删除临时索引
-- ALTER TABLE plikes DROP INDEX idx_plikes_user_target_tmp;

-- 7. 验证：应返回 0 行
SELECT p.postID, p.like_num, COUNT(l.plikeID) AS real_likes
FROM posts p
LEFT JOIN plikes l ON l.ptargetID = p.postID
GROUP BY p.postID, p.like_num
HAVING p.like_num <> COUNT(l.plikeID);
