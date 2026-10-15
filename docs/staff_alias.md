# 职位账号（开发人员匿名 @）设计与用法

> 2026-10-14 落地。目的：让用户在评论区/帖子能真实 @ 到开发人员（如 `@iwiki开发人员`），
> 但 **不暴露开发者的真实账号**——对外身份永远是「职位」，不是人。

## 一句话原理

管理员在 `staff_aliases` 表插一行「职位名 ↔ 真实开发者 userID」；
系统自动派生一个 **职位小号**（`users` 表一行：昵称=职位名、等级=满级 Lv8、称号=职称、不可登录）。
@ 解析、通知、回复、主页，全部走小号，真实开发者只在小号背后收通知、授权回复。

## 管理员要做什么

只有一件事（SQL 参考 `docs/migrations/20261014_staff_alias.sql`）：

```sql
INSERT INTO staff_aliases (alias, title, staff_user_id)
VALUES ('iwiki开发人员', 'iwiki开发人员', 123);
```

- `alias`：被 @ 的名字（也是主页昵称），≤50 字符，**不能与已有用户昵称重复**（冲突会打日志并拒绝派生）。
- `title`：职称（主页称号），≤32 字符。
- `staff_user_id`：真实开发者的 userID。
- `alias_user_id`：留 0，系统自动回填。

插完后 **无需重启**：任何人第一次打开 @ 下拉 / 渲染该 @ / 开发者打开该帖评论框时都会自动派生小号；
重启时 `common.SyncStaffAliases` 也会全量同步一遍（含改名/改职称的同步）。

## 数据流

| 场景 | 行为 |
|---|---|
| 用户在评论里 `@职位名` | `notifyMentions` 解析 token → 命中职位行 → 通知发给 **真实开发者**（发送人=评论者），小号本身不收通知 |
| 前端渲染 `@职位名` | `resolveMentions` 把职位名解析成小号 → 蓝色链接 → 点击进 **职位主页**（满级 Lv8 + 称号=职称） |
| @ 下拉候选 | `POST /api/auth/staff/alias/list` 返回全部职位（alias/title/小号 userID）；表为空时前端不显示 @ 入口 |
| 开发者回复 | 打开该帖评论框时前端调 `POST /api/auth/staff/alias/threadAliases`；仅当 **本人在该线程被 @ 到** 时出现「以 XX 身份回复」开关；开启后评论对外归属小号 |
| 以职位身份回复时 | 评论 userID=小号；被回复人/看帖人收到的通知发送人=小号；不参与抽奖（个人行为挂真实账号）；响应与列表中都不含真实 userID |

## 匿名性保障（改动时必须维持）

1. **对外接口零泄露**：`staff_aliases.staff_user_id` 不出现在任何 API 响应/通知/评论字段里；
   出参一律 `domain.StaffAliasPublic`（aliasID/alias/title/userID=小号）。
2. **小号不可登录**：邮箱固定 `staff-<aliasID>@staff.internal`（无人可收验证码），口令是随机串的 bcrypt 哈希；
   无手机号；邮件通知被屏蔽。
3. **通知转发**：`@职位` 的通知 receiver=真实开发者、sender=评论者，内容不涉及身份；
   开发者以职位回复时，所有通知的 sender=小号。
4. **以职位回复的鉴权在服务端**（`StaffService.AuthorizeAliasReply`）：
   必须是本人的职位行 + 该帖线程（正文+评论+楼中楼）确实 @ 过这个职位；前端开关只是展示。
5. 小号 score 固定 5000（前端 Lv8 满级阈值），`custom_title`=职称 —— 主页呈现「满级 + 职位称号」。

## 影响面（普通用户不受影响）

- 新增表 + 两个新接口，评论接口只加了可选字段 `asStaffUserID`；不传时行为与原来完全一致。
- `notifyMentions` / `resolveMentions` 只在有 @ token 时多一次 `staff_aliases` 的索引查询（命中才走新逻辑）。
- 前端仅在编辑器里多一个 @ 按钮（表为空时不显示）与被 @ 者的职位身份开关（普通用户永远看不到）。

## 代码索引

- 表：`model/staff_alias.go`；领域口径：`domain/staff.go`（+`staff_test.go`）
- 启动同步：`common/database.go SyncStaffAliases`；懒派生：`service/staff_service.go ensureShells`
- 接口：`controller/staffController.go`；路由 `route/routes.go`（`staff/alias/list`、`staff/alias/threadAliases`）
- 通知转发/身份切换：`service/comment_service.go`（`notifyMentions`、`resolveEffectiveAuthor`）
