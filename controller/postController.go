package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"log"
	"loginTest/api"
	"loginTest/apperr"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/mq"
	"loginTest/response"
	"loginTest/service"
	"loginTest/util"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

const (
	// 课程交流分区名称
	CoursePartition = "课程交流"
)

type PostMsg struct {
	UserTelephone string
	Title         string
	Content       string
	Partition     string
	Photos        string
	TagList       string
	IsPrivate     bool `json:"isPrivate"`
	IsAnonymous   bool `json:"isAnonymous"`
	// Pinned 前端「同时置顶 1 小时」勾选;旧 PostMsg 缺此字段导致永远不置顶。
	Pinned bool `json:"pinned"`
}

func Post(c *gin.Context) {
	var requestPostMsg PostMsg
	c.Bind(&requestPostMsg)
	userTelephone := requestPostMsg.UserTelephone
	title := requestPostMsg.Title
	content := requestPostMsg.Content
	partition := requestPostMsg.Partition
	photos := requestPostMsg.Photos
	tagList := requestPostMsg.TagList
	tags := strings.Split(tagList, "|")

	if len(title) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "标题不能为空")
		return
	}
	if utf8.RuneCountInString(title) > 30 {
		response.Response(c, http.StatusBadRequest, 400, nil, "标题最多为30个字")
		return
	}
	if len(content) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "内容不能为空")
		return
	}
	if utf8.RuneCountInString(content) > config.PostContentMaxRunes {
		response.Response(c, http.StatusBadRequest, 400, nil, fmt.Sprintf("内容最多为%d个字", config.PostContentMaxRunes))
		return
	}
	if len(partition) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "分区不能为空")
		return
	}

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if user.BanEnd.After(time.Now()) {
		response.Response(c, http.StatusBadRequest, 400, nil, "你尚处于禁言状态中，不得发帖")
		return
	}
	if partition == CoursePartition {
		response.Response(c, http.StatusBadRequest, 400, nil, "权限不足")
		return
	}

	createdPost, err := service.Posts.Create(c.Request.Context(), domain.PostCreateInput{
		UserID:      user.ID,
		Title:       title,
		Content:     content,
		Partition:   partition,
		Photos:      photos,
		Tags:        tags,
		IsPrivate:   requestPostMsg.IsPrivate,
		IsAnonymous: requestPostMsg.IsAnonymous,
		// 旧代码未传入 Pinned,前端勾选无效:
		// Pinned 字段缺失
		Pinned: requestPostMsg.Pinned,
	})
	if err == nil && createdPost != nil {
		RagNotify("upsert", "post", createdPost.ID)
	}
	if err != nil {
		var pinAfter *domain.PinAfterCreateError
		if errors.As(err, &pinAfter) {
			createdPost = pinAfter.Post
			// 发帖已成功;继续走归档注释块后返回带置顶失败提示的成功文案。
			// 入延迟归档队列（1h 后由 server-task 消费；若 1h 内删除则由 DeletePost 取消）
			// 暂时停止向 iwiki 提取服务 Redis 队列塞入数据，以节省服务器成本。恢复时取消下方注释即可。
			if createdPost != nil && createdPost.ID != 0 {
			RagNotify("upsert", "post", createdPost.ID)
				// if err := mq.EnqueuePost(c.Request.Context(), createdPost.ID, createdPost.Title); err != nil {
				// 	log.Printf("[archive-mq] EnqueuePost 失败 postID=%d: %v", createdPost.ID, err)
				// }
			}
			msg := "发帖成功，但置顶失败"
			if pinAfter.PinErr != nil {
				msg = "发帖成功，但置顶失败：" + pinAfter.PinErr.Error()
			}
			response.Response(c, http.StatusOK, 200, nil, msg)
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusBadRequest, 400, nil, "权限不足")
			return
		}
		apperr.WriteMsg(c, err, "发帖失败")
		return
	}
	// 入延迟归档队列（1h 后由 server-task 消费；若 1h 内删除则由 DeletePost 取消）
	// 暂时停止向 iwiki 提取服务 Redis 队列塞入数据，以节省服务器成本。恢复时取消下方注释即可。
	if createdPost != nil && createdPost.ID != 0 {
		// if err := mq.EnqueuePost(c.Request.Context(), createdPost.ID, createdPost.Title); err != nil {
		// 	log.Printf("[archive-mq] EnqueuePost 失败 postID=%d: %v", createdPost.ID, err)
		// }
	}
	// 发帖自动生成AI解读:返回新帖ID供前端触发预生成(2026-09-08)
	respData := gin.H{"postID": 0}
	if createdPost != nil {
		respData = gin.H{"postID": createdPost.ID}
	}
	response.Response(c, http.StatusOK, 200, respData, "发帖成功")
}

