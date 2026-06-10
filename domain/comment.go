package domain

import "time"

// Comment 领域评论实体。
type Comment struct {
	ID        int
	PostID    int
	UserID    int
	Content   string
	LikeNum   int
	DenyNum   int
	CreatedAt time.Time
}
