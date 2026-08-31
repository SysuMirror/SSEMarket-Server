package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// WorkbenchAppCreateMsg 平台中转「添加工作台站点」请求体。
type WorkbenchAppCreateMsg struct {
	Name       string `json:"name"`
	Icon       string `json:"icon"`
	TargetType string `json:"targetType"`
	Target     string `json:"target"`
	GroupName  string `json:"groupName"`
	ColorBg    string `json:"colorBg"`
	ColorFg    string `json:"colorFg"`
	SortOrder  int    `json:"sortOrder"`
	IsVisible  *bool  `json:"isVisible"`
}

// WorkbenchAppResponse 工作台应用响应体。
type WorkbenchAppResponse struct {
	AppID       int       `json:"appID"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	TargetType  string    `json:"targetType"`
	Target      string    `json:"target"`
	GroupName   string    `json:"groupName"`
	ColorBg     string    `json:"colorBg"`
	ColorFg     string    `json:"colorFg"`
	Source      string    `json:"source"`
	InvokerSlug string    `json:"invokerSlug,omitempty"`
	SortOrder   int       `json:"sortOrder"`
	IsVisible   bool      `json:"isVisible"`
	CreatedAt   time.Time `json:"createdAt"`
}

func workbenchAppToResponse(app *domain.WorkbenchApp) WorkbenchAppResponse {
	return WorkbenchAppResponse{
		AppID:       app.AppID,
		Name:        app.Name,
		Icon:        app.Icon,
		TargetType:  app.TargetType,
		Target:      app.Target,
		GroupName:   app.GroupName,
		ColorBg:     app.ColorBg,
		ColorFg:     app.ColorFg,
		Source:      app.Source,
		InvokerSlug: app.InvokerSlug,
		SortOrder:   app.SortOrder,
		IsVisible:   app.IsVisible,
		CreatedAt:   app.CreatedAt,
	}
}

// platformInvoker 从 gin context 取经平台验签的可信调用方模块 slug。
func platformInvoker(c *gin.Context) string {
	v, ok := c.Get("platformInvoker")
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// OpenWorkbenchAppCreate POST /api/open/workbench/apps
// 供集市开发者平台模块经 /api/open/invoke/:apiId 中转调用；平台验签中间件已先行通过。
func OpenWorkbenchAppCreate(c *gin.Context) {
	var msg WorkbenchAppCreateMsg
	if err := c.ShouldBindJSON(&msg); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误：请求体应为 JSON")
		return
	}
	visible := true
	if msg.IsVisible != nil {
		visible = *msg.IsVisible
	}
	app, err := service.WorkbenchApps.CreateApp(c.Request.Context(), domain.WorkbenchAppCreateInput{
		Name:        msg.Name,
		Icon:        msg.Icon,
		TargetType:  msg.TargetType,
		Target:      msg.Target,
		GroupName:   msg.GroupName,
		ColorBg:     msg.ColorBg,
		ColorFg:     msg.ColorFg,
		SortOrder:   msg.SortOrder,
		IsVisible:   visible,
		InvokerSlug: platformInvoker(c),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			response.Response(c, http.StatusConflict, 409, nil, "该站点名称已存在")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
		default:
			response.Response(c, http.StatusInternalServerError, 500, nil, "创建失败")
		}
		return
	}
	response.Success(c, gin.H{"app": workbenchAppToResponse(app)}, "站点已添加")
}

// OpenWorkbenchAppList GET /api/open/workbench/apps
// 平台中转查询当前可见站点列表（同样要求平台验签）。
func OpenWorkbenchAppList(c *gin.Context) {
	list, err := service.WorkbenchApps.ListVisible(c.Request.Context())
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "查询失败")
		return
	}
	out := make([]WorkbenchAppResponse, 0, len(list))
	for _, app := range list {
		out = append(out, workbenchAppToResponse(app))
	}
	c.JSON(http.StatusOK, out)
}
