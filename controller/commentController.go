package controller

import (
	"errors"
	"fmt"
	"log"
	"loginTest/api"
	"loginTest/apperr"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/mq"
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
	AuthorTitle     string
	AuthorAvatar    string
	AuthorIdentity  string
	CommentTime     time.Time
	Content         string
	LikeNum         int
	DenyNum         int
	SubComments     []Subcomment
	IsLiked         bool
	IsDenied        bool
	// IsOwn 当前查看者可删除这条评论：本人发的，或本人「以职位身份」发的（对外是小号）。
	// 前端据此显示删除入口，避免把真实开发者身份暴露给其它用户。
	IsOwn bool `json:"IsOwn"`
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
	AuthorTitle     string    `json:"authorTitle"`
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
	// IsOwn 含义同 CommentResponse.IsOwn
	IsOwn bool `json:"isOwn"`
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
		AuthorTitle:     author.CustomTitle,
		AuthorAvatar:    author.AvatarURL,
		AuthorIdentity:  author.Identity,
		CommentTime:     item.Pcomment.CreatedAt,
		Content:         item.Pcomment.Content,
		LikeNum:         item.Pcomment.LikeNum,
		DenyNum:         item.Pcomment.DenyNum,
		SubComments:     subs,
		IsLiked:         item.IsLiked,
		IsDenied:        item.IsDenied,
		IsOwn:           item.IsOwn,
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
		AuthorTitle:     author.CustomTitle,
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
		IsOwn:           item.IsOwn,
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
	// 1h 内删除评论：从延迟队列移除该评论的入队消息（避免触发对已删除评论的归档）
	if ID.PcommentID != 0 {
		if err := mq.CancelPcomment(c.Request.Context(), int(ID.PcommentID)); err != nil {
			log.Printf("[archive-mq] CancelPcomment 失败 pcommentID=%d: %v", ID.PcommentID, err)
		}
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
	// AsStaffUserID>0 表示以职位小号身份回复（开发人员匿名 @，见 service/staff_service.go）
	AsStaffUserID int `json:"asStaffUserID"`
}

func PostPcomment(c *gin.Context) {
	var msg PcommentMsg
	c.Bind(&msg)
	if len(msg.Content) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容不能为空")
		return
	}
	if utf8.RuneCountInString(msg.Content) > config.CommentContentMaxRunes {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容过长")
		return
	}
	if len(msg.UserTelephone) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论人不能为空")
		return
	}

	user, ok := requireViewerLegacyPhone(c, msg.UserTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得评论")
		return
	}

	result, err := service.Comments.CreatePcomment(c.Request.Context(), domain.PcommentCreateInput{
		UserID:        user.ID,
		PostID:        msg.PostID,
		Content:       msg.Content,
		AsStaffUserID: msg.AsStaffUserID,
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

	// @小秋：一级评论提及（内部异步，失败静默走兜底回帖；bot 自己的不触发）
	TryHandleAtMention("pcomment", result.Pcomment.ID, msg.PostID, result.Pcomment.ID, user.ID, user.Name, msg.Content)

	if result.NotifyReceiver != nil && result.NotifyReceiver.ID != 0 && !result.NotifyReceiver.ISEmailNotificationBlocked {
		EmailAlerter(result.NotifyReceiver.Email, 1, msg.Content)
	}
	for _, u := range result.PostWatchEmailReceivers {
		emailNotifyIfEnabled(u, 1, msg.Content)
	}

	// 抽奖参与挂在真实账号下；以职位小号身份发言时不参与（对外保持匿名、小号不抽奖）
	if result.Pcomment.UserID == user.ID {
		_ = service.Lotteries.ParticipateByComment(c.Request.Context(), domain.LotteryParticipateInput{
			UserID:    user.ID,
			PostID:    msg.PostID,
			CommentID: result.Pcomment.ID,
			Content:   msg.Content,
		})
	}

	// 入延迟归档队列：评论创建 1h 后触发该帖重新归档（worker 会读完整帖子+所有评论）
	// 暂时停止向 iwiki 提取服务 Redis 队列塞入数据，以节省服务器成本。恢复时取消下方注释即可。
	if result.Pcomment != nil && result.Pcomment.ID != 0 {
		// if err := mq.EnqueuePcomment(c.Request.Context(), msg.PostID, result.Pcomment.ID, msg.Content); err != nil {
		// 	log.Printf("[archive-mq] EnqueuePcomment 失败 postID=%d pcommentID=%d: %v",
		// 		msg.PostID, result.Pcomment.ID, err)
		// }
	}

	subs := make([]Subcomment, 0, len(result.Subcomments))
	for _, sc := range result.Subcomments {
		subs = append(subs, ccommentWithMetaToSubcomment(sc))
	}
	// 作者口径取服务层解析的对外身份（以职位小号回复时这里是小号，不回传真实开发者）
	responseAuthor := result.Author
	if responseAuthor == nil {
		responseAuthor = user
	}
	c.JSON(http.StatusOK, CommentResponse{
		PcommentID:     result.Pcomment.ID,
		AuthorID:       result.Pcomment.UserID,
		Author:         responseAuthor.Name,
		AuthorAvatar:   responseAuthor.AvatarURL,
		AuthorIdentity: responseAuthor.Identity,
		AuthorScore:    responseAuthor.Score,
		AuthorTitle:    responseAuthor.CustomTitle,
		CommentTime:    result.Pcomment.CreatedAt,
		Content:        result.Pcomment.Content,
		LikeNum:        result.Pcomment.LikeNum,
		DenyNum:        result.Pcomment.DenyNum,
		SubComments:    subs,
		IsLiked:        false,
		// 刚发出来的评论当然归本人可删（职位身份发言时也是本人，对外署名是小号）
		IsOwn: true,
	})
}

type CcommentMsg struct {
	UserTelephone  string `json:"userTelephone"`
	PcommentID     int    `json:"pcommentID"`
	PostID         int    `json:"postID"`
	Content        string `json:"content"`
	UserTargetName string `json:"userTargetName"`
	CcommentID     int    `json:"ccommentID"`
	// AsStaffUserID>0 表示以职位小号身份回复（开发人员匿名 @，见 service/staff_service.go）
	AsStaffUserID int `json:"asStaffUserID"`
	TargetUserID  int `json:"targetUserID"`
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
	if utf8.RuneCountInString(content) > config.CommentContentMaxRunes {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论内容过长")
		return
	}
	if len(msg.UserTelephone) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评论人不能为空")
		return
	}

	user, ok := requireViewerLegacyPhone(c, msg.UserTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得评论")
		return
	}

	result, err := service.Comments.CreateCcomment(c.Request.Context(), domain.CcommentCreateInput{
		UserID:          user.ID,
		PostID:          msg.PostID,
		PcommentID:      msg.PcommentID,
		Content:         content,
		UserTargetName:  msg.UserTargetName,
		ReplyCcommentID: msg.CcommentID,
		AsStaffUserID:   msg.AsStaffUserID,
		TargetUserID:    msg.TargetUserID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		return
	}

	// @小秋：楼中楼提及（挂在触发评论的楼下，服务层 ReplyCcommentID 分支负责通知到人）
	TryHandleAtMention("ccomment", result.Ccomment.ID, msg.PostID, msg.PcommentID, user.ID, user.Name, content)

	for _, n := range result.Notifies {
		if n.Receiver != nil && n.Receiver.ID != 0 && !n.Receiver.ISEmailNotificationBlocked {
			EmailAlerter(n.Receiver.Email, 3, content)
		}
	}
	for _, u := range result.PostWatchEmailReceivers {
		emailNotifyIfEnabled(u, 1, content)
	}
	response.Response(c, http.StatusOK, 200, gin.H{"ccommentID": result.Ccomment.ID}, "评论成功！")
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

	if pcommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
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

	if pcommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
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

	if ccommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
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

	if ccommentID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "请求参数有误")
		return
	}

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
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

type ResolveMentionsMsg struct {
	Names []string `json:"names"`
}

type MentionUserResp struct {
	Name   string `json:"name"`
	UserID uint   `json:"userID"`
}

func ResolveMentions(c *gin.Context) {
	var req ResolveMentionsMsg
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Names) == 0 {
		response.Success(c, gin.H{"users": []MentionUserResp{}}, "ok")
		return
	}
	users, err := service.Comments.ResolveMentionUsers(c.Request.Context(), req.Names)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	out := make([]MentionUserResp, 0, len(users))
	for _, u := range users {
		if u == nil || u.ID == 0 {
			continue
		}
		out = append(out, MentionUserResp{Name: u.Name, UserID: uint(u.ID)})
	}
	response.Success(c, gin.H{"users": out}, "ok")
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

func emailNotifyIfEnabled(receiver *domain.User, status int, msg string) {
	if receiver == nil || receiver.ID == 0 || receiver.ISEmailNotificationBlocked || receiver.Email == "" {
		return
	}
	EmailAlerter(receiver.Email, status, msg)
}
