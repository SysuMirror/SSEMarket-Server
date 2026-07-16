package domain

import "time"

// ChatHistoryItem 单条聊天记录。
type ChatHistoryItem struct {
	ChatMsgID       int
	TargetUserID    int
	SenderUserID    int
	Content         string
	IsAnonymous     bool
	ThreadAnonymous bool
	IsRecalled      bool
	CreatedAt       time.Time
}

// SaveChatMessageInput WS/HTTP 发送聊天消息入参。
type SaveChatMessageInput struct {
	SenderUserID    int
	TargetUserID    int
	Content         string
	IsAnonymous     bool
	ThreadAnonymous bool
}

// ChatPeerSummary 聊天列表中的对端用户摘要（未读数由 controller 从 Redis 填充）。
type ChatPeerSummary struct {
	Peer                      *User
	LastMessageAt             time.Time
	LastMessagePreview        string
	ThreadAnonymous           bool
	PeerMasked                bool
	PeerSentAnonymousToViewer bool
}
