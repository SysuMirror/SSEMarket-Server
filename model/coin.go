package model

import "time"

// CoinLedger 币流水。
//
// 幂等口径与 novel_ledger 一致：
//   - dedup_key 唯一索引 —— 同一件事重复提交只会有一条流水；
//   - settled_value —— 纯计数事件（当日阅读秒数、当日写作字数）的原始量游标。
//
// 与小说积分体系完全独立：那边记 experience / novel_points，这边只记 coins。
type CoinLedger struct {
	LedgerID int `gorm:"primaryKey;autoIncrement;column:ledger_id"`

	UserID int `gorm:"index:idx_coin_user_time,priority:1;column:user_id;type:int;not null"`

	// 投币/评分类事件挂在被投内容上；发帖、阅读、写作这类每日任务 target_type 为空。
	TargetType string `gorm:"index:idx_coin_target,priority:1;column:target_type;type:varchar(16);not null;default:''"`
	TargetID   int    `gorm:"index:idx_coin_target,priority:2;column:target_id;type:int;not null;default:0"`

	EventType string `gorm:"index:idx_coin_event;column:event_type;type:varchar(32);not null"`
	// Amount 正数=发币，负数=投币支出。
	Amount int `gorm:"column:amount;type:int;not null;default:0"`

	SettledValue int    `gorm:"column:settled_value;type:int;not null;default:0"`
	DedupKey     string `gorm:"uniqueIndex:uk_coin_dedup;column:dedup_key;type:varchar(128);not null"`

	CreatedAt time.Time `gorm:"index:idx_coin_user_time,priority:2;column:created_at;type:datetime"`
}

func (CoinLedger) TableName() string { return "coin_ledger" }

// CoinThrow 谁给哪个内容投了几个币。
// 唯一索引 (user_id, target_type, target_id) 是「每人每个内容最多 2 币」的硬约束，
// 服务层的判断只是提前给出更友好的错误文案。
type CoinThrow struct {
	ThrowID int `gorm:"primaryKey;autoIncrement;column:throw_id"`

	UserID     int `gorm:"uniqueIndex:uk_coin_throw,priority:1;index:idx_coin_throw_user;column:user_id;type:int;not null"`
	TargetType string `gorm:"uniqueIndex:uk_coin_throw,priority:2;column:target_type;type:varchar(16);not null"`
	TargetID   int `gorm:"uniqueIndex:uk_coin_throw,priority:3;index:idx_coin_throw_target,priority:1;column:target_id;type:int;not null"`

	// 当前在这条内容上投着的币数（0–2）；取消投币会减回 0。
	CoinNum int `gorm:"column:coin_num;type:int;not null;default:0"`
	// 历史累计投过的总枚数（取消不减）：投币流水的幂等键取这个序号，
	// 取消后再投也不会和旧流水撞 dedup_key。
	ThrownTotal int `gorm:"column:thrown_total;type:int;not null;default:0"`

	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:datetime"`
}

func (CoinThrow) TableName() string { return "coin_throw" }
