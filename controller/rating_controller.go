package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type RatingBrowseMsg struct {
	CategoryID int    `json:"categoryID"`
	Searchinfo string `json:"searchinfo"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type RatingCategoryCreateMsg struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SyncFeedPost *bool  `json:"syncFeedPost"`
}

type RatingPostCreateMsg struct {
	CategoryID int    `json:"categoryID"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Photos     string `json:"photos"`
}

type RatingPostIDMsg struct {
	RatingPostID int `json:"ratingPostID"`
}

type RatingStarSubmitMsg struct {
	RatingPostID int `json:"ratingPostID"`
	StarNum      int `json:"starNum"`
}

type RatingCommentCreateMsg struct {
	RatingPostID int    `json:"ratingPostID"`
	Content      string `json:"content"`
	StarNum      int    `json:"starNum"`
}

type RatingCommentListMsg struct {
	RatingPostID int `json:"ratingPostID"`
}

type RatingLikeMsg struct {
	RatingPostID int `json:"ratingPostID"`
}

type RatingCommentLikeMsg struct {
	CommentID int `json:"commentID"`
}

type RatingCategoryResponse struct {
	CategoryID  int    `json:"categoryID"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PostCount   int    `json:"postCount"`
	IsOfficial  bool   `json:"isOfficial"`
	SortOrder   int    `json:"sortOrder"`
}

type RatingPostListItem struct {
	RatingPostID   int       `json:"ratingPostID"`
	CategoryID     int       `json:"categoryID"`
	CategoryName   string    `json:"categoryName"`
	UserID         int       `json:"userID"`
	UserName       string    `json:"userName"`
	UserScore      int       `json:"userScore"`
	UserTelephone  string    `json:"userTelephone"`
	UserAvatar     string    `json:"userAvatar"`
	UserIdentity   string    `json:"userIdentity"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	Like           int       `json:"like"`
	Comment        int       `json:"comment"`
	Browse         int       `json:"browse"`
	CreatedAt      time.Time `json:"createdAt"`
	IsLiked        bool      `json:"isLiked"`
	Photos         string    `json:"photos"`
	AverageRating  float64   `json:"averageRating"`
	Stars          [5]int    `json:"stars"`
	RaterCount     int       `json:"raterCount"`
}

type RatingPostDetailResponse struct {
	RatingPostListItem
	UserStar int `json:"userStar"`
}

type RatingCommentResponse struct {
	CommentID       int       `json:"commentID"`
	AuthorID        int       `json:"authorID"`
	Author          string    `json:"author"`
	AuthorTelephone string    `json:"authorTelephone"`
	AuthorScore     int       `json:"authorScore"`
	AuthorAvatar    string    `json:"authorAvatar"`
	AuthorIdentity  string    `json:"authorIdentity"`
	CommentTime     time.Time `json:"commentTime"`
	Content         string    `json:"content"`
	LikeNum         int       `json:"likeNum"`
	IsLiked         bool      `json:"isLiked"`
	AuthorRating    int       `json:"authorRating"`
}

func ratingBrowseFilter(msg *RatingBrowseMsg) domain.RatingBrowseFilter {
	return domain.RatingBrowseFilter{
		CategoryID: msg.CategoryID,
		SearchInfo: msg.Searchinfo,
		Limit:      msg.Limit,
		Offset:     msg.Offset,
	}
}

func ratingCategoryToResponse(cat *domain.RatingCategory) RatingCategoryResponse {
	return RatingCategoryResponse{
		CategoryID:  cat.ID,
		Name:        cat.Name,
		Description: cat.Description,
		PostCount:   cat.PostCount,
		IsOfficial:  cat.IsOfficial,
		SortOrder:   cat.SortOrder,
	}
}

func ratingGetUserDisplay(m map[int]*domain.User, id int) *domain.User {
	if u, ok := m[id]; ok {
		return u
	}
	if id == 0 {
		return &domain.User{Name: "管理员", Phone: "11111111111"}
	}
	return &domain.User{}
}

