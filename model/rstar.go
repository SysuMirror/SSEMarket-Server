package model

// Cclike [...]
type Rstar struct {
	RstarID int  `gorm:"primary_key;column:rstarID"`
	PostID  int  `gorm:"index:rstarpost;column:postID;type:int;not null"`
	Post    Post `gorm:"association_foreignkey:postID;foreignkey:postID"`
	UserID  int  `gorm:"index:rstaruser;column:userID;type:int;not null"`
	User    User `gorm:"association_foreignkey:userID;foreignkey:userID"`
	StarNum int  `gorm:"column:star_num;type:int"`
}
