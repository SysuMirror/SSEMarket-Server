package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// NoticeRecord 通知原始记录。
type NoticeRecord struct {
	NoticeID int
	Receiver int
	Sender   int
	Type     string
	Content  string
	Read     bool
	Target   int
	Time     time.Time
}

// NoticeRepository 通知持久化接口。
type NoticeRepository interface {
	CountByReceiver(ctx context.Context, receiverID int) (int64, error)
	CountByReceiverAndRead(ctx context.Context, receiverID int, read bool) (int64, error)
	ListPage(ctx context.Context, receiverID, read, requireID, pageSize int) ([]NoticeRecord, error)
	MarkRead(ctx context.Context, noticeID int) error
	FindUserByID(ctx context.Context, userID int) (*domain.User, error)
	FindPcommentTargetPost(ctx context.Context, pcommentID int) (postID int, commentTime time.Time, err error)
	FindCcommentChain(ctx context.Context, ccommentID int) (pcommentID, postID int, commentTime time.Time, err error)
	// FindPostAnonMeta 返回帖子的作者 ID 与匿名标记，用于通知里脱敏匿名帖作者。
	FindPostAnonMeta(ctx context.Context, postID int) (authorID int, isAnonymous bool, err error)
}