func buildRatingPostListItems(posts []*domain.RatingPost, catMap map[int]*domain.RatingCategory, userMap map[int]*domain.User, likedSet map[int]struct{}, summary map[int]domain.RatingStarSummary) []RatingPostListItem {
	out := make([]RatingPostListItem, 0, len(posts))
	for _, p := range posts {
		u := ratingGetUserDisplay(userMap, p.UserID)
		cat := catMap[p.CategoryID]
		catName := ""
		if cat != nil {
			catName = cat.Name
		}
		sum := summary[p.ID]
		_, isLiked := likedSet[p.ID]
		out = append(out, RatingPostListItem{
			RatingPostID:  p.ID,
			CategoryID:    p.CategoryID,
			CategoryName:  catName,
			UserID:        p.UserID,
			UserName:      u.Name,
			UserScore:     u.Score,
			UserTelephone: u.Phone,
			UserAvatar:    u.AvatarURL,
			UserIdentity:  u.Identity,
			Title:         p.Title,
			Content:       p.Content,
			Like:          p.LikeNum,
			Comment:       p.CommentNum,
			Browse:        p.BrowseNum,
			CreatedAt:     p.CreatedAt,
			IsLiked:       isLiked,
			Photos:        p.Photos,
			AverageRating: sum.AverageRating,
			Stars:         sum.Stars,
			RaterCount:    sum.RaterCount,
		})
	}
	return out
}

func RatingCategoryList(c *gin.Context) {
	ctx := c.Request.Context()
	list, err := service.Ratings.ListCategories(ctx)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	out := make([]RatingCategoryResponse, 0, len(list))
	for _, cat := range list {
		out = append(out, ratingCategoryToResponse(cat))
	}
	c.JSON(http.StatusOK, out)
}

func RatingCategoryCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingCategoryCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	syncFeed := true
	if msg.SyncFeedPost != nil {
		syncFeed = *msg.SyncFeedPost
	}
	cat, err := service.Ratings.CreateCategory(c.Request.Context(), domain.RatingCategoryCreateInput{
		CreatorID:    user.UserID,
		Name:         msg.Name,
		Description:  msg.Description,
		SyncFeedPost: syncFeed,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, ratingCategoryCreateMsg(err, msg))
		case errors.Is(err, domain.ErrConflict):
			response.Response(c, http.StatusBadRequest, 400, nil, "该分类名称已存在")
		default:
			apperr.WriteMsg(c, err, "创建失败")
		}
		return
	}
	c.JSON(http.StatusOK, RatingCategoryResponse{
		CategoryID:  cat.ID,
		Name:        cat.Name,
		Description: cat.Description,
		PostCount:   0,
		IsOfficial:  false,
		SortOrder:   cat.SortOrder,
	})
}

func ratingCategoryCreateMsg(err error, msg RatingCategoryCreateMsg) string {
	name := strings.TrimSpace(msg.Name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 30 {
		return "分类名称需 2～30 个字"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Description)) > 200 {
		return "简介最多 200 字"
	}
	return err.Error()
}

func RatingPostList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingBrowseMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	ctx := c.Request.Context()
	posts, err := service.Ratings.ListPosts(ctx, ratingBrowseFilter(&msg))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	if len(posts) == 0 {
		c.JSON(http.StatusOK, []RatingPostListItem{})
		return
	}
	catMap, userMap, likedSet, summary, err := service.Ratings.EnrichPosts(ctx, posts, user.UserID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, buildRatingPostListItems(posts, catMap, userMap, likedSet, summary))
}

func RatingPostCount(c *gin.Context) {
	var msg RatingBrowseMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	count, err := service.Ratings.CountPosts(c.Request.Context(), ratingBrowseFilter(&msg))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func RatingPostCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingPostCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	id, err := service.Ratings.CreatePost(c.Request.Context(), domain.RatingPostCreateInput{
		CategoryID: msg.CategoryID,
		UserID:     user.UserID,
		Title:      msg.Title,
		Content:    msg.Content,
		Photos:     msg.Photos,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusBadRequest, 400, nil, "分类不存在")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, ratingPostCreateMsg(msg))
		default:
			apperr.Write(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ratingPostID": id})
}

func ratingPostCreateMsg(msg RatingPostCreateMsg) string {
	title := strings.TrimSpace(msg.Title)
	if title == "" {
		return "标题不能为空"
	}
	if utf8.RuneCountInString(title) > 50 {
		return "标题最多 50 个字"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Content)) > 5000 {
		return "内容过长"
	}
	if msg.CategoryID <= 0 {
		return "请选择分类"
	}
	return "参数错误"
}

func RatingPostDetail(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingPostIDMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil || msg.RatingPostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评分帖 ID")
		return
	}
	ctx := c.Request.Context()
	post, userStar, err := service.Ratings.GetPostDetail(ctx, msg.RatingPostID, user.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "评分帖不存在")
			return
		}
		apperr.Write(c, err)
		return
	}
	catMap, userMap, likedSet, summary, err := service.Ratings.EnrichPosts(ctx, []*domain.RatingPost{post}, user.UserID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	items := buildRatingPostListItems([]*domain.RatingPost{post}, catMap, userMap, likedSet, summary)
	if len(items) == 0 {
		response.Response(c, http.StatusNotFound, 404, nil, "评分帖不存在")
		return
	}
	c.JSON(http.StatusOK, RatingPostDetailResponse{RatingPostListItem: items[0], UserStar: userStar})
}

