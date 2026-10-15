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
// AsStaffUserID>0 表示「以职位身份回复」：必须是本人在该线程被 @ 到的职位小号，
// 服务端校验通过后评论对外归属职位小号（见 service/staff_service.go）。
type PcommentCreateInput struct {
	UserID        int
	PostID        int
	Content       string
	AsStaffUserID int
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
// AsStaffUserID 含义同 PcommentCreateInput（以职位身份回复楼中楼）。
type CcommentCreateInput struct {
	UserID          int
	PostID          int
	PcommentID      int
	Content         string
	UserTargetName  string
	ReplyCcommentID int
	AsStaffUserID   int
	TargetUserID    int
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
