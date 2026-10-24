# 用户自定义皮肤（桌面/手机双图 + 皮肤库 + 分享）

## 1. 设计

一套用户自定义皮肤 = **电脑横图 + 手机竖图**，使用图床绝对地址保存。
上传后默认私密；作者公开后可被搜索，并可生成「添加链接」分享给其他人。

| 表 | 作用 |
| --- | --- |
| `user_skins` | 皮肤主表：`owner_id`、名称、桌面/手机图、`is_public`、`share_token` |
| `user_skin_library` | 皮肤库：`(user_id, skin_id)` 唯一；自己上传的皮肤创建时自动入自己的库 |
| `user_theme_settings.custom_skin_id` | 当前启用的自定义皮肤；>0 时覆盖内置/单图背景 |

建表兜底：`docs/migrations/20261024_user_skins.sql`（代码 AutoMigrate 也会建）。

## 2. 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/auth/skins/mine` | 我的皮肤库（含自己的和添加别人的） |
| POST | `/api/auth/skins/create` | `{name,description,desktopUrl,mobileUrl}`，默认私密 |
| GET | `/api/auth/skins/search?q=&limit=` | 搜索公开皮肤（皮肤名/作者名） |
| POST | `/api/auth/skins/visibility` | `{skinID,isPublic}` 作者公开/私密 |
| POST | `/api/auth/skins/delete` | `{skinID}` 作者删除 |
| POST | `/api/auth/skins/add` | `{skinID}` 把公开皮肤加入皮肤库 |
| POST | `/api/auth/skins/remove` | `{skinID}` 从皮肤库移除（作者原皮肤保留） |
| POST | `/api/auth/skins/addByToken` | `{token}` 分享链接添加 |
| GET | `/api/skins/share/:token` | 分享链接预览（公开皮肤，游客可读） |

图片上传复用 `POST /api/auth/uploadPhotos`（图床），前端拿到 `fileURL` 后再调用 create。
`desktopUrl/mobileUrl` 只接受 http(s) 绝对地址。

## 3. 分享链接

作者先把皮肤设为公开，然后复制：

```
https://ssemarket.cn/new/?addSkin=<share_token>
```

任意页面打开该链接都会弹出 `SkinShareAddDialog`：

- 游客：引导登录，登录后带着同一链接返回并继续；
- 已登录：点击「添加到皮肤库」调用 `addByToken`，成功后提示去设置页使用；
- 作者把皮肤改回私密/删除后，链接会失效（403/404）。

## 4. 启用与回退

- 应用一套皮肤：保存 `user_theme_settings.custom_skin_id=<skinID>`，
  前端 `useSkin` 的 `backgroundSources` 优先使用这套皮肤的 desktop/mobile 图。
- 选择内置/默认皮肤 / 清空自定义背景时会把 `custom_skin_id` 归零。
- 从皮肤库移除或作者删除皮肤：
  - 删除会清理 `user_skin_library` 并把所有正在使用该皮肤的用户
    `custom_skin_id` 归零；
  - 主动移出自己的皮肤库也会把自己的 `custom_skin_id` 归零。
- 自定义皮肤只覆盖背景图；站点的文字/卡片配色仍跟随当前内置皮肤/活动皮肤。
