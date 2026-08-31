package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type questListReq struct {
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Keyword string `json:"keyword"`
	Sort    string `json:"sort"`
	MyPosts bool   `json:"myPosts"`
	Saved   bool   `json:"saved"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

type questCreateReq struct {
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	PostKind   string   `json:"postKind"`
	QuotePrice *float64 `json:"quotePrice"`
}

type questDetailReq struct {
	PostID int `json:"postID"`
}

type questStatusReq struct {
	PostID int    `json:"postID"`
	Status string `json:"status"`
}

func QuestList(c *gin.Context) {
	var req questListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	userID := GetTokenUserID(c)
	filter := domain.QuestListFilter{
		Kind:         req.Kind,
		Status:       req.Status,
		Keyword:      req.Keyword,
		Sort:         req.Sort,
		ViewerUserID: userID,
		Limit:        req.Limit,
		Offset:       req.Offset,
		SavedOnly:    req.Saved,
	}
	if req.MyPosts && userID > 0 {
		filter.AuthorUserID = userID
	}
	if req.Saved && userID <= 0 {
		response.Fail(c, gin.H{}, "请先登录")
		return
	}
	items, total, err := service.Quest.List(c.Request.Context(), filter)
	if err != nil {
		apperr.WriteMsg(c, err, "获取列表失败")
		return
	}
	response.Success(c, gin.H{"items": items, "total": total}, "ok")
}

func QuestDetail(c *gin.Context) {
	var req questDetailReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Fail(c, gin.H{}, "无效的帖子 ID")
		return
	}
	item, err := service.Quest.DetailFull(c.Request.Context(), req.PostID, GetTokenUserID(c))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, gin.H{}, "帖子不存在")
			return
		}
		apperr.WriteMsg(c, err, "获取详情失败")
		return
	}
	response.Success(c, gin.H{"post": item}, "ok")
}

func QuestCreate(c *gin.Context) {
	var req questCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	userID := GetTokenUserID(c)
	item, err := service.Quest.Create(c.Request.Context(), domain.QuestPostCreateInput{
		UserID:     userID,
		Title:      req.Title,
		Content:    req.Content,
		PostKind:   req.PostKind,
		QuotePrice: req.QuotePrice,
	})
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusForbidden, 403, gin.H{}, "仅外院用户可在 Quest 发帖")
			return
		}
		if errors.Is(err, domain.ErrBadRequest) {
			response.Fail(c, gin.H{}, "标题、内容或类型不合法（求助帖不可填金额）")
			return
		}
		apperr.WriteMsg(c, err, "发帖失败")
		return
	}
	response.Success(c, gin.H{"post": item}, "发帖成功")
}

func QuestUpdateStatus(c *gin.Context) {
	var req questStatusReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	userID := GetTokenUserID(c)
	if err := service.Quest.UpdateStatus(c.Request.Context(), req.PostID, userID, req.Status); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, gin.H{}, "帖子不存在或无权操作")
			return
		}
		if errors.Is(err, domain.ErrBadRequest) {
			response.Fail(c, gin.H{}, "无效的状态")
			return
		}
		apperr.WriteMsg(c, err, "更新失败")
		return
	}
	response.Success(c, gin.H{}, "状态已更新")
}

func QuestMe(c *gin.Context) {
	userID := GetTokenUserID(c)
	info, err := service.Quest.Me(c.Request.Context(), userID)
	if err != nil {
		apperr.WriteMsg(c, err, "获取用户信息失败")
		return
	}
	response.Success(c, gin.H{"me": info}, "ok")
}

func QuestChatPeers(c *gin.Context) {
	userID := GetTokenUserID(c)
	peers, err := service.Chats.ListRelevantPeers(c.Request.Context(), userID)
	if err != nil {
		apperr.WriteMsg(c, err, "获取会话列表失败")
		return
	}
	type peerItem struct {
		UserID             int    `json:"userID"`
		Name               string `json:"name"`
		AvatarURL          string `json:"avatarURL"`
		LastMessagePreview string `json:"lastMessagePreview"`
		LastMessageAt      string `json:"lastMessageAt"`
	}
	out := make([]peerItem, 0, len(peers))
	for _, p := range peers {
		if p.Peer == nil {
			continue
		}
		out = append(out, peerItem{
			UserID:             p.Peer.ID,
			Name:               p.Peer.Name,
			AvatarURL:          p.Peer.AvatarURL,
			LastMessagePreview: p.LastMessagePreview,
			LastMessageAt:      p.LastMessageAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	response.Success(c, gin.H{"peers": out}, "ok")
}
