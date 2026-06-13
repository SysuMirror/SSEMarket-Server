package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

type LotteryPrizeMsg struct {
	Name        string `json:"name"`
	Quantity    int    `json:"quantity"`
	WishKeyword string `json:"wishKeyword"`
}

type LotteryCreateMsg struct {
	UserTelephone string            `json:"userTelephone"`
	Title         string            `json:"title"`
	Content       string            `json:"content"`
	Partition     string            `json:"partition"`
	Photos        string            `json:"photos"`
	TagList       string            `json:"tagList"`
	IsPrivate     bool              `json:"isPrivate"`
	Mode          string            `json:"mode"`
	DrawAt        string            `json:"drawAt"`
	Prizes        []LotteryPrizeMsg `json:"prizes"`
}

func LotteryCreate(c *gin.Context) {
	var req LotteryCreateMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}
	if user.Phone != req.UserTelephone {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得发帖")
		return
	}
	drawAt, err := time.ParseInLocation("2006-01-02 15:04:05", req.DrawAt, time.Local)
	if err != nil {
		drawAt, err = time.Parse(time.RFC3339, req.DrawAt)
		if err != nil {
			response.Response(c, http.StatusBadRequest, 400, nil, "开奖时间格式错误")
			return
		}
	}
	prizes := make([]domain.LotteryPrizeInput, 0, len(req.Prizes))
	for _, p := range req.Prizes {
		prizes = append(prizes, domain.LotteryPrizeInput{
			Name:        p.Name,
			Quantity:    p.Quantity,
			WishKeyword: p.WishKeyword,
		})
	}
	tags := strings.Split(req.TagList, "|")
	postID, err := service.Lotteries.Create(c.Request.Context(), domain.LotteryCreateInput{
		UserID:    user.ID,
		Title:     req.Title,
		Content:   req.Content,
		Partition: req.Partition,
		Photos:    req.Photos,
		Tags:      tags,
		IsPrivate: req.IsPrivate,
		Mode:      req.Mode,
		DrawAt:    drawAt,
		Prizes:    prizes,
	})
	if err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, "抽奖帖参数无效")
			return
		}
		apperr.WriteMsg(c, err, "创建抽奖帖失败")
		return
	}
	response.Success(c, gin.H{"postID": postID}, "抽奖帖创建成功")
}

type LotteryDetailMsg struct {
	PostID        int    `json:"postID"`
	UserTelephone string `json:"userTelephone"`
}

func LotteryDetail(c *gin.Context) {
	var req LotteryDetailMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	viewerID := 0
	if user, ok := viewerFromAuth(c); ok && user.Phone == req.UserTelephone {
		viewerID = user.ID
	}
	detail, err := service.Lotteries.GetDetail(c.Request.Context(), req.PostID, viewerID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Success(c, gin.H{"isLottery": false}, "ok")
			return
		}
		apperr.WriteMsg(c, err, "获取抽奖信息失败")
		return
	}
	response.Success(c, lotteryDetailToJSON(detail), "ok")
}

func lotteryDetailToJSON(d *domain.LotteryPost) gin.H {
	prizes := make([]gin.H, 0, len(d.Prizes))
	for _, p := range d.Prizes {
		prizes = append(prizes, gin.H{
			"prizeID":           p.PrizeID,
			"name":              p.Name,
			"quantity":          p.Quantity,
			"wishKeyword":       p.WishKeyword,
			"participantCount":  p.ParticipantCount,
			"participated":      p.Participated,
		})
	}
	winners := make([]gin.H, 0, len(d.Winners))
	for _, w := range d.Winners {
		winners = append(winners, gin.H{
			"userID":    w.UserID,
			"userName":  w.UserName,
			"prizeID":   w.PrizeID,
			"prizeName": w.PrizeName,
		})
	}
	drawnAt := ""
	if !d.DrawnAt.IsZero() {
		drawnAt = d.DrawnAt.Format("2006-01-02 15:04:05")
	}
	return gin.H{
		"lotteryID":        d.LotteryID,
		"postID":           d.PostID,
		"mode":             d.Mode,
		"status":           d.Status,
		"drawAt":           d.DrawAt.Format("2006-01-02 15:04:05"),
		"drawnAt":          drawnAt,
		"participantCount": d.ParticipantCount,
		"participated":     d.Participated,
		"prizes":           prizes,
		"winners":          winners,
	}
}

type LotteryParticipateMsg struct {
	PostID        int    `json:"postID"`
	UserTelephone string `json:"userTelephone"`
}

func LotteryParticipate(c *gin.Context) {
	var req LotteryParticipateMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}
	err := service.Lotteries.ParticipateButton(c.Request.Context(), user.ID, req.PostID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			response.Response(c, http.StatusOK, 400, nil, "您已参与过该抽奖")
			return
		}
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, "当前无法参与抽奖")
			return
		}
		apperr.WriteMsg(c, err, "参与失败")
		return
	}
	response.Success(c, nil, "参与成功")
}

type LotteryCheckMsg struct {
	PostID        int    `json:"postID"`
	UserTelephone string `json:"userTelephone"`
}

func LotteryCheckParticipated(c *gin.Context) {
	var req LotteryCheckMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}
	participated, err := service.Lotteries.HasParticipated(c.Request.Context(), user.ID, req.PostID)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	response.Success(c, gin.H{"participated": participated}, "ok")
}
