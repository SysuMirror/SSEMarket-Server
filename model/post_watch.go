package model

import "time"

// PostWatch 用户「码住」帖子：该帖下任意新评论/回复均通知 watcher。
type PostWatch struct {
	PostWatchID int       `gorm:"primary_key;column:postWatchID"`
	UserID      int       `gorm:"column:userID;type:int;not null;uniqueIndex:post_watch_user_post"`
	PostID      int       `gorm:"column:postID;type:int;not null;uniqueIndex:post_watch_user_post"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime"`
}

func (PostWatch) TableName() string {
	return "post_watches"
}
