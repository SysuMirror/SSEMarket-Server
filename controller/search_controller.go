package controller

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// SearchUsersMsg 搜索页用户卡片请求。
type SearchUsersMsg struct {
	Keyword string `json:"keyword"`
	Limit   int    `json:"limit"`
}

// SearchUsers 搜索页「用户」卡片（POST /api/auth/search/users）。
//
// 命中规则：
//   - 用户名全等/包含/关键词全部命中 → 必定返回；
//   - 用户名或简介的相关度 ≥ domain.UserSearchRelevanceThreshold → 返回；
//   - 用户开启「隐藏发帖」时返回 hidden=true，前端显示「用户隐藏了历史对话」。
func SearchUsers(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "权限不足")
		return
	}
	var req SearchUsersMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	keyword := strings.TrimSpace(req.Keyword)
	if utf8.RuneCountInString(keyword) > domain.UserSearchKeywordMaxRunes {
		response.Response(c, http.StatusBadRequest, 400, nil, "搜索关键词过长")
		return
	}
	if keyword == "" {
		response.Success(c, gin.H{"users": []domain.UserSearchResult{}, "threshold": domain.UserSearchRelevanceThreshold}, "")
		return
	}
	users, err := service.UserSearch.Search(c.Request.Context(), keyword, req.Limit, viewer.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "用户搜索失败")
		return
	}
	response.Success(c, gin.H{
		"users":     users,
		"threshold": domain.UserSearchRelevanceThreshold,
	}, "")
}
