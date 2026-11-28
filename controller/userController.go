package controller

import (
	"context"
	"errors"
	"fmt"
	"image"
	"loginTest/api"
	"loginTest/apperr"
	"loginTest/common"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/response"
	"loginTest/service"
	"loginTest/util"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

func GetTokenUserID(c *gin.Context) int {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" || len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
		return 0
	}
	tokenString = tokenString[7:]
	token, claims, err := common.ParseToken(tokenString)
	if err != nil || !token.Valid {
		return 0
	}

	// 获取token中的用户标识符
	tokenUserID := claims.UserID
	return tokenUserID
}

type UserMsg struct {
	UserID int `json:"user_id"`
}
type registerUser struct {
	Name      string `gorm:"type:varchar(20);not null"`
	Phone     string `gorm:"type:varchar(11);not null"`
	Password  string `gorm:"size:255;not null"`
	Password2 string `gorm:"size:255;not null"`
	Email     string `gorm:"type:varchar(11);not null"`
	//Num       string `gorm:"type:varchar(8);not null"`
	ValiCode string `gorm:"type:varchar(10);not null"`
	CDKey    string `json:"CDKey"`
}

type identity struct {
	Email    string `gorm:"type:varchar(50);not null"`
	ValiCode string `gorm:"type:varchar(10);not null"`
}

type ModifyUser struct {
	Email     string `gorm:"type:varchar(50);not null"`
	Phone     string `gorm:"type:varchar(11);not null"`
	Password  string `gorm:"size:255;not null"`
	Password2 string `gorm:"size:255;not null"`
	ValiCode  string
}

type requestEmail struct {
	Email string `gorm:"type:varchar(50);not null"`
	Mode  int
}

func DeleteMe(c *gin.Context) {
	var requestUser = registerUser{}
	c.Bind(&requestUser)
	err := service.Users.DeleteMe(c.Request.Context(), domain.DeleteMeInput{
		Phone:    requestUser.Phone,
		Email:    requestUser.Email,
		ValiCode: requestUser.ValiCode,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "请用注册时使用的email完成注销")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "未找到该用户")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusBadRequest, 400, nil, "验证码错误")
		default:
			apperr.WriteMsg(c, err, "注销失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "成功删除该用户")
}

func Register(c *gin.Context) {
	// 统一身份模式（含故障兜底）下，注册的唯一入口是 sso.ssemarket.cn/register：
	// UniSSO 注册页校验验证码/CDKey 后会服务端调用本接口建集市账号，再把两边
	// 绑定起来。浏览器直接调本接口只会建出「UniSSO 里没有」的孤儿账号，之后
	// 永远登不上（2026-11-05/06 实际发生 2 例）。服务端调用不带 Origin/Referer，
	// 因此只拦浏览器请求，不破坏 UniSSO 的正常注册链路。
	if browserAuthRequest(c) {
		response.Response(c, http.StatusForbidden, 403, nil,
			"注册请前往统一身份认证：https://sso.ssemarket.cn/register")
		return
	}

	var requestUser = registerUser{}
	c.Bind(&requestUser)
	password1 := util.Decrypt(requestUser.Password)
	password2 := util.Decrypt(requestUser.Password2)
	if len(password2) < 6 {
		response.Response(c, http.StatusUnprocessableEntity, 422, nil, "密码不能少于6位")
		return
	}
	if password1 != password2 {
		response.Response(c, http.StatusUnprocessableEntity, 422, nil, "两次密码不相同，请重新输入")
		return
	}

	tokens, err := service.Users.Register(c.Request.Context(), domain.RegisterInput{
		Name:     requestUser.Name,
		Phone:    requestUser.Phone,
		Email:    strings.TrimSpace(requestUser.Email),
		Password: password1,
		ValiCode: requestUser.ValiCode,
		CDKey:    requestUser.CDKey,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrConflict):
			msg := "该邮箱已存在"
			if requestUser.Phone != "" {
				msg = "该手机号已存在"
			}
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, msg)
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusBadRequest, 400, nil, "验证码错误")
		case errors.Is(err, domain.ErrBadRequest):
			if strings.TrimSpace(requestUser.Email) == "" || !strings.Contains(requestUser.Email, "@") {
				response.Response(c, http.StatusUnprocessableEntity, 422, nil, "邮箱不符合要求")
			} else if requestUser.CDKey != "" {
				response.Response(c, http.StatusBadRequest, 400, nil, "激活码错误")
			} else {
				response.Response(c, http.StatusUnprocessableEntity, 422, nil, "手机号必须为11位!!!")
			}
		default:
			apperr.WriteMsg(c, err, "注册失败")
		}
		return
	}
	// 注册本地成功后再向 unisso 补取共享会话：unisso 尚无该账号时忽略失败，
	// 注册结果与本地登录态不受影响。
	if service.CurrentSsoMode() == service.SsoModeSso {
		if sess, sessErr := service.Sso.ObtainPasswordSession(c.Request.Context(), requestUser.Email, password1); sessErr == nil {
			setSsoCookie(c, sess)
		}
	}
	response.Success(c, gin.H{
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}, "注册成功")
}

