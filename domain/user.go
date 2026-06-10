package domain

import "time"

// User 领域用户实体。
type User struct {
	ID                         int
	Phone                      string
	Email                      string
	Name                       string
	Intro                      string
	AvatarURL                  string
	Identity                   string
	Score                      int
	BanEnd                     time.Time
	PunishNum                  int
	HideProfilePosts           bool
	ISEmailNotificationBlocked bool
}
