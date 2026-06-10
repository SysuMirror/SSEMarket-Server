package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"loginTest/api"
	"loginTest/common"
	"loginTest/core"
	"loginTest/model"
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
}

// 定义心跳结构体
type HeartBeat struct {
	UserID int `json:"userID"`
	Beat   int `json:"beat"`
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
	SenderUserID int `form:"senderUserID" binding:"required"` // 绑定 query 参数 senderUserID
	TargetUserID int `form:"targetUserID" binding:"required"` // 绑定 query 参数 targetUserID
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
	currentUser, _ := ctxUser.(model.User)
	if currentUser.UserID != chatQuery.SenderUserID {
		response.Fail(c, gin.H{}, "没有权利查看他人聊天记录")
		return
	}
	_ = removeChatViewInRedis(chatQuery.SenderUserID)

	redisCtx, cancel := context.WithTimeout(c.Request.Context(), 800*time.Millisecond)
	defer cancel()
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, chatQuery.TargetUserID)
	if err := core.MyRedis.Set(redisCtx, chatViewKey, 1, 24*time.Hour).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
	}
	_ = core.MyRedis.SAdd(redisCtx, util.GenerateChatViewIndexKey(chatQuery.SenderUserID), chatViewKey).Err()
	unreadKey := util.GenerateUnreadKey(chatQuery.SenderUserID, chatQuery.TargetUserID)
	if err := core.MyRedis.Del(redisCtx, unreadKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
	}
	_ = core.MyRedis.SRem(redisCtx, util.GenerateUnreadIndexKey(chatQuery.SenderUserID), unreadKey).Err()

	chatList, err := service.Chats.GetHistory(c.Request.Context(), chatQuery.SenderUserID, chatQuery.TargetUserID)
	if err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	history := make([]chatHistoryResp, 0, len(chatList))
	for _, item := range chatList {
		history = append(history, chatHistoryResp{
			ChatMsgID:    item.ChatMsgID,
			TargetUserID: item.TargetUserID,
			SenderUserID: item.SenderUserID,
			Content:      item.Content,
			CreatedAt:    item.CreatedAt,
		})
	}
	response.Success(c, gin.H{"chatHistoryList": history}, "获得聊天记录成功")
}

type chatHistoryResp struct {
	ChatMsgID    int       `gorm:"column:chatMsgID" json:"chatMsgID"`
	TargetUserID int       `gorm:"column:targetUserID" json:"targetUserID"`
	SenderUserID int       `gorm:"column:senderUserID" json:"senderUserID"`
	Content      string    `gorm:"column:content" json:"content"`
	CreatedAt    time.Time `gorm:"column:createdAt" json:"createdAt"`
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
		}
		userID = claim.UserID
	} else {
		user := userInterface.(model.User)
		userID = user.UserID
	}

	go removeChatViewInRedis(userID)

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

		heartB := HeartBeat{}
		if err = json.Unmarshal(data, &heartB); err != nil {
			errorMsg := []byte(`{"code": 0, "msg": "` + err.Error() + `"}`)
			node.Conn.WriteMessage(websocket.TextMessage, errorMsg)
			continue
		}

		if heartB.UserID != 0 {
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
	msg := model.ChatMsg{}
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

	saved, err := service.Chats.SaveMessage(context.Background(), msg.ChatMsgID, msg.SenderUserID, msg.TargetUserID, msg.Content)
	if err != nil {
		return err
	}
	chatResp := ChatMsgResp{
		ChatMsgID:    saved.ChatMsgID,
		TargetUserID: saved.TargetUserID,
		SenderUserID: saved.SenderUserID,
		Content:      saved.Content,
		CreatedAt:    saved.CreatedAt,
	}

	data, err = json.Marshal(chatResp)
	if err != nil {
		return err
	}

	if msg.SenderUserID != msg.TargetUserID {
		sendMsg(saved.ChatMsgID, msg.TargetUserID, data)
	}
	return nil
}

type ChatMsgResp struct {
	ChatMsgID    int       `json:"chatMsgID"`
	TargetUserID int       `json:"targetUserID"`
	SenderUserID int       `json:"senderUserID"`
	Content      string    `json:"content"`
	Unread       int       `json:"unread"`
	CreatedAt    time.Time `json:"createdAt"`
}

func sendMsg(chatMsgID, userID int, msg []byte) {
	chatMsg := ChatMsgResp{}
	val, ok := clientMap.Load(userID)
	if err := json.Unmarshal(msg, &chatMsg); err != nil {
		return
	}
	if !ok {
		Chat_EmailAlerter(chatMsg.TargetUserID, "")
		redisCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		unreadKey := util.GenerateUnreadKey(userID, chatMsg.SenderUserID)
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
			err := core.MyRedis.Get(redisCtx, util.GenerateChatViewKey(chatMsg.TargetUserID, chatMsg.SenderUserID)).Err()
			if err == redis.Nil {
				unreadKey := util.GenerateUnreadKey(chatMsg.TargetUserID, chatMsg.SenderUserID)
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
		unreadStr, err := core.MyRedis.Get(redisCtx, util.GenerateUnreadKey(userID, relevantUser.ID)).Result()
		cancel()
		unreadNum, _ := strconv.Atoi(unreadStr)
		if err != nil && err != redis.Nil {
			return nil, err
		}
		preview := peer.LastMessagePreview
		if len([]rune(preview)) > 80 {
			preview = string([]rune(preview)[:80]) + "…"
		}
		relevantRespUsers = append(relevantRespUsers, ChatRespUser{
			UserID:             relevantUser.ID,
			Email:              relevantUser.Email,
			Name:               relevantUser.Name,
			AvatarURL:          relevantUser.AvatarURL,
			Identity:           relevantUser.Identity,
			Score:              relevantUser.Score,
			UnRead:             unreadNum,
			LastMessageAt:      peer.LastMessageAt.Format(time.RFC3339),
			LastMessagePreview: preview,
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
	if err := c.ShouldBind(&chatQuery); err != nil {
		response.Fail(c, gin.H{}, "无效的查询参数")
		return
	}
	ctxUser, ok := c.Get("user")
	if !ok {
		response.Fail(c, gin.H{}, "操作失败，请重试")
		return
	}
	currentUser, _ := ctxUser.(model.User)
	if currentUser.UserID != chatQuery.SenderUserID {
		response.Fail(c, gin.H{}, "非法操作")
		return
	}

	redisCtx, cancel := context.WithTimeout(c.Request.Context(), 800*time.Millisecond)
	defer cancel()
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, chatQuery.TargetUserID)
	err := core.MyRedis.Del(redisCtx, chatViewKey).Err()
	_ = core.MyRedis.SRem(redisCtx, util.GenerateChatViewIndexKey(chatQuery.SenderUserID), chatViewKey).Err()
	if err != nil {
		response.Fail(c, gin.H{}, "操作异常")
	} else {
		response.Success(c, gin.H{}, "操作成功")
	}
}

func GetChatNotice(c *gin.Context) {
	userIDStr := c.Query("userID")
	userID, err := strconv.Atoi(userIDStr)
	userFromCtx, ok := c.Get("user")
	user := userFromCtx.(model.User)
	if err != nil || userID == 0 || !ok || user.UserID != userID {
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
