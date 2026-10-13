package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// 表情体系的数值口径集中在这里，改数值只改这一处。
const (
	EmojiNameMaxLen  = 30  // 单个表情名（rune）
	PackTitleMaxLen  = 30  // 表情包组标题（rune）
	PackStickerMax   = 30  // 一个组最多多少张
	UserEmojiMax     = 300 // 一个用户（个人表情组 + 自建组）最多多少张
	EmojiUploadMax   = 20  // 一次上传最多多少张
	EmojiSearchLimit = 50  // 搜索结果里表情 / 组各最多返回多少条
	EmojiPackPreview = 4   // 搜索结果里每个组预览多少张
)

// BuiltinEmojiFiles 内置表情：文件名（去扩展名）就是含义，
// 与前端 public/emoji/ 下的资源一一对应，种子数据按这份清单落库。
// URL 存完整文件名（带扩展名），Name 存去扩展名的含义。
var BuiltinEmojiFiles = []string{
	"不要.png", "乖巧.png", "你好.jpg", "偷笑.jpg", "哭泣.jpg", "啊.png", "困.jpg",
	"头疼.jpg", "好耶.png", "心动.png", "惊讶.jpg", "我可爱.jpg", "捏脸.jpg", "救命.png",
	"暗中观察.jpg", "汗流浃背.jpg", "燃起来了.jpg", "生气.jpg", "疑惑.jpg", "睡觉.jpg",
	"绷不住了.jpg", "脑子待机中.jpg", "让我康康.jpg", "贴贴.jpg", "赞.jpg", "转圈圈.jpg",
	"送你花花.jpg", "那咋了.jpg", "郁闷.jpg",
}

// BuiltinEmojiName 从内置文件名取出含义（去扩展名）。
func BuiltinEmojiName(file string) string {
	if i := strings.LastIndex(file, "."); i > 0 {
		return file[:i]
	}
	return file
}

// BuiltinPackTitle 内置组的标题（ownerUserID=0 的那一组）——
// 内置表情就是这一个组，组名「小汐（SSE娘）」，站点吉祥物的表情包。
// 改这个常量后，启动种子会把已存在的内置组标题同步成新值。
const BuiltinPackTitle = "小汐（SSE娘）"

// PersonalPackTitle 每个用户一个的个人表情组（isPersonal=true）：
// 默认是空的，上传的单张表情、长按聊天里的表情添加进来的，都进这个组。
const PersonalPackTitle = "我的表情"

// 聊天正文里的表情 token：[emoji:<emojiID>]。
// 前端渲染成图片、长按收藏靠它定位；后端只在会话预览里把它还原成 [表情]。
var emojiTokenPattern = regexp.MustCompile(`\[emoji:\d+\]`)

// StripEmojiTokens 把正文里的表情 token 替换成 [表情]，用于联系人列表的最后一句预览。
func StripEmojiTokens(content string) string {
	return emojiTokenPattern.ReplaceAllString(content, "[表情]")
}

// NormalizeEmojiName 清洗表情名：去扩展名残留、首尾空白，按 rune 截断。
func NormalizeEmojiName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\n", " ")
	if name == "" {
		return "表情"
	}
	if utf8.RuneCountInString(name) > EmojiNameMaxLen {
		name = string([]rune(name)[:EmojiNameMaxLen])
	}
	return name
}

// NormalizePackTitle 清洗组标题；空标题回落到「我的表情包」。
func NormalizePackTitle(title string) string {
	title = strings.TrimSpace(title)
	title = strings.ReplaceAll(title, "\n", " ")
	if title == "" {
		return "我的表情包"
	}
	if utf8.RuneCountInString(title) > PackTitleMaxLen {
		title = string([]rune(title)[:PackTitleMaxLen])
	}
	return title
}

// Emoji 表情领域实体。URL 对内置表情是裸文件名，对上传表情是图床绝对地址。
type Emoji struct {
	ID          int
	PackID      int
	Name        string
	URL         string
	IsBuiltin   bool
	OwnerUserID int
	IsDeleted   bool
	CreatedAt   time.Time
}

// EmojiPack 表情包组领域实体。StickerCount / AddedByMe 是面向面板的派生字段。
type EmojiPack struct {
	ID           int
	Title        string
	OwnerUserID  int
	IsPersonal   bool
	IsPublic     bool
	CoverEmojiID int
	StickerCount int
	AddedByMe    bool
	IsDeleted    bool
	CreatedAt    time.Time
	Preview      []*Emoji
}

// EmojiPanel 一次拉齐表情面板需要的全部数据。
// 内置组（小汐/SSE娘）始终展示；个人表情组默认为空，不上传/长按添加就没有内容。
type EmojiPanel struct {
	BuiltinPack  *EmojiPack
	Builtin      []*Emoji
	PersonalPack *EmojiPack
	Personal     []*Emoji
	Packs        []*EmojiPack // 用户添加 / 自建的分享组
	PackItems    map[int][]*Emoji
}

// EmojiSearchResult 搜索结果：命中的单张表情 + 命中的组。
type EmojiSearchResult struct {
	Stickers []*Emoji
	Packs    []*EmojiPack
}

// EmojiStickerInput 新建一张表情的入参（URL 已由控制层上传环节拿到）。
// IsBuiltin 标记「资产来自内置清单」（长按收藏内置表情的复制件）：
// 这类行 isBuiltin=true，前端仍从内置资源解析图片，不依赖图床。
type EmojiStickerInput struct {
	Name      string
	URL       string
	IsBuiltin bool
}