type PostResponse struct {
	PostID        uint
	Partition     string
	UserID        int
	UserName      string
	UserScore     int
	UserTitle     string
	UserTelephone string
	UserAvatar    string
	UserIdentity  string
	Title         string
	Content       string
	Like          int
	Comment       int
	Browse        int
	Heat          float64
	PostTime      time.Time
	IsSaved       bool
	IsLiked       bool
	IsWatched     bool
	// Coin 该帖被投币总数；MyCoins 我在这帖上累计投了几个（0–2，投币按钮态用）。
	Coin    int `json:"Coin"`
	MyCoins int `json:"MyCoins"`
	Photos  string
	Tag     string
	PostKind string `json:"PostKind"`
	IsPrivate     bool
	IsAnonymous   bool `json:"isAnonymous"`
	IsOwner       bool `json:"isOwner"`
	// IsPinned 是否当前为置顶态(主页用于显示置顶徽章)。
	IsPinned         bool `json:"isPinned"`
	PinnedAt         *time.Time `json:"pinnedAt,omitempty"`
	PinnedExpiresAt  *time.Time `json:"pinnedExpiresAt,omitempty"`
	// HotComment 该帖点赞最多的一级评论(并列按字数倒序),无评论时为 nil。
	HotComment *HotCommentResponse `json:"hotComment,omitempty"`
}

// HotCommentResponse 浏览列表热门评论预览(仅核心字段,避免暴露电话等敏感信息)。
type HotCommentResponse struct {
	ID        int       `json:"id"`
	UserID    int       `json:"userID"`
	UserName  string    `json:"userName"`
	Content   string    `json:"content"`
	LikeNum   int       `json:"likeNum"`
	CreatedAt time.Time `json:"createdAt"`
}

// anonymizeAuthor 对匿名帖的作者信息脱敏：匿名帖对所有查看者（含作者本人）脱敏作者信息，
// 但返回 isOwner 标志让前端能管理自己的帖子。
func anonymizeAuthor(isAnonymous bool, viewerUserID, authorUserID int, name, phone, avatar, identity, title string, score int) (string, string, string, string, string, int, int, bool) {
	isOwner := viewerUserID == authorUserID
	if !isAnonymous {
		return name, phone, avatar, identity, title, score, authorUserID, isOwner
	}
	return "匿名用户", "", "", "", "", 0, 0, isOwner
}

type BrowseMeg struct {
	UserTelephone string `json:"userTelephone"`
	Partition     string `json:"partition"`
	Searchinfo    string `json:"searchinfo"`
	Tag           string `json:"tag"`
	Searchsort    string `json:"searchsort"` // home | history | save | rating
	OrderBy       string `json:"orderBy"`    // time_desc|time_asc|like_desc|like_asc|comment_desc|comment_asc|browse_desc|browse_asc
	TimeFrom      string `json:"timeFrom"`
	TimeTo        string `json:"timeTo"`
	Limit         int    `json:"limit"`  // 默认 15；>15 时不走公共 Feed 缓存
	Offset        int    `json:"offset"` // 0~150 且 limit≤15 时可命中公共 Feed 缓存
	PostType      string `json:"postType"`
}

// Browse 帖子列表接口（POST /api/browse 或 /loadtest/browse）。
//
// 请求体 BrowseMeg 携带 partition/searchinfo/tag/searchsort/orderBy/分页等筛选；
// 登录用户 ID 从 JWT 注入，不参与前端传参。
// 服务层 Browse 当前直查 DB 列表 + 个人态实时组装，见 service/browse_feed.go。
func Browse(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		c.JSON(http.StatusOK, []PostResponse{})
		return
	}
	var requestBrowseMeg BrowseMeg
	if err := c.ShouldBindJSON(&requestBrowseMeg); err != nil {
		_ = c.ShouldBind(&requestBrowseMeg)
	}
	searchsort := requestBrowseMeg.Searchsort
	if searchsort == "" {
		searchsort = "home"
	}
	filter := domain.BrowseFilter{
		ViewerUserID: viewer.ID,
		Partition:    requestBrowseMeg.Partition,
		Searchinfo:   requestBrowseMeg.Searchinfo,
		Tag:          requestBrowseMeg.Tag,
		Searchsort:   searchsort,
		OrderBy:      requestBrowseMeg.OrderBy,
		TimeFrom:     requestBrowseMeg.TimeFrom,
		TimeTo:       requestBrowseMeg.TimeTo,
		Limit:        requestBrowseMeg.Limit,
		Offset:       requestBrowseMeg.Offset,
	}
	items, err := service.Posts.Browse(c.Request.Context(), filter)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	pinnedResponses := make([]PostResponse, 0, len(items.Pinned))
	for _, item := range items.Pinned {
		pinnedResponses = append(pinnedResponses, browseItemToResponse(item, viewer.ID))
	}
	postResponses := make([]PostResponse, 0, len(items.Posts))
	for _, item := range items.Posts {
		postResponses = append(postResponses, browseItemToResponse(item, viewer.ID))
	}
	c.JSON(http.StatusOK, gin.H{
		"pinned": pinnedResponses,
		"posts":  postResponses,
		"total":  items.Total,
	})
}

// PinPostRequest 置顶请求入参。
type PinPostRequest struct {
	PostID int `json:"postID"`
}

