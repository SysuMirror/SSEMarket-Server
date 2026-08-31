package config

import (
	"strings"

	"github.com/spf13/viper"
)

// GetPlatformSM2PublicKey 平台 SM2 公钥（hex，04 开头非压缩格式）。
// 与集市开发者平台 server/.env 的 PLATFORM_SM2_PUBLIC_KEY 同源；轮换时同步更新。
func GetPlatformSM2PublicKey() string {
	return strings.TrimSpace(viper.GetString("platform.sm2_public_key"))
}
