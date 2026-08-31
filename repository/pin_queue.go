package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// PinQueueRepository Scheme B 独立置顶队列持久化。
// 不读写 posts.is_pinned 字段；帖删除联动由上层 Delete 时调用 DequeueByPostID。
type PinQueueRepository interface {
	// ListActive 返回 scope 下未过期条目，按 enqueue_at ASC, post_id ASC（FIFO）。
	ListActive(ctx context.Context, scope string, limit int) ([]*domain.PinQueueEntry, error)
	// FindActiveByPostID 查询某帖是否在队且未过期。
	FindActiveByPostID(ctx context.Context, scope string, postID int) (*domain.PinQueueEntry, error)
	// CountActive 全站（某 scope）有效队列长度。
	CountActive(ctx context.Context, scope string) (int64, error)
	// CountActiveByOwner 某用户在某 scope 下有效置顶数。
	CountActiveByOwner(ctx context.Context, scope string, ownerUserID int) (int64, error)
	// Enqueue 入队；同 scope+post 已存在则更新 enqueue_at/expire_at（或由上层先查后拒）。
	Enqueue(ctx context.Context, in domain.PinEnqueueInput) (*domain.PinQueueEntry, error)
	// DequeueByPostID 手动出队；不存在返回 ErrNotFound。
	DequeueByPostID(ctx context.Context, scope string, postID int) error
	// ClearExpired 删除 expire_at <= now 的条目，返回删除行数；并写 expire 审计。
	ClearExpired(ctx context.Context, scope string, now time.Time) (int64, error)
	// AppendEvent 写入审计流水。
	AppendEvent(ctx context.Context, ev domain.PinEvent) error
}