func RatingPostDelete(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingPostIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.RatingPostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评分帖 ID")
		return
	}
	err := service.Ratings.DeletePost(c.Request.Context(), msg.RatingPostID, user.UserID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "评分帖不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "无权删除")
		default:
			apperr.Write(c, err)
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "删除成功")
}

func RatingPostLike(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingLikeMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.RatingPostID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评分帖 ID")
		return
	}
	err := service.Ratings.TogglePostLike(c.Request.Context(), msg.RatingPostID, user.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "操作成功")
}

func RatingStarSubmit(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingStarSubmitMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.RatingPostID <= 0 || msg.StarNum < 1 || msg.StarNum > 5 {
		response.Response(c, http.StatusBadRequest, 400, nil, "评分须为 1～5 星")
		return
	}
	err := service.Ratings.SubmitStar(c.Request.Context(), msg.RatingPostID, user.UserID, msg.StarNum)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "评分帖不存在")
			return
		}
		apperr.Write(c, err)
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "评分成功")
}

func RatingCommentList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingCommentListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.RatingPostID <= 0 {
		c.JSON(http.StatusOK, []RatingCommentResponse{})
		return
	}
	comments, userMap, likeMap, err := service.Ratings.ListComments(c.Request.Context(), msg.RatingPostID, user.UserID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	if len(comments) == 0 {
		c.JSON(http.StatusOK, []RatingCommentResponse{})
		return
	}
	out := make([]RatingCommentResponse, 0, len(comments))
	for _, cm := range comments {
		u := ratingGetUserDisplay(userMap, cm.UserID)
		out = append(out, RatingCommentResponse{
			CommentID:       cm.ID,
			AuthorID:        u.ID,
			Author:          u.Name,
			AuthorTelephone: u.Phone,
			AuthorScore:     u.Score,
			AuthorAvatar:    u.AvatarURL,
			AuthorIdentity:  u.Identity,
			CommentTime:     cm.CreatedAt,
			Content:         cm.Content,
			LikeNum:         cm.LikeNum,
			IsLiked:         likeMap[cm.ID],
			AuthorRating:    cm.StarNum,
		})
	}
	c.JSON(http.StatusOK, out)
}

func RatingCommentCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingCommentCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	cm, notifyReceiver, err := service.Ratings.CreateComment(c.Request.Context(), domain.RatingCommentCreateInput{
		RatingPostID: msg.RatingPostID,
		UserID:       user.UserID,
		Content:      msg.Content,
		StarNum:      msg.StarNum,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "评分帖不存在")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, ratingCommentCreateMsg(msg, user))
		default:
			apperr.Write(c, err)
		}
		return
	}
	if notifyReceiver != nil && notifyReceiver.ID != 0 && !notifyReceiver.ISEmailNotificationBlocked {
		EmailAlerter(notifyReceiver.Email, 1, cm.Content)
	}
	c.JSON(http.StatusOK, RatingCommentResponse{
		CommentID:       cm.ID,
		AuthorID:        user.UserID,
		Author:          user.Name,
		AuthorTelephone: user.Phone,
		AuthorScore:     user.Score,
		AuthorAvatar:    user.AvatarURL,
		AuthorIdentity:  user.Identity,
		CommentTime:     cm.CreatedAt,
		Content:         cm.Content,
		LikeNum:         0,
		IsLiked:         false,
		AuthorRating:    cm.StarNum,
	})
}

func ratingCommentCreateMsg(msg RatingCommentCreateMsg, user model.User) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "评论内容不能为空"
	}
	if utf8.RuneCountInString(content) > 1000 {
		return "评论内容过长"
	}
	if msg.StarNum < 0 || msg.StarNum > 5 {
		return "评分须在 0～5 之间"
	}
	currentDateTime := time.Now()
	if user.Banend.After(currentDateTime) {
		return "你尚处于禁言状态中，不得评论"
	}
	return "参数错误"
}

func RatingCommentLike(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg RatingCommentLikeMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.CommentID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评论 ID")
		return
	}
	err := service.Ratings.ToggleCommentLike(c.Request.Context(), msg.CommentID, user.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return
	}
	if err != nil {
		apperr.Write(c, err)
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "操作成功")
}
