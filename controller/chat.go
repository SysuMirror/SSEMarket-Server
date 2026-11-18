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

// anonMapTTL 控制伪 ID → 真实 ID 映射在 Redis 中的存活时间。
// 30 天足够覆盖长会话；过期后 buildRelevantUserJSON 会重新写入。
const anonMapTTL = 30 * 24 * time.Hour

// anonMapKey 构造 Redis 中存储伪 ID → 真实 ID 的键。
func anonMapKey(viewerID, pseudoID int) string {
	return fmt.Sprintf("chat:anonmap:v:%d:p:%d", viewerID, pseudoID)
}

// ensureAnonMapping 计算 (viewerID, realPeerID) 的伪 ID 并在 Redis 中保存双向映射。
// 返回伪 ID。即使 Redis 写入失败也返回伪 ID（降级：resolveAnonPeerID 会走 DB 回退）。
func ensureAnonMapping(ctx context.Context, viewerID, realPeerID int) int {
	pseudoID := util.AnonPeerID(viewerID, realPeerID)
	_ = core.MyRedis.Set(ctx, anonMapKey(viewerID, pseudoID), realPeerID, anonMapTTL).Err()
	return pseudoID
}

// resolveAnonPeerID 尝试将伪 ID 反解为真实 ID。
// 1) 先查 Redis；2) 未命中时遍历 viewer 的匿名会话重新计算并回填映射。
// 第二个返回值为 false 表示该 ID 不是伪 ID（发送方视角，直接用真实 ID）。
func resolveAnonPeerID(ctx context.Context, viewerID, maybePseudoID int) (int, bool) {
	if !util.IsAnonPeerID(maybePseudoID) {
		return maybePseudoID, false
	}
	if realID, err := core.MyRedis.Get(ctx, anonMapKey(viewerID, maybePseudoID)).Int(); err == nil && realID > 0 {
		return realID, true
	}
	// Redis 未命中：遍历 viewer 的匿名会话，重新计算伪 ID 查找匹配。
	peers, err := service.Chats.ListRelevantPeers(ctx, viewerID)
	if err != nil {
		return maybePseudoID, false
	}
	for _, p := range peers {
		if !p.ThreadAnonymous || p.Peer == nil {
			continue
		}
		realPeerID := p.Peer.ID
		pseudo := util.AnonPeerID(viewerID, realPeerID)
		if pseudo == maybePseudoID {
			_ = core.MyRedis.Set(ctx, anonMapKey(viewerID, pseudo), realPeerID, anonMapTTL).Err()
			return realPeerID, true
		}
	}
	return maybePseudoID, false
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
	// 匿名会话里前端持有的 targetUserID 是伪 ID，需反解为真实 ID 供 DB/Redis 使用。
	// masked=true 表示 viewer 是接收方（被隐藏身份的一方），响应中需把 peer 真实 ID 替换回伪 ID。
	realPeerID, masked := resolveAnonPeerID(redisCtx, chatQuery.SenderUserID, chatQuery.TargetUserID)
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, realPeerID, threadAnonymous)
	if err := core.MyRedis.Set(redisCtx, chatViewKey, 1, 24*time.Hour).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	if err := core.MyRedis.SAdd(redisCtx, util.GenerateChatViewIndexKey(chatQuery.SenderUserID), chatViewKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	unreadKey := util.GenerateUnreadKey(chatQuery.SenderUserID, realPeerID, threadAnonymous)
	if err := core.MyRedis.Del(redisCtx, unreadKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	if err := core.MyRedis.SRem(redisCtx, util.GenerateUnreadIndexKey(chatQuery.SenderUserID), unreadKey).Err(); err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}

	chatList, err := service.Chats.GetHistory(c.Request.Context(), chatQuery.SenderUserID, realPeerID, threadAnonymous)
	if err != nil {
		response.Fail(c, gin.H{}, "获得聊天记录失败，请重试")
		return
	}
	displayPeerID := chatQuery.TargetUserID
	if masked {
		displayPeerID = ensureAnonMapping(redisCtx, chatQuery.SenderUserID, realPeerID)
	}
	history := make([]chatHistoryResp, 0, len(chatList))
	for _, item := range chatList {
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
		if masked {
			if resp.SenderUserID == realPeerID {
				resp.SenderUserID = displayPeerID
			}
			if resp.TargetUserID == realPeerID {
				resp.TargetUserID = displayPeerID
			}
		}
		history = append(history, resp)
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
	base := chatHistoryResp{
		ChatMsgID:       item.ChatMsgID,
		TargetUserID:    item.TargetUserID,
		SenderUserID:    item.SenderUserID,
		Content:         item.Content,
		IsAnonymous:     item.IsAnonymous,
		ThreadAnonymous: item.ThreadAnonymous,
		IsRecalled:      item.IsRecalled,
		CreatedAt:       item.CreatedAt,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 发送方视角的 payload
	senderResp := base
	// 匿名线程里非匿名消息 = 接收方回复的撤回，发送方（接收方）被隐藏身份，
	// 需把 targetUserID 替换为伪 ID，否则发送方 WS 帧会暴露匿名发起方真实 ID。
	if item.ThreadAnonymous && !item.IsAnonymous {
		senderResp.TargetUserID = ensureAnonMapping(ctx, item.SenderUserID, item.TargetUserID)
	}
	if senderPayload, err := json.Marshal(chatRecallPush{Type: "recall", Recall: senderResp}); err == nil {
		pushJSONToUser(item.SenderUserID, senderPayload)
	}

	// 接收方视角的 payload
	recipientResp := base
	// 匿名消息 = 匿名发起方撤回，接收方被隐藏身份，
	// 需把 senderUserID 替换为伪 ID，否则接收方 WS 帧会暴露匿名发送方真实 ID。
	if item.ThreadAnonymous && item.IsAnonymous {
		recipientResp.SenderUserID = ensureAnonMapping(ctx, item.TargetUserID, item.SenderUserID)
	}
	if recipientPayload, err := json.Marshal(chatRecallPush{Type: "recall", Recall: recipientResp}); err == nil {
		pushJSONToUser(item.TargetUserID, recipientPayload)
	}
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
	// 撤回只能由发送方发起。匿名线程里非匿名消息 = 接收方（调用者）回复的撤回，
	// 调用方是被隐藏身份的一方，HTTP 响应里的 targetUserID 需替换为伪 ID。
	resp := chatHistoryResp{
		ChatMsgID:       recalled.ChatMsgID,
		TargetUserID:    recalled.TargetUserID,
		SenderUserID:    recalled.SenderUserID,
		Content:         recalled.Content,
		IsAnonymous:     recalled.IsAnonymous,
		ThreadAnonymous: recalled.ThreadAnonymous,
		IsRecalled:      recalled.IsRecalled,
		CreatedAt:       recalled.CreatedAt,
	}
	if recalled.ThreadAnonymous && !recalled.IsAnonymous {
		redisCtx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()
		resp.TargetUserID = ensureAnonMapping(redisCtx, recalled.SenderUserID, recalled.TargetUserID)
	}
	response.Success(c, gin.H{"message": resp}, "撤回成功")
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

	ctx := context.Background()
	// 匿名会话里接收方持有的 targetUserID 是伪 ID，需反解后才能写入 DB。
	realTargetID, senderMasked := resolveAnonPeerID(ctx, msg.SenderUserID, msg.TargetUserID)

	saved, err := service.Chats.SaveMessage(ctx, msg.ChatMsgID, msg.SenderUserID, realTargetID, msg.Content, msg.IsAnonymous, msg.ThreadAnonymous)
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

	if msg.SenderUserID != realTargetID {
		// 推送给接收方：sendMsg 内部在计算未读后会把匿名发送方的 ID 替换为伪 ID。
		sendMsg(saved.ChatMsgID, realTargetID, data)
		// 回推给发送方：若发送方是被隐藏身份的一方（接收方回复），需把 targetID 换回伪 ID。
		if senderMasked {
			echoResp := chatResp
			echoResp.TargetUserID = msg.TargetUserID
			if echoData, mErr := json.Marshal(echoResp); mErr == nil {
				pushJSONToUser(msg.SenderUserID, echoData)
			} else {
				pushJSONToUser(msg.SenderUserID, data)
			}
		} else {
			pushJSONToUser(msg.SenderUserID, data)
		}
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
			// 未读 key 用真实 ID 计算完毕后，把匿名发送方 ID 替换为伪 ID 再推送给接收方，
			// 防止接收方通过 WS 帧里的 senderUserID 反查发送方真实身份。
			if chatMsg.ThreadAnonymous && chatMsg.IsAnonymous && chatMsg.SenderUserID != userID {
				chatMsg.SenderUserID = ensureAnonMapping(redisCtx, userID, chatMsg.SenderUserID)
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
		displayUserID := relevantUser.ID
		convKey := chatConversationKey(relevantUser.ID, peer.ThreadAnonymous)
		if peer.PeerMasked {
			email = ""
			identity = ""
			// 用伪 ID 替换真实 ID，防止接收方通过 JSON 响应反查匿名发送方身份
			displayUserID = ensureAnonMapping(ctx, userID, relevantUser.ID)
			convKey = chatConversationKey(displayUserID, peer.ThreadAnonymous)
		}
		relevantRespUsers = append(relevantRespUsers, ChatRespUser{
			UserID:             displayUserID,
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
			ConversationKey:    convKey,
		})
	}
	return json.Marshal(struct {
		RelevantUsers []ChatRespUser `json:"relevantUsers"`
	}{relevantRespUsers})
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
	realPeerID, _ := resolveAnonPeerID(redisCtx, chatQuery.SenderUserID, chatQuery.TargetUserID)
	chatViewKey := util.GenerateChatViewKey(chatQuery.SenderUserID, realPeerID, chatQuery.ThreadAnonymous)
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
