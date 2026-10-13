package controller

import (
	"fmt"

	"loginTest/apperr"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// 排行榜（投币口径）。
//
// 原来的「优质榜 / 用户榜」按投票数排，投票功能下线后改成按投币数排：
// 帖子榜 = 真实投币数 + 历史投票数，作者榜 = 自己帖子与小说收到的币数。

const (
	rankDefaultLimit = 100
	rankMaxLimit     = 100
)

type PostRankItem struct {
	PostID    int    `json:"postID"`
	Title     string `json:"title"`
	Partition string `json:"partition"`
	CoinNum   int    `json:"coinNum"`
	PostTime  string `json:"postTime"`
}

type UserRankItem struct {
	UserID        int    `json:"userID"`
	UserName      string `json:"userName"`
	AvatarURL     string `json:"avatarURL"`
	ReceivedCoins int    `json:"receivedCoins"`
}

func rankLimit(raw string) int {
	limit := rankDefaultLimit
	if raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > rankMaxLimit {
		limit = rankMaxLimit
	}
	return limit
}

// GetPostRanking 帖子投币榜。
func GetPostRanking(c *gin.Context) {
	items, err := service.Ranks.PostCoinRanking(c.Request.Context(), rankLimit(c.Query("limit")))
	if err != nil {
		apperr.WriteMsg(c, err, "获取投币榜失败")
		return
	}
	ranking := make([]PostRankItem, 0, len(items))
	for _, post := range items {
		ranking = append(ranking, PostRankItem{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			CoinNum:   post.CoinNum,
			PostTime:  post.PostTime.Format("2006-01-02 15:04:05"),
		})
	}
	response.Success(c, gin.H{"ranking": ranking}, "ok")
}

// GetUserRanking 作者投币榜。
func GetUserRanking(c *gin.Context) {
	items, err := service.Ranks.UserCoinRanking(c.Request.Context(), rankLimit(c.Query("limit")))
	if err != nil {
		apperr.WriteMsg(c, err, "获取作者榜失败")
		return
	}
	ranking := make([]UserRankItem, 0, len(items))
	for _, u := range items {
		ranking = append(ranking, UserRankItem{
			UserID:        u.UserID,
			UserName:      u.UserName,
			AvatarURL:     u.AvatarURL,
			ReceivedCoins: u.ReceivedCoins,
		})
	}
	response.Success(c, gin.H{"ranking": ranking}, "ok")
}
