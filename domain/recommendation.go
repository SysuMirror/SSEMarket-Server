package domain

import "time"

// 推荐流用户开关。auto = 未表态（跟随灰度规则），on / off = 用户显式选择。
const (
	RecModeAuto = "auto"
	RecModeOn   = "on"
	RecModeOff  = "off"

	RecEffectiveTime      = "time"
	RecEffectiveRecommend = "recommend"
)

// RecommendationSettings 用户可见的推荐设置。
// Mode 是用户自己的选择；Effective 是结合灰度规则与模型可用性之后的实际推流模式。
type RecommendationSettings struct {
	Mode        string     `json:"mode"`
	Effective   string     `json:"effective"`
	GrayMatched bool       `json:"grayMatched"`
	Available   bool       `json:"available"`
	Reason      string     `json:"reason,omitempty"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// RecommendationGrayRule 按 UID 区间灰度开启推荐流。
// 同一用户命中任一 enabled 规则即视为灰度用户；用户显式 off 优先于灰度。
type RecommendationGrayRule struct {
	ID        int       `json:"id"`
	UIDMin    int       `json:"uidMin"`
	UIDMax    int       `json:"uidMax"`
	Enabled   bool      `json:"enabled"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// RecommendationStats 推荐系统运行状态（管理员排障用）。
type RecommendationStats struct {
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	Dims          int       `json:"dims"`
	ImageEnabled  bool      `json:"imageEnabled"`
	Available     bool      `json:"available"`
	PostsTotal    int64     `json:"postsTotal"`
	TextEmbedded  int64     `json:"textEmbedded"`
	ImageEmbedded int64     `json:"imageEmbedded"`
	PendingText   int64     `json:"pendingText"`
	PendingImage  int64     `json:"pendingImage"`
	IndexSize     int       `json:"indexSize"`
	IndexText     int       `json:"indexTextVectors"`
	IndexImage    int       `json:"indexImageVectors"`
	IndexLoadedAt time.Time `json:"indexLoadedAt"`
	LastRunAt     time.Time `json:"lastRunAt"`
	LastError     string    `json:"lastError,omitempty"`
}

// RecPostMeta 推荐索引里的帖子轻量元数据（不含正文，避免每次刷新拉全量文本）。
type RecPostMeta struct {
	ID              int
	UserID          int
	Partition       string
	Title           string
	Tag             string
	PostTime        time.Time
	LikeNum         int
	CommentNum      int
	CoinNum         int
	BrowseNum       int
	IsPrivate       bool
	IsAnonymous     bool
	IsPinned        bool
	PinnedExpiresAt *time.Time
}

// RecPostContent 构建向量所需的正文字段。
type RecPostContent struct {
	PostID    int
	Title     string
	Ptext     string
	Tag       string
	Partition string
	Photos    string
}

// RecEmbedding 一条帖子向量（text / image）。
type RecEmbedding struct {
	PostID      int
	Kind        string
	Model       string
	Dims        int
	ContentHash string
	ImageURL    string
	Vector      []float32
}

// RecInteraction 用户与帖子的正向互动；Weight 为基础权重（不含时间衰减）。
type RecInteraction struct {
	PostID int
	At     time.Time
	Weight float64
	// Text 评论正文（仅评论互动有值，预留给后续用评论文本强化画像）。
	Text string
}

// RecReadEvent 点进详情产生的已读事件（dwellMS=0 表示只标记已读）。
type RecReadEvent struct {
	PostID  int
	DwellMS int
	At      time.Time
}

// RecChatMessage 用户与小秋的聊天消息，用于补充兴趣画像。
type RecChatMessage struct {
	Content string
	At      time.Time
}

// RecommendSettingInput 更新推荐设置入参。
type RecommendSettingInput struct {
	UserID int
	Mode   string
}

// RecommendGrayRuleInput 管理员新增/更新灰度规则入参。
type RecommendGrayRuleInput struct {
	ID      int
	UIDMin  int
	UIDMax  int
	Enabled bool
	Note    string
}
