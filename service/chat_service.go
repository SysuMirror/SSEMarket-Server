package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"loginTest/domain"
	"loginTest/repository"
)

const chatRecallWindow = 24 * time.Hour

type chatService struct {
	chats repository.ChatRepository
}

func NewChatService(chats repository.ChatRepository) ChatService {
	return &chatService{chats: chats}
}

func (s *chatService) GetHistory(ctx context.Context, senderUserID, targetUserID int, threadAnonymous bool) ([]*domain.ChatHistoryItem, error) {
	return s.chats.ListHistory(ctx, senderUserID, targetUserID, threadAnonymous)
}

func (s *chatService) SaveMessage(ctx context.Context, chatMsgID, senderUserID, targetUserID int, content string, isAnonymous, threadAnonymous bool) (*domain.ChatHistoryItem, error) {
	if chatMsgID != 0 {
		return nil, errors.New("非法操作！")
	}
	if senderUserID == 0 {
		return nil, errors.New("非法操作！")
	}
	return s.chats.Create(ctx, domain.SaveChatMessageInput{
		SenderUserID:    senderUserID,
		TargetUserID:    targetUserID,
		Content:         content,
		IsAnonymous:     isAnonymous,
		ThreadAnonymous: threadAnonymous,
	})
}

func (s *chatService) RecallMessage(ctx context.Context, userID, chatMsgID int) (*domain.ChatHistoryItem, error) {
	msg, err := s.chats.FindByID(ctx, chatMsgID)
	if err != nil {
		return nil, err
	}
	if msg.SenderUserID != userID {
		return nil, errors.New("只能撤回自己发送的消息")
	}
	if msg.IsRecalled {
		return msg, nil
	}
	if time.Since(msg.CreatedAt) > chatRecallWindow {
		return nil, errors.New("超过24小时的消息无法撤回")
	}
	return s.chats.Recall(ctx, chatMsgID)
}

func (s *chatService) ListRelevantPeers(ctx context.Context, userID int) ([]*domain.ChatPeerSummary, error) {
	msgs, err := s.chats.ListMessagesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	type laneKey struct {
		peerID          int
		threadAnonymous bool
	}
	type laneState struct {
		user                      *domain.User
		lastAt                    domain.ChatHistoryItem
		lastPreview               string
		peerSentAnonymousToViewer bool
	}

	lanes := map[laneKey]*laneState{}

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
		if peerID == 0 || peerID == userID || peer == nil {
			continue
		}

		key := laneKey{peerID: peerID, threadAnonymous: msg.ThreadAnonymous}
		st, ok := lanes[key]
		if !ok {
			st = &laneState{user: peer}
			lanes[key] = st
		}

		if msg.ThreadAnonymous && msg.IsAnonymous && msg.SenderUserID == peerID && msg.TargetUserID == userID {
			st.peerSentAnonymousToViewer = true
		}

		if !ok || msg.CreatedAt.After(st.lastAt.CreatedAt) {
			preview := msg.Content
			if msg.IsRecalled {
				preview = "[消息已撤回]"
			} else if msg.IsAnonymous {
				preview = "[匿名] " + preview
			}
			st.lastAt = msg
			st.lastPreview = preview
		}
	}

	keys := make([]laneKey, 0, len(lanes))
	for k := range lanes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return lanes[keys[i]].lastAt.CreatedAt.After(lanes[keys[j]].lastAt.CreatedAt)
	})

	out := make([]*domain.ChatPeerSummary, 0, len(keys))
	for _, key := range keys {
		st := lanes[key]
		peerMasked := key.threadAnonymous && st.peerSentAnonymousToViewer
		out = append(out, &domain.ChatPeerSummary{
			Peer:                      st.user,
			LastMessageAt:             st.lastAt.CreatedAt,
			LastMessagePreview:        st.lastPreview,
			ThreadAnonymous:           key.threadAnonymous,
			PeerMasked:                peerMasked,
			PeerSentAnonymousToViewer: st.peerSentAnonymousToViewer,
		})
	}
	return out, nil
}

func (s *chatService) FindUserByID(ctx context.Context, userID int) (*domain.User, error) {
	return s.chats.FindUserByID(ctx, userID)
}
