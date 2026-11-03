package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"loginTest/domain"
)

// 小秋聊天不在集市 MySQL 里，而在 sse-agent 的会话库里。
// 集市这边不直连 agent 的 Postgres，只借用户自己的 JWT 调 agent 只读接口，
// 拿最近会话里的用户提问来补画像；token 只放在 request context 里，不落库、不打日志。

type chatTokenCtxKey struct{}

// WithChatToken 把当前请求的用户 JWT 放进 context，供推荐画像读取小秋聊天。
func WithChatToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, chatTokenCtxKey{}, token)
}

func chatTokenFromContext(ctx context.Context) string {
	v, _ := ctx.Value(chatTokenCtxKey{}).(string)
	return v
}

type recChatCacheEntry struct {
	messages   []domain.RecChatMessage
	at         time.Time
	vector     []float32
	vectorAt   time.Time
	vectorBusy bool
}

type agentConversationsData struct {
	Code int `json:"code"`
	Data struct {
		Conversations []struct {
			SessionID string    `json:"session_id"`
			Title     string    `json:"title"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"conversations"`
	} `json:"data"`
}

type agentMessagesData struct {
	Code int `json:"code"`
	Data struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	} `json:"data"`
}

// chatMessagesFor 返回用户最近与小秋的提问，带进程内缓存，避免每次首页都打 agent。
func (s *recommendationService) chatMessagesFor(ctx context.Context, userID int) []domain.RecChatMessage {
	if !s.cfg.chatEnabled {
		return nil
	}
	s.chatMu.Lock()
	if cached, ok := s.chatCache[userID]; ok && time.Since(cached.at) < s.cfg.chatProfileTTL {
		s.chatMu.Unlock()
		return cached.messages
	}
	s.chatMu.Unlock()

	// 兼容以后把用户↔小秋的消息直接落集市 chat_msgs 的情况。
	if msgs, err := s.repo.ListUserChatMessages(ctx, userID, s.cfg.chatBotUID, 40); err == nil && len(msgs) > 0 {
		s.cacheChatMessages(userID, msgs)
		logRecf("chat profile loaded user=%d messages=%d source=db", userID, len(msgs))
		return msgs
	}

	token := chatTokenFromContext(ctx)
	if token == "" || s.cfg.chatBase == "" {
		return nil
	}
	msgs, err := s.fetchAgentChatMessages(ctx, token)
	if err != nil {
		logRecf("chat profile fetch skipped user=%d: %v", userID, err)
		return nil
	}
	s.cacheChatMessages(userID, msgs)
	if len(msgs) > 0 {
		logRecf("chat profile loaded user=%d messages=%d source=agent", userID, len(msgs))
	}
	return msgs
}

func (s *recommendationService) cacheChatMessages(userID int, msgs []domain.RecChatMessage) {
	s.chatMu.Lock()
	if len(s.chatCache) >= 4096 {
		s.chatCache = map[int]recChatCacheEntry{}
	}
	prev := s.chatCache[userID]
	s.chatCache[userID] = recChatCacheEntry{
		messages:   msgs,
		at:         time.Now(),
		vector:     prev.vector,
		vectorAt:   prev.vectorAt,
		vectorBusy: prev.vectorBusy,
	}
	s.chatMu.Unlock()
}

// chatVectorFor 返回聊天文本向量。外部嵌入可能被图片任务挤慢，
// 因此不在首页请求里同步等待：先返回上次缓存，未命中时后台嵌入，
// 完成后清掉该用户画像，让下一次刷新自动用上聊天兴趣。
func (s *recommendationService) chatVectorFor(userID int, msgs []domain.RecChatMessage) []float32 {
	if len(msgs) == 0 || s.client == nil || !s.client.Enabled() {
		return nil
	}
	text := buildChatProfileText(msgs)
	if text == "" {
		return nil
	}

	now := time.Now()
	s.chatMu.Lock()
	entry := s.chatCache[userID]
	if len(entry.vector) > 0 && !entry.vectorAt.IsZero() && now.Sub(entry.vectorAt) < s.cfg.chatProfileTTL {
		vec := entry.vector
		s.chatMu.Unlock()
		return vec
	}
	if entry.vectorBusy {
		s.chatMu.Unlock()
		return nil
	}
	entry.vectorBusy = true
	s.chatCache[userID] = entry
	s.chatMu.Unlock()

	go func() {
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 && !sleepCtx(context.Background(), time.Duration(attempt)*20*time.Second) {
				break
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			vecs, err := s.client.EmbedTexts(ctx, []string{text})
			cancel()
			if err == nil && len(vecs) == 1 && len(vecs[0]) > 0 {
				s.chatMu.Lock()
				cur := s.chatCache[userID]
				cur.vectorBusy = false
				cur.vector = vecs[0]
				cur.vectorAt = time.Now()
				s.chatCache[userID] = cur
				s.chatMu.Unlock()
				// 画像里可能刚缓存了「还没算上聊天」的版本，丢掉让它按新向量重建。
				s.profileMu.Lock()
				delete(s.profiles, userID)
				s.profileMu.Unlock()
				logRecf("chat profile embedding ready user=%d dims=%d", userID, len(vecs[0]))
				return
			}
			lastErr = err
		}

		s.chatMu.Lock()
		cur := s.chatCache[userID]
		cur.vectorBusy = false
		s.chatCache[userID] = cur
		s.chatMu.Unlock()
		if lastErr != nil {
			logRecf("chat profile embedding failed user=%d: %v", userID, lastErr)
		}
	}()
	return nil
}

func (s *recommendationService) fetchAgentChatMessages(ctx context.Context, token string) ([]domain.RecChatMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 4 * time.Second}
	base := strings.TrimRight(s.cfg.chatBase, "/")

	var conversations agentConversationsData
	if err := s.agentGET(ctx, client, base+"/api/v1/agent/conversations?limit=6", token, &conversations); err != nil {
		return nil, err
	}

	out := make([]domain.RecChatMessage, 0, 12)
	for i, conv := range conversations.Data.Conversations {
		if i >= 3 {
			break
		}
		if conv.SessionID == "" {
			continue
		}
		var detail agentMessagesData
		var userMsgs []string
		path := base + "/api/v1/agent/conversations/" + url.PathEscape(conv.SessionID) + "/messages"
		if err := s.agentGET(ctx, client, path, token, &detail); err == nil {
			for _, m := range detail.Data.Messages {
				if m.Role != "user" {
					continue
				}
				var text string
				if err := json.Unmarshal(m.Content, &text); err != nil {
					continue
				}
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				userMsgs = append(userMsgs, text)
			}
			if len(userMsgs) > 4 {
				userMsgs = userMsgs[len(userMsgs)-4:]
			}
			for _, text := range userMsgs {
				out = append(out, domain.RecChatMessage{Content: text, At: conv.UpdatedAt})
			}
		}
		if len(userMsgs) == 0 && conv.Title != "" {
			out = append(out, domain.RecChatMessage{Content: conv.Title, At: conv.UpdatedAt})
		}
		if len(out) >= 12 {
			break
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out, nil
}

func (s *recommendationService) agentGET(ctx context.Context, client *http.Client, endpoint, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
		return fmt.Errorf("agent status=%d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return err
	}
	switch v := out.(type) {
	case *agentConversationsData:
		if v.Code != 0 {
			return fmt.Errorf("agent code=%d", v.Code)
		}
	case *agentMessagesData:
		if v.Code != 0 {
			return fmt.Errorf("agent code=%d", v.Code)
		}
	}
	return nil
}
