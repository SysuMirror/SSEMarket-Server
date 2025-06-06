package model

import (
	"time"
)

type Product struct {
	ProductID   int       `gorm:"primary_key;column:productID"`
	UserID      int       `gorm:"index:postuser;column:userID;type:int"`
	User        User      `gorm:"association_foreignkey:userID;foreignkey:userID"`
	Name        string    `gorm:"column:product_name;type:varchar(20)"`
	Description string    `gorm:"column:description;type:varchar(500)"`
	PostTime    time.Time `gorm:"column:post_time;type:datetime"`
	Photos      string   `gorm:"column:photos;type:varchar(1000)"`
	ISSold      bool      `gorm:"column:is_sold;type:bool"`
	Price       int       `gorm:"column:price;type:int"`
	ISAnonymous bool      `gorm:"column:is_anonymous;type:bool"`
	ISDelete    bool      `gorm:"column:is_delete;type:bool"`
}
