package controller

import (
	"loginTest/config"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"net/smtp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jordan-wright/email"
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
	var user model.User
	if !exisits {
		response.Response(c, http.StatusBadRequest, 400, nil, "游客无法访问通知")
		return
	}
	user = value.(model.User)

	page, err := service.Notices.List(c.Request.Context(), user.UserID, read, requireID, pageSize, user.Name)
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
	var user model.User
	if !exisits {
		response.Response(c, http.StatusBadRequest, 400, nil, "游客无法获得通知数量")
		return
	}
	user = value.(model.User)

	counts, err := service.Notices.GetCounts(c.Request.Context(), user.UserID)
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

func EmailNotice(c *gin.Context) {
	value, exists := c.Get("admin")
	var admin model.Admin
	if !exists {
		response.Fail(c, nil, "验证错误")
		return
	}
	admin = value.(model.Admin)
	if admin.AdminID == 0 {
		response.Fail(c, nil, "管理员不存在")
	}

	if config.NoticeEmailUsername == "" || config.NoticePassword == "" || config.NoticeAddr == "" || config.NoticeHost == "" {
		response.Fail(c, nil, "通知邮件配置未正确初始化")
		return
	}

	e := email.NewEmail()
	e.From = "软工集市 <" + config.NoticeEmailUsername + ">"
	e.To = []string{config.NoticeEmailUsername}
	e.Subject = "软工集市version2.0升级部署通知"
	e.HTML = []byte("<!DOCTYPE html>\n<html>\n<head>\n    <title>软工集市平台升级维护通知</title>\n</head>\n<body>\n    <h1>软工集市平台升级维护通知</h1>\n    <p>尊敬的软工集市用户，</p>\n    <p>为了提供更好的服务和功能，我们计划进行软工集市平台的升级维护。这次升级将带来一些重要的改进和新功能，以增强您的使用体验。</p>\n    <p>升级维护计划如下：</p>\n    <ul>\n        <li>维护开始时间：[维护开始日期和时间]</li>\n        <li>维护结束时间：[维护结束日期和时间]</li>\n        <li>期间软工集市将无法访问，为了不影响您的工作，请在维护前备份您的数据。</li>\n    </ul>\n    <p>升级后，您可以期待以下改进：</p>\n    <ul>\n        <li>[列出将引入的新功能和改进]</li>\n        <li>[列出其他相关信息]</li>\n    </ul>\n    <p>我们对给您带来的不便深感抱歉，但我们相信这些改进将使您更加满意。在维护期间，如果您有任何疑问或需要帮助，请随时联系我们的客户支持团队。</p>\n    <p>感谢您一直以来对软工集市的支持和信任。</p>\n    <p>祝您使用愉快！</p>\n    <p>谨启</p>\n    <p>软工集市团队</p>\n</body>\n</html>\n")
	err := e.Send(config.NoticeAddr, smtp.PlainAuth("", config.NoticeEmailUsername, config.NoticePassword, config.NoticeHost))
	if err != nil {
		response.Fail(c, nil, "邮件发送失败: "+err.Error())
		return
	}
	response.Success(c, nil, "通知邮件发送成功")
}
