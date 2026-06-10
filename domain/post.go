package domain

import "time"

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
	IsPrivate  bool
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
}

// BrowseItem 浏览列表单项（含作者与互动状态）。
type BrowseItem struct {
	Post     *Post
	Author   *User
	IsSaved  bool
	IsLiked  bool
}

// PostDetail 帖子详情（含作者、互动状态、实时浏览数）。
type PostDetail struct {
	Post      *Post
	Author    *User
	IsSaved   bool
	IsLiked   bool
	BrowseNum int
}

// AuthTokens 登录/注册返回。
type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	User         *User
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
