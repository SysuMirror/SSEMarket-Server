package controller

import (
	"errors"
	"fmt"
	"loginTest/api"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type CommentResponse struct {
	PcommentID      int
	AuthorID        int
	Author          string
	AuthorTelephone string
	AuthorScore     int
	AuthorAvatar    string
	AuthorIdentity  string
	CommentTime     time.Time
	Content         string
	LikeNum         int
	DenyNum         int
	SubComments     []Subcomment
	IsLiked         bool
	IsDenied        bool
}

type Commentsmsg struct {
	UserTelephone string `json:"userTelephone"`
	PostID        int    `json:"postID"`
	TypePost      string `json:"postType"`
}

type Subcomment struct {
	CcommentID      int       `json:"ccommentID"`
	Author          string    `json:"author"`
	AuthorID        int       `json:"authorID"`
	AuthorScore     int       `json:"authorScore"`
	AuthorTelephone string    `json:"authorTelephone"`
	AuthorAvatar    string    `json:"authorAvatar"`
	AuthorIdentity  string    `json:"authorIdentity"`
	CommentTime     time.Time `json:"commentTime"`
	Content         string    `json:"content"`
	LikeNum         int       `json:"likeNum"`
	DenyNum         int       `json:"denyNum"`
	IsLiked         bool      `json:"isLiked"`
	IsDenied        bool      `json:"isDenied"`
	UserTargetName  string    `json:"userTargetName"`
	ShowMenu        bool      `json:"showMenu"`
}

func pcommentWithMetaToResponse(item *domain.PcommentWithMeta) CommentResponse {
	author := item.Author
	if author == nil {
		author = &domain.User{}
	}
	subs := make([]Subcomment, 0, len(item.Subcomments))
	for _, sc := range item.Subcomments {
		subs = append(subs, ccommentWithMetaToSubcomment(sc))
	}
	return CommentResponse{
		PcommentID:      item.Pcomment.ID,
		AuthorID:        author.ID,
		Author:          author.Name,
		AuthorTelephone: author.Phone,
		AuthorScore:     author.Score,
		AuthorAvatar:    author.AvatarURL,
		AuthorIdentity:  author.Identity,
		CommentTime:     item.Pcomment.CreatedAt,
		Content:         item.Pcomment.Content,
		LikeNum:         item.Pcomment.LikeNum,
		DenyNum:         item.Pcomment.DenyNum,
		SubComments:     subs,
		IsLiked:         item.IsLiked,
		IsDenied:        item.IsDenied,
	}
}

func ccommentWithMetaToSubcomment(item *domain.CcommentWithMeta) Subcomment {
	author := item.Author
	if author == nil {
		author = &domain.User{}
	}
	return Subcomment{
		CcommentID:      item.Ccomment.ID,
		Author:          author.Name,
		AuthorID:        author.ID,
		AuthorScore:     author.Score,
		AuthorTelephone: author.Phone,
		AuthorAvatar:    author.AvatarURL,
		AuthorIdentity:  author.Identity,
		CommentTime:     item.Ccomment.CreatedAt,
		Content:         item.Ccomment.Content,
		LikeNum:         item.Ccomment.LikeNum,
		DenyNum:         item.Ccomment.DenyNum,
		IsLiked:         item.IsLiked,
		IsDenied:        item.IsDenied,
		UserTargetName:  item.Ccomment.UserTargetName,
		ShowMenu:        false,
	}
}

// GetComments 给前端返回对应帖子的评论以及每条帖子评论的评论
func GetComments(c *gin.Context) {
	var msg Commentsmsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "服务器无法成功解析请求")
		return
	}
	postid := msg.PostID
	postType := msg.TypePost

	if postid == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "服务器无法成功解析请求")
		return
	}
	if postType != "" && postType != "post" {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	temUser, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusNotFound, 404, nil, "用户不存在")
		return
	}

	items, err := service.Comments.ListForPost(c.Request.Context(), postid, postType, temUser.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		response.Response(c, http.StatusInternalServerError, 500, nil, "获取评论失败")
		return
	}
	if len(items) == 0 {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	comments := make([]CommentResponse, 0, len(items))
	for _, item := range items {
		comments = append(comments, pcommentWithMetaToResponse(item))
	}
	c.JSON(http.StatusOK, comments)
}

