package pin

import (
	"context"
	"log"

	"loginTest/service"

	"github.com/robfig/cron/v3"
)

// StartClearExpired 定时清理置顶队列中过期/无效条目(与 ListPinned 入队条件对齐)。
func StartClearExpired() {
	c := cron.New()
	_, _ = c.AddFunc("@every 1m", func() {
		n, err := service.Posts.ClearExpiredPins(context.Background())
		if err != nil {
			log.Printf("[pin] ClearExpiredPins 失败: %v", err)
			return
		}
		if n > 0 {
			log.Printf("[pin] ClearExpiredPins 已清理 %d 条过期/无效置顶", n)
		}
	})
	c.Start()
	log.Println("[pin] 置顶过期清理定时任务已启动（每分钟）")

	// 启动时立刻跑一次,修历史脏数据(如 is_pinned=1 但已过期)。
	if n, err := service.Posts.ClearExpiredPins(context.Background()); err != nil {
		log.Printf("[pin] 启动时 ClearExpiredPins 失败: %v", err)
	} else if n > 0 {
		log.Printf("[pin] 启动时已清理 %d 条过期/无效置顶", n)
	}
}
