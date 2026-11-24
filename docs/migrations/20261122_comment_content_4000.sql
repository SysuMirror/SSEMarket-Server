-- 2026-11-22 集市评论/楼中楼支持 4000 字（YZ-0003）。
--
-- 背景：前端评论输入框按 4000 字设计，但后端 controller/service/MCP 三处都把
-- 上限卡在 1000，且 pcomments.pctext 为 varchar(1000)、ccomments.cctext 为
-- varchar(100)，4000 字长文在参数校验阶段就被 400 拒绝。
--
-- 代码侧：config.CommentContentMaxRunes = 4000，model/pcomment.go、
-- model/ccomment.go 的列类型已改为 varchar(4000)，服务启动时 AutoMigrate
-- 会自动扩列；本文件是手动兜底 / 线上先行的 DDL。
--
-- 注意：MODIFY COLUMN 会连同 NULL / DEFAULT 一起重定义。执行前先确认现状：
--   SHOW CREATE TABLE pcomments;
--   SHOW CREATE TABLE ccomments;
-- 若当前列带 NOT NULL / DEFAULT，请把下面的定义补全成与之一致。

ALTER TABLE pcomments MODIFY COLUMN pctext VARCHAR(4000) NULL;
ALTER TABLE ccomments MODIFY COLUMN cctext VARCHAR(4000) NULL;

-- 验证：
-- SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.columns
--  WHERE table_schema = DATABASE() AND table_name = 'pcomments' AND column_name = 'pctext';
-- SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.columns
--  WHERE table_schema = DATABASE() AND table_name = 'ccomments' AND column_name = 'cctext';

-- 回滚（先确认没有超过旧长度的数据）：
-- ALTER TABLE pcomments MODIFY COLUMN pctext VARCHAR(1000) NULL;
-- ALTER TABLE ccomments MODIFY COLUMN cctext VARCHAR(100) NULL;
