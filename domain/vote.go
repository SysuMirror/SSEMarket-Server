package domain

import "time"

// VoteSearchItem 投票搜索单项。
type VoteSearchItem struct {
	PostID    int
	Title     string
	Partition string
	PostTime  time.Time
	VoteCount int
}

// PostRankItem 帖子优质榜单项。
type PostRankItem struct {
	PostID    int
	Title     string
	Partition string
	VoteCount int
	PostTime  time.Time
}

// UserRankItem 用户榜单项。
type UserRankItem struct {
	UserID          int
	UserName        string
	AvatarURL       string
	ReceivedLikeNum int
	ReceivedVoteNum int
	RankScore       int
}
