package domain

import "time"

const (
	QuestPostKindBounty  = "bounty"
	QuestPostKindInquiry = "inquiry"
	QuestPartitionHome   = "主页"

	QuestStatusOpen        = "open"
	QuestStatusInProgress  = "in_progress"
	QuestStatusCompleted   = "completed"
	QuestStatusCancelled   = "cancelled"
)

// QuestListFilter Quest 列表查询（仅 bounty/inquiry）。
type QuestListFilter struct {
	Kind         string // all | bounty | inquiry
	Status       string // all | open | in_progress | completed | cancelled
	Keyword      string
	Sort         string // time | price_asc | price_desc
	ViewerUserID int
	AuthorUserID int // >0 时仅该作者
	SavedOnly    bool
	Limit        int
	Offset       int
}

// QuestPostCreateInput Quest 发帖入参。
type QuestPostCreateInput struct {
	UserID     int
	Title      string
	Content    string
	PostKind   string
	QuotePrice *float64
}

// QuestPostItem 列表/详情对外展示。
type QuestPostItem struct {
	PostID        int        `json:"postID"`
	PostKind      string     `json:"postKind"`
	QuestStatus   string     `json:"questStatus"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	QuotePrice    *float64   `json:"quotePrice,omitempty"`
	CommentNum    int        `json:"commentNum"`
	PostTime      time.Time  `json:"postTime"`
	AuthorID      int        `json:"authorID"`
	AuthorName    string     `json:"authorName"`
	AuthorAvatar  string     `json:"authorAvatar"`
	AuthorScore   int        `json:"authorScore"`
	IsSaved       bool       `json:"isSaved"`
}

// QuestMeInfo 当前用户在 Quest 的权限摘要。
type QuestMeInfo struct {
	UserID     int    `json:"userID"`
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	AvatarURL  string `json:"avatarURL"`
	UserType   string `json:"userType"`
	Score      int    `json:"score"`
	CanPost    bool   `json:"canPost"`
	IsExternal bool   `json:"isExternal"`
}

func IsQuestPostKind(kind string) bool {
	return kind == QuestPostKindBounty || kind == QuestPostKindInquiry
}

func IsValidQuestStatus(status string) bool {
	switch status {
	case QuestStatusOpen, QuestStatusInProgress, QuestStatusCompleted, QuestStatusCancelled:
		return true
	default:
		return false
	}
}
