package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// ---------------------------------------------------------------------------
// 请求体
// ---------------------------------------------------------------------------

type NovelListMsg struct {
	Keyword  string `json:"keyword"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

type NovelIDMsg struct {
	NovelID int `json:"novelID"`
}

type NovelCreateMsg struct {
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
	CoverURL   string `json:"coverURL"`
	Category   string `json:"category"`
	Intro      string `json:"intro"`
	Status     string `json:"status"`
}

// NovelUpdateMsg 改书请求：字段为 nil 表示不改。
type NovelUpdateMsg struct {
	NovelID  int     `json:"novelID"`
	Title    *string `json:"title"`
	CoverURL *string `json:"coverURL"`
	Category *string `json:"category"`
	Intro    *string `json:"intro"`
	Status   *string `json:"status"`
}

type NovelChapterListMsg struct {
	NovelID int `json:"novelID"`
	Limit   int `json:"limit"`
	Offset  int `json:"offset"`
}

type NovelChapterIDMsg struct {
	ChapterID int `json:"chapterID"`
}

type NovelChapterCreateMsg struct {
	NovelID    int    `json:"novelID"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	AuthorNote string `json:"authorNote"`
}

type NovelCommentListMsg struct {
	NovelID        int    `json:"novelID"`
	TargetType     string `json:"targetType"`
	ChapterID      int    `json:"chapterID"`
	ParagraphIndex int    `json:"paragraphIndex"`
	Limit          int    `json:"limit"`
	Offset         int    `json:"offset"`
}

type NovelCommentCreateMsg struct {
	NovelID        int    `json:"novelID"`
	TargetType     string `json:"targetType"`
	ChapterID      int    `json:"chapterID"`
	ParagraphIndex int    `json:"paragraphIndex"`
	Content        string `json:"content"`
}

type NovelCommentIDMsg struct {
	CommentID int `json:"commentID"`
}

type NovelProgressMsg struct {
	NovelID        int     `json:"novelID"`
	ChapterID      int     `json:"chapterID"`
	ParagraphIndex int     `json:"paragraphIndex"`
	Percent        float64 `json:"percent"`
}

// ---------------------------------------------------------------------------
// 响应体
// ---------------------------------------------------------------------------

