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
