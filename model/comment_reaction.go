package model

const (
	CommentTargetPcomment = "pcomment"
	CommentTargetCcomment = "ccomment"
	ReactionLike          = "like"
	ReactionDeny          = "deny"
)

// CommentReaction 评论反应明细：一级评论和楼中楼共用一张表。
// (userID, targetType, targetID) 的唯一索引由 common.ensureCommentReactionUniqueIndex 创建。
type CommentReaction struct {
	ReactionID int    `gorm:"primary_key;column:reactionID"`
	UserID     int    `gorm:"column:userID;type:int;not null"`
	TargetType string `gorm:"column:targetType;type:varchar(16);not null"`
	TargetID   int    `gorm:"column:targetID;type:int;not null"`
	Reaction   string `gorm:"column:reaction;type:varchar(8);not null"`
}
