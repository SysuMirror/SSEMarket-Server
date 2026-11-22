-- 2026-11-18 首页 Feed 全表扫描优化。
-- 2026-11-22 YZ-0009：model.Post.PostTime 已加 index:idx_posts_post_time，
-- 新库由 AutoMigrate 自动建；本文件保留为存量库手动兜底/回滚参考。
--
-- 背景：50 并发压测下首页流 browse 只有 186 rps 封顶、p50 260ms，MySQL 吃满
-- 230-270% CPU（机器共 4 核）。慢日志里的主犯是：
--   SELECT ... FROM posts WHERE (is_private = 0 OR userID = ?)
--   ORDER BY post_time DESC LIMIT 10
-- posts 上没有 post_time 索引，OR 又让优化器放弃索引 → type=ALL，
-- 每次扫全表 5232 行（平均 ptext 631 字节）再 filesort。
--
-- 加索引后同一语句：type=index / key=idx_posts_post_time / rows=10 /
-- Backward index scan，无 filesort，单条 10.5ms → 0.5-0.9ms。
-- 50 并发 browse：p50 260ms → 93ms，吞吐 186 → 495 rps。
--
-- 注意：OR 跨 is_private / userID 两列，无法走联合索引，靠 post_time 索引
-- 反向扫描 + LIMIT 提前终止来收敛代价；如果以后去掉 is_private 或改成
-- 单独的可见性表，可以再评估 (is_private, post_time) 联合索引。

ALTER TABLE posts ADD INDEX idx_posts_post_time (post_time);

-- 回滚：
-- ALTER TABLE posts DROP INDEX idx_posts_post_time;
