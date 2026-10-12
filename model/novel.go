package model

import "time"

// 小说相关常量（状态、评论挂载目标）定义在 domain/novel.go，
// 这里只保留持久化结构，避免 controller/service 反向依赖 model。

// Novel 一本小说。
type Novel struct {
	NovelID     int    `gorm:"primaryKey;autoIncrement;column:novel_id"`
	UserID      int    `gorm:"index:idx_novel_user;column:user_id;type:int;not null"`
	Title       string `gorm:"column:title;type:varchar(100);not null"`
	AuthorName  string `gorm:"column:author_name;type:varchar(50);default:''"`
	CoverURL    string `gorm:"column:cover_url;type:varchar(512);default:''"`
	Category    string `gorm:"column:category;type:varchar(32);default:'';index:idx_novel_category"`
	Intro       string `gorm:"column:intro;type:varchar(1000);default:''"`
	Status      string `gorm:"column:status;type:varchar(16);default:'serializing';index:idx_novel_status"`

	ChapterNum int `gorm:"column:chapter_num;type:int;default:0"`
	WordNum    int `gorm:"column:word_num;type:int;default:0"`
	ReadNum    int `gorm:"column:read_num;type:int;default:0"`
	ShelfNum   int `gorm:"column:shelf_num;type:int;default:0"`
	CommentNum int `gorm:"column:comment_num;type:int;default:0"`

	// CoinNum 被投币总数（投币记录见 coin_throws，这里只是列表/详情展示用的计数）。
	CoinNum int `gorm:"column:coin_num;type:int;default:0"`
	// RatingSum / RatingCount 评分累加与人数，平均分 = sum / count。
	RatingSum   int `gorm:"column:rating_sum;type:int;default:0"`
	RatingCount int `gorm:"column:rating_count;type:int;default:0"`

	LatestChapterID    int        `gorm:"column:latest_chapter_id;type:int;default:0"`
	LatestChapterTitle string     `gorm:"column:latest_chapter_title;type:varchar(200);default:''"`
	LatestChapterAt    *time.Time `gorm:"column:latest_chapter_at;type:datetime"`

	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (Novel) TableName() string { return "novels" }

// NovelChapter 小说章节。content 为正文，段落以换行分隔（与前端分段规则一致）。
type NovelChapter struct {
	ChapterID  int    `gorm:"primaryKey;autoIncrement;column:chapter_id"`
	NovelID    int    `gorm:"index:idx_chapter_novel_num,priority:1;uniqueIndex:uk_chapter_novel_num,priority:1;column:novel_id;type:int;not null"`
	ChapterNum int    `gorm:"index:idx_chapter_novel_num,priority:2;uniqueIndex:uk_chapter_novel_num,priority:2;column:chapter_num;type:int;not null"`
	Title      string `gorm:"column:title;type:varchar(200);not null"`
	Content    string `gorm:"column:content;type:longtext"`
	AuthorNote string `gorm:"column:author_note;type:varchar(2000);default:''"`

	WordNum    int `gorm:"column:word_num;type:int;default:0"`
	CommentNum int `gorm:"column:comment_num;type:int;default:0"`
	ReadNum    int `gorm:"column:read_num;type:int;default:0"`

	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (NovelChapter) TableName() string { return "novel_chapters" }

// NovelComment 小说评论。target_type 区分书评/章评/段评；
// 段评用 chapter_id + paragraph_index 定位段落。
type NovelComment struct {
	CommentID      int    `gorm:"primaryKey;autoIncrement;column:comment_id"`
	NovelID        int    `gorm:"index:idx_novel_comment_novel;column:novel_id;type:int;not null"`
	ChapterID      int    `gorm:"column:chapter_id;type:int;default:0"`
	ParagraphIndex int    `gorm:"column:paragraph_index;type:int;default:-1"`
	TargetType     string `gorm:"column:target_type;type:varchar(16);not null;default:'book'"`
	ParagraphExcerpt string `gorm:"column:paragraph_excerpt;type:varchar(120);default:''"`

	UserID  int    `gorm:"index:idx_novel_comment_user;column:user_id;type:int;not null"`
	Content string `gorm:"column:content;type:varchar(500);not null"`
	LikeNum int    `gorm:"column:like_num;type:int;default:0"`
	DenyNum int    `gorm:"column:deny_num;type:int;default:0"`

	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
}

func (NovelComment) TableName() string { return "novel_comments" }

// NovelCommentLike 评论点赞（同一用户对同一评论只能点赞一次）。
type NovelCommentLike struct {
	LikeID    int       `gorm:"primaryKey;autoIncrement;column:like_id"`
	CommentID int       `gorm:"index:idx_novel_comment_like;uniqueIndex:uk_novel_comment_like,priority:1;column:comment_id;type:int;not null"`
	UserID    int       `gorm:"uniqueIndex:uk_novel_comment_like,priority:2;column:user_id;type:int;not null"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
}

func (NovelCommentLike) TableName() string { return "novel_comment_likes" }

// NovelBookshelf 书架（加入书架）。
type NovelBookshelf struct {
	ShelfID   int       `gorm:"primaryKey;autoIncrement;column:shelf_id"`
	NovelID   int       `gorm:"index:idx_novel_shelf_novel;uniqueIndex:uk_novel_shelf,priority:1;column:novel_id;type:int;not null"`
	UserID    int       `gorm:"index:idx_novel_shelf_user,priority:1;uniqueIndex:uk_novel_shelf,priority:2;column:user_id;type:int;not null"`
	CreatedAt time.Time `gorm:"index:idx_novel_shelf_user,priority:2;column:created_at;type:datetime"`
}

func (NovelBookshelf) TableName() string { return "novel_bookshelf" }

// NovelProgress 阅读进度（每人每本一条，覆盖更新）。
type NovelProgress struct {
	ProgressID     int     `gorm:"primaryKey;autoIncrement;column:progress_id"`
	NovelID        int     `gorm:"index:idx_novel_progress_novel;uniqueIndex:uk_novel_progress,priority:1;column:novel_id;type:int;not null"`
	UserID         int     `gorm:"index:idx_novel_progress_user,priority:1;uniqueIndex:uk_novel_progress,priority:2;column:user_id;type:int;not null"`
	ChapterID      int     `gorm:"column:chapter_id;type:int;default:0"`
	ChapterNum     int     `gorm:"column:chapter_num;type:int;default:0"`
	ParagraphIndex int     `gorm:"column:paragraph_index;type:int;default:0"`
	Percent        float64 `gorm:"column:percent;type:double;default:0"`
	UpdatedAt      time.Time `gorm:"index:idx_novel_progress_user,priority:2;column:updated_at;type:datetime"`
}

func (NovelProgress) TableName() string { return "novel_progress" }
