package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
)

// ---------------------------------------------------------------------------
// 推荐流
//
// GET    /api/auth/recommendation/settings        我的推荐设置
// POST   /api/auth/recommendation/settings        设置 auto / on / off
// POST   /api/auth/recommendation/read            点进详情标记已读（可选停留时长）
// GET    /api/auth/admin/recommendation/stats     运行状态
// GET    /api/auth/admin/recommendation/gray      灰度规则列表
// POST   /api/auth/admin/recommendation/gray      新增/更新灰度规则
// DELETE /api/auth/admin/recommendation/gray/:id  删除灰度规则
// POST   /api/auth/admin/recommendation/rebuild   清空向量并重建
// ---------------------------------------------------------------------------

func RecommendationSettings(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	settings, err := service.Recs.Settings(c.Request.Context(), user.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "读取推荐设置失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"settings": settings}, "")
}

type recommendationSettingRequest struct {
	Mode string `json:"mode"`
}

func UpdateRecommendationSettings(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	var req recommendationSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "参数错误")
		return
	}
	if err := service.Recs.SetMode(c.Request.Context(), user.ID, req.Mode); err != nil {
		apperr.WriteMsg(c, err, "保存推荐设置失败")
		return
	}
	settings, err := service.Recs.Settings(c.Request.Context(), user.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "读取推荐设置失败")
		return
	}
	response.Success(c, gin.H{"settings": settings}, "已保存")
}

type recommendationReadRequest struct {
	PostID  int `json:"postID"`
	DwellMS int `json:"dwellMS"`
}

// RecordRecommendationRead 点进详情标记已读；dwellMS 可选，后续计时器上报停留时长。
func RecordRecommendationRead(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	var req recommendationReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "参数错误")
		return
	}
	if err := service.Recs.RecordReadEvent(c.Request.Context(), user.ID, req.PostID, req.DwellMS); err != nil {
		apperr.WriteMsg(c, err, "上报失败")
		return
	}
	response.Success(c, nil, "")
}

func AdminRecommendationStats(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	stats, err := service.Recs.Stats(c.Request.Context())
	if err != nil {
		apperr.WriteMsg(c, err, "读取推荐状态失败")
		return
	}
	response.Success(c, gin.H{"stats": stats}, "")
}

func AdminListRecommendationGrayRules(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	rules, err := service.Recs.ListGrayRules(c.Request.Context())
	if err != nil {
		apperr.WriteMsg(c, err, "读取灰度规则失败")
		return
	}
	response.Success(c, gin.H{"rules": rules}, "")
}

type recommendationGrayRequest struct {
	ID      int    `json:"id"`
	UIDMin  int    `json:"uidMin"`
	UIDMax  int    `json:"uidMax"`
	Enabled bool   `json:"enabled"`
	Note    string `json:"note"`
}

func AdminUpsertRecommendationGrayRule(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	var req recommendationGrayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "参数错误")
		return
	}
	if req.UIDMin <= 0 || req.UIDMax < req.UIDMin {
		response.Fail(c, nil, "UID 区间不合法")
		return
	}
	rule, err := service.Recs.UpsertGrayRule(c.Request.Context(), domain.RecommendGrayRuleInput{
		ID:      req.ID,
		UIDMin:  req.UIDMin,
		UIDMax:  req.UIDMax,
		Enabled: req.Enabled,
		Note:    req.Note,
	})
	if err != nil {
		apperr.WriteMsg(c, err, "保存灰度规则失败")
		return
	}
	response.Success(c, gin.H{"rule": rule}, "已保存")
}

func AdminDeleteRecommendationGrayRule(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Fail(c, nil, "参数错误")
		return
	}
	if err := service.Recs.DeleteGrayRule(c.Request.Context(), id); err != nil {
		apperr.WriteMsg(c, err, "删除灰度规则失败")
		return
	}
	response.Success(c, nil, "已删除")
}

func AdminRebuildRecommendation(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	if service.Recs == nil {
		response.Fail(c, nil, "推荐服务未启用")
		return
	}
	if err := service.Recs.Rebuild(c.Request.Context()); err != nil {
		apperr.WriteMsg(c, err, "重建失败")
		return
	}
	response.Success(c, nil, "已清空向量，后台会重新构建")
}
