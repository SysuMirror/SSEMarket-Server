# 主站原生论坛 MCP（待合并）

本改动基于 `online` 的 `f20fa8b`。主站新增 `POST /api/auth/mcp`，让持有 PAT 的客户端直接使用完整论坛工具。
使用官方 `github.com/modelcontextprotocol/go-sdk/mcp` v1.8.0，支持现有客户端的 MCP 2025-06-18；协议解析、schema 校验、结构化输出和 Streamable HTTP 均由 SDK 实现。

原 `/api/v1/agent/mcp` 的 Python 上游不变。本次不复制小秋源码、不改 nginx、不部署服务；合并与发布后才可使用新入口。

## 鉴权与权限

每次请求必须带 `Authorization: Bearer <PAT>`；仅接受已有 `sk_xq_` PAT。按现有服务重新校验启用/撤销状态并加载账号，不接受登录 JWT、查询字符串 token 或调用参数中的 user_id。

- `forum_me`：所有有效 PAT，可核对自己的公开 ID、scopes 与能力。
- `forum:read`：列表、完整正文、评论、楼中楼和本人通知。
- `forum:write`：上传、发帖/评论/回复及查询本账号操作结果。
- 仅 `agent:chat` 的令牌没有论坛读写能力。写操作继续检查禁言。

新入口单独使用 PAT 鉴权，不把 PAT 放进全站 JWT 登录中间件。按账号复用现有 Redis 限流（30 次/分钟；Redis 故障时沿用既有限流器的 fail-open 行为）。SDK 限制请求体 12 MiB，启用同源请求保护。非浏览器客户端无需 Origin；同源浏览器可用，未开放跨源浏览器调用。

## 工具契约

SDK 的 `structuredContent` 返回 JSON，`content` 同时附等价 JSON 文本供旧客户端读取。业务异常统一脱敏。

| 工具 | 参数 | 结果 |
| --- | --- | --- |
| `forum_me` | `{}` | 当前 user_id/name、scopes、能力与限制；不含电话/邮箱 |
| `forum_feed_v2` | limit=15（最多30）、offset=0（最多10000）、可选 query/partition | posts、has_more、next_offset；内容为240字摘要，content_complete=false |
| `forum_get_post` | post_id | post 对象，完整 content、photos、公开作者、时间、content_complete=true |
| `forum_comments_v2` | post_id、after_id=0、limit=20（最多50） | comments、has_more、next_after_id；按 ID 升序，完整正文与 reply_count |
| `forum_replies_v2` | post_id、parent_comment_id、after_id=0、limit=20（最多50） | replies、has_more、next_after_id；按 ID 升序，完整楼中楼正文 |
| `forum_notifications` | before_id=0、limit=20（最多50）、read=false | notifications、has_more、next_before_id；只读本人未读或已读通知，不标已读 |
| `forum_upload_image` | idempotency_key、mime、sha256、data_base64 | confirmed、media_ref、url、尺寸与实际 MIME |
| `forum_publish_v2` | idempotency_key、action、content 和对应目标/标题字段，可选 media_refs | confirmed、准确 post_id/comment_id/reply_id、url；或 pending/uncertain/conflict |
| `forum_action_status` | idempotency_key | 本人操作的原始结果；或 pending/not_found |

私密帖沿用主站“本人可见”规则；匿名帖子和匿名楼主的评论沿用原服务脱敏。匿名作者 `author_id=null`，不能通过原始评论 UserID 恢复身份；职位小号继续显示平台对外身份。全部输出使用字段白名单，不直接序列化 domain.User。

`is_self` 由服务器计算。客户端处理收费生图时必须从完整原请求作者取身份，匿名或身份不足的请求拒绝；不能让语言模型指定扣额度的用户。

评论和楼中楼游标独立，必须按 has_more 读完所需页面。第一版复用原 `ListForPost` 的完整评论树/缓存，再对响应分页；**没有宣称已优化数据库层的大帖读取成本**。这保留了既有可见性、作者脱敏和职位身份逻辑。feed 使用 offset，新帖插入时可能重叠，客户端按 post_id 去重。

通知按 ID 降序。before_id 只用于向旧页翻；客户端每次从最新页追到已处理水位，逐条去重，处理完再推进水位，不把 before_id 当“读更新”的游标。read=false/true 分别查询未读/已读；如果账号同时由浏览器操作，应分别补读已读流。删除或不可见帖子通知会被过滤，但返回游标仍推进。

## 发布与防重

`idempotency_key` 使用客户端持久化 UUID（16–64 位 ASCII 字母、数字、下划线或连字符），每个逻辑操作一个键。发帖、上传分别用不同键，同用户同键同参数返回原结果，同键不同参数返回 conflict。

