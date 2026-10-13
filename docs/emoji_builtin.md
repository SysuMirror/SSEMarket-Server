# 内置表情组「小汐（SSE娘）」实现说明 & 图床上传迁移方案

> 写给后续接手的同学 / agent。回答两个问题：
> ① 内置表情包是怎么实现的（**是按照组来做的吗？—— 是**）；
> ② 后续怎么把这批内置图 **一次性批量上传到图床** 存储。
> 数据表 / 接口总览见 `PROJECT_STATUS.md` §5.5，本文只展开内置组这一块。

---

## 1. 内置表情包是怎么做的

### 1.1 它就是一个「组」，和用户自建组同一张表

内置表情不是一个特殊的页面或写死的数组渲染，而是 **`emoji_packs` 表里的一行真实数据**：

| 项 | 值 | 说明 |
|---|---|---|
| 组 | `emoji_packs` 一行 | `ownerUserID = 0`（系统所有）、`isPersonal = 0`、`isPublic = 1`、`title = 小汐（SSE娘）` |
| 组员 | `emojis` 若干行 | `isBuiltin = 1`、`packID = 内置组ID`、`ownerUserID = 0` |
| `name` | 文件名去扩展名 | 即表情含义（`乖巧`、`救命`…），搜索按它命中 |
| `url` | **裸文件名**（如 `乖巧.png`） | 与用户上传表情的「图床绝对地址」相区别，见 §1.2 |

因为它就是普通的组数据，所以**天然复用了组的一整套能力**：面板按组出页签、搜索能搜到组并可一键添加（内置组本来就在每个人的面板里，加了也幂等）、组预览图、每组的表情数统计。唯一特殊化的是权限：`ownerUserID = 0` 的组 **不能删除、不能被移出面板**（`service.DeletePack` / `DeleteSticker` 里有明确拦截）。

**内置组命名**：`domain/emoji.go` 的 `BuiltinPackTitle = "小汐（SSE娘）"`（站点吉祥物 SSE娘 的表情包）。
改组名只改这一个常量 —— 启动种子 `seedBuiltinEmoji` 发现已存在的内置组标题与常量不一致时，会自动 `UPDATE title`（2026-10-13 之前建的老库组名「默认表情」就是这样被改名过来的）。

### 1.2 图片从哪来：前端打包资源，不是图床

内置表情的图片（29 张，约 2.6MB）**打包在前端仓库** `newSSE/newSSE/public/emoji/` 下，文件名与 `domain.BuiltinEmojiFiles` 清单一一对应。数据库 `emojis.url` 只存裸文件名，前端拼资源地址：

```ts
// src/composables/useEmoji.ts
export function emojiSrc(emoji: EmojiItem): string {
  if (emoji.isBuiltin)
    return `${import.meta.env.BASE_URL}emoji/${encodeURIComponent(emoji.url)}` // 内置 → 前端静态资源
  return emoji.url                                                            // 上传 → 图床绝对地址
}
```

**为什么当初不直接上图床**：内置表情是聊天的基础设施，要求 *零外部依赖、永远能渲染*。走前端打包，图床（image.ssemarket.cn）挂了、换了、超时，内置表情都不受影响；历史消息里的 token 也永远有图可渲染。代价是仓库多 2.6MB、新增内置图要发前端。这是一次**有意的取舍**，不是漏做 —— 图床化方案见 §3。

### 1.3 种子机制（幂等，三层保障）

1. **启动种子** `common/database.go` 的 `seedBuiltinEmoji`：内置组不存在则建（标题取 `BuiltinPackTitle`），29 张表情按 `isBuiltin + name` 逐张判重、缺哪张补哪张，并给组补 `coverEmojiID`。Pod 重启重复执行无副作用；**已存在的行不会覆盖 `url`**（这点对 §3 的图床迁移很重要）。
2. **面板兜底** `service.GetPanel`：若查出来内置为空（例如 DB 账号缺 INSERT 权限导致种子失败），现场补跑一次 `EnsureBuiltinStickers` 再重查。
3. **新增内置表情的流程**：往 `domain/emoji.go` 的 `BuiltinEmojiFiles` 加一行 + 往 `newSSE/newSSE/public/emoji/` 放同名文件 → 重启后端，自动补种，前端重新构建即可。改清单要同时动这两处，`domain/emoji_test.go` 会盯清单形状（无重名、扩展名合法）。

### 1.4 聊天里怎么用（与组身份的关系）

