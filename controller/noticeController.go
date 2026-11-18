package controller

import (
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type NoticeResponse struct {
	NoticeID     int       `json:"noticeID"`
	ReceiverName string    `json:"receiverName"`
	SenderName   string    `json:"senderName"`
	SenderAvatar string    `json:"senderAvatar"`
	SenderID     int       `json:"senderID"`
	Type         string    `json:"type"`
	Content      string    `json:"content"`
	Read         bool      `json:"read"`
	PostID       int       `json:"postID"`
	Target       int       `json:"target"`
	PcommentID   int       `json:"pcommentID"`
	Time         time.Time `json:"time"`
}
type NoticeGet struct {
	NoticeList     []NoticeResponse `json:"noticeList"`
	LastID         int              `json:"lastID"`
	TotalNum       int              `json:"totalNum"`
	UnreadTotalNum int              `json:"unreadTotalNum"`
}
type NoticeNumResponse struct {
	TotalNum       int `json:"totalNum"`
	UnreadTotalNum int `json:"unreadTotalNum"`
	ReadTotalNum   int `json:"readTotalNum"`
}

func noticeItemToResponse(item domain.NoticeItem) NoticeResponse {
	return NoticeResponse{
		NoticeID:     item.NoticeID,
		ReceiverName: item.ReceiverName,
		SenderName:   item.SenderName,
		SenderAvatar: item.SenderAvatar,
		SenderID:     item.SenderID,
		Type:         item.Type,
		Content:      item.Content,
		Read:         item.Read,
		PostID:       item.PostID,
		Target:       item.Target,
		PcommentID:   item.PcommentID,
		Time:         item.Time,
	}
}

func GetNotice(c *gin.Context) {
	value, exisits := c.Get("user")
	pageSizeStr := c.Query("pageSize")
	requireIDStr := c.Query("requireID")
	readStr := c.Query("read")
	requireID, _ := strconv.Atoi(requireIDStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	read, _ := strconv.Atoi(readStr)
	var user *domain.User
	if !exisits {
		response.Response(c, http.StatusBadRequest, 400, nil, "游客无法访问通知")
		return
	}
	user = value.(*domain.User)

	page, err := service.Notices.List(c.Request.Context(), user.ID, read, requireID, pageSize, user.Name)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}
	if len(page.NoticeList) == 0 {
		response.Response(c, http.StatusOK, 201, nil, "没有更多通知")
		return
	}

	noticeGet := NoticeGet{
		TotalNum:       page.TotalNum,
		LastID:         page.LastID,
		UnreadTotalNum: page.UnreadTotalNum,
	}
	for _, item := range page.NoticeList {
		noticeGet.NoticeList = append(noticeGet.NoticeList, noticeItemToResponse(item))
	}
	c.JSON(http.StatusOK, &noticeGet)
}

func GetNoticeNum(c *gin.Context) {
	value, exisits := c.Get("user")
	var user *domain.User
	if !exisits {
		response.Response(c, http.StatusBadRequest, 400, nil, "游客无法获得通知数量")
		return
	}
	user = value.(*domain.User)

	counts, err := service.Notices.GetCounts(c.Request.Context(), user.ID)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}
	c.JSON(http.StatusOK, &NoticeNumResponse{
		TotalNum:       counts.TotalNum,
		ReadTotalNum:   counts.ReadTotalNum,
		UnreadTotalNum: counts.UnreadTotalNum,
	})
}

func ReadNotice(c *gin.Context) {
	noticeID, _ := strconv.Atoi(c.Param("noticeID"))
	err := service.Notices.MarkRead(c.Request.Context(), noticeID)
	if err != nil {
		c.JSON(500, gin.H{"更新已读出现error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "success"})
}
