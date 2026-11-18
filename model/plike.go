package model

import (
	"time"
)

// Plike 点赞明细。
// (userID, ptargetID) 的唯一索引由 common.ensurePlikeUniqueIndex 在确认无重复后创建，
// 避免 AutoMigrate 在历史脏数据上直接失败。
type Plike struct {
	PlikeID   int       `gorm:"primary_key;column:plikeID"`
	PtargetID int       `gorm:"index:pliketarget;column:ptargetID"`
	Post      Post      `gorm:"association_foreignkey:ptargetID;foreignkey:postID"`
	UserID    int       `gorm:"index:plikeuser;column:userID"`
	User      User      `gorm:"association_foreignkey:userID;foreignkey:userID"`
	Time      time.Time `gorm:"column:time;type:datetime"`
}
