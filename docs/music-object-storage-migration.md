# 音乐存储迁移对象存储（腾讯云 COS）指南

> 写给后续把音乐文件从本地盘迁到对象存储的人。当前状态：**本地盘为主存储 + COS 双写备份**（2026-09-11 上线）。
> 迁移动手前先通读"现状"一节，所有路径/表结构/代码位置都在那里。

## 1. 现状（迁移前必读）

### 存储布局

| 内容 | 容器内路径 | 宿主机路径（集市机） | COS 备份 key |
|---|---|---|---|
| 音频文件 | `/app/public/music/files/<yyyymm>/<unixnano>_<原文件名>` | `/root/SSE_Market/sse_market_server_resource/music/files/...` | `src/music/files/...` |
| 封面图 | `/app/public/music/covers/<unixnano>.<ext>` | `/root/SSE_Market/sse_market_server_resource/music/covers/...` | `src/music/covers/...` |

- `/app/public` 是 docker run 时 bind-mount 的宿主机 `sse_market_server_resource` 目录，**容器重建/升级不丢**。
- COS 桶：`sse-market-source-1320172928`（ap-guangzhou），coscmd 配置在集市机 `/root/.cos.conf`。
- COS 双写在 `controller/musicController.go` 的 `mirrorMusicToCOS()`，**best-effort**：失败只打日志不阻断上传。所以 COS 与本地盘可能短暂不一致，迁移前必须先做一次全量对账（见 §3 步骤 1）。

### 数据库

表 `musics`（GORM AutoMigrate 自动建，模型在 `model/music.go`）：

| 字段 | 说明 |
|---|---|
| `musicID` | 主键，对外即文件 URL 里的 id |
| `fileName` | 音频相对 key（`202609/1757..._xxx.mp3`），**不含根目录前缀** |
| `coverName` | 封面相对 key；空串 = 无封面，前端用自绘默认封面兜底 |
| `is_published` | 两段式上传：0=草稿（24h 未发布会定时清理），1=已发布 |
| `is_delete` | 软删除标志，删除=置 1（文件保留，需要彻底清理见 §5） |

### 文件出口（迁移时唯一要改的地方）

前端**不直接持有文件路径**，全部通过这两个端点取文件：

- `GET /api/music/file/:musicID` → `controller/musicController.go` 的 `MusicFile()`
- `GET /api/music/cover/:musicID` → `controller/musicController.go` 的 `MusicCover()`

两者都是 **noauth**（`<audio>` 媒体请求带不了 Authorization 头，这是刻意设计），且用 `http.ServeContent` 支持播放器拖动进度条所需的 Range/206 分段请求。前端 URL 由 `newSSE/src/store/musicStore.ts` 用 `VITE_API_BASE_URL` 拼接。

## 2. 迁移方案选择

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **A. 302 重定向（推荐）** | file/cover 端点改为查 DB 后 `c.Redirect(302, cos签名URL)` | 改动极小（只改两个 handler）；流量不过应用服务器，省带宽；Range/缓存由 COS 原生处理 | 桶需开放匿名 GET 或签短时效 URL；签名 URL 不宜长缓存 |
| B. 服务端代理 | handler 改为流式转发 COS 对象 | URL 不变、可加鉴权/限速 | 音乐流量全过 Go 服务，占用集市机带宽与内存 |

桶 `sse-market-source-1320172928` 目前已是**匿名可 GET 不可 list**，方案 A 零桶配置变更即可用。

## 3. 迁移步骤（方案 A）

1. **对账补同步**（把双写漏掉的文件补上，幂等可重跑）：
   ```bash
   # 宿主机目录 → COS，逐文件比对大小
   cd /root/SSE_Market/sse_market_server_resource/music
   find . -type f | while read f; do
     key="src/music/${f#./}"
     local_size=$(stat -c%s "$f")
     remote_size=$(coscmd list "$(dirname "$key")/" | grep -F "$(basename "$key")" | awk '{print $(NF-1)}')
     [ "$local_size" != "$remote_size" ] && coscmd upload "$f" "$key" && echo "补传 $key"
   done
   ```
2. **DB 快照**（改表前必备）：`mysqldump -h127.0.0.1 -P3506 -uroot -padmin ssemarket musics > /root/cos-migration-backup/musics-$(date +%F).sql`
3. **改代码**：`MusicFile()`/`MusicCover()` 里把"打开本地文件 + ServeContent"替换为 `c.Redirect(302, api.GetUrl("src/music/files/"+music.FileName))`（`api.GetUrl` 在 `api/cosBucket.go`，已存在）。保留 404/权限判断逻辑不动。
4. **灰度验证**：重建镜像（见 §6），curl 验证重定向与 Range：
   ```bash
   curl -sI https://ssemarket.cn/api/music/file/1 | grep -i location
   curl -s -H "Range: bytes=0-1023" -o /dev/null -w "%{http_code}\n" -L https://ssemarket.cn/api/music/file/1   # 期望 206
   ```
