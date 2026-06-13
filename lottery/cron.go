package lottery

import (
	"context"
	"log"

	"loginTest/service"

	"github.com/robfig/cron/v3"
)

// StartAutoDraw 定时扫描到期抽奖帖并自动开奖
func StartAutoDraw() {
	c := cron.New()
	_, _ = c.AddFunc("@every 1m", func() {
		service.Lotteries.AutoDrawDue(context.Background())
	})
	c.Start()
	log.Println("[lottery] 自动开奖定时任务已启动（每分钟）")
}
