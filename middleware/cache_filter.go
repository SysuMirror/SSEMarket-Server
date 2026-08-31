package middleware

import (
	"encoding/json"
	"io"
	"loginTest/common"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type postIDBody struct {
	PostID int `json:"postID"`
}

// BloomFilterMiddleware 布隆过滤器：全读接口统一校验 postID 是否存在，防缓存穿透。
func BloomFilterMiddleware() gin.HandlerFunc {
	readPaths := map[string]struct{}{
		"/api/auth/showDetails":   {},
		"/api/auth/showPcomments": {},
	}

	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if _, ok := readPaths[path]; !ok {
			c.Next()
			return
		}

		postID, err := extractPostID(c)
		if err != nil || postID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"msg": "参数解析失败"})
			c.Abort()
			return
		}
		if !common.CheckPostExist(postID) {
			msg := "帖子不存在"
			if strings.Contains(path, "comment") {
				msg = "帖子不存在，无评论可查"
			}
			c.JSON(http.StatusNotFound, gin.H{"msg": msg})
			c.Abort()
			return
		}
		c.Next()
	}
}

func extractPostID(c *gin.Context) (int, error) {
	if c.Request.Body == nil {
		return 0, io.EOF
	}
	raw, err := c.GetRawData()
	if err != nil {
		return 0, err
	}
	c.Request.Body = io.NopCloser(strings.NewReader(string(raw)))
	var body postIDBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return 0, err
	}
	return body.PostID, nil
}
