# 全站皮肤（程序员节等节日皮肤）后端说明

## 1. 职责边界

后端只做两件事：

1. **活动时间窗**：`theme_events` 表（一行一期活动，管理员直接 INSERT），
   时间按北京时间墙钟填写。`GET /api/auth/theme/state` 返回活动是否进行中、
   起止时间、服务器时间；表为空时 `event` 为 `null`。
2. **用户外观设置**：`user_theme_settings` 表（每人一行，没有行 = 全默认）。
   `POST /api/auth/theme/settings` 保存皮肤选择 / 开场画面 / 背景 / 帖子卡片透明度。

**后端不存任何皮肤图片路径。** 皮肤素材（开场画面、背景、贴纸）是前端
`public/skins/<skinID>/` 的静态资源；用户上传的背景才存图床绝对 URL。

## 2. 接口

### GET `/api/auth/theme/state`（noauth，可选登录）

带 `Authorization: Bearer` 时同时返回该用户设置；游客 `settings` 为 `null`。

```json
{
  "code": 200,
  "data": {
    "serverTime": "2026-10-23T20:00:00+08:00",
    "event": {
      "themeId": "programmers-day-2026",
      "name": "2026 程序员节",
      "active": false,
      "startsAt": "2026-10-24T00:00:00+08:00",
      "endsAt": "2026-10-25T00:00:00+08:00",
      "serverTime": "2026-10-23T20:00:00+08:00"
    },
    "settings": {
      "themeMode": "auto",
      "splashEnabled": true,
      "backgroundEnabled": true,
      "backgroundSource": "default",
      "customBackgroundUrl": "",
      "postCardOpacity": 70
    }
  },
  "msg": ""
}
```

`event.active` 为服务端当前时间算出的结果；前端同时保留起止时间，用于页面
跨零点自动结束（不要求刷新）。

### POST `/api/auth/theme/settings`（auth）

```json
{
  "themeMode": "auto | default | programmers-day-2026",
  "splashEnabled": true,
  "backgroundEnabled": true,
  "backgroundSource": "default | custom",
  "customBackgroundUrl": "https://<图床绝对地址>",
  "postCardOpacity": 70
}
```

- `themeMode=auto`：活动期间自动用活动皮肤，结束后自动回默认；
- `themeMode=default`：强制默认；
- `themeMode=<skinID>`：强制保留某套皮肤（活动结束后仍可用）。
- `backgroundSource=default`：用当前皮肤自带背景（默认皮肤没有则无图）；
- `backgroundSource=custom`：用 `customBackgroundUrl`，**只接受 http(s) 绝对地址**。
  相对路径 / `data:` / `javascript:` / 协议相对地址一律 400。
- `postCardOpacity`：全站卡片不透明度 30-100（默认 70），只在开启全站背景
  时有视觉效果——数值越小，所有卡片（帖子、设置分组等）越透明、背景图越明显。
  旧客户端不传该字段时后端回退默认值 70。
- 没有保存过设置的用户读取时直接得到默认值（不会写空行）。

## 3. 新增一期 / 一套节日皮肤

1. 后端：往 `theme_events` 插一行（不需要改代码、不需要迁移）：

   ```sql
   INSERT INTO theme_events (theme_id, name, starts_at, ends_at, enabled)
   VALUES ('programmers-day-2026', '2026 程序员节',
           '2026-10-24 00:00:00', '2026-10-25 00:00:00', 1);
   ```

   - `starts_at` / `ends_at` 是**北京时间墙钟**（DATETIME），服务器时区无关；
   - 窗口左闭右开 `[starts_at, ends_at)`；
   - 同一个 `theme_id` 可插多期，`(theme_id, starts_at)` 唯一；
   - `enabled=0` 可临时停用，不必删行；
   - 两期重叠时取开始时间最晚的一期。
2. 前端：按 `newSSE/newSSE/src/skins/README.md` 加图片、定义和配色，
   `src/skins/<id>.ts` 的 `id` 与 `theme_id` 完全一致。
3. 活动结束无需改代码：`auto` 用户自动回默认；手动选过的用户保留。

## 4. 迁移

`common/database.go` 已加：

```go
autoMigrate(db, &model.ThemeEvent{})
autoMigrate(db, &model.UserThemeSetting{})
```

手动兜底脚本见 `docs/migrations/20261023_theme_skin.sql`。表建好后直接 INSERT
活动行即可；代码不会自动插入任何活动。
