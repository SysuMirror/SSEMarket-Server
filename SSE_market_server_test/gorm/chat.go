package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type chatRepository struct {
	db *gorm.DB
}

func NewChatRepository(db *gorm.DB) repository.ChatRepository {
	return &chatRepository{db: db}
}

func (r *chatRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *chatRepository) ListHistory(ctx context.Context, userID, peerID int) ([]*domain.ChatHistoryItem, error) {
	var rows []model.ChatMsg
	err := r.session(ctx).Model(&model.ChatMsg{}).
		Where("(targetUserID = ? AND senderUserID = ?) OR (targetUserID = ? AND senderUserID = ?)",
			userID, peerID, peerID, userID).
		Order("createdAt").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.ChatHistoryItem, 0, len(rows))
	for i := range rows {
		out = append(out, chatMsgToDomain(&rows[i]))
	}
	return out, nil
}

func (r *chatRepository) Create(ctx context.Context, in domain.SaveChatMessageInput) (*domain.ChatHistoryItem, error) {
	msg := model.ChatMsg{
		SenderUserID: in.SenderUserID,
		TargetUserID: in.TargetUserID,
		Content:      in.Content,
		CreatedAt:    time.Now(),
	}
	if err := r.session(ctx).Create(&msg).Error; err != nil {
		return nil, err
	}
	return chatMsgToDomain(&msg), nil
}

func (r *chatRepository) ListMessagesForUser(ctx context.Context, userID int) ([]repository.ChatMsgWithUsers, error) {
	var chatMsgs []model.ChatMsg
	err := r.session(ctx).
		Where("senderUserID = ? OR targetUserID = ?", userID, userID).
		Preload("TargetUser").
		Preload("SenderUser").
		Find(&chatMsgs).Error
	if err != nil {
		return nil, err
	}
	out := make([]repository.ChatMsgWithUsers, 0, len(chatMsgs))
	for i := range chatMsgs {
		m := chatMsgs[i]
		item := domain.ChatHistoryItem{
			ChatMsgID:    m.ChatMsgID,
			TargetUserID: m.TargetUserID,
			SenderUserID: m.SenderUserID,
			Content:      m.Content,
			CreatedAt:    m.CreatedAt,
		}
		out = append(out, repository.ChatMsgWithUsers{
			Msg:        item,
			SenderUser: userToDomain(&m.SenderUser),
			TargetUser: userToDomain(&m.TargetUser),
		})
	}
	return out, nil
}

func (r *chatRepository) FindUserByID(ctx context.Context, userID int) (*domain.User, error) {
	var u model.User
	err := r.session(ctx).Where("userID = ?", userID).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return userToDomain(&u), nil
}