func IdentityValidate(c *gin.Context) {
	var requestUser = identity{}
	c.Bind(&requestUser)
	err := service.Users.IdentityValidate(c.Request.Context(), requestUser.Email, requestUser.ValiCode)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusBadRequest, 400, nil, "验证码错误")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "邮箱不符合要求")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "该邮箱不存在")
		default:
			apperr.WriteMsg(c, err, "身份验证失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "身份验证成功")
}

type loginuser struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// browserAuthRequest 判断请求是否由浏览器页面发起（同源 POST 也带 Origin，
// 链接跳转/XHR 通常还带 Referer，现代浏览器另带 Sec-Fetch-*）。
// UniSSO 后端服务端调用 /auth/register 建号时不带这些头，据此把「统一身份
// 注册」与「用户直接打开集市注册页」区分开。
func browserAuthRequest(c *gin.Context) bool {
	return c.GetHeader("Origin") != "" ||
		c.GetHeader("Referer") != "" ||
		c.GetHeader("Sec-Fetch-Site") != ""
}

func Login(c *gin.Context) {
	mode := service.CurrentSsoMode()
	if mode == service.SsoModeDisabled {
		response.Response(c, http.StatusServiceUnavailable, 503, nil, "统一身份服务暂时不可用，请稍后再试")
		return
	}
	var requestUser loginuser
	if err := c.ShouldBind(&requestUser); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}
	email := requestUser.Email
	password := util.Decrypt(requestUser.Password)
	// 健康探测只提供状态提示；密码始终由 UniSSO 校验。
	tokens, ssoSess, err := service.Sso.LoginViaUnisso(c.Request.Context(), email, password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSsoBadCredentials):
			service.SsoAudit("unisso_login", "mode", mode, "email", email, "result", "bad_credentials")
			response.Response(c, http.StatusUnauthorized, 401, nil, "邮箱或密码错误")
		case errors.Is(err, service.ErrSsoForbidden):
			service.SsoAudit("unisso_login", "mode", mode, "email", email, "result", "not_internal")
			response.Response(c, http.StatusForbidden, 403, nil, "该账号没有软件工程学院内部权限，无法登录集市")
		case errors.Is(err, service.ErrSsoConflict):
			response.Response(c, http.StatusConflict, 409, nil, "该统一身份已绑定其他集市账号")
		case errors.Is(err, service.ErrSsoNoLocalAccount):
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, "该邮箱没有对应的集市账号，请先注册")
		case errors.Is(err, service.ErrSsoVerifyForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "该账号不允许通过密码校验登录，请联系管理员")
		case errors.Is(err, service.ErrSsoTooManyAttempts):
			response.Response(c, http.StatusTooManyRequests, 429, nil, "失败次数过多，已被临时锁定，请15分钟后再试")
		case errors.Is(err, service.ErrSsoUnavailable):
			response.Response(c, http.StatusServiceUnavailable, 503, nil, "统一身份服务暂时不可用，请稍后再试")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, "密码不能少于6位")
		default:
			apperr.WriteMsg(c, err, "登录失败")
		}
		return
	}
	service.SsoAudit("unisso_login", "mode", mode, "email", email, "result", "success")
	setSsoCookie(c, ssoSess)
	response.Success(c, gin.H{
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}, "登录成功")
	return
}

