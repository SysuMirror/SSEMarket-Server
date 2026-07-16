package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"loginTest/api"
	"loginTest/common"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"loginTest/util"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// 定义响应用户结构体
type ChatRespUser struct {
	UserID             int    `json:"userID"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	AvatarURL          string `json:"avatarURL"`
	Identity           string `json:"identity"`
	Score              int    `json:"score"`
	UnRead             int    `json:"unRead"`
	LastMessageAt      string `json:"lastMessageAt"`
	LastMessagePreview string `json:"lastMessagePreview"`
	PeerMasked         bool   `json:"peerMasked"`
	DisplayName        string `json:"displayName"`
	DisplayAvatar      string `json:"displayAvatar"`
	ThreadAnonymous    bool   `json:"threadAnonymous"`
	ConversationKey    string `json:"conversationKey"`
}

// ws连接节点
type ChatNode struct {
	Conn      *websocket.Conn
	DataQueue chan []byte //发送消息的消息队列
}

// 创建用户id与ws连接映射map
var clientMap sync.Map

// 获得聊天记录

type ChatHistoryQuery struct {
	SenderUserID    int  `form:"senderUserID" binding:"required"`
	TargetUserID    int  `form:"targetUserID" binding:"required"`
	ThreadAnonymous bool `form:"threadAnonymous"`
}

func chatConversationKey(peerID int, threadAnonymous bool) string {
	if threadAnonymous {
		return fmt.Sprintf("%d_anon", peerID)
	}
	return fmt.Sprintf("%d_direct", peerID)
}

func GetChatHistory(c *gin.Context) {
	chatQuery := ChatHistoryQuery{}
	if err := c.ShouldBindQuery(&chatQuery); err != nil {
		response.Fail(c, gin.H{}, "无效的查询参数")
		return
	}
	ctxUser, ok := c.Get("user")
	if !ok {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	currentUser, _ := ctxUser.(*domain.User)
	if currentUser.ID != chatQuery.SenderUserID {
		response.Fail(c, gin.H{}, "没有权利查看他人聊天记录")
		return
	}

	redisCtx, cancel := context.WithTimeout(c.Request.Context(), 800*time.Millisecond)
	defer cancel()
	threadAnonymous := chatQuery.ThreadAnonymous
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, chatQuery.TargetUserID, threadAnonymous)
	if err := core.MyRedis.Set(redisCtx, chatViewKey, 1, 24*time.Hour).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	if err := core.MyRedis.SAdd(redisCtx, util.GenerateChatViewIndexKey(chatQuery.SenderUserID), chatViewKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	unreadKey := util.GenerateUnreadKey(chatQuery.SenderUserID, chatQuery.TargetUserID, threadAnonymous)
	if err := core.MyRedis.Del(redisCtx, unreadKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	if err := core.MyRedis.SRem(redisCtx, util.GenerateUnreadIndexKey(chatQuery.SenderUserID), unreadKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}

	chatList, err := service.Chats.GetHistory(c.Request.Context(), chatQuery.SenderUserID, chatQuery.TargetUserID, threadAnonymous)
	if err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	history := make([]chatHistoryResp, 0, len(chatList))
	for _, item := range chatList {
		history = append(history, chatHistoryResp{
			ChatMsgID:       item.ChatMsgID,
			TargetUserID:    item.TargetUserID,
			SenderUserID:    item.SenderUserID,
			Content:         item.Content,
			IsAnonymous:     item.IsAnonymous,
			ThreadAnonymous: item.ThreadAnonymous,
			IsRecalled:      item.IsRecalled,
			CreatedAt:       item.CreatedAt,
		})
	}
	response.Success(c, gin.H{"chatHistoryList": history}, "获得聊天记录成功")
}

type chatHistoryResp struct {
	ChatMsgID       int       `gorm:"column:chatMsgID" json:"chatMsgID"`
	TargetUserID    int       `gorm:"column:targetUserID" json:"targetUserID"`
	SenderUserID    int       `gorm:"column:senderUserID" json:"senderUserID"`
	Content         string    `gorm:"column:content" json:"content"`
	IsAnonymous     bool      `json:"isAnonymous"`
	ThreadAnonymous bool      `json:"threadAnonymous"`
	IsRecalled      bool      `json:"isRecalled"`
	CreatedAt       time.Time `gorm:"column:createdAt" json:"createdAt"`
}

type chatRecallReq struct {
	ChatMsgID int `json:"chatMsgID" binding:"required"`
}

type chatRecallPush struct {
	Type   string          `json:"type"`
	Recall chatHistoryResp `json:"recall"`
}

func pushJSONToUser(userID int, payload []byte) {
	val, ok := clientMap.Load(userID)
	if !ok {
		return
	}
	node, ok := val.(*ChatNode)
	if !ok {
		return
	}
	node.DataQueue <- payload
}

func broadcastChatRecall(item *domain.ChatHistoryItem) {
	if item == nil {
		return
	}
	resp := chatHistoryResp{
		ChatMsgID:       item.ChatMsgID,
		TargetUserID:    item.TargetUserID,
		SenderUserID:    item.SenderUserID,
		Content:         item.Content,
		IsAnonymous:     item.IsAnonymous,
		ThreadAnonymous: item.ThreadAnonymous,
		IsRecalled:      item.IsRecalled,
		CreatedAt:       item.CreatedAt,
	}
	payload, err := json.Marshal(chatRecallPush{Type: "recall", Recall: resp})
	if err != nil {
		return
	}
	pushJSONToUser(item.SenderUserID, payload)
	pushJSONToUser(item.TargetUserID, payload)
}

func RecallChatMessage(c *gin.Context) {
	req := chatRecallReq{}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChatMsgID <= 0 {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	ctxUser, ok := c.Get("user")
	if !ok {
		response.Fail(c, gin.H{}, "撤回失败，请重试")
		return
	}
	currentUser, _ := ctxUser.(*domain.User)
	recalled, err := service.Chats.RecallMessage(c.Request.Context(), currentUser.ID, req.ChatMsgID)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	broadcastChatRecall(recalled)
	response.Success(c, gin.H{"message": chatHistoryResp{
		ChatMsgID:       recalled.ChatMsgID,
		TargetUserID:    recalled.TargetUserID,
		SenderUserID:    recalled.SenderUserID,
		Content:         recalled.Content,
		IsAnonymous:     recalled.IsAnonymous,
		ThreadAnonymous: recalled.ThreadAnonymous,
		IsRecalled:      recalled.IsRecalled,
		CreatedAt:       recalled.CreatedAt,
	}}, "撤回成功")
}

func removeChatViewInRedis(userID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	indexKey := util.GenerateChatViewIndexKey(userID)
	keys, err := core.MyRedis.SMembers(ctx, indexKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if len(keys) > 0 {
		if _, err = core.MyRedis.Del(ctx, keys...).Result(); err != nil {
			return err
		}
	}
	return core.MyRedis.Del(ctx, indexKey).Err()
}

func ChatHandler(c *gin.Context) {
	var userID int
	userInterface, ok := c.Get("user")
	if !ok {
		tokenString := c.Query("token")
		_, claim, err := common.ParseToken(tokenString)
		if err != nil {
			response.Fail(c, gin.H{}, "请重新登录")
			return
		}
		userID = claim.UserID
	} else {
		user := userInterface.(*domain.User)
		userID = user.ID
	}

	chatFunc(c.Request, c.Writer, userID)
}

func chatFunc(request *http.Request, writer http.ResponseWriter, userID int) {
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}).Upgrade(writer, request, nil)
	if err != nil {
		http.Error(writer, "Failed to upgrade WebSocket connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	node := &ChatNode{
		conn,
		make(chan []byte, 200),
	}
	clientMap.Store(userID, node)

	go sendMsg2User(node)
	go recvMsgFromUser(node, userID)

	relevantUserJSON, err := buildRelevantUserJSON(request.Context(), userID)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
	}
	sendMsg(0, userID, relevantUserJSON)
}

func sendMsg2User(node *ChatNode) {
	for {
		select {
		case data := <-node.DataQueue:
			err := node.Conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				return
			}
		}
	}
}

func recvMsgFromUser(node *ChatNode, userID int) {
	defer func() {
		node.Conn.Close()
		clientMap.Delete(userID)
	}()

	readTimeout := 60 * time.Second

	for {
		node.Conn.SetReadDeadline(time.Now().Add(readTimeout))

		_, data, err := node.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				removeChatViewInRedis(userID)
				return
			}
			removeChatViewInRedis(userID)
			return
		}

		var envelope map[string]json.RawMessage
		if err = json.Unmarshal(data, &envelope); err != nil {
			errorMsg := []byte(`{"code": 0, "msg": "` + err.Error() + `"}`)
			node.Conn.WriteMessage(websocket.TextMessage, errorMsg)
			continue
		}

		if _, isHeartbeat := envelope["beat"]; isHeartbeat {
			node.Conn.SetReadDeadline(time.Now().Add(readTimeout))
			continue
		}

		err = dispatch(data, userID)
		if err != nil {
			errorMsg := []byte(`{"code": 0, "msg": "` + err.Error() + `"}`)
			node.Conn.WriteMessage(websocket.TextMessage, errorMsg)
		} else {
			successMsg := []byte(`{"code": 1, "msg": "发送成功"}`)
			node.Conn.WriteMessage(websocket.TextMessage, successMsg)
		}
	}
}

func dispatch(data []byte, userID int) error {
	type wsChatMsg struct {
		ChatMsgID       int    `json:"chatMsgID"`
		TargetUserID    int    `json:"targetUserID"`
		SenderUserID    int    `json:"senderUserID"`
		Content         string `json:"content"`
		IsAnonymous     bool   `json:"isAnonymous"`
		ThreadAnonymous bool   `json:"threadAnonymous"`
	}
	msg := wsChatMsg{}
	err := json.Unmarshal(data, &msg)
	if msg.ChatMsgID != 0 {
		return errors.New("非法操作！")
	}
	if msg.SenderUserID != userID {
		return errors.New("非法操作！")
	}
	if err != nil {
		return err
	}
	if !msg.ThreadAnonymous && msg.IsAnonymous {
		msg.ThreadAnonymous = true
	}
	if !msg.ThreadAnonymous {
		msg.IsAnonymous = false
	}

	saved, err := service.Chats.SaveMessage(context.Background(), msg.ChatMsgID, msg.SenderUserID, msg.TargetUserID, msg.Content, msg.IsAnonymous, msg.ThreadAnonymous)
	if err != nil {
		return err
	}
	chatResp := ChatMsgResp{
		ChatMsgID:       saved.ChatMsgID,
		TargetUserID:    saved.TargetUserID,
		SenderUserID:    saved.SenderUserID,
		Content:         saved.Content,
		IsAnonymous:     saved.IsAnonymous,
		ThreadAnonymous: saved.ThreadAnonymous,
		IsRecalled:      saved.IsRecalled,
		CreatedAt:       saved.CreatedAt,
	}

	data, err = json.Marshal(chatResp)
	if err != nil {
		return err
	}

	if msg.SenderUserID != msg.TargetUserID {
		sendMsg(saved.ChatMsgID, msg.TargetUserID, data)
		pushJSONToUser(msg.SenderUserID, data)
	}
	return nil
}

type ChatMsgResp struct {
	ChatMsgID       int       `json:"chatMsgID"`
	TargetUserID    int       `json:"targetUserID"`
	SenderUserID    int       `json:"senderUserID"`
	Content         string    `json:"content"`
	IsAnonymous     bool      `json:"isAnonymous"`
	ThreadAnonymous bool      `json:"threadAnonymous"`
	IsRecalled      bool      `json:"isRecalled"`
	Unread          int       `json:"unread"`
	CreatedAt       time.Time `json:"createdAt"`
}

func sendMsg(chatMsgID, userID int, msg []byte) {
	chatMsg := ChatMsgResp{}
	val, ok := clientMap.Load(userID)
	if err := json.Unmarshal(msg, &chatMsg); err != nil {
		return
	}
	if !ok {
		// chatMsgID == 0 表示系统推送（如相关联系人列表），并非真实聊天消息，
		// 离线时不应计入未读，否则会生成 senderUserID=0 的幽灵未读 key，
		// 导致前端小红点常驻无法清除。
		if chatMsgID == 0 {
			return
		}
		Chat_EmailAlerter(chatMsg.TargetUserID, "")
		redisCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		unreadKey := util.GenerateUnreadKey(userID, chatMsg.SenderUserID, chatMsg.ThreadAnonymous)
		core.MyRedis.Incr(redisCtx, unreadKey)
		core.MyRedis.SAdd(redisCtx, util.GenerateUnreadIndexKey(userID), unreadKey)
		return
	}
	node, ok := val.(*ChatNode)
	if ok {
		if chatMsgID != 0 {
			chatMsg.ChatMsgID = chatMsgID
			redisCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			err := core.MyRedis.Get(redisCtx, util.GenerateChatViewKey(chatMsg.TargetUserID, chatMsg.SenderUserID, chatMsg.ThreadAnonymous)).Err()
			if err == redis.Nil {
				unreadKey := util.GenerateUnreadKey(chatMsg.TargetUserID, chatMsg.SenderUserID, chatMsg.ThreadAnonymous)
				resUnread := core.MyRedis.Incr(redisCtx, unreadKey)
				core.MyRedis.SAdd(redisCtx, util.GenerateUnreadIndexKey(chatMsg.TargetUserID), unreadKey)
				unread, _ := resUnread.Result()
				chatMsg.Unread = int(unread)
				Chat_EmailAlerter(chatMsg.TargetUserID, "")
			}
			chatMsgByte, _ := json.Marshal(chatMsg)
			node.DataQueue <- chatMsgByte
			return
		}
		node.DataQueue <- msg
	}
}

func buildRelevantUserJSON(ctx context.Context, userID int) ([]byte, error) {
	peers, err := service.Chats.ListRelevantPeers(ctx, userID)
	if err != nil {
		return []byte{}, err
	}

	relevantRespUsers := make([]ChatRespUser, 0, len(peers))
	for _, peer := range peers {
		if peer.Peer == nil {
			continue
		}
		relevantUser := peer.Peer
		redisCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		unreadStr, err := core.MyRedis.Get(redisCtx, util.GenerateUnreadKey(userID, relevantUser.ID, peer.ThreadAnonymous)).Result()
		cancel()
		unreadNum, _ := strconv.Atoi(unreadStr)
		if err != nil && err != redis.Nil {
			return nil, err
		}
		preview := peer.LastMessagePreview
		if len([]rune(preview)) > 80 {
			preview = string([]rune(preview)[:80]) + "…"
		}
		displayName, displayAvatar := util.ChatPeerDisplay(relevantUser.Name, relevantUser.AvatarURL, peer.ThreadAnonymous, peer.PeerMasked)
		email := relevantUser.Email
		identity := relevantUser.Identity
		if peer.PeerMasked {
			email = ""
			identity = ""
		}
		relevantRespUsers = append(relevantRespUsers, ChatRespUser{
			UserID:             relevantUser.ID,
			Email:              email,
			Name:               displayName,
			AvatarURL:          displayAvatar,
			Identity:           identity,
			Score:              relevantUser.Score,
			UnRead:             unreadNum,
			LastMessageAt:      peer.LastMessageAt.Format(time.RFC3339),
			LastMessagePreview: preview,
			PeerMasked:         peer.PeerMasked,
			DisplayName:        displayName,
			DisplayAvatar:      displayAvatar,
			ThreadAnonymous:    peer.ThreadAnonymous,
			ConversationKey:    chatConversationKey(relevantUser.ID, peer.ThreadAnonymous),
		})
	}
	return json.Marshal(struct {
		RelevantUsers []ChatRespUser `json:"relevantUsers"`
	}{relevantRespUsers})
}

func GetRelevantUser(userID int) ([]byte, error) {
	return buildRelevantUserJSON(context.Background(), userID)
}

func LeaveChatView(c *gin.Context) {
	chatQuery := ChatHistoryQuery{}
	if err := c.ShouldBindQuery(&chatQuery); err != nil {
		response.Fail(c, gin.H{}, "无效的查询参数")
		return
	}
	ctxUser, ok := c.Get("user")
	if !ok {
		response.Fail(c, gin.H{}, "操作失败，请重试")
		return
	}
	currentUser, _ := ctxUser.(*domain.User)
	if currentUser.ID != chatQuery.SenderUserID {
		response.Fail(c, gin.H{}, "非法操作")
		return
	}

	redisCtx, cancel := context.WithTimeout(c.Request.Context(), 800*time.Millisecond)
	defer cancel()
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, chatQuery.TargetUserID, chatQuery.ThreadAnonymous)
	if err := core.MyRedis.Del(redisCtx, chatViewKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "操作异常")
		return
	}
	if err := core.MyRedis.SRem(redisCtx, util.GenerateChatViewIndexKey(chatQuery.SenderUserID), chatViewKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "操作异常")
		return
	}
	response.Success(c, gin.H{}, "操作成功")
}

func GetChatNotice(c *gin.Context) {
	userIDStr := c.Query("userID")
	userID, err := strconv.Atoi(userIDStr)
	userFromCtx, ok := c.Get("user")
	user := userFromCtx.(*domain.User)
	if err != nil || userID == 0 || !ok || user.ID != userID {
		response.Fail(c, gin.H{}, "操作异常")
		return
	}

	var totalUnread int

	redisCtx, cancel := context.WithTimeout(c.Request.Context(), 800*time.Millisecond)
	defer cancel()
	unreadKeys, err := core.MyRedis.SMembers(redisCtx, util.GenerateUnreadIndexKey(userID)).Result()
	if err != nil && err != redis.Nil {
		response.Fail(c, gin.H{}, "操作异常")
		return
	}
	if len(unreadKeys) > 0 {
		unreadVals, err := core.MyRedis.MGet(redisCtx, unreadKeys...).Result()
		if err != nil {
			response.Fail(c, gin.H{}, "获取未读消息失败")
			return
		}
		for i, v := range unreadVals {
			if v == nil {
				_ = core.MyRedis.SRem(redisCtx, util.GenerateUnreadIndexKey(userID), unreadKeys[i]).Err()
				continue
			}
			unreadStr, ok := v.(string)
			if !ok || unreadStr == "" {
				continue
			}
			unreadCount, convErr := strconv.Atoi(unreadStr)
			if convErr != nil {
				continue
			}
			totalUnread += unreadCount
		}
	}

	response.Success(c, gin.H{
		"noticeNum": totalUnread,
	}, "获取未读消息成功")
}

func Chat_EmailAlerter(receiverID int, msg string) {
	receiver, err := service.Chats.FindUserByID(context.Background(), receiverID)
	if err != nil || receiver == nil || receiver.ISEmailNotificationBlocked || receiver.Email == "" {
		return
	}
	emailAddr, body := receiver.Email, msg
	go func() {
		if err := api.SendEmail(emailAddr, 2, body); err != nil {
			fmt.Println("async chat notify email:", err)
		}
	}()
}
