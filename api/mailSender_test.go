package api

import (
	"errors"
	"strings"
	"testing"

	"loginTest/config"
)

func TestMailConfigError(t *testing.T) {
	origUser, origPass, origAddr, origHost := config.EmailUsername, config.Password, config.Addr, config.Host
	t.Cleanup(func() {
		config.EmailUsername, config.Password, config.Addr, config.Host = origUser, origPass, origAddr, origHost
	})

	// SMTP 未配置时必须返回可识别的错误，而不是让 net/mail 抛 “mail: invalid string”。
	config.EmailUsername, config.Password, config.Addr, config.Host = "", "", "", ""
	err := mailConfigError()
	if !errors.Is(err, ErrMailNotConfigured) {
		t.Fatalf("want ErrMailNotConfigured, got %v", err)
	}
	if !strings.Contains(err.Error(), "SMTP_EMAIL_USERNAME") {
		t.Fatalf("error should name the missing env vars, got %v", err)
	}

	// 配齐四项、发件人模板能解析时应通过。
	config.EmailUsername = "ssemarket@163.com"
	config.Password = "secret"
	config.Addr = "smtp.163.com:465"
	config.Host = "smtp.163.com"
	if err := mailConfigError(); err != nil {
		t.Fatalf("configured sender should pass, got %v", err)
	}
}