// setSsoCookie 把 unisso 签发的共享父域 Cookie 种给浏览器。
// 值/名称/域/有效期全部来自 unisso 响应，不从本仓库常量推导。
func setSsoCookie(c *gin.Context, sess *domain.SsoSession) {
	if sess == nil || sess.ID == "" || sess.CookieName == "" || sess.CookieDomain == "" {
		return
	}
	maxAge := sess.MaxAge
	if maxAge <= 0 {
		maxAge = config.SsoCookieMaxAge()
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sess.CookieName,
		Value:    sess.ID,
		Domain:   sess.CookieDomain,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSsoCookie 清父域 Cookie：新名/旧名（及 unisso 响应指名的 Cookie）一律清除，
// 域优先用 unisso 响应值，缺失时退到配置默认值，保证 unisso 不可达也能把浏览器里的会话指针删掉。
func clearSsoCookie(c *gin.Context, sess *domain.SsoSession) {
	cookieDomain := config.SsoCookieDomain()
	names := []string{config.SsoCookieName(), config.SsoCookieNameLegacy()}
	if sess != nil {
		if sess.CookieDomain != "" {
			cookieDomain = sess.CookieDomain
		}
		if sess.CookieName != "" && sess.CookieName != names[0] && sess.CookieName != names[1] {
			names = append(names, sess.CookieName)
		}
	}
	for _, name := range names {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Domain:   cookieDomain,
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

// SsoTry 静默 SSO：共享 Cookie（unisso_sid，兼容旧 sse_sso）→ unisso 校验 → 本地 JWT。
// 无 Cookie/会话失效/非院内 → 401，unisso 不可达 → 503；前端两种情况都静默回落登录表单。
func SsoTry(c *gin.Context) {
	sid, err := c.Cookie(config.SsoCookieName())
	if err != nil || sid == "" {
		// 旧名 Cookie 兜底：unisso 切换默认名前签发的会话指针仍是 sse_sso。
		sid, err = c.Cookie(config.SsoCookieNameLegacy())
	}
	if err != nil || sid == "" {
		// 探测失败或未配置时，不能仅凭缺少 Cookie 判定用户已登出，
		// 不能据此判定登出；返回 503，前端保留本地会话。
		if service.CurrentSsoMode() != service.SsoModeSso {
			response.Response(c, http.StatusServiceUnavailable, 503, nil, "sso_unavailable")
			return
		}
		response.Response(c, http.StatusUnauthorized, 401, nil, "no_sso")
		return
	}
	tokens, err := service.Sso.LoginViaSession(c.Request.Context(), sid)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrSsoUnavailable):
			response.Response(c, http.StatusServiceUnavailable, 503, nil, "sso_unavailable")
		case errors.Is(err, service.ErrSsoInvalidSession),
			errors.Is(err, service.ErrSsoForbidden),
			errors.Is(err, service.ErrSsoNoLocalAccount),
			errors.Is(err, service.ErrSsoConflict):
			response.Response(c, http.StatusUnauthorized, 401, nil, "invalid_sso")
		default:
			response.Response(c, http.StatusServiceUnavailable, 503, nil, "sso_unavailable")
		}
		return
	}
	service.SsoAudit("sso_silent", "result", "success")
	response.Success(c, gin.H{
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}, "登录成功")
}

// SsoLogout 单点登出：先吊销 unisso 会话，再清父域 Cookie，最后返回 200 让前端清本地会话。
// unisso 不可达时仍然清 Cookie 并返回 200。
func SsoLogout(c *gin.Context) {
	sid, _ := c.Cookie(config.SsoCookieName())
	if sid == "" {
		// 旧名 Cookie 兜底：否则仅持旧名 Cookie 的用户登出时吊销不到 unisso 会话。
		sid, _ = c.Cookie(config.SsoCookieNameLegacy())
	}
	service.Sso.RevokeSession(c.Request.Context(), sid)
	clearSsoCookie(c, nil)
	service.SsoAudit("sso_logout", "result", "done")
	response.Success(c, nil, "已退出登录")
}

// 通过 refresh token 刷新 access token
func RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		return
	}

	tokens, err := service.Users.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrTokenExpired):
			response.Response(c, http.StatusUnauthorized, 401, nil, "refresh token 已过期")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "refresh token 无效或已使用")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, "请求参数错误")
		default:
			apperr.WriteMsg(c, err, "刷新token失败")
		}
		return
	}
	response.Success(c, gin.H{
		"token":         tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
	}, "刷新token成功")
}

