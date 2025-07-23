package api

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/jordan-wright/email"

	// 	"log"
	"loginTest/common"
	"loginTest/config"
	"math/rand"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func formVcode(ctx string) (string, string) {
	rand.Seed(time.Now().Unix()) // unix 时间戳，秒
	data := rand.Int() % 1000000
	vcode := ""
	for i := 1; i <= 6; i++ {
		vcode += strconv.Itoa(data % 10)
		data = data / 10
	}
	ctx = strings.Replace(ctx, "vcode", vcode, -1)
	fmt.Println(ctx)
	fmt.Println(vcode)
	return ctx, vcode
}

func saveVcode(vcode, receiver string) {
	rds := common.MyRedis
	ctx := context.Background()
	_, err := rds.Get(ctx, receiver).Result()
	if err == nil {
		rds.Del(ctx, receiver)
	}
	rds.Set(ctx, receiver, vcode, 5*time.Minute)
}

func SendEmail(receiver string, status int ,msg string) error {
	e := email.NewEmail()
	senderString := config.ValidateSender
	senderString = strings.Replace(senderString, "emailUsername", config.EmailUsername, -1)
	e.From = senderString
	if len(strings.Split(msg, "")) > 20 {
		msg = msg[:20] + "..."
	}
	e.To = []string{receiver}
	e.Subject = config.ValidateTitle
	text := ""
	if status == 0 {
		text = config.ValidateText
	} else if status == 1 {
		e.Subject = "软工集市评论通知"
		text = "您好，您在软工集市上的发帖收到一条新评论'"+ msg +"'，请登录平台查阅。 如需关闭邮箱通知请前往个人主页最下方关闭。"
	} else if status == 2 {
		e.Subject = "软工集市私信通知"
		text = "您好，您在软工集市上收到一条新私信，请登录平台查阅。 如需关闭邮箱通知请前往个人主页最下方关闭。"
	} else if status == 3 {
		e.Subject = "软工集市回复通知"
		text = "您好，您在软工集市上收到一条新回复'"+ msg +"'，请登录平台查阅。 如需关闭邮箱通知请前往个人主页最下方关闭。"
	}

	text, vcode := formVcode(text)
	saveVcode(vcode, receiver)
	e.Text = []byte(text)
	auth := smtp.PlainAuth("", config.EmailUsername, config.Password, config.Host)
	err := e.SendWithTLS(config.Addr, auth, &tls.Config{ServerName: config.Host})
	if err != nil {
		fmt.Println(err)
		return err
	}
	fmt.Println("Send Successfully")
	return nil
}
