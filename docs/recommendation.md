# 首页推荐流

## 模式与优先级

- 默认仍是时间流，所有旧行为不变。
- 用户设置三态（表 `user_recommendation_settings`，按账号落库，多端登录共用）：
  - `auto`（无行时的默认）：命中 UID 灰度规则才走推荐；
  - `on`：主动开启，不受灰度限制；
  - `off`：主动关闭，优先于灰度。
- 前端设置页每次选择都会 `POST /api/auth/recommendation/settings` 写库；其他设备/浏览器登录后由首页 Browse 和设置页读取同一份后端状态，不需要重新选。
- 只有「主页默认信息流」会走推荐：`POST /api/auth/browse` 在 `partition=主页`、`searchsort=home`、`orderBy=time_desc`（或空）且没有搜索词/标签/时间范围时自动切到推荐；用户把设置切到「为你推荐」后，首页每一次加载/上拉都会走推荐，后端不需要额外参数。搜索、标签、时间筛选、其他分区和手动排序仍按时间/原逻辑查询。
- 模型不可用（向量不足）时自动回落时间流，接口不报错。

## 排序设计

老帖不再单纯靠时间衰减硬挤，而是按年龄分三桶后固定配额交错，保证每页都有历史帖：

| 桶 | 年龄 | 有画像用户每 15 条 | 冷启动用户每 15 条 |
|---|---|---|---|
| 新帖 | ≤90 天 | 7 | 3 |
| 中帖 | 90~365 天 | 4 | 3 |
| 老帖 | >365 天 | 4 | 9 |

桶内分数：

```
sim     = clamp01((cosine - 0.10) / 0.65)   # 文本 85% + 图片 15%
quality = clamp01(log1p(2*赞 + 3*评论 + 2*币) / 6)
已读：未读 +0.05，已读 -0.25
曝光：看过但没点 -0.20 起，重复曝光每次再 -0.05，最多 -0.50，48h 半衰
偏好：+0.04*分区偏好 +0.03*标签偏好；自己发帖 -0.08

新帖：  0.45*exp(-ageHours/168) + 0.35*sim + 0.15*quality + 已读 + 偏好
中/老： 0.60*sim + 0.30*quality + 已读 + 偏好
冷启动：0.70*quality + 0.20*sim + 已读 + 偏好
```

**浏览量不参与质量分**：`browse_num` 来自详情页曝光，不能代表喜欢，已彻底移除；质量只看点赞、评论、投币等主动行为。

**冷启动规则**：账号注册不足 3 天，或强兴趣信号不足 3 个（点赞/评论/收藏/投币 + ≥15 秒阅读）时，按冷启动配额优先推历史优质老帖；满 3 天且信号达标后自动切到个人画像排序。阈值可用 `RECOMMEND_COLD_START_DAYS` / `RECOMMEND_COLD_START_SIGNALS` 调整。

**刷新会换内容**：推荐流每下发一页 15 条就记一次曝光（Redis `rec:imp:v2:{uid}` / `rec:impc:v2:{uid}`，保留 7 天）。用户只是刷到、没点进详情，也会在下次刷新时被轻微降权，所以刷新一次就能看到后面的帖子，不用反复刷；多次曝光会继续往后排。曝光和已读分开：点进详情仍走 `post_read_events` 的 -0.25，更重。后台新帖轮询（limit=20）、置顶探测（limit=1）不记曝光，避免程序请求污染信号。

画像会尽量用上所有能拿到兴趣的地方，统一带 45 天半衰期：
- 帖子互动：浏览 0.2（弱信号）、点赞 3、评论 4、收藏 4.5、投币 5；
- 自己发的公开帖：2.5；
- 点进详情的已读：`dwellMS>0` 的停留越长权重越高，只点进去（dwell=0）只算已读、不加权；
- 与小秋的聊天：取最近 3 个会话里用户的提问（每个会话最多 4 条，最多 12 条），拼成文本做一次向量后按条数加权（1~6）。聊天在 sse-agent 侧，集市不直连它的库，而是借当前用户 JWT 调 agent 只读会话接口；token 只在 request context 里，不落库、不写日志。结果按 `RECOMMEND_CHAT_PROFILE_TTL`（默认 30 分钟）缓存。聊天向量在后台嵌（60 秒超时、最多重试 3 次），不阻塞首页；算好后自动清掉该用户画像，下一次刷新生效。
- 自己发帖和小秋聊天也计入「强兴趣信号」，帮助老用户/活跃用户更快脱离冷启动。

向量按 `kind=text` / `kind=image` 分开存，避免图片稀释文本相似度。

分页：每次 offset=0 的首页请求都会按最新曝光重新算一遍完整排序 ID 列表，缓存 Redis `rec:feed:v2:{uid}` 20 分钟供后续翻页复用，保证同一轮分页不重不漏；非 feed 请求（limit≠15）只复用缓存，不重算、不覆盖，避免后台轮询打断分页。

## 嵌入配置（环境变量）