func ModifyPassword(c *gin.Context) {
	response.Response(c, http.StatusForbidden, 403, nil,
		"请使用集市登录页的忘记密码入口，或前往 https://sso.ssemarket.cn/reset-password")
}

func ModifyEmail(c *gin.Context) {
	var requestUser struct {
		Email    string `json:"email"`
		NewEmail string `json:"newEmail"`
		ValiCode string `json:"valiCode"`
	}
	if err := c.ShouldBind(&requestUser); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}

	userValue, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	user, ok := userValue.(*domain.User)
	if !ok || user == nil || user.ID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}

	_, err := service.Users.ModifyEmail(c.Request.Context(), domain.ModifyEmailInput{
		UserID:   user.ID,
		NewEmail: requestUser.NewEmail,
		ValiCode: requestUser.ValiCode,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "新邮箱不符合要求或与原邮箱相同")
		case errors.Is(err, domain.ErrConflict):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "该邮箱已存在")
		case errors.Is(err, domain.ErrEmailUsed):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "不可使用曾用邮箱")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusBadRequest, 400, nil, "验证码错误")
		case errors.Is(err, domain.ErrStale):
			response.Response(c, http.StatusConflict, 409, nil, "账号信息已变更，请刷新后重试")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "未找到该用户")
		default:
			apperr.WriteMsg(c, err, "修改邮箱失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "成功修改邮箱")
}

type OAuth2Info struct {
	AppId        string `json:"app_id"`
	RedirectURL  string `json:"redirect_uri"`
	ResponseType string `json:"response_type"`
	Scope        string `json:"scope"`
	State        string `json:"state"`
}

// 检查OAuth2应用信息，前端传入appid，返回对应的app_name、app_icon
func CheckOAuth2App(c *gin.Context) {
	var requestInfo OAuth2Info
	if err := c.Bind(&requestInfo); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数格式错误")
		return
	}
	app, err := service.Users.CheckOAuth2App(c.Request.Context(), requestInfo.AppId)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "OAuth2认证应用不存在或未开通")
		} else {
			apperr.WriteMsg(c, err, "获取应用信息失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, gin.H{
		"app_id":      app.AppID,
		"app_name":    app.AppName,
		"app_icon":    app.AppIcon,
		"description": app.Description,
	}, "应用信息获取成功")
}

// OAuth2授权接口，传入app_id、redirect_uri、response_type、scope、state
// 如果app_id跟数据库里的redirect_url对应，则返回带着临时有效的code的redirect_uri
func AuthorizeOAuth2App(c *gin.Context) {
	var requestInfo OAuth2Info
	if err := c.Bind(&requestInfo); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数格式错误")
		return
	}
	userInterface, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user := userInterface.(*domain.User)
	redirectURL, err := service.Users.AuthorizeOAuth2App(c.Request.Context(), domain.OAuth2AuthorizeInput{
		AppID:        requestInfo.AppId,
		RedirectURI:  requestInfo.RedirectURL,
		ResponseType: requestInfo.ResponseType,
		Scope:        requestInfo.Scope,
		State:        requestInfo.State,
		UserID:       user.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.GetHeader("User-Agent"),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			msg := "不支持的response_type"
			if requestInfo.ResponseType == "code" {
				if requestInfo.Scope != "read_basic" {
					msg = "不支持的scope"
				} else {
					msg = "redirect_uri不匹配"
				}
			}
			response.Response(c, http.StatusBadRequest, 400, nil, msg)
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "应用不存在或未激活")
		default:
			apperr.WriteMsg(c, err, "授权失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, gin.H{
		"redirect_url": redirectURL,
	}, "授权成功")
}

