package model

import "time"

// PinQueue Scheme B 独立置顶队列表（当前有效条目）。
// 表名 pin_queue；与 posts 解耦，posts 内联置顶字段仅作迁移/回滚窗口兼容。
type PinQueue struct {
	PinID       int64     `gorm:"primary_key;column:pin_id;autoIncrement"`
	Scope       string    `gorm:"column:scope;type:varchar(32);not null;default:home;uniqueIndex:uk_pin_scope_post"`
	PostID      int       `gorm:"column:post_id;type:int;not null;uniqueIndex:uk_pin_scope_post"`
	OwnerUserID int       `gorm:"column:owner_user_id;type:int;not null;default:0"`
	Source      string    `gorm:"column:source;type:varchar(16);not null;default:user"`
	EnqueueAt   time.Time `gorm:"column:enqueue_at;type:datetime(3);not null"`
	ExpireAt    time.Time `gorm:"column:expire_at;type:datetime(3);not null"`
	CreatedAt   time.Time `gorm:"column:created_at;type:datetime(3)"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:datetime(3)"`
}

func (PinQueue) TableName() string { return "pin_queue" }

// PinEvent 置顶审计流水。
type PinEvent struct {
	EventID     int64      `gorm:"primary_key;column:event_id;autoIncrement"`
	Scope       string     `gorm:"column:scope;type:varchar(32);not null;default:home"`
	PostID      int        `gorm:"column:post_id;type:int;not null"`
	ActorUserID int        `gorm:"column:actor_user_id;type:int;not null;default:0"`
	Action      string     `gorm:"column:action;type:varchar(16);not null"`
	Source      string     `gorm:"column:source;type:varchar(16);not null;default:user"`
	Detail      string     `gorm:"column:detail;type:varchar(255);not null;default:''"`
	EnqueueAt   *time.Time `gorm:"column:enqueue_at;type:datetime(3)"`
	ExpireAt    *time.Time `gorm:"column:expire_at;type:datetime(3)"`
	CreatedAt   time.Time  `gorm:"column:created_at;type:datetime(3)"`
}

func (PinEvent) TableName() string { return "pin_events" }
