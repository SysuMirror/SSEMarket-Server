package model

import "time"

// ForumAgentAction reserves an operation before any side effect. Unfinished
// reservations are never automatically replayed, including after a restart.
type ForumAgentAction struct {
	ID          uint   `gorm:"primaryKey"`
	UserID      int    `gorm:"not null;uniqueIndex:uq_forum_agent_action,priority:1"`
	ActionKey   string `gorm:"type:varchar(64);not null;uniqueIndex:uq_forum_agent_action,priority:2"`
	PayloadHash string `gorm:"type:char(64);not null"`
	Result      string `gorm:"type:longtext"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ForumAgentMedia struct {
	MediaRef  string `gorm:"type:char(36);primaryKey"`
	UserID    int    `gorm:"not null;index"`
	SHA256    string `gorm:"type:char(64);not null"`
	URL       string `gorm:"type:text;not null"`
	Width     int
	Height    int
	CreatedAt time.Time
}

func (ForumAgentAction) TableName() string { return "forum_agent_actions" }
func (ForumAgentMedia) TableName() string  { return "forum_agent_media" }
