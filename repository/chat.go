package repository

import (
	"context"
)

// ChatMessage 领域聊天消息。
type ChatMessage struct {
	ID           int
	SenderUserID int
	TargetUserID int
	Content      string
	SentAt       int64
}

// ChatRepository 聊天持久化接口（为 WS 改造铺垫）。
type ChatRepository interface {
	ListHistory(ctx context.Context, userID, peerID int, limit, offset int) ([]*ChatMessage, error)
	Create(ctx context.Context, msg *ChatMessage) error
	CountUnread(ctx context.Context, userID int) (int64, error)
}
