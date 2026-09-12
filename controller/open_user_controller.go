package controller

import (
	"errors"
	"net/http"

	"loginTest/observability"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// OpenUserCreateMsg 平台中转「注册用户」请求体。
type OpenUserCreateMsg struct {
	Name     string `json:"name"`     // 选填，缺省随机生成
	Email    string `json:"email"`    // 必填
	Password string `json:"password"` // 必填，明文，≥6 位（走平台中转，建议调用方自行加密传输）
	Phone    string `json:"phone"`    // 选填，缺省随机 11 位
}

// OpenUserCreateResponse 平台建号响应体。
type OpenUserCreateResponse struct {
	UserID       int    `json:"userID"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

// OpenUserCreate POST /api/open/users
// 供集市开发者平台模块经 /api/open/invoke/:apiId 中转调用；平台验签中间件已先行通过。
// 鉴权方式与工作台站点接口完全一致（x-model-authorization + x-model-query-time）。
func OpenUserCreate(c *gin.Context) {
	var msg OpenUserCreateMsg
	if err := c.ShouldBindJSON(&msg); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误：请求体应为 JSON")
		return
	}

	user, tokens, err := service.Users.PlatformCreateUser(c.Request.Context(), service.PlatformCreateUserInput{
		Name:     msg.Name,
		Email:    msg.Email,
		Password: msg.Password,
		Phone:    msg.Phone,
	})
	if err != nil {
		var validationErr *service.WorkbenchValidationError
		switch {
		case errors.As(err, &validationErr):
			response.Response(c, http.StatusBadRequest, 400, nil, validationErr.Error())
		default:
			log.Errorf("[platform-user] 建号失败: %v (invoker=%s, request_id=%s)",
				err, platformInvoker(c), observability.RequestIDFromGin(c))
			response.Response(c, http.StatusInternalServerError, 500, nil, "创建失败")
		}
		return
	}

	resp := OpenUserCreateResponse{
		UserID: user.ID,
		Name:   user.Name,
		Email:  user.Email,
		Phone:  user.Phone,
	}
	if tokens != nil {
		resp.AccessToken = tokens.AccessToken
		resp.RefreshToken = tokens.RefreshToken
	}
	log.WithFields(log.Fields{
		"user_id":       user.ID,
		"email":         user.Email,
		"invoker":       platformInvoker(c),
		"request_id":    observability.RequestIDFromGin(c),
	}).Info("[platform-user] 平台建号成功")
	response.Success(c, gin.H{"user": resp}, "用户已创建")
}