// PinPost 置顶当前用户自己的帖子 1 小时。
//
// 权限校验/上限/同帖去重在 service 层完成。
// 错误码:403 无权限;400 业务错误(已置顶/超过 5 个上限等)。
func PinPost(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 403, nil, "未登录")
		return
	}
	var req PinPostRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "postID 无效")
		return
	}
	if err := service.Posts.PinPost(c.Request.Context(), req.PostID, viewer.ID); err != nil {
		switch {
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "无置顶权限")
		case errors.Is(err, domain.ErrPinnedWrongPartition):
			response.Response(c, http.StatusBadRequest, 400, nil, domain.ErrPinnedWrongPartition.Error())
		case errors.Is(err, domain.ErrAlreadyPinned):
			response.Response(c, http.StatusBadRequest, 400, nil, domain.ErrAlreadyPinned.Error())
		case errors.Is(err, domain.ErrPinnedLimitReached):
			response.Response(c, http.StatusBadRequest, 400, nil, domain.ErrPinnedLimitReached.Error())
		case errors.Is(err, domain.ErrGlobalPinnedLimitReached):
			response.Response(c, http.StatusBadRequest, 400, nil, domain.ErrGlobalPinnedLimitReached.Error())
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
		default:
			apperr.WriteMsg(c, err, "置顶失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "置顶成功")
}

// UnpinPost 取消当前用户自己帖子的置顶。
func UnpinPost(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 403, nil, "未登录")
		return
	}
	var req PinPostRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "postID 无效")
		return
	}
	if err := service.Posts.UnpinPost(c.Request.Context(), req.PostID, viewer.ID); err != nil {
		switch {
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "无置顶权限")
		case errors.Is(err, domain.ErrNotPinned):
			response.Response(c, http.StatusBadRequest, 400, nil, domain.ErrNotPinned.Error())
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
		default:
			apperr.WriteMsg(c, err, "取消置顶失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "取消置顶成功")
}

func browseItemToResponse(item domain.BrowseItem, viewerUserID int) PostResponse {
	p, a := item.Post, item.Author
	if p == nil {
		return PostResponse{}
	}
	name, phone, avatar, identity, title, score := "", "", "", "", "", 0
	authorID := 0
	var isOwner bool
	if a != nil {
		name, phone, avatar, identity, title, score = a.Name, a.Phone, a.AvatarURL, a.Identity, a.CustomTitle, a.Score
		authorID = a.ID
	}
	// 匿名帖对所有查看者脱敏作者信息，但返回 isOwner 标志
	name, phone, avatar, identity, title, score, authorID, isOwner = anonymizeAuthor(p.IsAnonymous, viewerUserID, authorID, name, phone, avatar, identity, title, score)

	// 从 Content 中提取 markdown 图片 URL，填充到 Photos（兼容新版帖子无 Photos 字段的情况）
	photos := p.Photos
	if photos == "" {
		photos = extractImageURLs(p.Content)
	}

	return PostResponse{
		PostID:        uint(p.ID),
		Partition:     p.Partition,
		UserID:        authorID,
		UserName:      name,
		UserScore:     score,
		UserTitle:     title,
		UserTelephone: phone,
		UserAvatar:    avatar,
		UserIdentity:  identity,
		Title:         p.Title,
		Content:       util.TruncateRunes(p.Content, util.BrowseContentPreviewRunes),
		Like:          p.LikeNum,
		Comment:       p.CommentNum,
		Coin:          p.CoinNum, // 已含历史投票折算
		MyCoins:       item.MyCoins,
		Browse:        p.BrowseNum,
		Heat:          p.Heat,
		PostTime:      p.PostTime,
		IsSaved:       item.IsSaved,
		IsLiked:       item.IsLiked,
		Photos:        photos,
		Tag:           p.Tag,
		PostKind:      p.PostKind,
		IsPrivate:     p.IsPrivate,
		IsAnonymous:   p.IsAnonymous,
		IsOwner:       isOwner,
		// 旧逻辑直接返回库字段,过期后仍可能显示置顶角标:
		// IsPinned:        p.IsPinned,
		// PinnedAt:        p.PinnedAt,
		// PinnedExpiresAt: p.PinnedExpiresAt,
		IsPinned:        domain.EffectiveIsPinned(p),
		PinnedAt:        p.PinnedAt,
		PinnedExpiresAt: p.PinnedExpiresAt,
		HotComment:      buildHotCommentResponse(item.HotComment, item.HotCommentAuthor, p.IsAnonymous, p.UserID),
	}
}

// buildHotCommentResponse 构造热门评论预览响应。
// - hc 为空时返回 nil,前端 hotComment 字段 omitempty 不输出。
// - 匿名帖场景下,若热门评论作者就是该帖作者,则脱敏作者信息(只留 userID=0、显示"匿名用户")。
func buildHotCommentResponse(hc *domain.Pcomment, author *domain.User, postIsAnonymous bool, postAuthorID int) *HotCommentResponse {
	if hc == nil {
		return nil
	}
	name := ""
	userID := hc.UserID
	if author != nil {
		name = author.Name
	}
	// 匿名帖作者自己评论也脱敏
	if postIsAnonymous && hc.UserID == postAuthorID {
		name = "匿名用户"
		userID = 0
	}
	return &HotCommentResponse{
		ID:        hc.ID,
		UserID:    userID,
		UserName:  name,
		Content:   hc.Content,
		LikeNum:   hc.LikeNum,
		CreatedAt: hc.CreatedAt,
	}
}

// mdImageURLRe 匹配 markdown 图片语法中的 URL
var mdImageURLRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// extractImageURLs 从 markdown 内容中提取图片 URL，最多 9 张，以 | 分割返回
func extractImageURLs(content string) string {
	if content == "" {
		return ""
	}
	matches := mdImageURLRe.FindAllStringSubmatch(content, 9)
	if len(matches) == 0 {
		return ""
	}
	urls := make([]string, 0, len(matches))
	for _, m := range matches {
		url := strings.TrimSpace(m[1])
		if url != "" {
			urls = append(urls, url)
		}
	}
	return strings.Join(urls, "|")
}

type UserProfilePostsMsg struct {
	UserTelephone string `json:"userTelephone"`
	TargetUserID  int    `json:"targetUserID"`
	Limit         int    `json:"limit"`
	Offset        int    `json:"offset"`
}

// UserProfilePosts 个人主页发帖列表；对方开启「隐藏发帖」且浏览者非本人时返回空。
func UserProfilePosts(c *gin.Context) {
	var req UserProfilePostsMsg
	_ = c.Bind(&req)
	if req.TargetUserID <= 0 {
		c.JSON(http.StatusOK, []PostResponse{})
		return
	}
	viewer, ok := viewerFromAuth(c)
	if !ok {
		c.JSON(http.StatusOK, []PostResponse{})
		return
	}
	items, err := service.Posts.UserProfilePosts(c.Request.Context(), domain.UserProfilePostsFilter{
		TargetUserID: req.TargetUserID,
		ViewerUserID: viewer.ID,
		Limit:        req.Limit,
		Offset:       req.Offset,
	})
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	responses := make([]PostResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, browseItemToResponse(item, viewer.ID))
	}
	c.JSON(http.StatusOK, responses)
}

