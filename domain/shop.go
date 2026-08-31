package domain

import "time"

// Product 商城商品。
type Product struct {
	ID          int
	UserID      int
	Name        string
	Description string
	PostTime    time.Time
	Photos      string
	IsSold      bool
	Price       int
	IsAnonymous bool
	IsDelete    bool
}

// ProductFilter 商品浏览筛选。
type ProductFilter struct {
	ViewerUserID int
	SearchInfo   string
	SearchSort   string
	Limit        int
	Offset       int
}

// ProductCreateInput 发布商品入参。
type ProductCreateInput struct {
	UserID       int
	Title        string
	Content      string
	Photos       []string
	Price        int
	IsAnonymous  bool
	SyncFeedPost bool
}
