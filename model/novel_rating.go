package model

import "time"

// NovelRating 读者给一本小说的评分（1–5 星），每人每本一条，可以改。
//
// 评分只影响两件事：书本页面上的平均分，以及作者的收益
// （同一读者首次给某本书评分，作者 +1 币，见 domain.CoinEventRated）。
// 评分不给读者任何币或经验，读者的收益仍然只来自阅读时长那条线；
// 也不给作者积分——币与积分是两条独立的线。
type NovelRating struct {
	RatingID int `gorm:"primaryKey;autoIncrement;column:rating_id"`
	NovelID  int `gorm:"uniqueIndex:uk_novel_rating,priority:1;index:idx_novel_rating_novel;column:novel_id;type:int;not null"`
	UserID   int `gorm:"uniqueIndex:uk_novel_rating,priority:2;index:idx_novel_rating_user;column:user_id;type:int;not null"`
	Score    int `gorm:"column:score;type:int;not null;default:0"`
	// CommentID 评分自动附带的那条书评（novel_comments.comment_id，is_rating=1）；
	// 0 = 还没生成过。改分时更新这条评论的内容。
	CommentID int       `gorm:"column:comment_id;type:int;not null;default:0"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (NovelRating) TableName() string { return "novel_ratings" }