type UserProfilePostNumMsg struct {
	UserTelephone string `json:"userTelephone"`
	TargetUserID  int    `json:"targetUserID"`
}

// UserProfilePostNum 个人主页可展示的发帖总数（不含「打分」）；隐藏时对非本人返回 0。
func UserProfilePostNum(c *gin.Context) {
	var req UserProfilePostNumMsg
	_ = c.Bind(&req)
	var count int64
	if req.TargetUserID <= 0 {
		c.JSON(http.StatusOK, gin.H{"Postcount": count})
		return
	}
	viewer, ok := viewerFromAuth(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"Postcount": count})
		return
	}
	count, err := service.Posts.UserProfilePostNum(c.Request.Context(), req.TargetUserID, viewer.ID)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"Postcount": count})
}

func GetPostNum(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"Postcount": 0})
		return
	}
	var requestBrowseMeg BrowseMeg
	if err := c.ShouldBindJSON(&requestBrowseMeg); err != nil {
		_ = c.ShouldBind(&requestBrowseMeg)
	}
	searchsort := requestBrowseMeg.Searchsort
	if searchsort == "" {
		searchsort = "home"
	}
	filter := domain.BrowseFilter{
		ViewerUserID: viewer.ID,
		Partition:    requestBrowseMeg.Partition,
		Searchinfo:   requestBrowseMeg.Searchinfo,
		Tag:          requestBrowseMeg.Tag,
		Searchsort:   searchsort,
		OrderBy:      requestBrowseMeg.OrderBy,
		TimeFrom:     requestBrowseMeg.TimeFrom,
		TimeTo:       requestBrowseMeg.TimeTo,
	}
	count, err := service.Posts.CountBrowse(c.Request.Context(), filter)
	if err != nil {
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"Postcount": count})
}

type PostPrivacyMsg struct {
	UserTelephone string `json:"userTelephone"`
	PostID        uint   `json:"postID"`
	IsPrivate     bool   `json:"isPrivate"`
}

func UpdatePostPrivacy(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	var req PostPrivacyMsg
	if err := c.Bind(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	err := service.Posts.UpdatePrivacy(c.Request.Context(), int(req.PostID), viewer.ID, req.IsPrivate)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusBadRequest, 400, nil, "帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
			return
		}
		apperr.WriteMsg(c, err, "更新失败")
		return
	}
	if req.IsPrivate {
		RagNotify("delete", "post", int(req.PostID))
	} else {
		RagNotify("upsert", "post", int(req.PostID))
	}
	response.Success(c, gin.H{"isPrivate": req.IsPrivate}, "更新成功")
}

type SaveMsg struct {
	UserTelephone string `json:"userTelephone"`
	PostID        uint   `json:"postID"`
}

