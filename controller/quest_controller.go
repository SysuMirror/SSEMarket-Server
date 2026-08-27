package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strings"
	"time"

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
	Title           string                  `json:"title"`
	Content         string                  `json:"content"`
	PostKind        string                  `json:"postKind"`
	PriceType       string                  `json:"priceType"`
	QuotePrice      *float64                `json:"quotePrice"`
	QuotePriceMin   *float64                `json:"quotePriceMin"`
	QuotePriceMax   *float64                `json:"quotePriceMax"`
	Deadline        string                  `json:"deadline"`
	Attachments     []domain.QuestAttachment `json:"attachments"`
}

type questDetailReq struct {
	PostID int `json:"postID"`
}

type questOrderReq struct {
	PostID     int    `json:"postID"`
	SubmitNote string `json:"submitNote"`
}

type questStatusReq struct {
	PostID int    `json:"postID"`
	Status string `json:"status"`
}

func parseQuestDeadline(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
	}
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02", s, time.Local)
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
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
	deadline, err := parseQuestDeadline(req.Deadline)
	if err != nil {
		response.Fail(c, gin.H{}, "截止时间格式无效")
		return
	}
	item, err := service.Quest.Create(c.Request.Context(), domain.QuestPostCreateInput{
		UserID:        userID,
		Title:         req.Title,
		Content:       req.Content,
		PostKind:      req.PostKind,
		PriceType:     req.PriceType,
		QuotePrice:    req.QuotePrice,
		QuotePriceMin: req.QuotePriceMin,
		QuotePriceMax: req.QuotePriceMax,
		Deadline:      deadline,
		Attachments:   req.Attachments,
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

func QuestLevelRules(c *gin.Context) {
	info, err := service.Quest.LevelRules(c.Request.Context(), GetTokenUserID(c))
	if err != nil {
		apperr.WriteMsg(c, err, "获取等级规则失败")
		return
	}
	response.Success(c, gin.H{"level": info}, "ok")
}

func QuestOrderAccept(c *gin.Context) {
	var req questOrderReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	order, err := service.Quest.AcceptOrder(c.Request.Context(), req.PostID, GetTokenUserID(c))
	if err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Fail(c, gin.H{}, "当前帖子不可接单")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusForbidden, 403, gin.H{}, "无权操作")
			return
		}
		apperr.WriteMsg(c, err, "接单失败")
		return
	}
	response.Success(c, gin.H{"order": order}, "接单成功")
}

func QuestOrderSubmitComplete(c *gin.Context) {
	var req questOrderReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Quest.SubmitComplete(c.Request.Context(), req.PostID, GetTokenUserID(c), req.SubmitNote); err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Fail(c, gin.H{}, "当前状态不可提交完成")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusForbidden, 403, gin.H{}, "仅接单者可提交完成")
			return
		}
		apperr.WriteMsg(c, err, "提交失败")
		return
	}
	response.Success(c, gin.H{}, "已提交，等待发布者确认")
}

func QuestOrderConfirmComplete(c *gin.Context) {
	var req questOrderReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Quest.ConfirmComplete(c.Request.Context(), req.PostID, GetTokenUserID(c)); err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Fail(c, gin.H{}, "当前状态不可确认完成")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusForbidden, 403, gin.H{}, "仅发布者可确认完成")
			return
		}
		apperr.WriteMsg(c, err, "确认失败")
		return
	}
	response.Success(c, gin.H{}, "任务已完结")
}
