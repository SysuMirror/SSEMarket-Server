package domain

import "time"

// RatingCategory 评分分区。
type RatingCategory struct {
	ID          int
	Name        string
	Description string
	CreatorID   int
	PostCount   int
	IsOfficial  bool
	SortOrder   int
	CreatedAt   time.Time
}

// RatingPost 评分帖。
type RatingPost struct {
	ID         int
	CategoryID int
	UserID     int
	Title      string
	Content    string
	CommentNum int
	LikeNum    int
	BrowseNum  int
	Photos     string
	CreatedAt  time.Time
}

// RatingBrowseFilter 评分帖浏览筛选。
type RatingBrowseFilter struct {
	CategoryID int
	SearchInfo string
	Limit      int
	Offset     int
}

// RatingCategoryCreateInput 创建分区入参。
type RatingCategoryCreateInput struct {
	CreatorID    int
	Name         string
	Description  string
	SyncFeedPost bool
}

// RatingPostCreateInput 创建评分帖入参。
type RatingPostCreateInput struct {
	CategoryID int
	UserID     int
	Title      string
	Content    string
	Photos     string
}

// RatingComment 评分评论。
type RatingComment struct {
	ID           int
	RatingPostID int
	UserID       int
	Content      string
	StarNum      int
	LikeNum      int
	CreatedAt    time.Time
}

// RatingCommentCreateInput 创建评分评论入参。
type RatingCommentCreateInput struct {
	RatingPostID int
	UserID       int
	Content      string
	StarNum      int
}

// RatingStarSummary 评分帖星级汇总。
type RatingStarSummary struct {
	Stars         [5]int
	AverageRating float64
	RaterCount    int
}