func UpdateSave(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	var requestSaveMsg SaveMsg
	if err := c.ShouldBindJSON(&requestSaveMsg); err != nil {
		_ = c.ShouldBind(&requestSaveMsg)
	}
	postID := int(requestSaveMsg.PostID)
	if postID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	isSaved, err := service.Posts.ToggleSave(c.Request.Context(), viewer.ID, postID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		apperr.WriteMsg(c, err, "操作失败")
		return
	}
	msg := "收藏成功"
	if !isSaved {
		msg = "已取消收藏"
	}
	response.Success(c, gin.H{"isSaved": isSaved}, msg)
}

type WatchMsg struct {
	UserTelephone string
	PostID        uint
}

func ToggleWatch(c *gin.Context) {
	var req WatchMsg
	c.Bind(&req)
	postID := int(req.PostID)

	user, ok := requireViewerLegacyPhone(c, req.UserTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	watched, err := service.Posts.ToggleWatch(c.Request.Context(), user.ID, postID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		apperr.WriteMsg(c, err, "操作失败")
		return
	}
	msg := "已码住，有新回复会通知你"
	if !watched {
		msg = "已取消码住"
	}
	response.Success(c, gin.H{"watched": watched}, msg)
}

type LikeMsg struct {
	UserTelephone string
	PostID        uint
	//IsLiked       bool
}

func UpdateLike(c *gin.Context) {
	var requestLikeMsg LikeMsg
	c.Bind(&requestLikeMsg)
	userTelephone := requestLikeMsg.UserTelephone
	postID := int(requestLikeMsg.PostID)

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	err := service.Posts.ToggleLike(c.Request.Context(), user.ID, postID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		apperr.WriteMsg(c, err, "操作失败")
	}
}

type BrowseMsg struct {
	UserTelephone string
	PostID        uint
	PostType      string
	// BrowseNum     int
}

// UpdateBrowseNum 仅记录「该用户是否已浏览过该帖」（Pbrowse）并更新 heat。
// 帖子浏览人次 browse_num 仅由 ShowDetails 中的 HotData.IncrBrowseCount（Redis）及 syncToDB 写入 MySQL，避免双写与随机注水。
func UpdateBrowseNum(c *gin.Context) {
	var requestBrowseMsg BrowseMsg
	c.Bind(&requestBrowseMsg)
	userTelephone := requestBrowseMsg.UserTelephone
	postID := int(requestBrowseMsg.PostID)

	user, ok := requireViewerLegacyPhone(c, userTelephone)
	if !ok {
		return
	}
	_ = service.Posts.RecordBrowse(c.Request.Context(), user.ID, postID)
}

type IDmsg struct {
	PostID   uint
	PostType string
}

func DeletePost(c *gin.Context) {
	var ID IDmsg
	c.Bind(&ID)
	postID := int(ID.PostID)
	tokenUserID := GetTokenUserID(c)
	err := service.Posts.Delete(c.Request.Context(), postID, tokenUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusBadRequest, 400, nil, "需要删除的帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
			return
		}
		apperr.WriteMsg(c, err, "删除失败")
		return
	}
	RagNotify("delete", "post", postID)
	// 1h 内删除帖子：从延迟队列移除该帖子的入队消息（避免触发对已删除帖子的归档）
	if postID != 0 {
		if err := mq.CancelPost(c.Request.Context(), postID); err != nil {
			log.Printf("[archive-mq] CancelPost 失败 postID=%d: %v", postID, err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "帖子删除成功"})
}

type Reportmsg struct {
	TargetID      uint
	Targettype    string
	UserTelephone string
	Reason        string
}

func SubmitReport(c *gin.Context) {
	var reportmsg Reportmsg
	c.Bind(&reportmsg)
	if len(reportmsg.Reason) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "举报内容不能为空")
		return
	}
	user, ok := requireViewerLegacyPhone(c, reportmsg.UserTelephone)
	if !ok {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	err := service.Posts.SubmitReport(c.Request.Context(), domain.SubmitReportInput{
		TargetID:   int(reportmsg.TargetID),
		TargetType: reportmsg.Targettype,
		UserID:     user.ID,
		Reason:     reportmsg.Reason,
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "帖子不存在")
			return
		}
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, "举报内容不能为空")
			return
		}
		apperr.WriteMsg(c, err, "举报失败")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "举报发送成功")
}

type PostDetailsResponse struct {
	PostID        uint
	UserID        uint
	UserName      string
	UserScore     int
	UserTitle     string
	UserTelephone string
	UserAvatar    string
	UserIdentity  string
	Title         string
	Content       string
	Like          int
	Comment       int
	Browse        int
	Heat          float64
	PostTime      time.Time
	IsSaved       bool
	IsLiked       bool
	IsWatched     bool
	// Coin 该帖被投币总数（含历史投票折算）；MyCoins 我累计投了几个（0–2）。
	Coin          int `json:"Coin"`
	MyCoins       int `json:"MyCoins"`
	Photos        string
	Tag           string
	IsPrivate     bool
	IsAnonymous   bool `json:"isAnonymous"`
	IsOwner       bool `json:"isOwner"`
	IsLottery     bool
	// IsPinned 是否当前为置顶态(用于已发帖列表显示置顶/取消置顶按钮)。
	IsPinned         bool `json:"isPinned"`
	PinnedAt         *time.Time `json:"pinnedAt,omitempty"`
	PinnedExpiresAt  *time.Time `json:"pinnedExpiresAt,omitempty"`
}

