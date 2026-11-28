package api

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/jordan-wright/email"
	"github.com/redis/go-redis/v9"
	"loginTest/core"
	// 	"log"
	"loginTest/config"
)

func formVcode(ctx string) (string, string) {
	vcode := ""
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// 如果生成出错退回到时间戳拼接
			/*
				vcode += strconv.Itoa(int(time.Now().UnixNano()%10))
				continue
			*/
		}
		vcode += strconv.Itoa(int(n.Int64()))
	}
	ctx = strings.Replace(ctx, "{{vcode}}", vcode, -1)
	return ctx, vcode
}

func saveVcode(vcode, receiver string) error {
	rds := core.MyRedis
	if rds == nil {
		return errors.New("验证码服务不可用")
	}
	ctx := context.Background()
	receiver = strings.ToLower(strings.TrimSpace(receiver))
	codeKey := "vcode:" + receiver
	failKey := "vcode_fail:" + receiver
	_, err := rds.Get(ctx, codeKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("检查验证码状态失败: %w", err)
	}
	if err == nil {
		if err := rds.Del(ctx, codeKey).Err(); err != nil {
			return err
		}
	}
	// 重置失败次数
	if err := rds.Del(ctx, failKey).Err(); err != nil {
		return err
	}
	return rds.Set(ctx, codeKey, vcode, 5*time.Minute).Err()
}

// ErrMailNotConfigured SMTP 配置缺失或发件人地址非法。
// 调用方可用 errors.Is 区分「服务没配好」与「用户操作问题」，
// 避免把 net/mail 的原始错误（mail: invalid string）直接甩给用户。
var ErrMailNotConfigured = errors.New("邮件服务未配置（缺少 SMTP_EMAIL_USERNAME/SMTP_EMAIL_PASSWORD/SMTP_ADDR/SMTP_HOST），请联系管理员")

func mailConfigError() error {
	if !config.EmailConfigured() {
		return ErrMailNotConfigured
	}
	sender := strings.Replace(config.Sender, "emailUsername", config.EmailUsername, -1)
	if _, err := mail.ParseAddress(sender); err != nil {
		return fmt.Errorf("%w: 发件人地址非法（%v）", ErrMailNotConfigured, err)
	}
	return nil
}

func SendEmail(receiver string, status int, msg string) error {
	if err := mailConfigError(); err != nil {
		return err
	}
	e := email.NewEmail()
	senderString := config.Sender
	senderString = strings.Replace(senderString, "emailUsername", config.EmailUsername, -1)
	e.From = senderString
	if len([]rune(msg)) > 40 {
		msg = string([]rune(msg)[:40]) + "..."
	}
	e.To = []string{receiver}
	text := ""
	// 验证码先落 Redis 再发信；若发信失败必须回收，否则 5 分钟内重试会被
	// “验证码已发送”挡住，用户会以为注册坏了。
	var rollbackVcode func()
	if status == 0 {
		rds := core.MyRedis
		if rds == nil {
			return errors.New("验证码服务不可用")
		}
		ctx := context.Background()
		receiver = strings.ToLower(strings.TrimSpace(receiver))
		codeKey := "vcode:" + receiver
		_, err := rds.Get(ctx, codeKey).Result()
		// 如果 err == nil，说明找到了验证码（key存在），说明验证码还未过期
		// 如果 err == redis.Nil，说明 key 不存在，可以发送新验证码
		if err == nil {
			return errors.New("验证码已发送，请5分钟后再试")
		}
		// 如果是其他错误（如Redis连接错误），也阻止发送，避免异常情况
		if err != redis.Nil {
			return fmt.Errorf("检查验证码状态失败: %v", err)
		}
		e.Subject = config.ValidateTitle
		text = config.ValidateText
		text, vcode := formVcode(text)
		if err := saveVcode(vcode, receiver); err != nil {
			return fmt.Errorf("保存验证码失败: %w", err)
		}
		rollbackVcode = func() {
			if delErr := rds.Del(context.Background(), codeKey).Err(); delErr != nil {
				fmt.Printf("回收未送达的验证码失败: %v\n", delErr)
			}
		}
		e.HTML = []byte(text)
	} else if status == 1 {
		e.Subject = config.CommentTitle
		text = strings.Replace(config.CommentText, "{{msg}}", msg, -1)
		e.HTML = []byte(text)
	} else if status == 2 {
		e.Subject = config.ChatTitle
		text = config.ChatText
		e.HTML = []byte(text)
	} else if status == 3 {
		e.Subject = config.ReplyTitle
		text = strings.Replace(config.ReplyText, "{{msg}}", msg, -1)
		e.HTML = []byte(text)
	}

	auth := smtp.PlainAuth("", config.EmailUsername, config.Password, config.Host)
	err := e.SendWithTLS(config.Addr, auth, &tls.Config{ServerName: config.Host})
	if err != nil {
		if rollbackVcode != nil {
			rollbackVcode()
		}
		fmt.Println(err)
		return err
	}
	fmt.Println("Send Successfully")
	return nil
}

// SendPlainEmail 发送纯文本邮件(系统报警等内部用途,不走通知模板)。
func SendPlainEmail(receiver, subject, body string) error {
	if err := mailConfigError(); err != nil {
		return err
	}
	e := email.NewEmail()
	senderString := config.Sender
	senderString = strings.Replace(senderString, "emailUsername", config.EmailUsername, -1)
	e.From = senderString
	e.To = []string{receiver}
	e.Subject = subject
	e.Text = []byte(body)
	auth := smtp.PlainAuth("", config.EmailUsername, config.Password, config.Host)
	return e.SendWithTLS(config.Addr, auth, &tls.Config{ServerName: config.Host})
}
