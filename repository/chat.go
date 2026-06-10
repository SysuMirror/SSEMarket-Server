package repository

import (
	"context"

	"loginTest/domain"
)

// ChatRepository 聊天持久化接口。
type ChatRepository interface {
	ListHistory(ctx context.Context, userID, peerID int) ([]*domain.ChatHistoryItem, error)
	Create(ctx context.Context, in domain.SaveChatMessageInput) (*domain.ChatHistoryItem, error)
	ListMessagesForUser(ctx context.Context, userID int) ([]ChatMsgWithUsers, error)
	FindUserByID(ctx context.Context, userID int) (*domain.User, error)
}

// ChatMsgWithUsers 聊天消息及关联用户。
type ChatMsgWithUsers struct {
	Msg        domain.ChatHistoryItem
	SenderUser *domain.User
	TargetUser *domain.User
}