type PostDetailsMsg struct {
	UserTelephone string `json:"userTelephone"`
	PostID        uint   `json:"postID"`
	PostType      string `json:"postType"`
}

// showDetailStepTimer 单次 ShowDetails 请求内各阶段耗时，结束时由 defer 统一打日志
type showDetailStepTimer struct {
	start  time.Time
	last   time.Time
	steps  []showDetailTimedStep
	postID uint
}

type showDetailTimedStep struct {
	name string
	d    time.Duration
}

func newShowDetailStepTimer() *showDetailStepTimer {
	now := time.Now()
	return &showDetailStepTimer{start: now, last: now}
}

func (s *showDetailStepTimer) setPostID(id uint) {
	s.postID = id
}

func (s *showDetailStepTimer) step(name string) {
	now := time.Now()
	s.steps = append(s.steps, showDetailTimedStep{name: name, d: now.Sub(s.last)})
	s.last = now
}

func (s *showDetailStepTimer) finish() {
	total := time.Since(s.start)
	var b strings.Builder
	b.WriteString("[ShowDetails] postID=")
	b.WriteString(strconv.FormatUint(uint64(s.postID), 10))
	b.WriteString(" total=")
	b.WriteString(total.String())
	for _, st := range s.steps {
		b.WriteString(" ")
		b.WriteString(st.name)
		b.WriteString("=")
		b.WriteString(st.d.String())
	}
	// 与 Gin 默认访问日志一致写到 stdout；标准库 log 默认 stderr，在部分终端/采集里不可见
	fmt.Println(b.String())
}

// ShowDetails 帖子详情：Service 层多级缓存 + 浏览数 Redis 计数。
func ShowDetails(c *gin.Context) {
	timing := newShowDetailStepTimer()
	defer timing.finish()

	var requestPostDetailsMsg PostDetailsMsg
	if err := c.ShouldBindBodyWith(&requestPostDetailsMsg, binding.JSON); err != nil {
		timing.step("bind_body")
		apperr.WriteMsg(c, domain.ErrBadRequest, "参数解析失败")
		return
	}
	timing.step("bind_body")

	postID := requestPostDetailsMsg.PostID
	viewer, ok := viewerFromAuth(c)
	if !ok {
		timing.step("load_viewer_from_auth")
		apperr.WriteMsg(c, domain.ErrNotFound, "无法解析当前用户")
		return
	}
	timing.step("load_viewer_from_auth")

	if postID == 0 {
		timing.setPostID(0)
		timing.step("validate_post_id")
		apperr.WriteMsg(c, domain.ErrBadRequest, "接收到的postID为空")
		return
	}
	timing.setPostID(postID)
	timing.step("validate_post_id")

	detail, err := service.Posts.GetDetail(c.Request.Context(), int(postID), viewer.ID)
	timing.step("service_get_detail")
	if err != nil {
		apperr.WriteMsg(c, err, "帖子不存在")
		return
	}
	timing.step("check_visibility")

	resp := postDetailToResponse(detail, viewer.ID)
	if isLottery, err := service.Lotteries.IsLotteryPost(c.Request.Context(), int(postID)); err == nil {
		resp.IsLottery = isLottery
	}
	c.JSON(http.StatusOK, resp)
	timing.step("write_json")
}

func postDetailToResponse(d *domain.PostDetail, viewerUserID int) PostDetailsResponse {
	p, u := d.Post, d.Author
	name, phone, avatar, identity, title, score := "", "", "", "", "", 0
	userID := 0
	if u != nil {
		name, phone, avatar, identity, title, score = u.Name, u.Phone, u.AvatarURL, u.Identity, u.CustomTitle, u.Score
		userID = u.ID
	}
	// 匿名帖对所有查看者脱敏作者信息，但返回 isOwner 标志
	name, phone, avatar, identity, title, score, userID, isOwner := anonymizeAuthor(p.IsAnonymous, viewerUserID, userID, name, phone, avatar, identity, title, score)
	return PostDetailsResponse{
		PostID:        uint(p.ID),
		UserID:        uint(userID),
		UserName:      name,
		UserScore:     score,
		UserTitle:     title,
		UserTelephone: phone,
		UserAvatar:    avatar,
		UserIdentity:  identity,
		Title:         p.Title,
		Content:       p.Content,
		Like:          p.LikeNum,
		Comment:       p.CommentNum,
		Coin:          p.CoinNum, // 已含历史投票折算
		MyCoins:       d.MyCoins,
		PostTime:      p.PostTime,
		IsSaved:       d.IsSaved,
		IsLiked:       d.IsLiked,
		IsWatched:     d.IsWatched,
		Browse:        d.BrowseNum,
		Heat:          p.Heat,
		Photos:        p.Photos,
		Tag:           p.Tag,
		IsPrivate:     p.IsPrivate,
		IsAnonymous:   p.IsAnonymous,
		IsOwner:       isOwner,
		// 旧逻辑: IsPinned: p.IsPinned — 过期后角标仍为 true
		IsPinned:        domain.EffectiveIsPinned(p),
		PinnedAt:        p.PinnedAt,
		PinnedExpiresAt: p.PinnedExpiresAt,
	}
}

