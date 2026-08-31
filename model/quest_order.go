package model

import "time"

type QuestOrder struct {
	OrderID     int64      `gorm:"primary_key;column:order_id;autoIncrement"`
	PostID      int        `gorm:"column:post_id;uniqueIndex:uk_quest_order_post;not null"`
	PublisherID int        `gorm:"column:publisher_id;not null"`
	WorkerID    int        `gorm:"column:worker_id;not null;index:idx_quest_order_worker"`
	Status      string     `gorm:"column:status;type:varchar(32);not null"`
	AcceptedAt  time.Time  `gorm:"column:accepted_at;type:datetime;not null"`
	SubmittedAt *time.Time `gorm:"column:submitted_at;type:datetime"`
	CompletedAt *time.Time `gorm:"column:completed_at;type:datetime"`
	CancelledAt *time.Time `gorm:"column:cancelled_at;type:datetime"`
	SubmitNote  string     `gorm:"column:submit_note;type:varchar(500)"`
	CreatedAt   time.Time  `gorm:"column:created_at;type:datetime;not null"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;type:datetime;not null"`
}
