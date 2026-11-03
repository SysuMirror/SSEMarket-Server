package model

import "time"

// UserRecommendationSetting 用户对首页推荐流的选择。
// 无行 = auto（跟随灰度），on/off 是用户显式开关。
type UserRecommendationSetting struct {
	UserID    int       `gorm:"primaryKey;column:user_id;type:int;not null"`
	Mode      string    `gorm:"column:mode;type:varchar(8);not null;default:'auto'"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (UserRecommendationSetting) TableName() string { return "user_recommendation_settings" }

// RecommendationGrayRule 按 UID 区间批量开启推荐流的灰度规则。
type RecommendationGrayRule struct {
	ID        int       `gorm:"primaryKey;column:id;autoIncrement"`
	UIDMin    int       `gorm:"column:uid_min;type:int;not null;index:idx_rec_gray_range"`
	UIDMax    int       `gorm:"column:uid_max;type:int;not null;index:idx_rec_gray_range"`
	Enabled   bool      `gorm:"column:enabled;type:tinyint(1);not null;default:1"`
	Note      string    `gorm:"column:note;type:varchar(255);not null;default:''"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (RecommendationGrayRule) TableName() string { return "recommendation_gray_rules" }

// PostEmbedding 帖子向量。kind=text 是正文向量，kind=image 是首图向量。
// 两条向量分开存，避免图片稀释文本相似度（见 Spark RAG skill 的实测结论）。
type PostEmbedding struct {
	ID          int64     `gorm:"primaryKey;column:id;autoIncrement"`
	PostID      int       `gorm:"column:post_id;type:int;not null;uniqueIndex:uk_post_kind,priority:1"`
	Kind        string    `gorm:"column:kind;type:varchar(8);not null;uniqueIndex:uk_post_kind,priority:2"`
	Model       string    `gorm:"column:model;type:varchar(64);not null;default:''"`
	Dims        int       `gorm:"column:dims;type:smallint;not null;default:0"`
	ContentHash string    `gorm:"column:content_hash;type:char(32);not null;default:''"`
	ImageURL    string    `gorm:"column:image_url;type:varchar(1024);not null;default:''"`
	Embedding   []byte    `gorm:"column:embedding;type:mediumblob"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime"`
}

func (PostEmbedding) TableName() string { return "post_embeddings" }

// PostReadEvent 推荐流已读标记：用户点进详情即写一行，默认未读。
// 后续前端计时器可用同一个接口补充/更新 dwell_ms（取最大值），用于强化画像。
// (user_id, post_id) 唯一，标记已读幂等。
type PostReadEvent struct {
	ID        int64     `gorm:"primaryKey;column:id;autoIncrement"`
	UserID    int       `gorm:"column:user_id;type:int;not null;index:idx_rec_read_user;uniqueIndex:uk_rec_read_user_post,priority:1"`
	PostID    int       `gorm:"column:post_id;type:int;not null;uniqueIndex:uk_rec_read_user_post,priority:2"`
	DwellMS   int       `gorm:"column:dwell_ms;type:int;not null;default:0"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime;index:idx_rec_read_user,priority:2"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (PostReadEvent) TableName() string { return "post_read_events" }
