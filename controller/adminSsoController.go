package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"loginTest/config"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// writeAdminSsoErr 把管理台准入错误映射为前端可区分的状态码与文案。
func writeAdminSsoErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrBadRequest):
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "邮箱或密码格式不正确")
	case errors.Is(err, service.ErrSsoBadCredentials):
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "统一身份认证邮箱或密码错误")
	case errors.Is(err, service.ErrSsoTooManyAttempts):
		response.Response(c, http.StatusTooManyRequests, 429, nil, "尝试次数过多，请 15 分钟后再试")
	case errors.Is(err, service.ErrSsoVerifyForbidden):
		response.Response(c, http.StatusForbidden, 403, nil, "管理台登录通道未开通，请联系运维")
	case errors.Is(err, service.ErrSsoUnavailable):
		response.Response(c, http.StatusServiceUnavailable, 503, nil, "统一身份认证暂不可用，请稍后重试")
	case errors.Is(err, service.ErrSsoInvalidSession):
		response.Response(c, http.StatusUnauthorized, 401, nil, "invalid_sso")
	case errors.Is(err, service.ErrAdminNotInternal):
		response.Response(c, http.StatusForbidden, 403, nil, "仅学院成员可登录管理台")
	case errors.Is(err, service.ErrAdminNotWhitelisted):
		response.Response(c, http.StatusForbidden, 403, nil, "当前账号不在管理台管理员名单内")
	case errors.Is(err, service.ErrAdminInactive):
		response.Response(c, http.StatusForbidden, 403, nil, "管理台权限已停用")
	case errors.Is(err, service.ErrAdminLastActive):
		response.Response(c, http.StatusForbidden, 403, nil, "至少需要保留一名启用中的管理员")
	case errors.Is(err, domain.ErrNotFound):
		response.Response(c, http.StatusNotFound, 404, nil, "管理员条目不存在")
	case errors.Is(err, domain.ErrConflict):
		response.Response(c, http.StatusConflict, 409, nil, "该邮箱已在管理员名单内")
	default:
		response.Response(c, http.StatusInternalServerError, 500, nil, "系统异常")
	}
}

// AdminSsoLogin 管理台主登录：unisso 邮箱密码 → 白名单准入 → 管理台 JWT。
func AdminSsoLogin(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Email) == "" || body.Password == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "请填写邮箱与密码")
		return
	}
	admin, token, sess, err := service.Admins.LoginWithSso(c.Request.Context(), body.Email, body.Password)
	if err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	setSsoCookie(c, sess)
	service.SsoAudit("admin_sso_login", "result", "success")
	response.Success(c, gin.H{"token": token, "admin": dto.ToAdminDto(*admin)}, "登录成功")
}

// AdminSsoTry 管理台静默登录：共享 Cookie → unisso 校验 → 白名单准入。
func AdminSsoTry(c *gin.Context) {
	sid, err := c.Cookie(config.SsoCookieName())
	if err != nil || sid == "" {
		sid, _ = c.Cookie(config.SsoCookieNameLegacy())
	}
	if sid == "" {
		response.Response(c, http.StatusUnauthorized, 401, nil, "no_sso")
		return
	}
	admin, token, err := service.Admins.LoginWithSession(c.Request.Context(), sid)
	if err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	service.SsoAudit("admin_sso_silent", "result", "success")
	response.Success(c, gin.H{"token": token, "admin": dto.ToAdminDto(*admin)}, "登录成功")
}

// AdminSsoLogout 单点登出：吊销 unisso 共享会话并清父域 Cookie。
func AdminSsoLogout(c *gin.Context) {
	sid, _ := c.Cookie(config.SsoCookieName())
	if sid == "" {
		sid, _ = c.Cookie(config.SsoCookieNameLegacy())
	}
	service.Sso.RevokeSession(c.Request.Context(), sid)
	clearSsoCookie(c, nil)
	service.SsoAudit("admin_sso_logout", "result", "done")
	response.Success(c, nil, "已退出登录")
}

// AdminListMembers 管理员白名单。
func AdminListMembers(c *gin.Context) {
	items, err := service.Admins.ListAdmins(c.Request.Context())
	if err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	response.Success(c, gin.H{"admins": dto.ToAdminListDto(items)}, "ok")
}

// AdminAddMember 按邮箱新增管理员（对方须为 unisso 学院成员，首次登录回填 sso_sub）。
func AdminAddMember(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	admin, err := service.Admins.AddAdminByEmail(c.Request.Context(), body.Email, body.Name)
	if err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	response.Success(c, gin.H{"admin": dto.ToAdminDto(*admin)}, "已加入管理员名单")
}

// AdminUpdateMember 启用/停用管理员。不能停用自己，也不能停用最后一名启用中的管理员。
func AdminUpdateMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "管理员 ID 非法")
		return
	}
	var body struct {
		IsActive *bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.IsActive == nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	if cur, ok := currentAdmin(c); ok && cur.ID == id && !*body.IsActive {
		response.Response(c, http.StatusForbidden, 403, nil, "不能停用自己的管理台权限")
		return
	}
	if err := service.Admins.SetAdminActive(c.Request.Context(), id, *body.IsActive); err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	response.Success(c, nil, "已更新")
}

// AdminDeleteMember 移除管理员。不能移除自己，也不能移除最后一名启用中的管理员。
func AdminDeleteMember(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "管理员 ID 非法")
		return
	}
	if cur, ok := currentAdmin(c); ok && cur.ID == id {
		response.Response(c, http.StatusForbidden, 403, nil, "不能移除自己的管理台权限")
		return
	}
	if err := service.Admins.DeleteAdminByID(c.Request.Context(), id); err != nil {
		writeAdminSsoErr(c, err)
		return
	}
	response.Success(c, nil, "已移除")
}

func currentAdmin(c *gin.Context) (*domain.Admin, bool) {
	v, ok := c.Get("admin")
	if !ok {
		return nil, false
	}
	admin, ok := v.(*domain.Admin)
	if !ok || admin == nil {
		return nil, false
	}
	return admin, true
}
