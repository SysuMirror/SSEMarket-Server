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
	// CustomTitle 满级(Lv8, score>=5000)用户自定义称号;空=未自定义,对外显示用户名
	CustomTitle                string    `gorm:"column:custom_title;type:varchar(32)"`
	ReceivedLikeNum            int       `gorm:"column:received_like_num;type:int;default:0"`
	ReceivedVoteNum            int       `gorm:"column:received_vote_num;type:int;default:0"`
	ISEmailNotificationBlocked bool      `gorm:"column:is_email_notification_blocked;type:bool"`
	// HideProfilePosts 为 true 时，其他用户查看个人主页不展示其历史发帖（本人仍可见）
	HideProfilePosts bool `gorm:"column:hide_profile_posts;type:tinyint(1);default:0"`
	// UserType market=默认；external=外院（Quest 可发帖）。
	UserType string `gorm:"column:user_type;type:varchar(16);default:market"`
}