// 三方APP取得临时code后，其服务端带着code来请求，根据scope返回对应的access_token
func GetTokenbyOAuth2(c *gin.Context) {
	result, err := service.Users.GetTokenByOAuth2(c.Request.Context(), domain.OAuth2TokenInput{
		Code:      c.PostForm("code"),
		AppID:     c.PostForm("app_id"),
		AppSecret: c.PostForm("app_secret"),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			msg := "缺少必要参数"
			if c.PostForm("code") != "" && c.PostForm("app_id") != "" && c.PostForm("app_secret") != "" {
				msg = "授权码已过期"
			}
			response.Response(c, http.StatusBadRequest, 400, nil, msg)
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "授权码不存在或已使用")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "应用密钥错误")
		default:
			apperr.WriteMsg(c, err, "获取访问令牌失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, gin.H{
		"access_token": result.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   result.ExpiresIn,
		"scope":        result.Scope,
	}, "获取访问令牌成功")
}

// 三方APP取得access_token后，其服务端带着access_token来请求，验证token确实有对应权限后（这在oauthMiddleware实现），返回对应的用户信息
func GetInfobyOAuth2(c *gin.Context) {
	// 从OAuth中间件获取用户信息
	userInterface, exists := c.Get("oauth2_user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "OAuth2认证失败")
		return
	}
	user := userInterface.(*domain.User)

	// 获取权限范围
	scopeInterface, exists := c.Get("oauth2_scope")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "权限范围获取失败")
		return
	}
	scope := scopeInterface.(string)

	// 根据权限范围返回不同信息
	userInfo := gin.H{}

	if scope == "read_basic" {
		userInfo = gin.H{
			"user_id":    user.ID,
			"name":       user.Name,
			"email":      user.Email,
			"avatar_url": user.AvatarURL,
			"intro":      user.Intro,
			"identity":   user.Identity,
		}
	} else {
		response.Response(c, http.StatusForbidden, 403, nil, "权限不足")
		return
	}

	response.Response(c, http.StatusOK, 200, userInfo, "获取用户信息成功")
}

// 用户获取OAuth2授权记录
func GetOAuth2AuthRecords(c *gin.Context) {
	userInterface, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户未登录")
		return
	}
	user := userInterface.(*domain.User)
	page := 1
	pageSize := 10
	if p := c.Query("page"); p != "" {
		if parsedPage, err := strconv.Atoi(p); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsedPageSize, err := strconv.Atoi(ps); err == nil && parsedPageSize > 0 && parsedPageSize <= 100 {
			pageSize = parsedPageSize
		}
	}
	result, err := service.Users.ListOAuth2AuthRecords(c.Request.Context(), user.ID, page, pageSize)
	if err != nil {
		apperr.WriteMsg(c, err, "查询授权记录失败")
		return
	}
	var recordList []gin.H
	for _, record := range result.Records {
		recordList = append(recordList, gin.H{
			"record_id":     record.RecordID,
			"app_id":        record.AppID,
			"app_name":      record.AppName,
			"scope":         record.Scope,
			"ip_address":    record.IPAddress,
			"user_agent":    record.UserAgent,
			"status":        record.Status,
			"authorized_at": record.AuthorizedAt.Format("2006-01-02 15:04:05"),
			"expires_at":    record.ExpiresAt.Format("2006-01-02 15:04:05"),
		})
	}
	response.Response(c, http.StatusOK, 200, gin.H{
		"records":     recordList,
		"total":       result.Total,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
	}, "获取授权记录成功")
}

func ValidateEmail(c *gin.Context) {
	var request requestEmail
	c.Bind(&request)
	email := strings.ToLower(strings.TrimSpace(request.Email))
	mode := request.Mode
	fmt.Println("email = ", email)
	fmt.Println("mode = ", mode)
	if err := service.Users.ValidateEmailExists(c.Request.Context(), email, mode); err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusBadRequest, 400, gin.H{"data": email, "mode": mode}, "该邮箱未注册，请先完成注册操作")
		case errors.Is(err, domain.ErrConflict):
			response.Response(c, http.StatusBadRequest, 400, gin.H{"data": email, "mode": mode}, "邮箱已注册")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, gin.H{"data": email}, "邮箱获取错误")
		default:
			apperr.WriteMsg(c, err, "邮箱校验失败")
		}
		return
	}
	err := api.SendEmail(email, 0, "")
	if err != nil {
		msg := err.Error()
		if msg == "" {
			msg = "发送邮箱错误"
		}
		// 服务端 SMTP 未配置属于运维故障，不是用户输入问题：返回 503，
		// 让前端/SSO 显示“邮件服务未配置”的明确提示，而不是 400。
		if errors.Is(err, api.ErrMailNotConfigured) {
			response.Response(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, gin.H{"data": email}, msg)
			return
		}
		response.Response(c, http.StatusBadRequest, 400, gin.H{"data": email}, msg)
		return
	}
	response.Success(c, gin.H{"data": email}, "邮箱发送成功")
}

