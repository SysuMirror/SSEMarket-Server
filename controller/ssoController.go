package controller

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"loginTest/apperr"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"loginTest/util"

	"github.com/gin-gonic/gin"
)

// ssoAuthUser 从 gin context 取当前登录用户（与 StrictAuthMiddleware 一致）。
func ssoAuthUser(c *gin.Context) *domain.User {
	v, ok := c.Get("user")
	if !ok {
		return nil
	}
	u, ok := v.(*domain.User)
	if !ok || u == nil || u.ID == 0 {
		return nil
	}
	return u
}

// MirrorProvision 只接受 Mirror 服务端密钥；邮箱已在 UniSSO 侧完成验证。
func MirrorProvision(c *gin.Context) {
	expected := config.SsoClientSecret()
	provided := c.GetHeader("X-Unisso-Trust")
	if expected == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "镜像服务未授权")
		return
	}

	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}

	user, tokens, err := service.Users.MirrorProvision(c.Request.Context(), service.MirrorProvisionInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		var validationErr *service.WorkbenchValidationError
		if errors.As(err, &validationErr) {
			response.Response(c, http.StatusBadRequest, 400, nil, validationErr.Error())
			return
		}
		apperr.WriteMsg(c, err, "镜像账号处理失败")
		return
	}

	resp := gin.H{
		"user": gin.H{
			"userID": user.ID,
			"name":   user.Name,
			"email":  user.Email,
			"phone":  user.Phone,
		},
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}
	response.Success(c, resp, "账号已就绪")
}

// SsoLogin GET /api/sso/login —— 未登录入口，302 到 UniSSO authorize。
func SsoLogin(c *gin.Context) {
	authorizeURL, err := service.Sso.BuildAuthorizeURL(c.Request.Context(), 0)
	if err != nil {
		response.Response(c, http.StatusServiceUnavailable, 503, nil, "SSO 登录未启用或暂时不可用")
		return
	}
	c.Redirect(http.StatusFound, authorizeURL)
}

// SsoMode GET /api/sso/mode —— 登录模式快照，前端据此决定是否展示本地密码兜底入口。
func SsoMode(c *gin.Context) {
	service.EnsureSsoModeFresh()
	info := service.CurrentSsoModeInfo()
	response.Success(c, gin.H{
		"mode":                 info.Mode,
		"password_login":       info.PasswordLogin,
		"unisso_healthy":       info.UnissoHealthy,
		"consecutive_failures": info.ConsecutiveFailures,
		"checked_at":           info.CheckedAt,
		"last_error":           info.LastError,
	}, "成功")
}

// SsoBindStart POST /api/sso/bind/start —— 已登录用户发起绑定，前端拿 authorize_url 跳转。
func SsoBindStart(c *gin.Context) {
	user := ssoAuthUser(c)
	if user == nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	if st, err := service.Sso.GetStatus(c.Request.Context(), user.ID); err == nil && st.Bound {
		response.Response(c, http.StatusConflict, 409, nil, "当前账号已绑定 SSO，请先解绑")
		return
	}
	authorizeURL, err := service.Sso.BuildAuthorizeURL(c.Request.Context(), user.ID)
	if err != nil {
		response.Response(c, http.StatusServiceUnavailable, 503, nil, "SSO 未启用或暂时不可用")
		return
	}
	response.Success(c, gin.H{"authorize_url": authorizeURL}, "获取绑定地址成功")
}

// SsoCallback GET /api/sso/callback —— UniSSO 回调，302 到前端结果页。
// 成功: /new/auth/sso/result?ticket=xxx；失败: /new/auth/sso/result?error=state|exchange|conflict
// 前端部署在 /new/ 子路径下，故 redirect 前缀 /new/auth/sso/result。
func SsoCallback(c *gin.Context) {
	ticket, err := service.Sso.ConsumeCallback(c.Request.Context(), c.Query("code"), c.Query("state"))
	if err != nil {
		errCode := "exchange"
		switch {
		case errors.Is(err, service.ErrSsoStateInvalid):
			errCode = "state"
		case errors.Is(err, service.ErrSsoConflict):
			errCode = "conflict"
		case errors.Is(err, service.ErrSsoForbidden):
			errCode = "internal"
		}
		c.Redirect(http.StatusFound, "/new/auth/sso/result?error="+errCode)
		return
	}
	c.Redirect(http.StatusFound, "/new/auth/sso/result?ticket="+ticket)
}

// SsoExchange POST /api/sso/exchange {ticket} —— 登录令牌或 bind_needed。
func SsoExchange(c *gin.Context) {
	var req struct {
		Ticket string `json:"ticket"`
	}
	if err := c.Bind(&req); err != nil || req.Ticket == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	result, err := service.Sso.ExchangeTicket(c.Request.Context(), req.Ticket)
	if err != nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "登录凭证无效或已过期，请重新发起 SSO 登录")
		return
	}
	response.Success(c, gin.H{
		"type":          result.Type,
		"token":         result.Token,
		"refresh_token": result.RefreshToken,
		"sso_username":  result.SsoUsername,
		"sso_email":     result.SsoEmail,
	}, "成功")
}

// SsoBind POST /api/sso/bind {ticket, email, password} —— 邮箱密码验证后写绑定并发令牌。
func SsoBind(c *gin.Context) {
	var req struct {
		Ticket   string `json:"ticket"`
		Email    string `json:"email"`
		Password string `json:"password"` // AES 加密传输
	}
	if err := c.Bind(&req); err != nil || req.Ticket == "" || req.Email == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	tokens, err := service.Sso.BindWithCredentials(c.Request.Context(), req.Ticket, req.Email, util.Decrypt(req.Password))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, "该邮箱未注册集市账号，请先注册")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "邮箱或密码错误")
		case errors.Is(err, domain.ErrConflict):
			response.Response(c, http.StatusConflict, 409, nil, "该账号或 SSO 已存在绑定")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		default:
			apperr.WriteMsg(c, err, "绑定失败")
		}
		return
	}
	response.Success(c, gin.H{
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}, "绑定并登录成功")
}

// SsoStatus GET /api/sso/status —— 绑定状态。
func SsoStatus(c *gin.Context) {
	user := ssoAuthUser(c)
	if user == nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	st, err := service.Sso.GetStatus(c.Request.Context(), user.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	response.Success(c, gin.H{
		"bound":        st.Bound,
		"sso_username": st.SsoUsername,
		"bound_at":     st.BoundAt,
	}, "成功")
}

// SsoUnbind POST /api/sso/unbind {password} —— 验证密码后解绑。
func SsoUnbind(c *gin.Context) {
	user := ssoAuthUser(c)
	if user == nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	var req struct {
		Password string `json:"password"` // AES 加密传输
	}
	if err := c.Bind(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	if err := service.Sso.Unbind(c.Request.Context(), user.ID, util.Decrypt(req.Password)); err != nil {
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "密码错误")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusConflict, 409, nil, "当前账号未绑定 SSO")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		default:
			apperr.WriteMsg(c, err, "解绑失败")
		}
		return
	}
	response.Success(c, nil, "解绑成功")
}
