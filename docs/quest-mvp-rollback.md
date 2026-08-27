# Quest MVP 回滚说明

## 代码回滚

Quest 为独立模块，删除或 revert 以下路径即可，**不修改**原有 `Post`/`Browse`/`Chat` 控制器逻辑：

- `controller/quest_controller.go`
- `service/quest_service.go`
- `config/quest.go`
- `domain/quest.go`
- `repository/quest.go`
- `repository/gorm/quest.go`
- `route/routes.go` 中 Quest 路由段
- `service/registry.go` 中 `Quest` 初始化
- `quest/` 前端目录

`model/post.go`、`model/user.go`、`repository/gorm/mapper.go` 中新增字段为可选列，回滚代码后旧二进制忽略新列即可。

## 数据库回滚（可选，确认无 Quest 帖后再执行）

```sql
-- 仅删除 Quest 帖（post_kind 非空）
DELETE FROM posts WHERE post_kind IN ('bounty', 'inquiry');

DROP INDEX idx_posts_post_kind_time ON posts;

ALTER TABLE posts
  DROP COLUMN quote_price,
  DROP COLUMN post_kind;

ALTER TABLE users DROP COLUMN user_type;
```

## 测试外院账号

测试期在服务器环境变量设置：

```bash
QUEST_EXTERNAL_USER_IDS=123,456
```

或 `application.yml` → `quest.external_user_ids`。