func Info(c *gin.Context) {
	user, _ := c.Get("user")
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"user": dto.ToUserDto(*user.(*domain.User))}})
}

func UploadAvatar(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "文件上传失败")
		return
	}

	// db := common.GetDB()
	// var user model.User

	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s", timestamp, file.Filename)
	filepath := "public/uploads/" + filename

	// 保存文件到本地
	err = c.SaveUploadedFile(file, filepath)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "文件保存失败")
		return
	}
	// 动图（GIF/动图WebP/APNG）解码只剩首帧，跳过常规缩图管线：
	// GIF 用 ffmpeg 保帧压缩（400 封顶），WebP/APNG 原文件直通图床
	animatedURL, err := processAnimatedImage(filepath, 400)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "动图上传失败" + err.Error()})
		return
	}
	if animatedURL != "" {
		response.Success(c, gin.H{"avatar_url": animatedURL}, "上传成功")
		return
	}

	// 压缩头像
	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件打开失败"})
		return
	}
	defer src.Close()
	// 读取上传文件的内容，并解码为 image.Image 对象
	img, _, err := image.Decode(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件解码失败"})
		return
	}

	// 对图像进行压缩处理
	resizedImage := imaging.Thumbnail(img, 400, 400, imaging.Lanczos)
	// 这里似乎只支持绝对路径，这里要根据服务器修改
	resizedPath := "public/resized/" + filename
	// 服务器本地备份
	err = imaging.Save(resizedImage, resizedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图生成保存失败" + err.Error()})
		return
	}

	fileURL := ""
	fileURL, err = api.UploadImage("/src/images/resized/"+filename, resizedPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图生成保存失败" + err.Error()})
		return
	}

	// 更新用户头像URL
	// 我们存储的是可以通过 HTTP 访问的 URL，而不是服务器本地的文件路径
	// user.AvatarURL = fileURL
	// user.AvatarURL = "https://ssemarket.cn:8080/uploads/" + filename
	// db.Save(&user)

	// 返回成功
	response.Success(c, gin.H{"avatar_url": fileURL}, "上传成功")
}

func UpdateAvatar(c *gin.Context) {
	// 用于移动端
	phone := c.PostForm("phone")
	if len(phone) != 11 {
		response.Response(c, http.StatusBadRequest, 400, nil, "Invalid phone number")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "文件上传失败")
		return
	}

	// Add a check for the file extension
	ext := filepath.Ext(file.Filename)
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" { // Add more formats if needed
		response.Response(c, http.StatusBadRequest, 400, nil, "Invalid file format")
		return
	}

	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s", timestamp, filepath.Base(file.Filename)) // Use base to avoid path traversal
	filepath := "public/uploads/" + filename

	err = c.SaveUploadedFile(file, filepath)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "文件保存失败")
		return
	}
	// 动图（GIF/动图WebP/APNG）解码只剩首帧，跳过常规缩图管线：
	// GIF 用 ffmpeg 保帧压缩（400 封顶），WebP/APNG 原文件直通图床
	fileURL := ""
	if animatedURL, aerr := processAnimatedImage(filepath, 400); aerr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "动图上传失败" + aerr.Error()})
		return
	} else if animatedURL != "" {
		fileURL = animatedURL
	} else {
		// 压缩头像
		// 打开上传的文件
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件打开失败"})
			return
		}
		defer src.Close()
		// 读取上传文件的内容，并解码为 image.Image 对象
		img, _, err := image.Decode(src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件解码失败"})
			return
		}

		// 对图像进行压缩处理
		resizedImage := imaging.Thumbnail(img, 400, 400, imaging.Lanczos)
		// 这里似乎只支持绝对路径，这里要根据服务器修改
		resizedPath := "public/resized/" + filename
		// 服务器本地备份
		err = imaging.Save(resizedImage, resizedPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图生成保存失败" + err.Error()})
			return
		}

		fileURL, err = api.UploadImage("/src/images/resized/"+filename, resizedPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图生成保存失败" + err.Error()})
			return
		}
	}

	user, err := service.Users.UpdateAvatarByPhone(c.Request.Context(), phone, fileURL)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "User not found")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, "Invalid phone number")
		default:
			response.Response(c, http.StatusInternalServerError, 500, nil, "Database update failed")
		}
		return
	}
	response.Success(c, gin.H{"phone": user.Phone, "avatar_url": user.AvatarURL}, "上传成功")
}