type IDmesg struct {
	PcommentID uint
}

func DeletePcomment(c *gin.Context) {
	var ID IDmesg
	c.Bind(&ID)
	err := service.Comments.DeletePcomment(c.Request.Context(), int(ID.PcommentID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusBadRequest, 400, nil, "未找到该评论")
			return
		}
		return
	}
}

type IDmesag struct {
	CcommentID uint
}

func DeleteCcomment(c *gin.Context) {
	var ID IDmesag
	c.Bind(&ID)
	err := service.Comments.DeleteCcomment(c.Request.Context(), int(ID.CcommentID))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusBadRequest, 400, nil, "未找到该评论")
			return
		}
		return
	}
}

type PcommentMsg struct {
	UserTelephone string
	PostID        int
	Content       string
}

func PostPcomment(c *gin.Context) {
	var msg PcommentMsg
	c.Bind(&msg)
	if len(msg.Content) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容不能为空")
		return
	}
	if utf8.RuneCountInString(msg.Content) > 1000 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容过长")
		return
	}
	if len(msg.UserTelephone) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论人不能为空")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), msg.UserTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得评论")
		return
	}

	result, err := service.Comments.CreatePcomment(c.Request.Context(), domain.PcommentCreateInput{
		UserID:  user.ID,
		PostID:  msg.PostID,
		Content: msg.Content,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, "评论内容无效")
			return
		}
		return
	}

	if result.NotifyReceiver != nil && result.NotifyReceiver.ID != 0 && !result.NotifyReceiver.ISEmailNotificationBlocked {
		EmailAlerter(result.NotifyReceiver.Email, 1, msg.Content)
	}

	_ = service.Lotteries.ParticipateByComment(c.Request.Context(), domain.LotteryParticipateInput{
		UserID:    user.ID,
		PostID:    msg.PostID,
		CommentID: result.Pcomment.ID,
		Content:   msg.Content,
	})

	subs := make([]Subcomment, 0, len(result.Subcomments))
	for _, sc := range result.Subcomments {
		subs = append(subs, ccommentWithMetaToSubcomment(sc))
	}
	c.JSON(http.StatusOK, CommentResponse{
		PcommentID:     result.Pcomment.ID,
		Author:         user.Name,
		AuthorAvatar:   user.AvatarURL,
		AuthorIdentity: user.Identity,
		CommentTime:    result.Pcomment.CreatedAt,
		Content:        result.Pcomment.Content,
		LikeNum:        result.Pcomment.LikeNum,
		DenyNum:        result.Pcomment.DenyNum,
		SubComments:    subs,
		IsLiked:        false,
	})
}

type CcommentMsg struct {
	UserTelephone  string `json:"userTelephone"`
	PcommentID     int    `json:"pcommentID"`
	PostID         int    `json:"postID"`
	Content        string `json:"content"`
	UserTargetName string `json:"userTargetName"`
	CcommentID     int    `json:"ccommentID"`
}