func UploadPhotos(c *gin.Context) {
	// UserID := c.PostForm("UserID")
	// 获取前端传过来的图片
	file, err := c.FormFile("file")
	if err != nil {
		log.Printf("[UploadPhotos] FormFile failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件上传失败"})
		return
	}
	// 文件保存路径和文件名可以根据实际情况修改
	// 文件名我们采用了当前时间戳和原始文件名的组合，以避免文件名冲突
	// 时间戳采用 nanoseconds 级别，可以几乎确保每个文件名都是唯一的
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s", timestamp, file.Filename)
	filepath := "public/uploads/" + filename
	// 保存文件到本地
	err = c.SaveUploadedFile(file, filepath)
	if err != nil {
		log.Printf("[UploadPhotos] SaveUploadedFile failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败"})
		return
	}

	// 生成略缩图
	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		log.Printf("[UploadPhotos] file.Open failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件打开失败"})
		return
	}
	defer src.Close()
	// 读取上传文件的内容，并解码为 image.Image 对象
	img, _, err := image.Decode(src)
	if err != nil {
		log.Printf("[UploadPhotos] image.Decode failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件解码失败" + err.Error()})
		return
	}

	// 对图像进行缩略图处理
	resizedImage := imaging.Fit(img, 1920, 1920, imaging.Lanczos)
	// 这里似乎只支持绝对路径，这里要根据服务器修改
	resizedPath := "public/resized/" + filename
	if err := os.MkdirAll("public/resized", 0o755); err != nil {
		log.Printf("[UploadPhotos] MkdirAll resized failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图目录创建失败" + err.Error()})
		return
	}
	err = imaging.Save(resizedImage, resizedPath)
	if err != nil {
		log.Printf("[UploadPhotos] imaging.Save failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图生成保存失败" + err.Error()})
		return
	}
	fileURL, err := api.UploadImage("/src/images/resized/"+filename, resizedPath)
	if err != nil {
		log.Printf("[UploadPhotos] COS UploadImage(resized) failed: path=%s err=%v", resizedPath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "缩略图上传失败" + err.Error()})
		return
	}

	// 返回成功
	c.JSON(http.StatusOK, gin.H{"fileURL": fileURL, "message": "上传成功"})
}

func UploadZip(c *gin.Context) {
	const maxUploadSize = 10 << 20 // 10 MB

	// 获取前端传过来的文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件上传失败"})
		return
	}

	if file.Size > maxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件太大，不能超过10MB"})
		return
	}

	fileBytes, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取文件"})
		return
	}
	defer fileBytes.Close()

	buffer := make([]byte, 512)
	_, err = fileBytes.Read(buffer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取文件"})
		return
	}

	if http.DetectContentType(buffer) != "application/zip" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件必须是zip格式"})
		return
	}

	// 文件保存路径和文件名可以根据实际情况修改
	// 文件名我们采用了当前时间戳和原始文件名的组合，以避免文件名冲突
	// 时间戳采用 nanoseconds 级别，可以几乎确保每个文件名都是唯一的
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d_%s", timestamp, file.Filename)
	filepath := "public/uploads/" + filename

	// 保存文件到本地
	err = c.SaveUploadedFile(file, filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败"})
		return
	}

	// 更新 Post 的 Photos 字段
	// 我们存储的是可以通过 HTTP 访问的 URL，而不是服务器本地的文件路径
	fileURL := "https://ssemarket.cn/api/uploads/" + filename

	// 返回成功
	c.JSON(http.StatusOK, gin.H{"zipURL": fileURL, "message": "上传成功"})
}

func SubmitFeedback(c *gin.Context) {
	var feedbackInput struct {
		Ftext      string `json:"ftext"`
		Attachment string `json:"attachment"`
	}
	if err := c.Bind(&feedbackInput); err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "Invalid request body")
		return
	}
	feedback, err := service.Posts.SubmitFeedback(c.Request.Context(), feedbackInput.Ftext, feedbackInput.Attachment)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "Failed to submit feedback")
		return
	}
	response.Success(c, gin.H{
		"feedbackID": feedback.ID,
		"ftext":      feedback.Text,
		"attachment": feedback.Attachment,
	}, "Feedback submitted successfully")
}

func GetAllFeedback(c *gin.Context) {
	list, err := service.Posts.ListAllFeedback(c.Request.Context())
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "No feedback found")
			return
		}
		apperr.WriteMsg(c, err, "查询失败")
		return
	}
	feedbacks := make([]domain.Feedback, 0, len(list))
	for _, f := range list {
		feedbacks = append(feedbacks, domain.Feedback{
			ID:         f.ID,
			Text:       f.Text,
			Attachment: f.Attachment,
			Time:       f.Time,
			Status:     f.Status,
		})
	}
	response.Success(c, gin.H{"feedbacks": feedbacks}, "Feedback retrieved successfully")
}

