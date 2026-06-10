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
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")
	_ = viper.BindEnv("crypto.jwtKey", "JWT_KEY")
	_ = viper.BindEnv("cos.secretId", "COS_SECRET_ID")
	_ = viper.BindEnv("cos.secretKey", "COS_SECRET_KEY")
	_ = viper.BindEnv("cos.appid", "COS_APPID")

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}
