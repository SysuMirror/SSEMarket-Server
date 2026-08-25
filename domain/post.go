package domain

import "time"

// MaxPinnedQueueSize 主页置顶队列最大长度(先进先出,全站共享)。
const MaxPinnedQueueSize = 5

// Post 领域帖子实体（与持久化 model 解耦）。
type Post struct {
	ID         int
	UserID     int
	Partition  string
	Title      string
	Content    string
	CommentNum int
	LikeNum    int
	VoteNum    int
	BrowseNum  int
	PostTime   time.Time
	Heat       float64
	Photos     string
	Tag        string
	IsPrivate   bool
	IsAnonymous bool
	IsPinned         bool
	PinnedAt         *time.Time
	PinnedExpiresAt  *time.Time
}

// EffectiveIsPinned 是否「当前仍有效」的置顶。
// 过期后库中 is_pinned 可能仍为 1(无回写时),列表/角标应以此为准,避免假置顶。
func EffectiveIsPinned(p *Post) bool {
	if p == nil || !p.IsPinned || p.PinnedExpiresAt == nil {
		return false
	}
	return p.PinnedExpiresAt.After(time.Now())
}

// BrowseFilter 帖子列表查询条件。
type BrowseFilter struct {
	ViewerUserID int
	Partition    string
	Searchinfo   string
	Tag          string
	Searchsort   string
	OrderBy      string
	TimeFrom     string
	TimeTo       string
	Limit        int
	Offset       int
	// ExcludePostIDs 命中后从结果中排除的帖子 ID 列表(主页用:排除已置顶帖避免重复)。
	ExcludePostIDs []int
}

// BrowseResult 帖子浏览列表合并返回(主页用)。
type BrowseResult struct {
	Pinned []BrowseItem // 置顶队列:未过期置顶帖(最多 MaxPinnedQueueSize,按 PinnedAt 先进先出)
	Posts  []BrowseItem // 正常帖(已排除 Pinned 中的 PostID)
	Total  int64        // 正常帖总数(不含 Pinned)
}

// BrowseItem 浏览列表单项（含作者与互动状态）。
type BrowseItem struct {
	Post             *Post
	Author           *User
	IsSaved          bool
	IsLiked          bool
	HotComment       *Pcomment // 该帖点赞最多的一条一级评论(并列按字数倒序),可能为 nil
	HotCommentAuthor *User    // HotComment.UserID 对应的用户,可能为 nil
}

// PostDetail 帖子详情（含作者、互动状态、实时浏览数）。
type PostDetail struct {
	Post      *Post
	Author    *User
	IsSaved   bool
	IsLiked   bool
	IsWatched bool
	BrowseNum int
}

// AuthTokens 登录/注册返回。
type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	User         *User
}

// PostCreateInput 发帖入参。
type PostCreateInput struct {
	UserID      int
	Title       string
	Content     string
	Partition   string
	Photos      string
	Tags        []string
	IsPrivate   bool
	IsAnonymous bool
	// Pinned 为 true 时,发帖成功后由 service 调 PinPost 给当前帖置顶 1 小时。
	Pinned bool
}

// UserProfilePostsFilter 个人主页发帖列表查询。
type UserProfilePostsFilter struct {
	TargetUserID int
	ViewerUserID int
	Limit        int
	Offset       int
}

// SubmitReportInput 举报入参。
type SubmitReportInput struct {
	TargetID   int
	TargetType string
	UserID     int
	Reason     string
}

// Tag 标签实体。
type Tag struct {
	ID    int
	Name  string
	Value string
	Type  string
	Num   int
}

// Feedback 反馈实体。
type Feedback struct {
	ID         int
	Text       string
	Attachment string
	Time       time.Time
	Status     string
}

// HeatItem 热度榜单项。
type HeatItem struct {
	PostID uint
	Title  string
	Heat   float64
}

// HotPostZSetMember Redis 热度 ZSet 成员 JSON。
type HotPostZSetMember struct {
	PostID    uint    `json:"postID"`
	Title     string  `json:"title"`
	BrowseNum float64 `json:"browseNum"`
}

// RegisterInput 注册入参（密码为明文，Controller 负责解密）。
type RegisterInput struct {
	Name     string
	Phone    string
	Email    string
	Password string
	ValiCode string
	CDKey    string
}
