package config

import (
	"os"
)

var (
	EmailUsername  string
	Password       string
	Addr           string
	Host           string
	ValidateTitle  = "校园集市验证邮件"
	ValidateText   = "您正在注册软工集市账户。验证码为vcode, 请在5分钟内输入。请不要将验证码泄露给他人。"
	ValidateSender = "Email Assistant <emailUsername>"

	//通知邮件配置
	NoticeEmailUsername string
	NoticePassword      string
	NoticeAddr          string
	NoticeHost          string
)

// 从环境变量读取邮箱配置
func InitEmailConfig() {
	EmailUsername = os.Getenv("SMTP_EMAIL_USERNAME")
	Password = os.Getenv("SMTP_EMAIL_PASSWORD")
	Addr = os.Getenv("SMTP_ADDR")
	Host = os.Getenv("SMTP_HOST")

	NoticeEmailUsername = os.Getenv("SMTP_NOTICE_EMAIL_USERNAME")
	NoticePassword = os.Getenv("SMTP_NOTICE_EMAIL_PASSWORD")
	NoticeAddr = os.Getenv("SMTP_NOTICE_ADDR")
	NoticeHost = os.Getenv("SMTP_NOTICE_HOST")
}
