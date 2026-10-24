# 图片上传链路与提速

## 问题

主站后端原来用 `https://image.ssemarket.cn/api/upload` 上传图片。这个域名解析到本机公网 IP，
请求会绕出去再回来，经公网入口 → 本机 nginx → frp 隧道（13002）→ 图床，上传一张 1MB 图片
经常要 2-10 秒，且抖动很大。

图床在本机的实际链路：

```
公网 443 → sdpy nginx → host.docker.internal:23002 → 127.0.0.1:13002 (frps) → 图床
```

## 处理

1. **后端改走本机 frp 隧道，不再走公网域名**

   `go_server.env`：

   ```ini
   IMAGEBED_UPLOAD_URL=http://host.docker.internal:13002/api/upload
   IMAGEBED_PUBLIC_BASE=https://image.ssemarket.cn
   ```

   - `upload_url` 只用于后端把文件送进图床；
   - `public_base` / 图床返回的绝对地址仍是 `https://image.ssemarket.cn/...`，
     前端展示不受影响。

   重建 `server` 容器时必须带：

   ```bash
   --add-host host.docker.internal:host-gateway
   ```

   （`run_server_v9_mcpfix.sh` 已有这行；用 `docker restart` 不会丢。）
   容器内 `host.docker.internal` 会解析到宿主网关，13002 端口是 frps 的本机入口。

2. **复用 HTTP 连接**

   `api/cosBucket.go` 使用包级 `imagebedHTTPClient`（keep-alive），避免每次上传重新建连。

3. **前端上传前压缩**

   `src/utils/compressImage.ts` 在上传前把图片等比缩到最长边 1920，并转 WebP（不支持时转 JPEG）：
   - 自定义皮肤的电脑图/手机图、个人主页栏板图都走这一步；
   - 典型手机原图 3-8MB，压缩后通常只有几百 KB，客户端上行时间和后端二次上传一起下降。

## 实测

同一台机器、同一张 1.4MB 图片：

| 链路 | 图床耗时 |
| --- | --- |
| 公网 `image.ssemarket.cn/api/upload` | 2.3-9.8s（抖动大） |
| 本机 frp `13002` | 0.2-1.6s（稳定） |

压缩后的图通常还能再快一截。
