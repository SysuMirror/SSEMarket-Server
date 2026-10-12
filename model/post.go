package model

import (
	"time"
)

// Post [...]
type Post struct {
	PostID     int       `gorm:"primary_key;column:postID"`
	UserID     int       `gorm:"index:postuser;column:userID;type:int"`
	User       User      `gorm:"association_foreignkey:userID;foreignkey:userID"`
	Partition  string    `gorm:"column:partition;type:varchar(10)"`
	Title      string    `gorm:"column:title;type:varchar(90)"`
	Ptext      string    `gorm:"column:ptext;type:varchar(10000)"`
	CommentNum int       `gorm:"column:comment_num;type:int"`
	LikeNum    int       `gorm:"column:like_num;type:int"`
	VoteNum    int       `gorm:"column:vote_num;type:int;default:0"`
	// CoinNum 被投币总数（展示用，不参与任何结算；投币明细见 coin_throws）。
	CoinNum    int       `gorm:"column:coin_num;type:int;default:0"`
	BrowseNum  int       `gorm:"column:browse_num;type:int"`
	PostTime   time.Time `gorm:"column:post_time;type:datetime"`
	Heat       float64   `gorm:"column:heat;type:double"`
	Photos     string    `gorm:"column:photos;type:varchar(1000)"`
	Tag        string    `gorm:"column:tag;type:varchar(100)"`
	// PostKind 空=普通帖；bounty/inquiry 为 Quest 帖（不影响既有 browse 逻辑）。
	PostKind   string   `gorm:"column:post_kind;type:varchar(16);default:''"`
	QuotePrice *float64 `gorm:"column:quote_price;type:decimal(10,2)"`
	// QuestStatus open|in_progress|completed|cancelled
	QuestStatus string `gorm:"column:quest_status;type:varchar(16);default:''"`
	IsPrivate  bool     `gorm:"column:is_private;type:tinyint(1);default:0"`
	IsAnonymous bool     `gorm:"column:is_anonymous;type:tinyint(1);default:0"`
	IsPinned       bool       `gorm:"column:is_pinned;type:tinyint(1);default:0"`
	// PinnedAt 进入主页置顶队列的时间戳;ListPinned 按此字段先进先出排序。
	PinnedAt       *time.Time `gorm:"column:pinned_at;type:datetime"`
	PinnedExpiresAt *time.Time `gorm:"column:pinned_expires_at;type:datetime"`
}
