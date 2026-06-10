package domain

import "time"

// User 领域用户实体。
type User struct {
	ID                         int
	Phone                      string
	Email                      string
	Name                       string
	Intro                      string
	AvatarURL                  string
	Identity                   string
	Score                      int
	BanEnd                     time.Time
	PunishNum                  int
	HideProfilePosts           bool
	ISEmailNotificationBlocked bool
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
