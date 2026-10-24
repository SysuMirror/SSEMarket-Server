package domain

import "time"

// User 领域用户实体。
type User struct {
	ID        int
	Phone     string
	Email     string
	Name      string
	Intro     string
	AvatarURL string
	Identity  string
	Score     int
	// Experience 读者经验；NovelPoints 作者积分（单位：分，1 积分 = 10 分）；
	// Coins 站内币余额（发帖/阅读/写作/评分赚来，投币时花掉，见 domain/coin.go）。
	Experience  int
	NovelPoints int
	// Coins 站内币余额，保留两位小数（DECIMAL(12,2)）。
	Coins float64
	// CustomTitle 满级(Lv8, score>=5000)用户自定义称号;空=未自定义,对外显示用户名
	CustomTitle string
	// ReceivedLikeNum 收到的赞总数（用户卡片/排行榜展示用）。
	ReceivedLikeNum            int
	BanEnd                     time.Time
	PunishNum                  int
	HideProfilePosts           bool
	ISEmailNotificationBlocked bool
	UserType                   string
}

// DeleteMeInput 用户自助注销入参。
type DeleteMeInput struct {
	Phone    string
	Email    string
	ValiCode string
}

// ModifyPasswordInput 修改密码入参（密码为明文）。
type ModifyPasswordInput struct {
	Email    string
	Password string
	ValiCode string
}

// ModifyEmailInput 修改邮箱入参。
// UserID 来自登录态，服务层不会信任客户端传入的旧邮箱。
type ModifyEmailInput struct {
	UserID   int
	NewEmail string
	ValiCode string
}

// UpdateUserInput 更新用户信息入参。
type UpdateUserInput struct {
	UserID           int
	Name             string
	Intro            string
	AvatarURL        string
	HideProfilePosts *bool
}

// OAuth2App OAuth2 应用信息。
type OAuth2App struct {
	AppID       string
	AppName     string
	AppSecret   string
	AppIcon     string
	RedirectURI string
	Description string
}

// OAuth2AuthorizeInput OAuth2 授权入参。
type OAuth2AuthorizeInput struct {
	AppID        string
	RedirectURI  string
	ResponseType string
	Scope        string
	State        string
	UserID       int
	IPAddress    string
	UserAgent    string
}

// OAuth2TokenInput OAuth2 换取令牌入参。
type OAuth2TokenInput struct {
	Code      string
	AppID     string
	AppSecret string
}

// OAuth2TokenResult OAuth2 令牌结果。
type OAuth2TokenResult struct {
	AccessToken string
	Scope       string
	ExpiresIn   int
}

// OAuth2Code OAuth2 授权码。
type OAuth2Code struct {
	CodeID      int
	Code        string
	AppID       string
	UserID      int
	Scope       string
	RedirectURI string
	State       string
	ExpiresAt   time.Time
	IsUsed      bool
}

// OAuth2AuthRecord OAuth2 授权记录。
type OAuth2AuthRecord struct {
	RecordID     int
	UserID       int
	AppID        string
	AppName      string
	Scope        string
	IPAddress    string
	UserAgent    string
	Status       string
	AuthorizedAt time.Time
	ExpiresAt    time.Time
}

// OAuth2AuthRecordPage OAuth2 授权记录分页。
type OAuth2AuthRecordPage struct {
	Records    []OAuth2AuthRecord
	Total      int64
	Page       int
	PageSize   int
	TotalPages int64
}

// UserAPITokenScope 单个 scope 定义。
type UserAPITokenScope struct {
	Name        string `json:"name"`        // 如 "agent:chat"
	Description string `json:"description"` // 中文描述
}

// UserAPITokenCreateInput 创建 PAT 入参。
type UserAPITokenCreateInput struct {
	Name   string
	Scopes []string
	UserID int
}

// UserAPITokenCreateResult 创建 PAT 结果。PlainText 仅在此返回一次。
type UserAPITokenCreateResult struct {
	TokenID   int
	PlainText string // 完整明文 token，仅创建时返回
	Prefix    string
	Name      string
	Scopes    string
	IsActive  bool
	CreatedAt time.Time
}

// UserAPITokenItem PAT 列表项（不含明文）。
type UserAPITokenItem struct {
	TokenID    int        `json:"token_id"`
	Prefix     string     `json:"prefix"`
	Name       string     `json:"name"`
	Scopes     string     `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
}

// UserAPITokenPage PAT 分页结果。
type UserAPITokenPage struct {
	Records    []UserAPITokenItem `json:"records"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalPages int64              `json:"total_pages"`
}

// UserAPITokenVerify PAT 验证结果，供智能体侧使用。
type UserAPITokenVerify struct {
	TokenID  int
	UserID   int
	Scopes   []string
	IsActive bool
}

// RefreshTokenRecord 刷新令牌记录。
type RefreshTokenRecord struct {
	ID        int
	UserID    int
	Token     string
	ExpiresAt time.Time
	IsUsed    bool
}

// PostSummary 用户帖子统计摘要。
type PostSummary struct {
	ID         int
	LikeNum    int
	BrowseNum  int
	CommentNum int
	PostTime   time.Time
}

// ChatMessagePeer 聊天消息及关联用户。
type ChatMessagePeer struct {
	TargetUserID int
	SenderUserID int
	TargetUser   ChatUserSummary
	SenderUser   ChatUserSummary
}

// ChatUserSummary 聊天相关用户摘要。
type ChatUserSummary struct {
	UserID    int
	Name      string
	AvatarURL string
	Identity  string
	Score     int
}

// UserStatistics 用户年度统计。
type UserStatistics struct {
	Name             string
	Intro            string
	Score            int
	AvatarURL        string
	DurationTime     time.Duration
	TotalPostCnt     int
	TotalPostLikeNum int
	ThisYearPostCnt  int
	ThisYearLikeNum  int
	MaxLikeNum       int
	MaxBrowseNum     int
	MaxCommentNum    int
	PCommentCnt      int
	PCommentLikeNum  int
	CcommentCnt      int
	CcommentLikeNum  int
	BeSavedCount     int64
	BeCommentedCount int64
	RepliedCount     int64
	SavedCount       int64
	ChatCount        int
	RelevantUsers    []ChatUserSummary
	MaxSayUser       ChatUserSummary
	MaxSayUserCnt    int
}
