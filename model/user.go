package model

import "time"

// User [...]
type User struct {
	UserID   int    `gorm:"primary_key;column:userID"`
	Phone    string `gorm:"column:phone;type:char(20)" `
	Email    string `gorm:"column:email;type:varchar(255);uniqueIndex:idx_users_email"`
	Password string `gorm:"column:password;type:varchar(255)" json:"-"`
	Name     string `gorm:"column:name;type:varchar(50)"`
	//Num       int       `gorm:"column:num;type:int"`
	Intro string `gorm:"column:intro;type:varchar(255)"`
	//IDpass    bool      `gorm:"column:idpass;type:tinyint(1)"`
	Banend                     time.Time `gorm:"column:banend;type:datetime"`
	Punishnum                  int       `gorm:"column:punishnum;type:int"`
	AvatarURL                  string    `gorm:"column:avatar_url;type:varchar(255)"`
	Identity                   string    `gorm:"column:identity;type:enum('student','teacher');default:'student'"`
	Score                      int       `gorm:"column:score;type:int;default:0"`
	// Experience 读者经验（阅读/评论/签到等）；
	// NovelPoints 作者积分，以「分」记账（1 积分 = 10 分），便于 0.1 积分也能整数记账。
	Experience                 int       `gorm:"column:experience;type:int;default:0"`
	NovelPoints                int       `gorm:"column:novel_points;type:int;default:0"`
	// Coins 站内币余额（发帖/阅读/写作/评分赚来，投币时花掉）。
	// 收支流水见 coin_ledger，投币记录见 coin_throws，与经验/积分互不相通。
	Coins                      int       `gorm:"column:coins;type:int;default:0"`
	ReceivedLikeNum            int       `gorm:"column:received_like_num;type:int;default:0"`
	ReceivedVoteNum            int       `gorm:"column:received_vote_num;type:int;default:0"`
	ISEmailNotificationBlocked bool      `gorm:"column:is_email_notification_blocked;type:bool"`
	// HideProfilePosts 为 true 时，其他用户查看个人主页不展示其历史发帖（本人仍可见）
	HideProfilePosts bool `gorm:"column:hide_profile_posts;type:tinyint(1);default:0"`
	// UserType market=默认；external=外院（Quest 可发帖）。
	UserType string `gorm:"column:user_type;type:varchar(16);default:market"`
}
