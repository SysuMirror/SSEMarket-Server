package controller

import (
	"errors"
	"net/http"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

type VoteSearchMsg struct {
	Keyword       string `json:"keyword"`
	UserTelephone string `json:"userTelephone"`
}

type VoteSearchResult struct {
	PostID    int    `json:"postID"`
	Title     string `json:"title"`
	Partition string `json:"partition"`
	PostTime  string `json:"postTime"`
	VoteCount int    `json:"voteCount"`
}

func SearchPostsByTitle(c *gin.Context) {
	var req VoteSearchMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}

	keyword := req.Keyword
	if len(keyword) == 0 {
		response.Success(c, gin.H{"posts": []VoteSearchResult{}}, "ok")
		return
	}

	items, err := service.Votes.SearchByTitle(c.Request.Context(), keyword, user.ID)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "搜索失败")
		return
	}

	results := make([]VoteSearchResult, 0, len(items))
	for _, post := range items {
		results = append(results, VoteSearchResult{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			PostTime:  post.PostTime.Format("2006-01-02 15:04:05"),
			VoteCount: post.VoteCount,
		})
	}

	response.Success(c, gin.H{"posts": results}, "ok")
}

type SubmitVoteMsg struct {
	PostID        int    `json:"postID"`
	UserTelephone string `json:"userTelephone"`
}

func SubmitVote(c *gin.Context) {
	var req SubmitVoteMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}

	voted, err := service.Votes.ToggleVote(c.Request.Context(), user.ID, req.PostID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusBadRequest, 400, nil, "帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrConflict) {
			response.Response(c, http.StatusOK, 400, nil, "您已为该帖投过票")
			return
		}
		apperr.WriteMsg(c, err, "投票失败")
		return
	}

	msg := "投票成功"
	if !voted {
		msg = "已取消投票"
	}
	response.Success(c, gin.H{"voted": voted}, msg)
}

type VoteRankItem struct {
	PostID    int    `json:"postID"`
	Title     string `json:"title"`
	Partition string `json:"partition"`
	VoteCount int    `json:"voteCount"`
	PostTime  string `json:"postTime"`
}

func GetPostRanking(c *gin.Context) {
	items, err := service.Votes.GetPostRanking(c.Request.Context())
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "获取优质榜失败")
		return
	}

	ranking := make([]VoteRankItem, 0, len(items))
	for _, post := range items {
		ranking = append(ranking, VoteRankItem{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			VoteCount: post.VoteCount,
			PostTime:  post.PostTime.Format("2006-01-02 15:04:05"),
		})
	}

	response.Success(c, gin.H{"ranking": ranking}, "ok")
}

type UserRankItem struct {
	UserID          int    `json:"userID"`
	UserName        string `json:"userName"`
	AvatarURL       string `json:"avatarURL"`
	ReceivedLikeNum int    `json:"receivedLikeNum"`
	ReceivedVoteNum int    `json:"receivedVoteNum"`
	RankScore       int    `json:"rankScore"`
}

func GetUserRanking(c *gin.Context) {
	items, err := service.Votes.GetUserRanking(c.Request.Context())
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "获取用户榜失败")
		return
	}

	ranking := make([]UserRankItem, 0, len(items))
	for _, u := range items {
		ranking = append(ranking, UserRankItem{
			UserID:          u.UserID,
			UserName:        u.UserName,
			AvatarURL:       u.AvatarURL,
			ReceivedLikeNum: u.ReceivedLikeNum,
			ReceivedVoteNum: u.ReceivedVoteNum,
			RankScore:       u.RankScore,
		})
	}

	response.Success(c, gin.H{"ranking": ranking}, "ok")
}

func GetVoteRanking(c *gin.Context) {
	GetPostRanking(c)
}

type CheckVotedMsg struct {
	PostID        int    `json:"postID"`
	UserTelephone string `json:"userTelephone"`
}

func CheckUserVoted(c *gin.Context) {
	var req CheckVotedMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}

	voted, err := service.Votes.HasVoted(c.Request.Context(), user.ID, req.PostID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		apperr.WriteMsg(c, err, "查询失败")
		return
	}

	response.Success(c, gin.H{"voted": voted}, "ok")
}
