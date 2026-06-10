package service

import (
	"context"
	"errors"
	"sort"

	"loginTest/domain"
	"loginTest/repository"
)

type chatService struct {
	chats repository.ChatRepository
}

func NewChatService(chats repository.ChatRepository) ChatService {
	return &chatService{chats: chats}
}

func (s *chatService) GetHistory(ctx context.Context, senderUserID, targetUserID int) ([]*domain.ChatHistoryItem, error) {
	return s.chats.ListHistory(ctx, senderUserID, targetUserID)
}

func (s *chatService) SaveMessage(ctx context.Context, chatMsgID, senderUserID, targetUserID int, content string) (*domain.ChatHistoryItem, error) {
	if chatMsgID != 0 {
		return nil, errors.New("非法操作！")
	}
	if senderUserID == 0 {
		return nil, errors.New("非法操作！")
	}
	return s.chats.Create(ctx, domain.SaveChatMessageInput{
		SenderUserID: senderUserID,
		TargetUserID: targetUserID,
		Content:      content,
	})
}

func (s *chatService) ListRelevantPeers(ctx context.Context, userID int) ([]*domain.ChatPeerSummary, error) {
	msgs, err := s.chats.ListMessagesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	type peerState struct {
		user        *domain.User
		lastAt      domain.ChatHistoryItem
		lastPreview string
	}
	peerStates := map[int]*peerState{}
	for _, row := range msgs {
		msg := row.Msg
		if msg.SenderUserID == msg.TargetUserID {
			continue
		}
		var peer *domain.User
		var peerID int
		if msg.SenderUserID == userID {
			peer = row.TargetUser
			peerID = msg.TargetUserID
		} else {
			peer = row.SenderUser
			peerID = msg.SenderUserID
		}
		if peerID == 0 || peerID == userID {
			continue
		}
		if st, ok := peerStates[peerID]; !ok || msg.CreatedAt.After(st.lastAt.CreatedAt) {
			peerStates[peerID] = &peerState{
				user:        peer,
				lastAt:      msg,
				lastPreview: msg.Content,
			}
		}
	}

	peerIDs := make([]int, 0, len(peerStates))
	for id := range peerStates {
		peerIDs = append(peerIDs, id)
	}
	sort.Slice(peerIDs, func(i, j int) bool {
		return peerStates[peerIDs[i]].lastAt.CreatedAt.After(peerStates[peerIDs[j]].lastAt.CreatedAt)
	})

	out := make([]*domain.ChatPeerSummary, 0, len(peerIDs))
	for _, peerID := range peerIDs {
		st := peerStates[peerID]
		out = append(out, &domain.ChatPeerSummary{
			Peer:               st.user,
			LastMessageAt:      st.lastAt.CreatedAt,
			LastMessagePreview: st.lastPreview,
		})
	}
	return out, nil
}

func (s *chatService) FindUserByID(ctx context.Context, userID int) (*domain.User, error) {
	return s.chats.FindUserByID(ctx, userID)
}
