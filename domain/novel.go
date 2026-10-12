package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 小说连载状态。
const (
	NovelStatusSerializing = "serializing"
	NovelStatusCompleted   = "completed"
)

// 评论挂载目标：书评 / 章评 / 段评。
const (
	NovelTargetBook      = "book"
	NovelTargetChapter   = "chapter"
	NovelTargetParagraph = "paragraph"
)

// 评分：1–5 星，每人每本一条，可以改。
const (
	NovelRatingMin = 1
	NovelRatingMax = 5
)

// Novel 小说领域实体。
type Novel struct {
	ID         int
	UserID     int
	Title      string
	AuthorName string
	CoverURL   string
	Category   string
	Intro      string
	Status     string

	ChapterNum int
	WordNum    int
	ReadNum    int
	ShelfNum   int
	CommentNum int

	// CoinNum 被投币总数；RatingSum/RatingCount 评分总和与人数（平均分 = Sum/Count）。
	CoinNum     int
	RatingSum   int
	RatingCount int

	LatestChapterID    int
	LatestChapterTitle string
	LatestChapterAt    *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NovelChapter 章节实体。
type NovelChapter struct {
	ID         int
	NovelID    int
	ChapterNum int
	Title      string
	Content    string
	AuthorNote string

	WordNum    int
	CommentNum int
	ReadNum    int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// NovelComment 小说评论（书评 / 章评 / 段评）。
type NovelComment struct {
	ID             int
	NovelID        int
	ChapterID      int
	ParagraphIndex int
	TargetType     string
	ParagraphExcerpt string

	UserID  int
	Content string
	LikeNum int
	DenyNum int

	CreatedAt time.Time
}

// NovelParagraph 渲染用段落：Index 与前端分段序号一致。
// ImageURL 非空表示这是一段图片（正文里单独一行的图片），前端按图片渲染。
type NovelParagraph struct {
	Index      int    `json:"index"`
	Text       string `json:"text"`
	ImageURL   string `json:"imageURL"`
	CommentNum int    `json:"commentNum"`
}

// NovelBrowseFilter 书列表筛选。
type NovelBrowseFilter struct {
	Keyword  string
	Category string
	Status   string
	ShelfFor int // >0 时只取该用户书架上的书
	AuthorFor int // >0 时只取该用户写的书
	Limit    int
	Offset   int
}

// NovelCommentFilter 评论列表筛选。
type NovelCommentFilter struct {
	NovelID        int
	TargetType     string
	ChapterID      int
	ParagraphIndex int
	Limit          int
	Offset         int
}

// NovelCreateInput 建书入参。
type NovelCreateInput struct {
	UserID     int
	Title      string
	AuthorName string
	CoverURL   string
	Category   string
	Intro      string
	Status     string
}

// NovelUpdateInput 改书入参：指针字段表示「本次要改的」，nil 表示不动。
type NovelUpdateInput struct {
	NovelID  int
	UserID   int
	Title    *string
	CoverURL *string
	Category *string
	Intro    *string
	Status   *string
}

// NovelChapterCreateInput 发章入参。
type NovelChapterCreateInput struct {
	NovelID    int
	UserID     int
	Title      string
	Content    string
	AuthorNote string
}

// NovelCommentCreateInput 发评论入参。
type NovelCommentCreateInput struct {
	NovelID        int
	TargetType     string
	ChapterID      int
	ParagraphIndex int
	UserID         int
	Content        string
}

// NovelProgressInput 上报阅读进度入参。
// ReadSeconds 是本次心跳新增的阅读秒数（服务端封顶后累计到经验游标）。
type NovelProgressInput struct {
	NovelID        int
	UserID         int
	ChapterID      int
	ParagraphIndex int
	Percent        float64
	ReadSeconds    int
}

// NovelProgress 阅读进度实体。
type NovelProgress struct {
	NovelID        int
	UserID         int
	ChapterID      int
	ChapterNum     int
	ParagraphIndex int
	Percent        float64
	UpdatedAt      time.Time
}

// NovelViewerStats 当前用户在这批书上的个人状态（列表页按钮态用）。
type NovelViewerStats struct {
	// Ratings 我给每本书打了几分（0 = 没打过）。
	Ratings map[int]int
	// Coins 我给每本书投了几个币（0 = 没投）。
	Coins map[int]int
}

// NovelCategory 分类聚合。
type NovelCategory struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// NovelChapterAdjacent 上下章导航。
type NovelChapterAdjacent struct {
	ID    int    `json:"chapterID"`
	Num   int    `json:"chapterNum"`
	Title string `json:"title"`
}

// NovelChapterDetail 阅读页数据：正文 + 段落（含段评数） + 上下章。
type NovelChapterDetail struct {
	Chapter    *NovelChapter
	NovelTitle string
	AuthorName string
	Paragraphs []NovelParagraph
	Prev       *NovelChapterAdjacent
	Next       *NovelChapterAdjacent
}

// NovelRatingResult 一次评分的结果。
type NovelRatingResult struct {
	NovelID     int     `json:"novelID"`
	Score       int     `json:"score"`   // 这次打了几分
	Changed     bool    `json:"changed"` // 分数真的变了（重复提交同一个分数为 false）
	FirstTime   bool    `json:"firstTime"`
	RatingCount int     `json:"ratingCount"`
	RatingAvg   float64 `json:"ratingAvg"`
	Coins       int     `json:"coins"` // 作者因此拿到的币（同一读者只给一次）
}

// NovelRatingAvg 平均分；没人评返回 0。
func NovelRatingAvg(n *Novel) float64 {
	if n == nil || n.RatingCount <= 0 {
		return 0
	}
	return float64(n.RatingSum) / float64(n.RatingCount)
}

// 小说业务错误。
var (
	ErrNovelNotAuthor = errors.New("只有本书作者可以管理这本书")
	ErrNovelRateSelf  = errors.New("不能给自己的书评分")
)

// SplitNovelParagraphs 正文分段规则：换行分隔、去空白行。
// 前端 src/utils/novelText.ts 必须与此完全一致，否则段评序号会错位。
func SplitNovelParagraphs(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// novelImageExts 正文图片支持的扩展名（裸链接自动识别时用）。
var novelImageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp"}

// novelNonImageExts 一眼就不是图片的后缀。
// Markdown 图片语法里作者已经写明「这是一张图」，而图床返回的 URL 常常没有扩展名
// （例如 https://image.example/i/1728abc），所以那种写法只排除这些明确的非图片文件。
var novelNonImageExts = []string{
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".md", ".csv",
	".mp3", ".wav", ".m4a", ".ogg", ".flac", ".mp4", ".mov", ".avi", ".mkv", ".webm",
	".zip", ".rar", ".7z", ".gz", ".tar", ".exe", ".apk", ".js", ".json", ".html", ".htm",
}

// normalizeNovelURL 归一化并校验链接形态：无空白、必须是 http(s):// 或协议相对 //。
// 返回小写形态供扩展名判断，原始 URL 由调用方自己保留。
func normalizeNovelURL(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" || strings.ContainsAny(u, " \t\n") {
		return "", false
	}
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "//") {
		return "", false
	}
	return lower, true
}

// novelURLPath 去掉 query / fragment，图床常带 ?x-oss-process=...。
func novelURLPath(lower string) string {
	if i := strings.IndexAny(lower, "?#"); i >= 0 {
		return lower[:i]
	}
	return lower
}

// isNovelImageURL 是否是一个图片直链（必须带图片扩展名）。
func isNovelImageURL(raw string) bool {
	lower, ok := normalizeNovelURL(raw)
	if !ok {
		return false
	}
	path := novelURLPath(lower)
	for _, ext := range novelImageExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// isNovelMarkdownImageURL Markdown 图片语法里的 URL：允许无扩展名的图床链接，
// 但排除 novelNonImageExts 里明确的非图片文件。
func isNovelMarkdownImageURL(raw string) bool {
	lower, ok := normalizeNovelURL(raw)
	if !ok {
		return false
	}
	path := novelURLPath(lower)
	for _, ext := range novelNonImageExts {
		if strings.HasSuffix(path, ext) {
			return false
		}
	}
	return true
}

// NovelParagraphImage 判断段落是否为「图片段落」，是则返回图片 URL。
//
// 支持两种写法（都要求整段只有图片本身）：
//  1. Markdown 图片：![任意说明](https://.../a.png) —— 作者明确意图，URL 可以没有扩展名
//  2. 单独一行的图片直链：https://.../a.png —— 自动识别，必须带图片扩展名
//
// 分段规则按换行切段、丢弃空白行，所以「图片上下各留一个空行」天然就是一段。
func NovelParagraphImage(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "![") {
		i := strings.Index(s, "](")
		if i < 0 {
			return ""
		}
		rest := s[i+2:]
		end := strings.LastIndex(rest, ")")
		if end < 0 {
			return ""
		}
		url := strings.TrimSpace(rest[:end])
		// ![alt](url "标题")：只取第一个空格前的 URL。
		if j := strings.IndexAny(url, " \t"); j >= 0 {
			url = strings.TrimSpace(url[:j])
		}
		if !isNovelMarkdownImageURL(url) {
			return ""
		}
		return url
	}
	if isNovelImageURL(s) {
		return s
	}
	return ""
}

// NovelContentWordCount 正文字数：图片段落不计入（图片链接不算「写了多少字」）。
func NovelContentWordCount(content string) int {
	total := 0
	for _, p := range SplitNovelParagraphs(content) {
		if NovelParagraphImage(p) != "" {
			continue
		}
		total += NovelWordCount(p)
	}
	return total
}

// NovelParagraphExcerpt 段落摘要（用于评论面板标注「针对哪一段」）。
func NovelParagraphExcerpt(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if NovelParagraphImage(text) != "" {
		return "[图片]"
	}
	if maxRunes <= 0 {
		maxRunes = 40
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

// NovelWordCount 按 rune 统计正文字数（与番茄「字数」口径一致，不计空白）。
func NovelWordCount(content string) int {
	count := 0
	for _, r := range content {
		if unicode.IsSpace(r) {
			continue
		}
		count++
	}
	return count
}
