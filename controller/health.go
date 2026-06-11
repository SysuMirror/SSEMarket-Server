package controller

import (
	"context"
	"net/http"
	"time"

	"loginTest/cache"
	"loginTest/common"
	"loginTest/core"

	"github.com/gin-gonic/gin"
)

// Health 存活探针：进程可响应即 200。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready 就绪探针：MySQL + Redis 均可连通才 200。
func Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	db := common.GetDB()
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "db": "uninitialized"})
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "db": err.Error()})
		return
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "db": err.Error()})
		return
	}

	if core.MyRedis == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "redis": "uninitialized"})
		return
	}
	if err := core.MyRedis.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "redis": err.Error()})
		return
	}

	if !cache.IsReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "warmup": "in progress"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
