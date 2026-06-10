package domain

import "time"

// Sue 举报记录。
type Sue struct {
	ID         int
	TargetType string
	TargetID   int
	UserID     int
	Reason     string
	Time       time.Time
	Status     string
	Finish     bool
}

// SueListItem 待处理举报列表项（含目标摘要）。
type SueListItem struct {
	ID           int
	TargetType   string
	TargetTitle  string
	TargetDetail string
	Reason       string
	Time         time.Time
}

// ViolationResult 违规处理结果。
type ViolationResult struct {
	TargetUserID int
	SueType      string
	Content      string
	SueID        int
	ReporterID   int
	BanEnd       time.Time
}
