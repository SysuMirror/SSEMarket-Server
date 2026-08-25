package controller

import (
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
	"loginTest/util"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminOAuthAppReq struct {
	AppID       string  `json:"app_id"`
	AppName     string  `json:"app_name"`
	AppIcon     *string `json:"app_icon"`
	RedirectURI string  `json:"redirect_uri"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
	RegenSecret bool    `json:"regenerate_secret"`
}

func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "********"
	}
	return secret[:4] + "********" + secret[len(secret)-4:]
}

func AdminListOAuthApps(c *gin.Context) {
	db := common.GetDB()
	var apps []model.OAuth2App
	if err := db.Order("updated_at desc").Find(&apps).Error; err != nil {
		response.Fail(c, nil, "读取 OAuth 应用失败")
		return
	}
	out := make([]gin.H, 0, len(apps))
	for _, app := range apps {
		out = append(out, gin.H{
			"app_id":       app.AppID,
			"app_name":     app.AppName,
			"app_icon":     app.AppIcon,
			"redirect_uri": app.RedirectURI,
			"description":  app.Description,
			"is_active":    app.IsActive,
			"app_secret":   maskSecret(app.AppSecret),
			"created_at":   app.CreatedAt,
			"updated_at":   app.UpdatedAt,
		})
	}
	response.Success(c, gin.H{"apps": out}, "ok")
}

func AdminCreateOAuthApp(c *gin.Context) {
	var req adminOAuthAppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	req.AppName = strings.TrimSpace(req.AppName)
	req.RedirectURI = strings.TrimSpace(req.RedirectURI)
	if req.AppName == "" || req.RedirectURI == "" {
		response.Fail(c, nil, "应用名和回调地址不能为空")
		return
	}
	appID := strings.TrimSpace(req.AppID)
	if appID == "" {
		appID = "sse-" + util.GenerateRandomString(16)
	}
	secret := util.GenerateRandomString(32)
	now := time.Now()
	appIcon := ""
	if req.AppIcon != nil {
		appIcon = strings.TrimSpace(*req.AppIcon)
	}
	desc := ""
	if req.Description != nil {
		desc = strings.TrimSpace(*req.Description)
	}
	app := model.OAuth2App{
		AppID:       appID,
		AppName:     req.AppName,
		AppSecret:   secret,
		AppIcon:     appIcon,
		RedirectURI: req.RedirectURI,
		Description: desc,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := common.GetDB().Create(&app).Error; err != nil {
		response.Fail(c, nil, "写入失败，app_id 可能已存在")
		return
	}
	response.Success(c, gin.H{
		"app_id":       app.AppID,
		"app_name":     app.AppName,
		"app_secret":   secret,
		"redirect_uri": app.RedirectURI,
		"description":  app.Description,
		"is_active":    app.IsActive,
	}, "已写入 oauth2_apps，请立刻保存 app_secret（只显示一次）")
}

func AdminUpdateOAuthApp(c *gin.Context) {
	appID := c.Param("appId")
	var req adminOAuthAppReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	db := common.GetDB()
	var app model.OAuth2App
	if err := db.Where("app_id = ?", appID).First(&app).Error; err != nil {
		response.Fail(c, nil, "应用不存在")
		return
	}
	updates := map[string]interface{}{"updated_at": time.Now()}
	if strings.TrimSpace(req.AppName) != "" {
		updates["app_name"] = strings.TrimSpace(req.AppName)
	}
	if strings.TrimSpace(req.RedirectURI) != "" {
		updates["redirect_uri"] = strings.TrimSpace(req.RedirectURI)
	}
	// 仅当显式传入 description / app_icon 时才更新，避免每次停用/重置密钥把已有值清空。
	if req.Description != nil {
		updates["description"] = strings.TrimSpace(*req.Description)
	}
	if req.AppIcon != nil {
		updates["app_icon"] = strings.TrimSpace(*req.AppIcon)
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	plainSecret := ""
	if req.RegenSecret {
		plainSecret = util.GenerateRandomString(32)
		updates["app_secret"] = plainSecret
	}
	if err := db.Model(&app).Updates(updates).Error; err != nil {
		response.Fail(c, nil, "更新失败")
		return
	}
	data := gin.H{"app_id": appID}
	if plainSecret != "" {
		data["app_secret"] = plainSecret
	}
	response.Success(c, data, "已更新")
}

func AdminDeleteOAuthApp(c *gin.Context) {
	appID := c.Param("appId")
	if err := common.GetDB().Where("app_id = ?", appID).Delete(&model.OAuth2App{}).Error; err != nil {
		response.Fail(c, nil, "删除失败")
		return
	}
	response.Success(c, nil, "已从 oauth2_apps 删除")
}

func AdminListOAuthRecords(c *gin.Context) {
	appID := strings.TrimSpace(c.Query("app_id"))
	page := 1
	size := 20
	if v := c.Query("page"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			page = n
		}
	}
	if v := c.Query("size"); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n <= 100 {
			size = n
		}
	}
	db := common.GetDB().Model(&model.OAuth2AuthRecord{})
	if appID != "" {
		db = db.Where("app_id = ?", appID)
	}
	var total int64
	db.Count(&total)
	var rows []model.OAuth2AuthRecord
	if err := db.Order("authorized_at desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		response.Fail(c, nil, "读取授权记录失败")
		return
	}
	response.Success(c, gin.H{"records": rows, "total": total, "page": page, "size": size}, "ok")
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, http.ErrNotSupported
		}
		n = n*10 + int(ch-'0')
	}
	if n <= 0 {
		return 0, http.ErrNotSupported
	}
	return n, nil
}