//func GetFeedback(c *gin.Context) {
//	db := common.GetDB()
//
//	feedbackID, err := strconv.Atoi(c.PostForm("feedbackID"))
//	if err != nil {
//		response.Response(c, http.StatusBadRequest, 400, nil, "Invalid feedback ID")
//		return
//	}
//
//	var feedback model.Feedback
//	if err := db.First(&feedback, feedbackID).Error; err != nil {
//		if gorm.IsRecordNotFoundError(err) {
//			response.Response(c, http.StatusNotFound, 404, nil, "Feedback not found")
//		} else {
//			response.Response(c, http.StatusInternalServerError, 500, nil, "Database error")
//		}
//		return
//	}
//
//	response.Success(c, gin.H{"feedback": feedback}, "Feedback retrieved successfully")
//}

type HeatResponse struct {
	PostID uint
	Title  string
	Heat   float64
}

// 表示存在zset的val
type PostInfo struct {
	PostID    uint    `json:"postID"`
	Title     string  `json:"title"`
	BrowseNum float64 `json:"browseNum"`
}

func CalculateHeat(c *gin.Context) {
	items, err := service.Posts.CalculateHeat(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取热度前10帖子失败"})
		return
	}
	heatResponsesTop10 := make([]HeatResponse, 0, len(items))
	for _, item := range items {
		heatResponsesTop10 = append(heatResponsesTop10, HeatResponse{
			PostID: item.PostID,
			Title:  item.Title,
			Heat:   item.Heat,
		})
	}
	c.JSON(http.StatusOK, heatResponsesTop10)
}

func GetTags(c *gin.Context) {
	value, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}
	user := value.(*domain.User)
	if user.ID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户错误")
		return
	}
	tagList, err := service.Posts.ListTags(c.Request.Context())
	if err != nil {
		apperr.WriteMsg(c, err, "获取tag列表失败")
		return
	}
	tags := make([]domain.Tag, 0, len(tagList))
	for _, t := range tagList {
		tags = append(tags, domain.Tag{
			ID:    t.ID,
			Name:  t.Name,
			Value: t.Value,
			Type:  t.Type,
			Num:   t.Num,
		})
	}
	response.Success(c, gin.H{"tags": tags}, "获取tag列表成功！")
}
func GetTitle(c *gin.Context) {
	// 从中间件存入的user中获取userID
	_, exists := c.Get("user")
	if !exists {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}
	text := c.PostForm("text")
	if text == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "请输入正文内容")
		return
	}
	// 构造POST请求的body
	// 使用bytes.Buffer创建一个新的buffer
	var requestBody bytes.Buffer

	// 创建multipart.Writer实例，将文件流写入buffer
	writer := multipart.NewWriter(&requestBody)

	// 写入text字段
	writer.WriteField("text", text)

	// 关闭multipart.Writer以设置边界
	writer.Close()

	// 创建一个通道用于接收异步操作结果
	resultChan := make(chan []byte, 1)

	// 启动goroutine执行异步HTTP请求
	go func() {
		// 向ssemarket.cn:5000/title转发POST请求
		resp, err := http.Post("http://ssemarket.cn:5000/title", writer.FormDataContentType(), &requestBody)
		if err != nil {
			response.Response(c, http.StatusInternalServerError, 500, nil, "请求失败")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			response.Response(c, http.StatusInternalServerError, 500, nil, "请求titlegpt失败")
			return
		}
		// 读取response
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			response.Response(c, http.StatusInternalServerError, 500, nil, "读取响应失败")
			return
		}

		// 将结果发送到通道
		resultChan <- respBody
	}()

	// 设置超时时间，等待结果
	select {
	case result := <-resultChan:
		var r map[string]map[string]string
		err := json.Unmarshal(result, &r)
		if err != nil {
			response.Response(c, http.StatusInternalServerError, 500, nil, "解析响应失败")
			return
		}
		c.JSON(http.StatusOK, gin.H{"title": r["data"]["title"]})
	case <-time.After(60 * time.Second):
		// 如果超时，返回超时信息
		response.Response(c, http.StatusGatewayTimeout, 504, nil, "请求超时")
	}

}

// UploadAttachmentCredential 返回文档附件上传所需的短期凭证与 tus 端点，供前端直连图床断点续传。
// 前端拿到凭证后用 tus-js-client 直连 endpoint 分片上传，本接口不接收文件流。
func UploadAttachmentCredential(c *gin.Context) {
	if ssoAuthUser(c) == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	token, expiresIn, endpoint, err := api.RequestUploadCredential()
	if err != nil {
		log.Printf("[UploadAttachmentCredential] request imagebed credential failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取上传凭证失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_in": expiresIn,
		"endpoint":   endpoint,
	})
}