type NovelListItem struct {
	NovelID            int        `json:"novelID"`
	Title              string     `json:"title"`
	AuthorID           int        `json:"authorID"`
	AuthorName         string     `json:"authorName"`
	CoverURL           string     `json:"coverURL"`
	Category           string     `json:"category"`
	Intro              string     `json:"intro"`
	Status             string     `json:"status"`
	ChapterNum         int        `json:"chapterNum"`
	WordNum            int        `json:"wordNum"`
	ReadNum            int        `json:"readNum"`
	ShelfNum           int        `json:"shelfNum"`
	CommentNum         int        `json:"commentNum"`
	LatestChapterID    int        `json:"latestChapterID"`
	LatestChapterTitle string     `json:"latestChapterTitle"`
	LatestChapterAt    *time.Time `json:"latestChapterAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	OnShelf            bool       `json:"onShelf"`
	ProgressPercent    float64    `json:"progressPercent"`
	ProgressChapterID  int        `json:"progressChapterID"`
	ProgressChapterNum int        `json:"progressChapterNum"`
}

type NovelDetailResponse struct {
	NovelListItem
	AuthorAvatar   string `json:"authorAvatar"`
	AuthorScore    int    `json:"authorScore"`
	AuthorIdentity string `json:"authorIdentity"`
	AuthorIntro    string `json:"authorIntro"`
	IsAuthor       bool   `json:"isAuthor"`
}

type NovelChapterItem struct {
	ChapterID  int       `json:"chapterID"`
	NovelID    int       `json:"novelID"`
	ChapterNum int       `json:"chapterNum"`
	Title      string    `json:"title"`
	WordNum    int       `json:"wordNum"`
	CommentNum int       `json:"commentNum"`
	ReadNum    int       `json:"readNum"`
	CreatedAt  time.Time `json:"createdAt"`
}

type NovelParagraphItem struct {
	Index      int    `json:"index"`
	Text       string `json:"text"`
	CommentNum int    `json:"commentNum"`
}

type NovelChapterAdjacentItem struct {
	ChapterID  int    `json:"chapterID"`
	ChapterNum int    `json:"chapterNum"`
	Title      string `json:"title"`
}

type NovelChapterDetailResponse struct {
	Chapter      NovelChapterItem         `json:"chapter"`
	NovelID      int                      `json:"novelID"`
	NovelTitle   string                   `json:"novelTitle"`
	AuthorName   string                   `json:"authorName"`
	Paragraphs   []NovelParagraphItem     `json:"paragraphs"`
	AuthorNote   string                   `json:"authorNote"`
	Prev         *NovelChapterAdjacentItem `json:"prev"`
	Next         *NovelChapterAdjacentItem `json:"next"`
	OnShelf      bool                     `json:"onShelf"`
	ProgressPercent float64               `json:"progressPercent"`
}

type NovelCommentItem struct {
	CommentID        int       `json:"commentID"`
	NovelID          int       `json:"novelID"`
	ChapterID        int       `json:"chapterID"`
	ParagraphIndex   int       `json:"paragraphIndex"`
	TargetType       string    `json:"targetType"`
	ParagraphExcerpt string    `json:"paragraphExcerpt"`
	AuthorID         int       `json:"authorID"`
	Author           string    `json:"author"`
	AuthorTelephone  string    `json:"authorTelephone"`
	AuthorScore      int       `json:"authorScore"`
	AuthorAvatar     string    `json:"authorAvatar"`
	AuthorIdentity   string    `json:"authorIdentity"`
	CommentTime      time.Time `json:"commentTime"`
	Content          string    `json:"content"`
	LikeNum          int       `json:"likeNum"`
	IsLiked          bool      `json:"isLiked"`
}

// ---------------------------------------------------------------------------
// 组装辅助
// ---------------------------------------------------------------------------

func novelBrowseFilter(msg *NovelListMsg) domain.NovelBrowseFilter {
	return domain.NovelBrowseFilter{
		Keyword:  msg.Keyword,
		Category: msg.Category,
		Status:   msg.Status,
		Limit:    msg.Limit,
		Offset:   msg.Offset,
	}
}

func novelChapterToItem(c *domain.NovelChapter) NovelChapterItem {
	return NovelChapterItem{
		ChapterID:  c.ID,
		NovelID:    c.NovelID,
		ChapterNum: c.ChapterNum,
		Title:      c.Title,
		WordNum:    c.WordNum,
		CommentNum: c.CommentNum,
		ReadNum:    c.ReadNum,
		CreatedAt:  c.CreatedAt,
	}
}

func novelAdjacentToItem(a *domain.NovelChapterAdjacent) *NovelChapterAdjacentItem {
	if a == nil {
		return nil
	}
	return &NovelChapterAdjacentItem{ChapterID: a.ID, ChapterNum: a.Num, Title: a.Title}
}

func novelGetUserDisplay(m map[int]*domain.User, id int) *domain.User {
	if u, ok := m[id]; ok {
		return u
	}
	return &domain.User{}
}

func buildNovelListItems(
	novels []*domain.Novel,
	userMap map[int]*domain.User,
	shelfMap map[int]bool,
	progressMap map[int]*domain.NovelProgress,
) []NovelListItem {
	out := make([]NovelListItem, 0, len(novels))
	for _, n := range novels {
		u := novelGetUserDisplay(userMap, n.UserID)
		authorName := n.AuthorName
		if authorName == "" {
			authorName = u.Name
		}
		item := NovelListItem{
			NovelID:            n.ID,
			Title:              n.Title,
			AuthorID:           n.UserID,
			AuthorName:         authorName,
			CoverURL:           n.CoverURL,
			Category:           n.Category,
			Intro:              n.Intro,
			Status:             n.Status,
			ChapterNum:         n.ChapterNum,
			WordNum:            n.WordNum,
			ReadNum:            n.ReadNum,
			ShelfNum:           n.ShelfNum,
			CommentNum:         n.CommentNum,
			LatestChapterID:    n.LatestChapterID,
			LatestChapterTitle: n.LatestChapterTitle,
			LatestChapterAt:    n.LatestChapterAt,
			CreatedAt:          n.CreatedAt,
			UpdatedAt:          n.UpdatedAt,
			OnShelf:            shelfMap[n.ID],
		}
		if p := progressMap[n.ID]; p != nil {
			item.ProgressPercent = p.Percent
			item.ProgressChapterID = p.ChapterID
			item.ProgressChapterNum = p.ChapterNum
		}
		out = append(out, item)
	}
	return out
}

func novelCommentToItem(c *domain.NovelComment, userMap map[int]*domain.User, likedMap map[int]bool) NovelCommentItem {
	u := novelGetUserDisplay(userMap, c.UserID)
	return NovelCommentItem{
		CommentID:        c.ID,
		NovelID:          c.NovelID,
		ChapterID:        c.ChapterID,
		ParagraphIndex:   c.ParagraphIndex,
		TargetType:       c.TargetType,
		ParagraphExcerpt: c.ParagraphExcerpt,
		AuthorID:         u.ID,
		Author:           u.Name,
		AuthorTelephone:  u.Phone,
		AuthorScore:      u.Score,
		AuthorAvatar:     u.AvatarURL,
		AuthorIdentity:   u.Identity,
		CommentTime:      c.CreatedAt,
		Content:          c.Content,
		LikeNum:          c.LikeNum,
		IsLiked:          likedMap[c.ID],
	}
}

func novelWriteError(c *gin.Context, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		response.Response(c, http.StatusNotFound, 404, nil, notFoundMsg)
	case errors.Is(err, domain.ErrNovelNotAuthor):
		response.Response(c, http.StatusForbidden, 403, nil, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		response.Response(c, http.StatusForbidden, 403, nil, "无权操作")
	case errors.Is(err, domain.ErrNovelChapterLimit):
		response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
	case errors.Is(err, domain.ErrBadRequest):
		response.Response(c, http.StatusBadRequest, 400, nil, "参数不符合要求")
	default:
		apperr.Write(c, err)
	}
}

// ---------------------------------------------------------------------------
// 书目
// ---------------------------------------------------------------------------

func NovelList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	ctx := c.Request.Context()

	novels, err := service.Novels.ListNovels(ctx, novelBrowseFilter(&msg))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	if len(novels) == 0 {
		c.JSON(http.StatusOK, gin.H{"novels": []NovelListItem{}, "total": 0})
		return
	}
	userMap, shelfMap, progressMap, err := service.Novels.EnrichNovels(ctx, novels, user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	total, err := service.Novels.CountNovels(ctx, novelBrowseFilter(&msg))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"novels": buildNovelListItems(novels, userMap, shelfMap, progressMap), "total": total})
}

// NovelMine 我写的书（写作页选书、书籍管理入口）。
func NovelMine(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	ctx := c.Request.Context()

	filter := novelBrowseFilter(&msg)
	filter.AuthorFor = user.ID

	novels, err := service.Novels.ListNovels(ctx, filter)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	if len(novels) == 0 {
		c.JSON(http.StatusOK, gin.H{"novels": []NovelListItem{}, "total": 0})
		return
	}
	userMap, shelfMap, progressMap, err := service.Novels.EnrichNovels(ctx, novels, user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	total, err := service.Novels.CountNovels(ctx, filter)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"novels": buildNovelListItems(novels, userMap, shelfMap, progressMap), "total": total})
}

func NovelCategories(c *gin.Context) {
	list, err := service.Novels.Categories(c.Request.Context())
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

func NovelDetail(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelIDMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil || msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	ctx := c.Request.Context()
	novel, author, onShelf, progress, err := service.Novels.GetNovelDetail(ctx, msg.NovelID, user.ID)
	if err != nil {
		novelWriteError(c, err, "小说不存在")
		return
	}
	authorMap := map[int]*domain.User{}
	if author != nil {
		authorMap[author.ID] = author
	}
	item := buildNovelListItems([]*domain.Novel{novel}, authorMap, map[int]bool{novel.ID: onShelf}, map[int]*domain.NovelProgress{})
	if progress != nil {
		item[0].ProgressPercent = progress.Percent
		item[0].ProgressChapterID = progress.ChapterID
		item[0].ProgressChapterNum = progress.ChapterNum
	}
	detail := NovelDetailResponse{NovelListItem: item[0], IsAuthor: novel.UserID == user.ID}
	if author != nil {
		detail.AuthorAvatar = author.AvatarURL
		detail.AuthorScore = author.Score
		detail.AuthorIdentity = author.Identity
		detail.AuthorIntro = author.Intro
		if detail.AuthorName == "" {
			detail.AuthorName = author.Name
		}
	}
	c.JSON(http.StatusOK, detail)
}

func NovelCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	id, err := service.Novels.CreateNovel(c.Request.Context(), domain.NovelCreateInput{
		UserID:     user.ID,
		Title:      msg.Title,
		AuthorName: msg.AuthorName,
		CoverURL:   msg.CoverURL,
		Category:   msg.Category,
		Intro:      msg.Intro,
		Status:     msg.Status,
	})
	if err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, novelCreateMsg(msg))
			return
		}
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"novelID": id})
}

func novelCreateMsg(msg NovelCreateMsg) string {
	title := strings.TrimSpace(msg.Title)
	if utf8.RuneCountInString(title) < 2 {
		return "书名至少 2 个字"
	}
	if utf8.RuneCountInString(title) > 60 {
		return "书名最多 60 个字"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Intro)) > 1000 {
		return "简介最多 1000 字"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Category)) > 20 {
		return "分类名最多 20 个字"
	}
	return "参数错误"
}

// NovelUpdate 作者改书（封面 / 书名 / 分类 / 简介 / 状态），只更新请求里出现的字段。
func NovelUpdate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelUpdateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil || msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	if msg.Title == nil && msg.CoverURL == nil && msg.Category == nil && msg.Intro == nil && msg.Status == nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "没有要修改的内容")
		return
	}
	err := service.Novels.UpdateNovel(c.Request.Context(), domain.NovelUpdateInput{
		NovelID:  msg.NovelID,
		UserID:   user.ID,
		Title:    msg.Title,
		CoverURL: msg.CoverURL,
		Category: msg.Category,
		Intro:    msg.Intro,
		Status:   msg.Status,
	})
	if err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, novelUpdateMsg(msg))
			return
		}
		novelWriteError(c, err, "小说不存在")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "已更新")
}

func novelUpdateMsg(msg NovelUpdateMsg) string {
	if msg.Title != nil {
		title := strings.TrimSpace(*msg.Title)
		if utf8.RuneCountInString(title) < 2 {
			return "书名至少 2 个字"
		}
		if utf8.RuneCountInString(title) > 60 {
			return "书名最多 60 个字"
		}
	}
	if msg.CoverURL != nil && utf8.RuneCountInString(strings.TrimSpace(*msg.CoverURL)) > 500 {
		return "封面链接过长"
	}
	if msg.Intro != nil && utf8.RuneCountInString(strings.TrimSpace(*msg.Intro)) > 1000 {
		return "简介最多 1000 字"
	}
	if msg.Category != nil && utf8.RuneCountInString(strings.TrimSpace(*msg.Category)) > 20 {
		return "分类名最多 20 个字"
	}
	return "参数错误"
}

func NovelDelete(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	if err := service.Novels.DeleteNovel(c.Request.Context(), msg.NovelID, user.ID); err != nil {
		novelWriteError(c, err, "小说不存在")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "已删除")
}

// ---------------------------------------------------------------------------
// 章节
// ---------------------------------------------------------------------------

func NovelChapters(c *gin.Context) {
	if _, ok := requireLoginUser(c); !ok {
		return
	}
	var msg NovelChapterListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	chapters, total, err := service.Novels.ListChapters(c.Request.Context(), msg.NovelID, msg.Limit, msg.Offset)
	if err != nil {
		novelWriteError(c, err, "小说不存在")
		return
	}
	items := make([]NovelChapterItem, 0, len(chapters))
	for _, ch := range chapters {
		items = append(items, novelChapterToItem(ch))
	}
	c.JSON(http.StatusOK, gin.H{"chapters": items, "total": total})
}

func NovelChapter(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelChapterIDMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil || msg.ChapterID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的章节 ID")
		return
	}
	ctx := c.Request.Context()
	detail, err := service.Novels.GetChapterDetail(ctx, msg.ChapterID, user.ID)
	if err != nil {
		novelWriteError(c, err, "章节不存在")
		return
	}
	paragraphs := make([]NovelParagraphItem, 0, len(detail.Paragraphs))
	for _, p := range detail.Paragraphs {
		paragraphs = append(paragraphs, NovelParagraphItem{Index: p.Index, Text: p.Text, CommentNum: p.CommentNum})
	}
	resp := NovelChapterDetailResponse{
		Chapter:    novelChapterToItem(detail.Chapter),
		NovelID:    detail.Chapter.NovelID,
		NovelTitle: detail.NovelTitle,
		AuthorName: detail.AuthorName,
		Paragraphs: paragraphs,
		AuthorNote: detail.Chapter.AuthorNote,
		Prev:       novelAdjacentToItem(detail.Prev),
		Next:       novelAdjacentToItem(detail.Next),
	}
	if onShelf, err := service.Novels.IsOnShelf(ctx, detail.Chapter.NovelID, user.ID); err == nil {
		resp.OnShelf = onShelf
	}
	if progress, err := service.Novels.GetProgress(ctx, detail.Chapter.NovelID, user.ID); err == nil && progress != nil {
		resp.ProgressPercent = progress.Percent
	}
	c.JSON(http.StatusOK, resp)
}

func NovelChapterCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelChapterCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	chapter, err := service.Novels.CreateChapter(c.Request.Context(), domain.NovelChapterCreateInput{
		NovelID:    msg.NovelID,
		UserID:     user.ID,
		Title:      msg.Title,
		Content:    msg.Content,
		AuthorNote: msg.AuthorNote,
	})
	if err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, novelChapterCreateMsg(msg))
			return
		}
		novelWriteError(c, err, "小说不存在")
		return
	}
	c.JSON(http.StatusOK, novelChapterToItem(chapter))
}

func novelChapterCreateMsg(msg NovelChapterCreateMsg) string {
	if strings.TrimSpace(msg.Title) == "" {
		return "章节标题不能为空"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Title)) > 100 {
		return "章节标题最多 100 个字"
	}
	if strings.TrimSpace(msg.Content) == "" {
		return "章节正文不能为空"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.Content)) > 50000 {
		return "单章正文最多 5 万字"
	}
	if utf8.RuneCountInString(strings.TrimSpace(msg.AuthorNote)) > 1000 {
		return "作者有话说最多 1000 字"
	}
	return "参数错误"
}

func NovelChapterDelete(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelChapterIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.ChapterID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的章节 ID")
		return
	}
	if err := service.Novels.DeleteChapter(c.Request.Context(), msg.ChapterID, user.ID); err != nil {
		novelWriteError(c, err, "章节不存在")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "已删除")
}

// ---------------------------------------------------------------------------
// 评论
// ---------------------------------------------------------------------------

func NovelCommentList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelCommentListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 {
		c.JSON(http.StatusOK, gin.H{"comments": []NovelCommentItem{}, "total": 0})
		return
	}
	filter := domain.NovelCommentFilter{
		NovelID:        msg.NovelID,
		TargetType:     msg.TargetType,
		ChapterID:      msg.ChapterID,
		ParagraphIndex: msg.ParagraphIndex,
		Limit:          msg.Limit,
		Offset:         msg.Offset,
	}
	comments, userMap, likedMap, total, err := service.Novels.ListComments(c.Request.Context(), filter, user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	items := make([]NovelCommentItem, 0, len(comments))
	for _, cm := range comments {
		items = append(items, novelCommentToItem(cm, userMap, likedMap))
	}
	c.JSON(http.StatusOK, gin.H{"comments": items, "total": total})
}

func NovelCommentCreate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelCommentCreateMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	cm, author, err := service.Novels.CreateComment(c.Request.Context(), domain.NovelCommentCreateInput{
		NovelID:        msg.NovelID,
		TargetType:     msg.TargetType,
		ChapterID:      msg.ChapterID,
		ParagraphIndex: msg.ParagraphIndex,
		UserID:         user.ID,
		Content:        msg.Content,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "章节不存在，请刷新后重试")
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, novelCommentCreateMsg(msg, user))
		default:
			apperr.Write(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, novelCommentToItem(cm, map[int]*domain.User{author.ID: author}, map[int]bool{}))
}

func novelCommentCreateMsg(msg NovelCommentCreateMsg, user *domain.User) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "评论内容不能为空"
	}
	if utf8.RuneCountInString(content) > 500 {
		return "评论内容最多 500 字"
	}
	switch msg.TargetType {
	case domain.NovelTargetBook, domain.NovelTargetChapter, domain.NovelTargetParagraph:
	default:
		return "评论类型不合法"
	}
	if user.BanEnd.After(time.Now()) {
		return "你尚处于禁言状态中，不得评论"
	}
	return "该段落不存在，请刷新后重试"
}

func NovelCommentLike(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelCommentIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.CommentID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评论 ID")
		return
	}
	if err := service.Novels.ToggleCommentLike(c.Request.Context(), msg.CommentID, user.ID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		apperr.Write(c, err)
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "操作成功")
}

func NovelCommentDelete(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelCommentIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.CommentID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的评论 ID")
		return
	}
	if err := service.Novels.DeleteComment(c.Request.Context(), msg.CommentID, user.ID); err != nil {
		novelWriteError(c, err, "评论不存在")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "已删除")
}

// ---------------------------------------------------------------------------
// 书架与阅读进度
// ---------------------------------------------------------------------------

func NovelShelfToggle(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	added, err := service.Novels.ToggleShelf(c.Request.Context(), msg.NovelID, user.ID)
	if err != nil {
		novelWriteError(c, err, "小说不存在")
		return
	}
	msgText := "已移出书架"
	if added {
		msgText = "已加入书架"
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"onShelf": added}, "msg": msgText})
}

func NovelShelfList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelListMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	ctx := c.Request.Context()
	novels, total, err := service.Novels.ListShelf(ctx, user.ID, msg.Limit, msg.Offset)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	userMap, shelfMap, progressMap, err := service.Novels.EnrichNovels(ctx, novels, user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"novels": buildNovelListItems(novels, userMap, shelfMap, progressMap), "total": total})
}

func NovelProgressReport(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelProgressMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 || msg.ChapterID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的进度参数")
		return
	}
	if err := service.Novels.ReportProgress(c.Request.Context(), domain.NovelProgressInput{
		NovelID:        msg.NovelID,
		UserID:         user.ID,
		ChapterID:      msg.ChapterID,
		ParagraphIndex: msg.ParagraphIndex,
		Percent:        msg.Percent,
	}); err != nil {
		novelWriteError(c, err, "章节不存在")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "已保存进度")
}

func NovelProgressGet(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var msg NovelIDMsg
	_ = c.ShouldBindBodyWith(&msg, binding.JSON)
	if msg.NovelID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "无效的小说 ID")
		return
	}
	progress, err := service.Novels.GetProgress(c.Request.Context(), msg.NovelID, user.ID)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	if progress == nil {
		c.JSON(http.StatusOK, gin.H{"progress": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"progress": gin.H{
		"chapterID":      progress.ChapterID,
		"chapterNum":     progress.ChapterNum,
		"paragraphIndex": progress.ParagraphIndex,
		"percent":        progress.Percent,
		"updatedAt":      progress.UpdatedAt,
	}})
}