5. **观察 1-2 周**无回退诉求后，可选择：上传链路改为 COS 单写（`UploadMusic()` 里删掉本地保存），或维持双写（本地当冷备）。**建议维持双写一段时间**。
6. 本地旧文件**先归档不删除**（mv 到 `/root/music-archive-<日期>/`），确认一个季度无问题再清理。

## 4. 回滚

方案 A 的回滚 = 把两个 handler 恢复为本地文件版（git revert 对应提交）+ 重建镜像。本地文件在归档前始终完好，无需从 COS 拉回。

## 5. 数据清理（软删除的彻底清理）

`is_delete=1` 的记录文件仍在盘上。确认要物理清理时（2026-09-11 首次执行过，两个坑）：

> ⚠️ **坑 1**：`coscmd delete <key>` 不带 `-f` 会交互式确认，脚本里调用等于空操作（返回 0 但什么都没删）。批量删务必 `coscmd delete -f`。
> ⚠️ **坑 2**：`mysql` 客户端默认字符集会把中文文件名打成 `?`，用它拼删除路径必失败。要么 `mysql --default-character-set=utf8mb4`，要么**用文件名里的纳秒时间戳前缀从文件系统侧 find 定位**（时间戳全局唯一且是 ASCII）。


```bash
mysql -h127.0.0.1 -P3506 -uroot -padmin ssemarket -e \
  "SELECT musicID, file_name, cover_name FROM musics WHERE is_delete=1"   # 先核对清单
# 按 fileName/coverName 删宿主机文件与 COS 对象后：
mysql ... -e "DELETE FROM musics WHERE is_delete=1"
```

## 6. 后端重建镜像流程（集市机）

```bash
# 注意：server 容器是手动 docker run 创建的，不受 compose 管，compose up 会撞名
docker build -t sse_market_server:latest /root/SSE_Market/SSE_Market/SSE_market_server 2>/dev/null \
  || docker build -t sse_market_server:latest /root/SSE_Market/SSE_market_server
docker rename server server-old-$(date +%Y%m%d)   # 保留回滚
docker run -d --name server --env-file /root/SSE_Market/server.env \
  -v /root/SSE_Market/sse_market_server_resource:/app/public \
  -v /root/SSE_Market/log/server:/app/log \
  -v /root/SSE_Market/database:/app/database \
  --network sse_market_server_default --add-host host.docker.internal:host-gateway \
  -p 8080:8080 sse_market_server:latest
# 确认正常后 docker rm 旧容器
```

（挂载/网络参数以 `docker inspect server` 实际输出为准，上面是 2026-09 的记录值。）

> ⚠️ **容器别名陷阱（2026-09-11 实测踩坑）**：`server` 容器是手动 docker run 的，但 nginx upstream 写的是 `server sse_market_server:8080`（compose 服务名 DNS）。重建容器时**必须带 `--network-alias sse_market_server`**，否则 nginx 解析不到新容器、/api 全部 404（打到旧容器上）。完整参数：

```bash
docker run -d --name server --restart unless-stopped \
  --network sse_market_server_default --network-alias sse_market_server \
  --add-host host.docker.internal:host-gateway \
  --env-file /root/SSE_Market/server.env \
  -v /root/SSE_Market/sse_market_server_resource:/app/public \
  -v /root/SSE_Market/log/server:/app/log \
  -v /root/SSE_Market/database:/app/database \
  -v /root/SSE_Market/hexo_blog:/app/hexo_blog \
  sse_market_server:<tag>
```


## 7. 其他注意事项

- 上传体积上限：Go 侧 60MB（`musicController.go` 常量 `maxMusicSize`），**nginx_proxy 容器（custom.conf）ssemarket.cn:443 server 块已有 `client_max_body_size 200m`，60MB 上限无需调整；若改大上限记得同步检查该值。
- 时长 `duration_sec` 由前端浏览器读音频元数据后随表单上传，不是服务端解析的；老数据可能为 0，前端显示 "--:--"。
- `musics` 表建表靠 AutoMigrate（`common/database.go`），没有 SQL migration 文件；DB 全量备份在容器 `/app/database`（宿主 `/root/SSE_Market/database`，12h 一次 mysqldump）。
- 音乐功能的前端实现在 `newSSE` 仓库 `src/store/musicStore.ts`、`src/views/MusicView.vue`、`src/components/music/`，播放引擎 Howler.js（`html5: true` 流式模式）。
