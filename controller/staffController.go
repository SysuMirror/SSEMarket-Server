package controller

import (
	"loginTest/apperr"
	"loginTest/response"
	"loginTest/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 职位账号（开发人员匿名 @）对外接口。
// 出参一律是 domain.StaffAliasPublic：alias / title / 职位小号 userID，
// 永远不含真实开发者的 userID（匿名性红线，见 service/staff_service.go）。

// StaffAliasList @ 下拉候选：全部职位账号（公开字段）。
// 前端编辑器的 @ 入口拉这份列表；表为空时前端不显示入口，对普通用户零打扰。
func StaffAliasList(c *gin.Context) {
	aliases, err := service.Staffs.ListPublicAliases(c.Request.Context())
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	response.Success(c, gin.H{"aliases": aliases}, "ok")
}

type staffAliasThreadReq struct {
	PostID int `json:"postID"`
}

// StaffAliasThreadAliases 当前用户在该帖子线程内被 @ 到的职位身份。
// 前端在打开该帖的评论框时调用，用于展示「以 XX 身份回复」开关；
// 普通用户（无职位映射）恒为空列表。
func StaffAliasThreadAliases(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	var req staffAliasThreadReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	aliases, err := service.Staffs.ThreadAliasesForUser(c.Request.Context(), user.ID, req.PostID)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	response.Success(c, gin.H{"aliases": aliases}, "ok")
}