func PostCcomment(c *gin.Context) {
	var msg CcommentMsg
	err := c.Bind(&msg)
	if err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "Bind()"+err.Error())
		return
	}
	content := msg.Content
	if len(content) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容不能为空")
		return
	}
	if utf8.RuneCountInString(content) > 1000 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容过长")
		return
	}
	if len(msg.UserTelephone) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论人不能为空")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), msg.UserTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得评论")
		return
	}

	notifies, err := service.Comments.CreateCcomment(c.Request.Context(), domain.CcommentCreateInput{
		UserID:          user.ID,
		PostID:          msg.PostID,
		PcommentID:      msg.PcommentID,
		Content:         content,
		UserTargetName:  msg.UserTargetName,
		ReplyCcommentID: msg.CcommentID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}
	for _, n := range notifies {
		if n.Receiver != nil && n.Receiver.ID != 0 && !n.Receiver.ISEmailNotificationBlocked {
			EmailAlerter(n.Receiver.Email, 3, content)
		}
	}
	response.Response(c, http.StatusOK, 200, nil, "评论成功！")
}

type PcMsg struct {
	UserTelephone string `json:"userTelephone"`
	PcommentID    uint   `json:"pcommentID"`
}

func UpdatePcommentLike(c *gin.Context) {
	var requestMsg PcMsg
	c.Bind(&requestMsg)

	userTelephone := requestMsg.UserTelephone
	pcommentID := requestMsg.PcommentID

	if len(userTelephone) == 0 || pcommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), userTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if _, err := service.Comments.FindPcomment(c.Request.Context(), int(pcommentID)); err != nil {
		return
	}
	if err := service.Comments.ValidatePcommentAccess(c.Request.Context(), int(pcommentID), user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}
	_ = service.Comments.TogglePcommentLike(c.Request.Context(), user.ID, int(pcommentID))
}

func UpdatePcommentDeny(c *gin.Context) {
	var requestMsg PcMsg
	c.Bind(&requestMsg)

	userTelephone := requestMsg.UserTelephone
	pcommentID := requestMsg.PcommentID

	if len(userTelephone) == 0 || pcommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), userTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if _, err := service.Comments.FindPcomment(c.Request.Context(), int(pcommentID)); err != nil {
		return
	}
	if err := service.Comments.ValidatePcommentAccess(c.Request.Context(), int(pcommentID), user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}
	_ = service.Comments.TogglePcommentDeny(c.Request.Context(), user.ID, int(pcommentID))
}

type CcMsg struct {
	UserTelephone string `json:"userTelephone"`
	CcommentID    uint   `json:"ccommentID"`
}

func UpdateCcommentLike(c *gin.Context) {
	var requestMsg CcMsg
	c.Bind(&requestMsg)

	userTelephone := requestMsg.UserTelephone
	ccommentID := requestMsg.CcommentID

	if len(userTelephone) == 0 || ccommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), userTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if _, err := service.Comments.FindCcomment(c.Request.Context(), int(ccommentID)); err != nil {
		return
	}
	if err := service.Comments.ValidateCcommentAccess(c.Request.Context(), int(ccommentID), user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}
	_ = service.Comments.ToggleCcommentLike(c.Request.Context(), user.ID, int(ccommentID))
}

func UpdateCcommentDeny(c *gin.Context) {
	var requestMsg CcMsg
	c.Bind(&requestMsg)

	userTelephone := requestMsg.UserTelephone
	ccommentID := requestMsg.CcommentID

	if len(userTelephone) == 0 || ccommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, err := service.Comments.FindUserByPhone(c.Request.Context(), userTelephone)
	if err != nil || user == nil || user.ID == 0 {
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if _, err := service.Comments.FindCcomment(c.Request.Context(), int(ccommentID)); err != nil {
		return
	}
	if err := service.Comments.ValidateCcommentAccess(c.Request.Context(), int(ccommentID), user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}
	_ = service.Comments.ToggleCcommentDeny(c.Request.Context(), user.ID, int(ccommentID))
}

// EmailAlerter 异步发信，避免 SMTP 握手阻塞 HTTP 响应（评论/回复通知）。
func EmailAlerter(receiver string, status int, msg string) {
	if receiver == "" {
		return
	}
	r, s, m := receiver, status, msg
	go func() {
		if err := api.SendEmail(r, s, m); err != nil {
			fmt.Println("async notify email:", err)
		}
	}()
}
