package model

import "time"

// LotteryPost 抽奖帖元数据，与 posts 表一对一关联
type LotteryPost struct {
	LotteryID int       `gorm:"primary_key;column:lotteryID"`
	PostID    int       `gorm:"unique_index:lottery_post;column:postID;not null"`
	Post      Post      `gorm:"association_foreignkey:postID;foreignkey:postID"`
	UserID    int       `gorm:"index:lottery_user;column:userID;not null"`
	Mode      string    `gorm:"column:mode;type:varchar(16);not null"`   // random | wish
	Status    string    `gorm:"column:status;type:varchar(16);not null"` // active | drawn | closed
	DrawAt    time.Time  `gorm:"column:draw_at;type:datetime;not null"`
	DrawnAt   *time.Time `gorm:"column:drawn_at;type:datetime"`
	CreatedAt time.Time  `gorm:"column:created_at;type:datetime"`
}

// LotteryPrize 抽奖奖品
type LotteryPrize struct {
	PrizeID     int    `gorm:"primary_key;column:prizeID"`
	LotteryID   int    `gorm:"index:prize_lottery;column:lotteryID;not null"`
	Name        string `gorm:"column:name;type:varchar(100);not null"`
	Quantity    int    `gorm:"column:quantity;type:int;not null"`
	WishKeyword string `gorm:"column:wish_keyword;type:varchar(100)"`
}

// LotteryParticipant 抽奖参与者
type LotteryParticipant struct {
	ParticipantID int       `gorm:"primary_key;column:participantID"`
	LotteryID     int       `gorm:"uniqueIndex:lottery_user_prize;column:lotteryID;not null"`
	UserID        int       `gorm:"uniqueIndex:lottery_user_prize;column:userID;not null"`
	PrizeID       int       `gorm:"uniqueIndex:lottery_user_prize;column:prizeID;type:int;default:0"`
	EntryType     string    `gorm:"column:entry_type;type:varchar(16);not null"` // button | comment
	CommentID     int       `gorm:"column:commentID;type:int;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;type:datetime"`
}

// LotteryWinner 中奖记录
type LotteryWinner struct {
	WinnerID  int       `gorm:"primary_key;column:winnerID"`
	LotteryID int       `gorm:"index:winner_lottery;column:lotteryID;not null"`
	UserID    int       `gorm:"index:winner_user;column:userID;not null"`
	PrizeID   int       `gorm:"column:prizeID;not null"`
	Notified  bool      `gorm:"column:notified;type:tinyint(1);default:0"`
	CreatedAt time.Time `gorm:"column:created_at;type:datetime"`
}
