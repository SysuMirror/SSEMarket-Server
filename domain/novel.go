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
type NovelParagraph struct {
	Index      int    `json:"index"`
	Text       string `json:"text"`
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
type NovelProgressInput struct {
	NovelID        int
	UserID         int
	ChapterID      int
	ParagraphIndex int
	Percent        float64
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

// 小说业务错误。
var (
	ErrNovelChapterLimit = errors.New("单本书最多 200 章")
	ErrNovelNotAuthor    = errors.New("只有本书作者可以管理这本书")
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

// NovelParagraphExcerpt 段落摘要（用于评论面板标注「针对哪一段」）。
func NovelParagraphExcerpt(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
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
