package controller

// 内部加精通道：秋坞 post-curator 定时任务 → Go 后端（/internal/mail 反向同款，
// X-Internal-Token 守卫）。
//
// 「精华帖」= posts.is_high_quality。原名「优质贴」，2026-10-14 概念整体转正为
// 「精华帖」：存量 64 篇同字段自动成为精华，查询入参同时认「精华帖」（新）与
// 「优质贴」（旧，兼容保留）。防线：
//  1. INTERNAL_TOKEN 未配置或头不匹配 → 403（fail-closed）
//  2. 只做加精方向（0→1），幂等（已加精自然跳过），不提供撤销——撤销走人工 SQL
//  3. 单批上限 500，防误传超大列表

import (
	"os"
	"strings"
	"sync"

	"loginTest/common"
	"loginTest/model"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

var internalFeaturedOnce sync.Once

func loadInternalFeaturedEnv() {
	internalFeaturedOnce.Do(func() {
		internalToken = strings.TrimSpace(os.Getenv("INTERNAL_TOKEN")) // 与 /internal/mail 共用同一 token
	})
}

// InternalFeatured POST /internal/featured {post_ids: [int]}
func InternalFeatured(c *gin.Context) {
	loadInternalFeaturedEnv()
	if internalToken == "" || c.Request.Header.Get("X-Internal-Token") != internalToken {
		c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "forbidden"})
		return
	}
	var req struct {
		PostIDs []int `json:"post_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.PostIDs) == 0 {
		c.AbortWithStatusJSON(422, gin.H{"code": 422, "msg": "post_ids required"})
		return
	}
	if len(req.PostIDs) > 500 {
		c.AbortWithStatusJSON(422, gin.H{"code": 422, "msg": "post_ids too many (max 500)"})
		return
	}
	// 模型结构体未映射 is_high_quality（全库仅查询分支引用），用列名直更；
	// WHERE IFNULL(...,0)<>1 保证幂等。
	res := common.GetDB().Model(&model.Post{}).
		Where("postID IN (?) AND IFNULL(is_high_quality, 0) <> 1", req.PostIDs).
		Update("is_high_quality", 1)
	if res.Error != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "update failed"})
		return
	}
	if res.RowsAffected > 0 {
		service.InvalidateHomeBrowse() // 主页浏览缓存含加精过滤结果，与置顶变更同款失效
	}
	c.JSON(200, gin.H{"code": 200, "data": gin.H{"marked": res.RowsAffected}, "msg": "featured"})
}
