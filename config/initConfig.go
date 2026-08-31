package config

import (
	"strings"

	"github.com/spf13/viper"
)

// InitConfig 加载 application.yml，敏感项由环境变量覆盖（12-Factor）。
func InitConfig() {
	viper.SetConfigName("application")
	viper.SetConfigType("yml")
	viper.AddConfigPath("./config")

	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	_ = viper.BindEnv("datasource.password", "DATASOURCE_PASSWORD")
	_ = viper.BindEnv("datasource.username", "DATASOURCE_USERNAME")
	_ = viper.BindEnv("datasource.host", "DATASOURCE_HOST")
	_ = viper.BindEnv("datasource.port", "DATASOURCE_PORT")
	_ = viper.BindEnv("datasource.database", "DATASOURCE_DATABASE")
	_ = viper.BindEnv("datasource.pool.max_open", "DATASOURCE_POOL_MAX_OPEN")
	_ = viper.BindEnv("datasource.pool.max_idle", "DATASOURCE_POOL_MAX_IDLE")
	_ = viper.BindEnv("datasource.pool.conn_max_lifetime", "DATASOURCE_POOL_CONN_MAX_LIFETIME")
	_ = viper.BindEnv("redis.host", "REDIS_HOST")
	_ = viper.BindEnv("redis.port", "REDIS_PORT")
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")
	_ = viper.BindEnv("redis.pool.size", "REDIS_POOL_SIZE")
	_ = viper.BindEnv("redis.pool.min_idle", "REDIS_POOL_MIN_IDLE")
	_ = viper.BindEnv("redis.timeout.dial", "REDIS_DIAL_TIMEOUT")
	_ = viper.BindEnv("redis.timeout.read", "REDIS_READ_TIMEOUT")
	_ = viper.BindEnv("redis.timeout.write", "REDIS_WRITE_TIMEOUT")
	_ = viper.BindEnv("crypto.jwtKey", "JWT_KEY")
	_ = viper.BindEnv("cos.secretId", "COS_SECRET_ID")
	_ = viper.BindEnv("cos.secretKey", "COS_SECRET_KEY")
	_ = viper.BindEnv("cos.appid", "COS_APPID")
	_ = viper.BindEnv("observability.tracing.enabled", "OBSERVABILITY_TRACING_ENABLED")
	_ = viper.BindEnv("observability.tracing.jaeger_endpoint", "OBSERVABILITY_JAEGER_ENDPOINT")
	_ = viper.BindEnv("sso.enabled", "SSO_ENABLED")
	_ = viper.BindEnv("sso.client_id", "SSO_CLIENT_ID")
	_ = viper.BindEnv("sso.client_secret", "SSO_CLIENT_SECRET")
	_ = viper.BindEnv("sso.authorize_url", "SSO_AUTHORIZE_URL")
	_ = viper.BindEnv("sso.token_url", "SSO_TOKEN_URL")
	_ = viper.BindEnv("sso.userinfo_url", "SSO_USERINFO_URL")
	_ = viper.BindEnv("sso.redirect_uri", "SSO_REDIRECT_URI")
	_ = viper.BindEnv("platform.sm2_public_key", "PLATFORM_SM2_PUBLIC_KEY")

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}
