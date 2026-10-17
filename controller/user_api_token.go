package controller

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)


// VerifyAPIToken POST /api/auth/token/verify
// PAT 校验：MCP 网关等服务端以 Bearer PAT 换取 (userID, scopes)。
// 无需登录态；凭据错误一律 401，不区分"不存在/已吊销/停用"。
func VerifyAPIToken(c *gin.Context) {
	auth := c.GetHeader("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		response.Response(c, http.StatusUnauthorized, 401, nil, "missing bearer token")
		return
	}
	res, err := service.Users.VerifyAPIToken(c.Request.Context(), token)
	if err != nil || res == nil || !res.IsActive {
		response.Response(c, http.StatusUnauthorized, 401, nil, "invalid or revoked token")
		return
	}
	response.Success(c, gin.H{
		"user_id":   res.UserID,
		"scopes":    res.Scopes,
		"is_active": res.IsActive,
	}, "ok")
}

// ListAPITokenScopes GET /api/auth/token/scopes
// 返回当前支持的所有 scope 列表，供前端勾选。
func ListAPITokenScopes(c *gin.Context) {
	scopes := service.Users.ListAPITokenScopes(c.Request.Context())
	response.Success(c, gin.H{"scopes": scopes}, "ok")
}

type createTokenReq struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

// CreateAPIToken POST /api/auth/token/create
// 创建 PAT。明文 token 仅在此响应中返回一次。
func CreateAPIToken(c *gin.Context) {
	var req createTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数格式错误")
		return
	}
	userVal, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user, ok := userVal.(*domain.User)
	if !ok || user == nil || user.ID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	result, err := service.Users.CreateAPIToken(c.Request.Context(), domain.UserAPITokenCreateInput{
		Name:   req.Name,
		Scopes: req.Scopes,
		UserID: user.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Fail(c, nil, "参数错误：name 必填且非空，scopes 至少包含一个合法值")
		case errors.Is(err, domain.ErrTooManyRequests):
			response.Response(c, http.StatusTooManyRequests, 429, nil, "已达 token 数量上限，请先吊销旧 token")
		default:
			apperr.WriteMsg(c, err, "创建 token 失败")
		}
		return
	}
	response.Success(c, gin.H{
		"token_id":   result.TokenID,
		"token":      result.PlainText, // 明文，仅此一次
		"prefix":     result.Prefix,
		"name":       result.Name,
		"scopes":     result.Scopes,
		"is_active":  result.IsActive,
		"created_at": result.CreatedAt,
	}, "创建成功，请立即复制保存，明文 token 仅此一次返回")
}

// ListAPITokens GET /api/auth/token/list?page=1&page_size=20
// 列出当前用户的 PAT（不含明文）。
func ListAPITokens(c *gin.Context) {
	userVal, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user, ok := userVal.(*domain.User)
	if !ok || user == nil || user.ID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	result, err := service.Users.ListAPITokens(c.Request.Context(), user.ID, page, pageSize)
	if err != nil {
		apperr.WriteMsg(c, err, "查询 token 列表失败")
		return
	}
	response.Success(c, gin.H{
		"records":     result.Records,
		"total":       result.Total,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
	}, "ok")
}

type revokeTokenReq struct {
	TokenID int `json:"token_id"`
}

// RevokeAPIToken POST /api/auth/token/revoke
// 软删 PAT。token_id 必须属于当前登录用户。
func RevokeAPIToken(c *gin.Context) {
	var req revokeTokenReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TokenID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误：token_id 必填")
		return
	}
	userVal, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user, ok := userVal.(*domain.User)
	if !ok || user == nil || user.ID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	if err := service.Users.RevokeAPIToken(c.Request.Context(), req.TokenID, user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "token 不存在或已吊销")
			return
		}
		log.Printf("[pat] revoke: %v", err)
		apperr.WriteMsg(c, err, "吊销 token 失败")
		return
	}
	response.Success(c, gin.H{"token_id": req.TokenID}, "已吊销")
}

// DeleteAPIToken POST /api/auth/token/delete
// 硬删已吊销的 PAT 记录。活跃 token 必须先吊销才能删除。
func DeleteAPIToken(c *gin.Context) {
	var req revokeTokenReq
	if err := c.ShouldBindJSON(&req); err != nil || req.TokenID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误：token_id 必填")
		return
	}
	userVal, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user, ok := userVal.(*domain.User)
	if !ok || user == nil || user.ID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	if err := service.Users.DeleteAPIToken(c.Request.Context(), req.TokenID, user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "token 不存在或仍处于活跃状态，请先吊销")
			return
		}
		log.Printf("[pat] delete: %v", err)
		apperr.WriteMsg(c, err, "删除 token 失败")
		return
	}
	response.Success(c, gin.H{"token_id": req.TokenID}, "已删除")
}
