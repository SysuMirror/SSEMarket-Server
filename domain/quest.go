package domain

import "time"

const (
	QuestPostKindBounty  = "bounty"
	QuestPostKindInquiry = "inquiry"
	QuestPartitionHome   = "主页"

	QuestStatusOpen           = "open"
	QuestStatusInProgress     = "in_progress"
	QuestStatusPendingConfirm = "pending_confirm"
	QuestStatusCompleted      = "completed"
	QuestStatusCancelled      = "cancelled"

	QuestPriceFixed      = "fixed"
	QuestPriceRange      = "range"
	QuestPriceNegotiable = "negotiable"

	QuestOrderAccepted  = "accepted"
	QuestOrderSubmitted = "submitted"
	QuestOrderCompleted = "completed"
	QuestOrderCancelled = "cancelled"
)

// QuestListFilter Quest 列表查询（仅 bounty/inquiry）。
type QuestListFilter struct {
	Kind         string
	Status       string
	Keyword      string
	Sort         string
	ViewerUserID int
	AuthorUserID int
	SavedOnly    bool
	Limit        int
	Offset       int
}

// QuestAttachment 图片附件元数据。
type QuestAttachment struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// QuestPostCreateInput Quest 发帖入参。
type QuestPostCreateInput struct {
	UserID          int
	Title           string
	Content         string
	PostKind        string
	PriceType       string
	QuotePrice      *float64
	QuotePriceMin   *float64
	QuotePriceMax   *float64
	Deadline        *time.Time
	Attachments     []QuestAttachment
}

// QuestOrderSummary 任务单摘要（详情展示）。
type QuestOrderSummary struct {
	OrderID     int64      `json:"orderID"`
	PostID      int        `json:"postID"`
	PublisherID int        `json:"publisherID"`
	WorkerID    int        `json:"workerID"`
	WorkerName  string     `json:"workerName,omitempty"`
	WorkerAvatar string    `json:"workerAvatar,omitempty"`
	Status      string     `json:"status"`
	AcceptedAt  time.Time  `json:"acceptedAt"`
	SubmittedAt *time.Time `json:"submittedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	SubmitNote  string     `json:"submitNote,omitempty"`
}

// QuestPostItem 列表/详情对外展示。
type QuestPostItem struct {
	PostID           int                `json:"postID"`
	PostKind         string             `json:"postKind"`
	QuestStatus      string             `json:"questStatus"`
	Title            string             `json:"title"`
	Content          string             `json:"content"`
	PriceType        string             `json:"priceType"`
	QuotePrice       *float64           `json:"quotePrice,omitempty"`
	QuotePriceMin    *float64           `json:"quotePriceMin,omitempty"`
	QuotePriceMax    *float64           `json:"quotePriceMax,omitempty"`
	QuestDeadline    *time.Time         `json:"questDeadline,omitempty"`
	Attachments      []QuestAttachment  `json:"attachments,omitempty"`
	CommentNum       int                `json:"commentNum"`
	PostTime         time.Time          `json:"postTime"`
	AuthorID         int                `json:"authorID"`
	AuthorName       string             `json:"authorName"`
	AuthorAvatar     string             `json:"authorAvatar"`
	AuthorScore      int                `json:"authorScore"`
	IsSaved          bool               `json:"isSaved"`
	Order            *QuestOrderSummary `json:"order,omitempty"`
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

// QuestLevelRule 等级规则条目。
type QuestLevelRule struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Min  int    `json:"min"`
	Max  int    `json:"max"`
}

// QuestLevelInfo 等级说明。
type QuestLevelInfo struct {
	Score      int              `json:"score"`
	Level      QuestLevelRule   `json:"level"`
	NextLevel  *QuestLevelRule  `json:"nextLevel,omitempty"`
	NeedPoints int              `json:"needPoints"`
	Rules      []QuestScoreRule `json:"rules"`
}

// QuestScoreRule 集市积分获取说明。
type QuestScoreRule struct {
	Action  string `json:"action"`
	Points  int    `json:"points"`
}

func IsQuestPostKind(kind string) bool {
	return kind == QuestPostKindBounty || kind == QuestPostKindInquiry
}

func IsValidQuestStatus(status string) bool {
	switch status {
	case QuestStatusOpen, QuestStatusInProgress, QuestStatusPendingConfirm, QuestStatusCompleted, QuestStatusCancelled:
		return true
	default:
		return false
	}
}

func IsValidQuestPriceType(t string) bool {
	switch t {
	case QuestPriceFixed, QuestPriceRange, QuestPriceNegotiable, "":
		return true
	default:
		return false
	}
}

// QuestLevels 与前端展示一致的集市积分等级表。
var QuestLevels = []QuestLevelRule{
	{Code: "rookie", Name: "Lv.新手", Min: 0, Max: 99},
	{Code: "advanced", Name: "Lv.进阶", Min: 100, Max: 499},
	{Code: "active", Name: "Lv.活跃", Min: 500, Max: 999},
	{Code: "master", Name: "Lv.大神", Min: 1000, Max: 999999},
}

func QuestLevelForScore(score int) (QuestLevelRule, *QuestLevelRule, int) {
	for i, lv := range QuestLevels {
		if score >= lv.Min && score <= lv.Max {
			var next *QuestLevelRule
			need := 0
			if i+1 < len(QuestLevels) {
				n := QuestLevels[i+1]
				next = &n
				need = n.Min - score
				if need < 0 {
					need = 0
				}
			}
			return lv, next, need
		}
	}
	last := QuestLevels[len(QuestLevels)-1]
	return last, nil, 0
}