| 变量 | 默认 | 说明 |
|---|---|---|
| `RECOMMEND_ENABLED` | true | 总开关 |
| `RECOMMEND_EMBED_PROVIDER` | dashscope | `dashscope` / `spark`；spark 额外支持图片向量 |
| `RECOMMEND_EMBED_BASE` | 按 provider | OpenAI 兼容 API 根地址 |
| `RECOMMEND_EMBED_KEY` | 空 | API key；为空则推荐自动降级为时间流 |
| `RECOMMEND_EMBED_MODEL` | text-embedding-v4 | 模型名 |
| `RECOMMEND_EMBED_DIMS` | 1024 | 向量维度 |
| `RECOMMEND_EMBED_BATCH` | 10 | 每批文本条数（Spark 100） |
| `RECOMMEND_EMBED_IMAGE` | spark 下 true | 是否构建首图向量 |
| `RECOMMEND_PROFILE_TTL` | 5m | 用户画像缓存 |
| `RECOMMEND_RANK_TTL` | 20m | 排序结果缓存 |
| `RECOMMEND_INDEX_REFRESH` | 2m | 内存索引刷新间隔 |
| `RECOMMEND_COLD_START_DAYS` | 3 | 新账号冷启动天数 |
| `RECOMMEND_COLD_START_SIGNALS` | 3 | 切画像所需强兴趣信号数 |
| `RECOMMEND_CHAT_ENABLED` | true | 是否用小秋聊天补画像 |
| `RECOMMEND_CHAT_BASE` | `http://host.docker.internal:8110` | sse-agent chat 面地址（内网） |
| `RECOMMEND_CHAT_PROFILE_TTL` | 30m | 聊天画像缓存时间 |
| `XQ_BOT_UID` | 95317 | 小秋 bot 账号；若聊天落集市 `chat_msgs` 时用于过滤 |

密钥只放 `/root/SSE_Market/go_server.env`，不要提交仓库或写进日志。

当前线上配置：`provider=spark`、`base=https://spark.ssemarket.cn/embed/v1`、`model=bge-m3`、`dims=1024`、`batch=100`、`image=true`。文本与首图分别写入 `kind=text` / `kind=image`，图片只取帖子的第一张白名单图片（`image.ssemarket.cn`）；失效图片会写 `dims=0` 占位跳过，不会卡住构建队列。切换嵌入模型或 provider 后需要调用一次管理员 `rebuild`，否则新旧向量不可比。

## 灰度操作

```bash
# 查看规则
./scripts/recommendation_gray.sh list
# 灰度 UID 1~1000
./scripts/recommendation_gray.sh add 1 1000 "首批灰度"
# 暂停 / 恢复 / 删除
./scripts/recommendation_gray.sh disable <id>
./scripts/recommendation_gray.sh enable <id>
./scripts/recommendation_gray.sh delete <id>
```

脚本直连 DB，最迟 1 分钟生效。管理员 API 实时生效：

```
GET    /api/auth/admin/recommendation/stats
GET    /api/auth/admin/recommendation/gray
POST   /api/auth/admin/recommendation/gray
DELETE /api/auth/admin/recommendation/gray/:id
POST   /api/auth/admin/recommendation/rebuild
```

`stats` 可看 `textEmbedded / pendingText / lastError`，以及 `available / imageEnabled / indexSize / indexTextVectors / indexImageVectors`。当前线上使用 Spark 多模态：文本全量约 5500 帖，图片向量按帖子首图后台逐批补齐；`pendingImage` 表示尚未处理的图片帖数。

## 前端

- 设置页「推荐流」：默认 / 为你推荐 / 按时间三选一。
- 推荐生效时主页标题旁显示「为你推荐」徽标，并隐藏排序栏。
- 已读标记：点进帖子详情时前端自动调用 `POST /api/auth/recommendation/read {"postID":1}`（`dwellMS` 可省略）。默认全部未读，只有点进详情才算已读；同一帖子只保留一行，后续计时器传 `dwellMS` 时取最大值。`pbrowse`/`browse_num` 不参与已读判断。
- 新帖提醒：设置里开启「为你推荐」（或灰度实际生效）后，前端停掉新帖轮询并收起已有提醒；切回按时间排序后自动恢复。推荐流本身会轮换内容，不需要再用「有新帖」催刷新。

## 回滚

```bash
# 后端：回到旧镜像（旧容器保留为 server-old-rec-20261103）
# 当前线上镜像以实际 docker ps 为准；env 备份：
#   /root/SSE_Market/go_server.env.bak-20261103-spark（切 Spark 前）
#   /root/SSE_Market/go_server.env.bak-20261103（上推荐前）
docker stop server && docker rm server
docker run -d --name server --restart unless-stopped --network sse_market_server_default \
  -p 127.0.0.1:8080:8080 --add-host host.docker.internal:host-gateway \
  --log-opt max-size=50m --log-opt max-file=3 \
  --env-file /root/SSE_Market/go_server.env \
  -v /root/SSE_Market/sse_market_server_resource:/app/public \
  -v /root/SSE_Market/log/server:/app/log \
  -v /root/SSE_Market/database:/app/database \
  -v /root/SSE_Market/hexo_blog:/app/hexo_blog \
  sse_market_server:rollback-20261103

# 前端：恢复构建产物。dist 是 nginx 的目录 bind mount，
# 必须原地清空再拷回（不要 rm -rf dist 后重建，否则容器仍指向旧 inode）。
cd /root/SSE_Market/sse_market_new_client/newSSE
find dist -mindepth 1 -delete
cp -a dist.bak-20261103/. dist/
```

新增表与旧代码不冲突，回滚后端无需删表；需要彻底清理时再删 `post_embeddings`、`post_read_events`、`user_recommendation_settings`、`recommendation_gray_rules`。