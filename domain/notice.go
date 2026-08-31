package domain

import "time"

// NoticeItem 通知列表项。
type NoticeItem struct {
	NoticeID     int
	ReceiverName string
	SenderName   string
	SenderAvatar string
	SenderID     int
	Type         string
	Content      string
	Read         bool
	PostID       int
	Target       int
	PcommentID   int
	Time         time.Time
}

// NoticePage 分页通知结果。
type NoticePage struct {
	NoticeList     []NoticeItem
	LastID         int
	TotalNum       int
	UnreadTotalNum int
}

// NoticeCounts 通知数量统计。
type NoticeCounts struct {
	TotalNum       int
	UnreadTotalNum int
	ReadTotalNum   int
}
