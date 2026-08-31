package domain

import "time"

// 置顶队列作用域与来源常量（Scheme B）。
const (
	PinScopeHome = "home"

	PinSourceUser   = "user"
	PinSourceAdmin  = "admin"
	PinSourceSystem = "system"

	PinActionEnqueue = "enqueue"
	PinActionDequeue = "dequeue"
	PinActionExpire  = "expire"
	PinActionReject  = "reject"
)

// PinQueueEntry 独立置顶队列条目（与 Post 解耦）。
type PinQueueEntry struct {
	PinID       int64
	Scope       string
	PostID      int
	OwnerUserID int
	Source      string
	EnqueueAt   time.Time
	ExpireAt    time.Time
}

// Active 是否未过期。
func (e *PinQueueEntry) Active(now time.Time) bool {
	if e == nil {
		return false
	}
	return e.ExpireAt.After(now)
}

// PinEnqueueInput 入队参数。
type PinEnqueueInput struct {
	Scope       string
	PostID      int
	OwnerUserID int
	ActorUserID int // 审计操作者；用户自置顶时通常等于 OwnerUserID
	Source      string
	ExpireAt    time.Time
}

// PinEvent 审计事件领域对象。
type PinEvent struct {
	Scope       string
	PostID      int
	ActorUserID int
	Action      string
	Source      string
	Detail      string
	EnqueueAt   *time.Time
	ExpireAt    *time.Time
}