// GetAvatar 用于处理获取用户头像的请求
func GetAvatar(c *gin.Context) {
	phone := c.PostForm("phone")
	user, err := service.Users.GetAvatarByPhone(c.Request.Context(), phone)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, "用户不存在")
		} else {
			response.Response(c, http.StatusInternalServerError, 500, nil, "数据库错误")
		}
		return
	}
	response.Success(c, gin.H{"phone": user.Phone, "AvatarURL": user.AvatarURL}, "获取成功")
}
func GetInfo(c *gin.Context) {
	type getInfoReq struct {
		Phone  string `json:"phone"`
		UserID int    `json:"userID"`
	}
	var req getInfoReq
	c.Bind(&req)
	if req.UserID <= 0 && !util.IsQueryablePhone(req.Phone) {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	user, err := service.Users.GetInfo(c.Request.Context(), req.Phone, req.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusUnprocessableEntity, 422, nil, "用户不存在")
		} else {
			response.Response(c, http.StatusInternalServerError, 500, nil, "数据库错误")
		}
		return
	}
	info := gin.H{
		"userID":    user.ID,
		"name":      user.Name,
		"phone":     user.Phone,
		"intro":     user.Intro,
		"score":     user.Score,
		"ban":       user.BanEnd,
		"punishnum": user.PunishNum,
		"avatarURL": user.AvatarURL,
		// 主页栏板背景是公开展示信息：访问者也用主人设置的不透明度。
		"profileBannerUrl":     user.ProfileBannerURL,
		"profileBannerOpacity": user.ProfileBannerOpacity,
		"hideProfilePosts":     user.HideProfilePosts,
		"customTitle":          user.CustomTitle,
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != 0 && tokenUserID == user.ID {
		info["email"] = user.Email
		info["emailNotificationBlocked"] = user.ISEmailNotificationBlocked
		info["emailpush"] = user.ISEmailNotificationBlocked
		// 币余额属于个人资产口径，只在本人查看自己主页时返回（前端据此决定是否展示）。
		info["coins"] = user.Coins
	}
	c.JSON(http.StatusOK, info)
}

type profileBannerReq struct {
	BannerURL string `json:"bannerUrl"`
	// 不传时保持默认 100；显式 0 表示完全透明。
	BannerOpacity *int `json:"bannerOpacity"`
}

// ProfileBannerSave 保存本人个人主页顶部栏板背景（图床地址 + 不透明度）。
func ProfileBannerSave(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req profileBannerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数格式错误")
		return
	}
	opacity := 100
	if req.BannerOpacity != nil {
		opacity = *req.BannerOpacity
	}
	saved, err := service.Users.UpdateProfileBanner(c.Request.Context(), user.ID, req.BannerURL, opacity)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProfileBannerURLInvalid),
			errors.Is(err, domain.ErrProfileBannerOpacityInvalid):
			response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "用户不存在")
		default:
			apperr.WriteMsg(c, err, "保存主页背景失败")
		}
		return
	}
	response.Success(c, gin.H{
		"profileBannerUrl":     saved.ProfileBannerURL,
		"profileBannerOpacity": saved.ProfileBannerOpacity,
	}, "主页背景已保存")
}

