package domain

import "time"

// Pcomment 一级评论。
type Pcomment struct {
	ID        int
	PostID    int
	UserID    int
	Content   string
	LikeNum   int
	DenyNum   int
	CreatedAt time.Time
}

// Ccomment 二级评论。
type Ccomment struct {
	ID             int
	PcommentID     int
	UserID         int
	Content        string
	LikeNum        int
	DenyNum        int
	UserTargetName string
	CreatedAt      time.Time
}

// CcommentWithMeta 带作者与互动状态的二级评论。
type CcommentWithMeta struct {
	Ccomment *Ccomment
	Author   *User
	IsLiked  bool
	IsDenied bool
}

// PcommentWithMeta 带作者、子评论与互动状态的一级评论。
type PcommentWithMeta struct {
	Pcomment    *Pcomment
	Author      *User
	Subcomments []*CcommentWithMeta
	IsLiked     bool
	IsDenied    bool
}

// PcommentCreateInput 发表一级评论入参。
type PcommentCreateInput struct {
	UserID  int
	PostID  int
	Content string
}

// PcommentCreateResult 发表一级评论结果。
type PcommentCreateResult struct {
	Pcomment                *Pcomment
	Author                  *User
	Subcomments             []*CcommentWithMeta
	NotifyReceiver          *User
	PostWatchEmailReceivers []*User
}

// CcommentCreateInput 发表二级评论入参。
type CcommentCreateInput struct {
	UserID          int
	PostID          int
	PcommentID      int
	Content         string
	UserTargetName  string
	ReplyCcommentID int
}

// CcommentNotifyInfo 二级评论通知（供 controller 发邮件）。
type CcommentNotifyInfo struct {
	Receiver *User
}

// CcommentCreateResult 发表二级评论结果。
type CcommentCreateResult struct {
	Notifies                []*CcommentNotifyInfo
	PostWatchEmailReceivers []*User
}
