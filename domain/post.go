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
	TimeFrom     string
	TimeTo       string
	Limit        int
	Offset       int
}