```json
{
  "name": "forum_publish_v2",
  "arguments": {
    "idempotency_key": "43ae63aa-4898-44f1-9140-38229ff173ec",
    "action": "reply",
    "post_id": 123,
    "parent_comment_id": 456,
    "reply_comment_id": 789,
    "content": "收到，后续进展也可以在这里聊。"
  }
}
```

- post：必填 title（最多30字）、partition、content（最多10000字）；不能置顶、代职位发言或绕过课程分区限制。
- comment：必填 post_id、content（包括附图 Markdown 最多4000字）。
- reply：额外必填 parent_comment_id；reply_comment_id 可选。先校验目标属于同一帖子/一级评论，再写入。
- media_refs：最多4个，由相同 PAT 账号上传的媒体记录。帖子的 photos 沿用现有逗号分隔格式，评论附图沿用 Markdown `![图片](URL)`，前端实际渲染需在维护者发布后联调。

写入在进程内调用既有 controller，保留现有抽奖参与、邮件、通知、缓存与 RAG 钩子，不重新实现或直接 INSERT 论坛表。新增楼中楼返回值 `data.ccommentID`，原 code/msg 保持兼容；评论 service 在 INSERT 前验证整个回复链，避免跨帖目标造成错误写入。MCP 只把返回了真实目标 ID 的结果标为 confirmed，不用“作者最新帖”推测。

`forum_agent_actions` 在任何外部副作用前插入唯一 `(user_id, action_key)` 预占；重启和并发不会再次执行同键。**这是持久化的单次尝试，不是与业务事务原子提交的 exactly-once**：业务成功后进程崩溃可能留下 pending；业务的后续通知等也可能失败。pending/uncertain 必须人工核对，禁止改键重发或删除记录来“修复”。此策略优先避免重复发言/重复上传，会牺牲自动恢复能力。

只允许在明确确认未执行或人工完成核对后处理未决记录；本次没有提供自动清空/重试接口。记录不自动过期，以免旧键恢复后再次执行。

## 图片

只收 PNG/JPEG，原始文件最多8 MiB、单边最多4096、总像素最多1600万。校验真实格式和 SHA256，完整解码后重新编码为 PNG，去除元数据/尾随字节，重新编码后仍限制8 MiB。使用随机临时文件名并清理，禁止 SVG/HTML、客户端路径与任意远程 URL 抓取。

复用 `api.UploadImage` 与现有图床，发布只引用服务端保存的媒体归属记录。返回图片 URL 必须为 `imagebed.public_base` 同主机的 HTTPS 地址（默认 image.ssemarket.cn），不接受含凭证或可破坏 Markdown 的地址。现有图床如使用其他公共域名，需要正确配置 public_base。

图片生成、内容审核以及“同一真实请求者滚动7天一次”在龙娘客户端执行；这个主站接口不调用模型、不消耗生图 key，也不承诺图片内容自动审核。客户端必须完成原请求/提示词/成图审核，禁止不健康、政治相关与违法用途，审核不可用时不上传。

上传失败也可能已在图床产生文件，记 uncertain，不盲目上传第二次；孤立图片回收与图床可用性不在本 PR 实现范围。

## 数据与上线

新增两张表：`forum_agent_actions`、`forum_agent_media`，沿用主站 AutoMigrate。受限数据库账号可由维护者先执行 `docs/migrations/20261027_forum_agent.sql`。若表创建失败，写工具不能预占就不会执行，完整读帖不依赖这两张表。

发布使用主站现有构建/发布流程，不运行 Python 的旧 deploy 脚本。`/api/auth/mcp` 走主站现有 API upstream，维护者应核对 nginx 没有单独拦截此路径，以及请求体上限至少12 MiB。无需改动 `/api/v1/agent/` 上游。

回滚旧主站镜像即可停止新入口；保留新增两张表，不能删掉操作记录。既有登录、PAT 管理和 Python MCP 不切换。客户端只有在合并发布后显式配置新路径才使用，禁止在旧端点写失败后自动改用新端点重发。

本地验证：`go test ./controller ./service ./middleware ./route`，并覆盖了工具权限/输入伪造、每请求身份、匿名与私密内容、长正文、分页、并发防重、确认失败不重试、图片格式/哈希/URL，以及回复链先校验后写入。未调用生产发帖、上传或图片生成。

实际主程序 `go build .` 编译通过。当时全仓 `go test ./...` / `go build ./...` 受两处基线问题影响：`SSE_market_server_test` 备份目录混用 package，以及 `api/cosBucket.go` 的动态格式串触发 vet。两处均已于 2026-11-18 清理，现全仓 `go build ./...`、`go vet ./...`、`go test ./...` 全部通过。
