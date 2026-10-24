package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 勋章系统
//
// GET  /api/auth/badges          我的全部勋章 + 展示偏好（auth）
// POST /api/auth/badges/prefs    保存对外挂载 / 主页展示范围（auth）
// GET  /api/badges/user/:userID  他人主页可见的勋章（noauth）
// GET  /api/badges/worn?ids=1,2  批量取一批用户对外挂载的勋章（noauth）
//
// 勋章图片全部来自图床绝对地址；目录/发放在 DB 侧维护，接口只读。
// ---------------------------------------------------------------------------

func badgeItemData(b domain.AwardedBadge) gin.H {
	data := gin.H{
		"badgeID":     b.ID,
		"code":        b.Code,
		"name":        b.Name,
		"description": b.Description,
		"imageURL":    b.ImageURL,
		"category":    b.Category,
	}
	if !b.AwardedAt.IsZero() {
		data["awardedAt"] = b.AwardedAt.Format(time.RFC3339)
	}
	return data
}

func badgeItemsData(badges []domain.AwardedBadge) []gin.H {
	items := make([]gin.H, 0, len(badges))
	for _, b := range badges {
		items = append(items, badgeItemData(b))
	}
	return items
}

// MyBadges 当前用户全部已获得勋章与展示偏好。
func MyBadges(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	view, err := service.Badges.GetMyBadges(c.Request.Context(), user.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "读取勋章失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{
		"badges":          badgeItemsData(view.Badges),
		"wornBadgeId":     view.WornBadgeID,
		"profileBadgeIds": view.ProfileBadgeIDs,
	}, "")
}

type badgePrefsReq struct {
	WornBadgeID int `json:"wornBadgeId"`
	// nil/null = 恢复默认全部展示；[] = 明确不展示；[1,2] = 只展示这两枚。
	ProfileBadgeIDs *[]int `json:"profileBadgeIds"`
}

// BadgeSavePrefs 保存「对外挂载一枚」与「主页展示哪些」。
func BadgeSavePrefs(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req badgePrefsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数格式错误")
		return
	}
	var profile []int
	if req.ProfileBadgeIDs != nil {
		profile = *req.ProfileBadgeIDs
	}
	view, err := service.Badges.SavePrefs(c.Request.Context(), user.ID, domain.BadgePrefsInput{
		WornBadgeID:     req.WornBadgeID,
		ProfileBadgeIDs: profile,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadgeNotOwned):
			response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
		case errors.Is(err, domain.ErrBadgePrefsTooBig):
			response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		default:
			apperr.WriteMsg(c, err, "保存勋章展示设置失败")
		}
		return
	}
	response.Success(c, gin.H{
		"badges":          badgeItemsData(view.Badges),
		"wornBadgeId":     view.WornBadgeID,
		"profileBadgeIds": view.ProfileBadgeIDs,
	}, "勋章展示设置已保存")
}

// UserBadges 他人个人主页可见的勋章（已按对方的主页展示偏好过滤）。
func UserBadges(c *gin.Context) {
	userID, _ := strconv.Atoi(strings.TrimSpace(c.Param("userID")))
	if userID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "userID 无效")
		return
	}
	view, err := service.Badges.GetPublicBadges(c.Request.Context(), userID)
	if err != nil {
		apperr.WriteMsg(c, err, "读取勋章失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{
		"userID":          view.UserID,
		"badges":          badgeItemsData(view.Badges),
		"wornBadgeId":     view.WornBadgeID,
		"profileBadgeIds": view.ProfileBadgeIDs,
	}, "")
}

// WornBadges 批量查询一批用户对外挂载的勋章。
// 帖子/评论列表里作者很多，前端合并成一次请求，避免 N+1。
func WornBadges(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("ids"))
	if raw == "" {
		response.Success(c, gin.H{"badges": []gin.H{}}, "")
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > domain.BadgeMaxWornQuery {
		response.Response(c, http.StatusBadRequest, 400, nil, "一次最多查询 100 个用户")
		return
	}
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	result, err := service.Badges.WornBadges(c.Request.Context(), ids)
	if err != nil {
		apperr.WriteMsg(c, err, "读取勋章失败")
		return
	}
	items := make([]gin.H, 0, len(result))
	for _, b := range result {
		items = append(items, gin.H{
			"userID":   b.UserID,
			"badgeID":  b.BadgeID,
			"name":     b.Name,
			"imageURL": b.ImageURL,
		})
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"badges": items}, "")
}
