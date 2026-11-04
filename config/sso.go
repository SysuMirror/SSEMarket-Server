package config

import "github.com/spf13/viper"

// SSO（UniSSO）配置读取。client_secret 仅后端持有，禁止入 git/前端/日志。

func SsoEnabled() bool        { return viper.GetBool("sso.enabled") }
func SsoClientID() string     { return viper.GetString("sso.client_id") }
func SsoClientSecret() string { return viper.GetString("sso.client_secret") }
func SsoAuthorizeURL() string { return viper.GetString("sso.authorize_url") }
func SsoTokenURL() string     { return viper.GetString("sso.token_url") }
func SsoUserinfoURL() string  { return viper.GetString("sso.userinfo_url") }
func SsoRedirectURI() string  { return viper.GetString("sso.redirect_uri") }
func SsoDiscoveryURL() string { return viper.GetString("sso.discovery_url") }

// SsoPasswordVerifyURL M2M 邮箱密码校验端点（登录主路径，无跳转）。
func SsoPasswordVerifyURL() string { return viper.GetString("sso.password_verify_url") }

// 静默 SSO（共享父域 Cookie）端点与 Cookie 兜底参数。
// 登录/静默校验以 unisso 响应里的 sso_cookie_* 为准；这里的默认值只用于登出清 Cookie，
// 避免 unisso 不可达时无法清掉浏览器里的会话指针。
func SsoSessionVerifyURL() string { return viper.GetString("sso.session_verify_url") }
func SsoSessionRevokeURL() string { return viper.GetString("sso.session_revoke_url") }

func SsoCookieName() string {
	if v := viper.GetString("sso.cookie_name"); v != "" {
		return v
	}
	return "unisso_sid"
}

// SsoCookieNameLegacy 旧共享 Cookie 名。共享 Cookie 改名 sse_sso -> unisso_sid 后，
// 浏览器里两个名字都可能存在（unisso 切默认名前签发的一律是旧名），
// 读取与清除都要兼顾两个名字；存量旧 Cookie 全部过期后可移除。
func SsoCookieNameLegacy() string { return "sse_sso" }

func SsoCookieDomain() string {
	if v := viper.GetString("sso.cookie_domain"); v != "" {
		return v
	}
	return ".ssemarket.cn"
}

func SsoCookieMaxAge() int {
	if v := viper.GetInt("sso.cookie_max_age"); v > 0 {
		return v
	}
	return 604800
}
