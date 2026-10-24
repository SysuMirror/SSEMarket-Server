# 个人主页栏板背景

## 1. 存储

`users` 表新增两列：

| 列 | 说明 |
| --- | --- |
| `profile_banner_url` | 图床返回的绝对 http(s) 地址；空串 = 默认藏青渐变 |
| `profile_banner_opacity` | 0-100 不透明度，默认 100；主页主人设置，所有访问者共用 |

兜底迁移：`docs/migrations/20261024_profile_banner.sql`。

## 2. 接口

公开读取：`POST /api/auth/getInfo`（个人主页现有接口）返回：

```json
{
  "profileBannerUrl": "https://image.ssemarket.cn/i/images/2026/10/24/xxxx.png",
  "profileBannerOpacity": 55
}
```

保存：`POST /api/auth/profile/banner`（登录，只能改自己）：

```json
{ "bannerUrl": "https://image.ssemarket.cn/i/images/2026/10/24/xxxx.png", "bannerOpacity": 55 }
```

- `bannerUrl` 为空串表示恢复默认渐变；
- 只接受 http(s) 绝对地址（与皮肤自定义背景同一套校验，拒绝相对路径/`data:`）；
- `bannerOpacity` 缺省 100，越界 400；
- 图片上传复用 `POST /api/auth/uploadPhotos`（图床），前端拿 `fileURL` 再调用本接口保存。

## 3. 前端行为

- 个人主页顶部栏板用 `background-image` 铺图，`opacity` 由主人设置值决定；
- 主人看到「设置背景」按钮：上传/更换/移除图片 + 不透明度滑块，拖动时上方栏板实时预览；
- 访客不显示按钮，但读取并应用主人保存的图片与不透明度；
- 点击保存后才落库，取消会恢复原样。
