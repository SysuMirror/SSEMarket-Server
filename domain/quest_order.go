package domain

import "time"

// QuestOrder 任务单（一人一单）。
type QuestOrder struct {
	ID          int64
	PostID      int
	PublisherID int
	WorkerID    int
	Status      string
	AcceptedAt  time.Time
	SubmittedAt *time.Time
	CompletedAt *time.Time
	CancelledAt *time.Time
	SubmitNote  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
