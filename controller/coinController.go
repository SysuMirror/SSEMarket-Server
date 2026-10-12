package controller

import (
	"errors"
	"net/http"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// ---------------------------------------------------------------------------
// 请求体
// ---------------------------------------------------------------------------

// CoinThrowMsg 投币入参：TargetType 只接受 post / novel，Coins 1–2。
type CoinThrowMsg struct {
	TargetType string `json:"targetType"`
	TargetID   int    `json:"targetID"`
	Coins      int    `json:"coins"`
}

// CoinLedgerListMsg 币流水分页入参。
type CoinLedgerListMsg struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// coinTargetType 校验投币目标类型：只允许帖子和小说，别的值一律拒绝，
// 免得有人拿 targetType 去投不存在的目标刷经验。
func coinTargetType(t string) string {
	switch t {
	case domain.CoinTargetPost, domain.CoinTargetNovel:
		return t
	default:
		return ""
	}
}

func coinError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.Response(c, http.StatusNotFound, 404, nil, "要投币的内容不存在")
	case errors.Is(err, domain.ErrCoinNotEnough):
		response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
	case errors.Is(err, domain.ErrCoinLimit):
		response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
	case errors.Is(err, domain.ErrCoinSelf):
		response.Response(c, http.StatusForbidden, 403, nil, err.Error())
	case errors.Is(err, domain.ErrBadRequest):
		response.Response(c, http.StatusBadRequest, 400, nil, "参数不符合要求")
	default:
		apperr.Write(c, err)
	}
}

// ---------------------------------------------------------------------------
// 投币
// ---------------------------------------------------------------------------

// CoinThrow 给帖子或小说投币。上限、扣币、加经验都在一个事务里完成，
// 前端只负责把「投 1 个还是 2 个」传上来。
func CoinThrow(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg CoinThrowMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	targetType := coinTargetType(msg.TargetType)
	if targetType == "" || msg.TargetID <= 0 || msg.Coins <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的投币参数")
		return
	}
	result, err := service.Coins.Throw(c.Request.Context(), user.ID, targetType, msg.TargetID, msg.Coins)
	if err != nil {
		coinError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"coins":      result.Coins,
			"total":      result.Total,
			"exp":        result.Exp,
			"coinsLeft":  result.CoinsLeft,
			"targetType": result.TargetType,
			"targetID":   result.TargetID,
		},
		"msg": "已投币",
	})
}

// ---------------------------------------------------------------------------
// 我的币
// ---------------------------------------------------------------------------

// CoinSummary 币总览：余额、今天各项任务的进度、累计投币与经验加成。
func CoinSummary(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	summary, err := service.Coins.Summary(c.Request.Context(), user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": summary, "msg": "ok"})
}

// CoinLedger 我的币流水（分页）。
func CoinLedger(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg CoinLedgerListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	items, total, err := service.Coins.Ledger(c.Request.Context(), user.ID, msg.Limit, msg.Offset)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	list := make([]gin.H, 0, len(items))
	for _, l := range items {
		list = append(list, gin.H{
			"ledgerID":   l.ID,
			"targetType": l.TargetType,
			"targetID":   l.TargetID,
			"event":      l.EventType,
			"label":      l.Label,
			"amount":     l.Amount,
			"createdAt":  l.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"items": list, "total": total}, "msg": "ok"})
}

// CoinVisit 每日进入网页 +1 币：前端进站点时上报一次，服务端按日期幂等，
// 重复刷新不会重复发币。
func CoinVisit(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	granted, err := service.Coins.GrantDailyVisit(c.Request.Context(), user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	summary, err := service.Coins.Summary(c.Request.Context(), user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{"granted": granted, "coins": summary.Coins},
		"msg":  "已发放每日进入网页奖励",
	})
}
