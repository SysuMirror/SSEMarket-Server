package controller

// 内部回帖通道：小秋前厅（monitor / notify）以 bot 账号在论坛发回复。
// 只用 INTERNAL_TOKEN 鉴权；nginx 不路由 /internal/*。

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"loginTest/domain"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

type xqReplyInput struct {
	PostID     int    `json:"post_id"`
	PcommentID int    `json:"pcomment_id"`
	Content    string `json:"content"`
}

func InternalXiaoqiuReply(c *gin.Context) {
	token := strings.TrimSpace(os.Getenv("INTERNAL_TOKEN"))
	if token == "" || strings.TrimSpace(c.GetHeader("X-Internal-Token")) != token {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if xqBotUID <= 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bot_not_configured"})
		return
	}
	var in xqReplyInput
	if err := json.NewDecoder(c.Request.Body).Decode(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_json"})
		return
	}
	content := xqAtRuneTrunc(strings.TrimSpace(in.Content), xqAtReplyMaxRunes-1)
	if in.PostID <= 0 || content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "post_id_and_content_required"})
		return
	}
	ctx := c.Request.Context()
	if _, err := service.Posts.FindByID(ctx, in.PostID, xqBotUID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "post_not_found"})
		return
	}
	if in.PcommentID > 0 {
		if _, err := service.Comments.FindPcomment(ctx, in.PcommentID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "comment_not_found"})
			return
		}
		_, err := service.Comments.CreateCcomment(ctx, domain.CcommentCreateInput{
			UserID:     xqBotUID,
			PostID:     in.PostID,
			PcommentID: in.PcommentID,
			Content:    content,
		})
		if err != nil {
			log.Printf("[xq-reply] Ccomment post=%d pc=%d failed: %v", in.PostID, in.PcommentID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
			return
		}
	} else {
		_, err := service.Comments.CreatePcomment(ctx, domain.PcommentCreateInput{
			UserID:  xqBotUID,
			PostID:  in.PostID,
			Content: content,
		})
		if err != nil {
			log.Printf("[xq-reply] Pcomment post=%d failed: %v", in.PostID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create_failed"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"ok": true}})
}