type updateUser struct {
	UserID           int    `json:"userID"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	Name             string `json:"name"`
	Num              int    `json:"num"`
	Intro            string `json:"intro"`
	AvatarURL        string `json:"avatarURL"`
	HideProfilePosts *bool  `json:"hideProfilePosts,omitempty"`
}

func UpdateUserInfo(c *gin.Context) {
	var userInfo updateUser
	if err := c.Bind(&userInfo); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	tokenUserID := GetTokenUserID(c)
	err := service.Users.UpdateUserInfo(c.Request.Context(), domain.UpdateUserInput{
		UserID:           userInfo.UserID,
		Name:             userInfo.Name,
		Intro:            userInfo.Intro,
		AvatarURL:        userInfo.AvatarURL,
		HideProfilePosts: userInfo.HideProfilePosts,
	}, tokenUserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "用户不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		default:
			response.Response(c, http.StatusInternalServerError, 500, nil, "数据库错误")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "用户信息更新成功")
}

func SetCustomTitle(c *gin.Context) {
	var req struct {
		Title string `json:"title"`
	}
	if err := c.Bind(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	if err := service.Users.SetCustomTitle(c.Request.Context(), GetTokenUserID(c), req.Title); err != nil {
		switch {
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "达到 Lv8(5000 经验)后才能自定义称号")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, "称号需为 1~12 个字符")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "用户不存在")
		default:
			response.Response(c, http.StatusInternalServerError, 500, nil, "数据库错误")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "称号设置成功")
}

func CalculateAndSaveScores() {
	service.Users.CalculateAndSaveScores(context.Background())
}

func ChangeEmailPushStatus(c *gin.Context) {
	var user_id UserMsg
	if err := c.Bind(&user_id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := service.Users.ToggleEmailNotification(c.Request.Context(), user_id.UserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
	}
}

type ChatUser struct {
	UserID int `json:"userID"`
	// Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	Identity  string `json:"identity"`
	Score     int    `json:"score"`
}

func GetStatistics(c *gin.Context) {
	userId := GetTokenUserID(c)
	stats, err := service.Users.GetStatistics(c.Request.Context(), userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	relevantRespUsers := make([]ChatUser, 0, len(stats.RelevantUsers))
	for _, u := range stats.RelevantUsers {
		relevantRespUsers = append(relevantRespUsers, ChatUser{
			UserID:    u.UserID,
			Name:      u.Name,
			AvatarURL: u.AvatarURL,
			Identity:  u.Identity,
			Score:     u.Score,
		})
	}
	maxSayUser := ChatUser{
		UserID:    stats.MaxSayUser.UserID,
		Name:      stats.MaxSayUser.Name,
		AvatarURL: stats.MaxSayUser.AvatarURL,
		Identity:  stats.MaxSayUser.Identity,
		Score:     stats.MaxSayUser.Score,
	}
	statistics := gin.H{
		"name":              stats.Name,
		"intro":             stats.Intro,
		"score":             stats.Score,
		"avatarUrl":         stats.AvatarURL,
		"durationTime":      stats.DurationTime,
		"totalPostCnt":      stats.TotalPostCnt,
		"totalPostLikeNum":  stats.TotalPostLikeNum,
		"thisYearPostCnt":   stats.ThisYearPostCnt,
		"thisYearLikeNum":   stats.ThisYearLikeNum,
		"maxLikeNum":        stats.MaxLikeNum,
		"maxBrowseNum":      stats.MaxBrowseNum,
		"maxCommentNum":     stats.MaxCommentNum,
		"pCommentCnt":       stats.PCommentCnt,
		"pCommentLikeNum":   stats.PCommentLikeNum,
		"ccommentCnt":       stats.CcommentCnt,
		"ccommentLikeNum":   stats.CcommentLikeNum,
		"beSavedCount":      stats.BeSavedCount,
		"beCommentedCount":  stats.BeCommentedCount,
		"repliedCount":      stats.RepliedCount,
		"savedCount":        stats.SavedCount,
		"chatCount":         stats.ChatCount,
		"relevantRespUsers": relevantRespUsers,
		"maxSayUser":        maxSayUser,
		"maxSayUserCnt":     stats.MaxSayUserCnt,
	}
	response.Response(c, http.StatusOK, 200, statistics, "获取用户统计信息成功")
}
