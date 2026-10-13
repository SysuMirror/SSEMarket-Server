package model

import "time"

// NovelLedger 积分/经验流水。
//
// 幂等口径：
//   - dedup_key 唯一索引 —— 同一事件重复提交只会有一条流水；
//   - settled_value —— 里程碑类事件（阅读秒数、写作字数）的原始量游标，
//     amount 记已发放份数，重复上报不会重复发放。
type NovelLedger struct {
	LedgerID int `gorm:"primaryKey;autoIncrement;column:ledger_id"`

	UserID  int `gorm:"index:idx_ledger_user_time,priority:1;column:user_id;type:int;not null"`
	NovelID int `gorm:"index:idx_ledger_novel;column:novel_id;type:int;default:0"`

	EventType string `gorm:"index:idx_ledger_event;column:event_type;type:varchar(32);not null"`
	Amount    int    `gorm:"column:amount;type:int;not null;default:0"`

	SettledValue int    `gorm:"column:settled_value;type:int;not null;default:0"`
	DedupKey     string `gorm:"uniqueIndex:uk_ledger_dedup;column:dedup_key;type:varchar(128);not null"`

	CreatedAt time.Time `gorm:"index:idx_ledger_user_time,priority:2;column:created_at;type:datetime"`
}

func (NovelLedger) TableName() string { return "novel_ledger" }
