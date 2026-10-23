# 全站皮肤（程序员节等节日皮肤）后端说明

## 1. 职责边界

后端只做两件事：

1. **活动时间窗**：`domain/theme.go` 的 `themeEvents` 注册表（北京时间）。
   `GET /api/auth/theme/state` 返回活动是否进行中、起止时间、服务器时间。
2. **用户外观设置**：`user_theme_settings` 表（每人一行，没有行 = 全默认）。
   `POST /api/auth/theme/settings` 保存皮肤选择 / 开场画面 / 背景设置。

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
      "customBackgroundUrl": ""
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
  "customBackgroundUrl": "https://<图床绝对地址>"
}
```

- `themeMode=auto`：活动期间自动用活动皮肤，结束后自动回默认；
- `themeMode=default`：强制默认；
- `themeMode=<skinID>`：强制保留某套皮肤（活动结束后仍可用）。
- `backgroundSource=default`：用当前皮肤自带背景（默认皮肤没有则无图）；
- `backgroundSource=custom`：用 `customBackgroundUrl`，**只接受 http(s) 绝对地址**。
  相对路径 / `data:` / `javascript:` / 协议相对地址一律 400。
- 没有保存过设置的用户读取时直接得到默认值（不会写空行）。

## 3. 新增一套节日皮肤

1. 后端：在 `domain/theme.go` 的 `themeEvents` 里加一条时间窗，`ThemeID` 与前端
   `src/skins/<id>.ts` 的 `id` 完全一致。`IsKnownThemeMode` 会自动接受新 ID。
2. 前端：按 `newSSE/newSSE/src/skins/README.md` 加图片、定义和配色。
3. 不需要数据库迁移（活动时间窗不落库）。
4. 活动结束无需改代码：`auto` 用户自动回默认；手动选过的用户保留。

## 4. 迁移

`common/database.go` 已加 `autoMigrate(db, &model.UserThemeSetting{})`。
手动兜底脚本见 `docs/migrations/20261023_theme_skin.sql`。