- 发送：聊天正文里写 token **`[emoji:<emojiID>]`**（纯文本，后端不解析、照常存 `chat_msgs.content`）。
- 渲染：`ChatEmojiText.vue` 切分文本/表情片段，未知的 ID 走 `emoji/resolve` 按需补拉；**`Resolve` 故不过滤 `isDeleted`**，历史消息永远能渲染。
- 长按：把表情 **复制一份** 加进自己的个人表情组「我的表情」（`emoji_packs.isPersonal = 1`，每人一个，**默认空**，首次放表情才落库）。内置表情的复制件保留 `isBuiltin = 1`，`url` 仍是裸文件名，即图片仍走前端内置资源，不占图床。

---

## 2. 「我的表情」为什么默认是空的

「我的表情」不是内置组的镜像，也不是收藏表，而是 **每人一个的个人表情组**（`isPersonal = 1`，不公开、不参与搜索）。它满足：

- **初始为空**：组行在第一次放表情时才创建（`EnsurePersonalPack`）；面板读取时组不存在就展示一个未落库的空组视图（`personalPackView`），**读面板不产生任何写库**。
- **谁会往里放东西**：① 上传单张/多张（`emoji/upload`）；② 聊天里长按表情（`emoji/addFromChat`，复制语义、按 URL 幂等去重）。分享组的内容**不会**自动进来。
- 配额统一算在用户头上：个人组 + 自建分享组合计 ≤ 300 张（`domain.UserEmojiMax`）。

> 历史备注：最初版本用的是 `user_emojis` 收藏表 + 扁平「我的表情」列表，2026-10-13 按产品口径改为「表情进个人表情组」，收藏表已从 model/AutoMigrate 移除。老库若已建过 `user_emojis` 可直接留着不管（无人再读写），想清理就确认无数据后 `DROP TABLE user_emojis`。

---

## 3. 后续一次性批量上传图床的方案（当前未执行）

### 3.1 目标

把 29 张内置图一次性传到图床（复用现有 `api.UploadImage` 链路，落 `emoji/builtin/<文件名>`），然后把内置表情行的 `url` 从裸文件名 **原地换成图床绝对地址**。完成后前端 `emojiSrc` 对内置表情也不再需要特判，新增/更换内置图不用发前端。

### 3.2 执行步骤（一次性，跑一遍即可）

1. **写一个一次性小命令**（建议放 `cmd/emojiimagebed/main.go`，跑完可删）：
   - 遍历 `domain.BuiltinEmojiFiles`，逐张读前端仓库 `public/emoji/<file>`（路径用绝对路径或启动参数传入）；
   - 调 `api.UploadImage("emoji/builtin/"+file, localPath)` 拿绝对 URL；
   - **先 HEAD 校验**图床 URL 可访问（200）再动库；
   - 按名字更新，**两条 UPDATE 都要跑**（内置原件 + 用户长按收藏产生的内置副本）：
     ```sql
     UPDATE emojis SET url = ? WHERE isBuiltin = 1 AND name = ?;   -- ? = 图床URL, 名字
     ```
   - **打印「裸文件名 → 图床URL」映射日志并留存**（回滚就靠它反向刷回去）。
2. **前端改 `emojiSrc`**：删掉 `isBuiltin` 特判，直接 `return emoji.url`。
3. **保留 `public/emoji/` 资源一个发布周期**，线上验证无误后再删（省 2.6MB）。**种子逻辑不用改**：`seedBuiltinEmoji` 对已存在的行不覆盖 `url`，图床化后的库重启不会被打回裸文件名。

### 3.3 必须知道的注意点

- **新环境仍会走裸文件名**：种子只补「缺的行」，新库插的还是裸文件名 —— 所以前端打包资源在迁移期**必须保留**（新环境先打包渲染，再对新库跑一次迁移脚本）。
- **历史消息跟着换源**：token 里只有 `emojiID`，渲染读的就是这份 `url`，刷库后历史消息自动切到图床；代价是从此历史内置表情依赖图床可用性（这正是 §1.2 保守打包的原因）。图床大面积故障时表现为内置表情裂图，回滚方案 = 按映射日志反向 UPDATE + 恢复 `emojiSrc` 特判。
- **执行时机**：串行上传 29 张小图，几秒完成，但仍按容器规范先看 `/shared/pressure/pressure.json`，`bw` 为 `high` 时延后。
- **幂等性**：脚本可重复跑（同路径重复上传图床会得到新 URL，属可接受浪费；介意就先按文件名查一次映射缓存）。
- 建议在**测试环境先完整走一遍**（上传 → 刷库 → 前端发表情/历史消息渲染 → 长按收藏内置表情 → 图床停机演练回滚），再动正式库。

### 3.4 现状结论

**截至 2026-10-13，内置表情 = 前端打包资源 + 数据库裸文件名，未上图床。** §3 方案是留给图床稳定后的可选优化，做与不影响现有功能。
