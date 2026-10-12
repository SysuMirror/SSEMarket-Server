package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/repository"
)

// 小说板块内容上限。
const (
	novelTitleMaxRunes    = 60
	novelAuthorNameMax    = 30
	novelCategoryMaxRunes = 20
	novelIntroMaxRunes    = 1000
	novelChapterTitleMax  = 100
	novelChapterMaxRunes  = 50000
	novelAuthorNoteMax    = 1000
	novelCommentMaxRunes  = 500
	novelCoverURLMaxRunes = 500
)

type novelService struct {
	novels repository.NovelRepository
	// points 积分/经验发放；coins 币发放。两者都允许为 nil（单测与未开通的场景），
	// 所有调用都做了 nil 判断。
	points NovelPointsService
	coins  CoinService
}

func NewNovelService(novels repository.NovelRepository, points NovelPointsService, coins CoinService) NovelService {
	return &novelService{novels: novels, points: points, coins: coins}
}

// ---------------------------------------------------------------------------
// 书目
// ---------------------------------------------------------------------------

func (s *novelService) ListNovels(ctx context.Context, filter domain.NovelBrowseFilter) ([]*domain.Novel, error) {
	return s.novels.ListNovels(ctx, filter)
}

func (s *novelService) CountNovels(ctx context.Context, filter domain.NovelBrowseFilter) (int64, error) {
	return s.novels.CountNovels(ctx, filter)
}

func (s *novelService) Categories(ctx context.Context) ([]domain.NovelCategory, error) {
	return s.novels.CategoriesWithCount(ctx)
}

// GetNovelDetail 书籍详情页：书本 + 作者 + 是否在书架 + 我的阅读进度。进入详情计一次阅读量。
func (s *novelService) GetNovelDetail(ctx context.Context, novelID, viewerID int) (*domain.Novel, *domain.User, bool, *domain.NovelProgress, error) {
	novel, err := s.novels.FindNovelByID(ctx, novelID)
	if err != nil {
		return nil, nil, false, nil, err
	}
	_ = s.novels.IncrNovelRead(ctx, novelID)
	novel.ReadNum++
	s.grantReaderUV(ctx, novel, viewerID)

	author, err := s.novels.FindUserByID(ctx, novel.UserID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, nil, false, nil, err
	}

	onShelf, err := s.novels.IsOnShelf(ctx, novelID, viewerID)
	if err != nil {
		return nil, nil, false, nil, err
	}
	progress, err := s.novels.FindProgress(ctx, novelID, viewerID)
	if err != nil {
		return nil, nil, false, nil, err
	}
	return novel, author, onShelf, progress, nil
}

// EnrichNovels 列表页批量补全：作者、是否在书架、阅读进度。
func (s *novelService) EnrichNovels(ctx context.Context, novels []*domain.Novel, viewerID int) (map[int]*domain.User, map[int]bool, map[int]*domain.NovelProgress, error) {
	if len(novels) == 0 {
		return nil, nil, nil, nil
	}
	ids := make([]int, 0, len(novels))
	userIDs := make([]int, 0, len(novels))
	seen := make(map[int]struct{}, len(novels))
	for _, n := range novels {
		ids = append(ids, n.ID)
		if n.UserID == 0 {
			continue
		}
		if _, ok := seen[n.UserID]; !ok {
			seen[n.UserID] = struct{}{}
			userIDs = append(userIDs, n.UserID)
		}
	}
	userMap, err := s.novels.UsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	shelfMap, err := s.novels.OnShelfNovelIDs(ctx, viewerID, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	progressMap, err := s.novels.ProgressByNovelIDs(ctx, viewerID, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	return userMap, shelfMap, progressMap, nil
}

func (s *novelService) CreateNovel(ctx context.Context, in domain.NovelCreateInput) (int, error) {
	title := strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(title) < 2 || utf8.RuneCountInString(title) > novelTitleMaxRunes {
		return 0, domain.ErrBadRequest
	}
	intro := strings.TrimSpace(in.Intro)
	if utf8.RuneCountInString(intro) > novelIntroMaxRunes {
		return 0, domain.ErrBadRequest
	}
	category := strings.TrimSpace(in.Category)
	if utf8.RuneCountInString(category) > novelCategoryMaxRunes {
		return 0, domain.ErrBadRequest
	}
	authorName := strings.TrimSpace(in.AuthorName)
	if authorName == "" {
		if user, err := s.novels.FindUserByID(ctx, in.UserID); err == nil {
			authorName = user.Name
		}
	}
	if utf8.RuneCountInString(authorName) > novelAuthorNameMax {
		return 0, domain.ErrBadRequest
	}
	status := strings.TrimSpace(in.Status)
	if status != domain.NovelStatusCompleted {
		status = domain.NovelStatusSerializing
	}
	cover := strings.TrimSpace(in.CoverURL)
	if utf8.RuneCountInString(cover) > novelCoverURLMaxRunes {
		return 0, domain.ErrBadRequest
	}
	novel := &domain.Novel{
		UserID:     in.UserID,
		Title:      title,
		AuthorName: authorName,
		CoverURL:   cover,
		Category:   category,
		Intro:      intro,
		Status:     status,
		CreatedAt:  time.Now(),
	}
	if err := s.novels.CreateNovel(ctx, novel); err != nil {
		return 0, err
	}
	return novel.ID, nil
}

// UpdateNovel 作者改书：只改传入的字段（书名 / 封面 / 分类 / 简介 / 状态）。
func (s *novelService) UpdateNovel(ctx context.Context, in domain.NovelUpdateInput) error {
	novel, err := s.novels.FindNovelByID(ctx, in.NovelID)
	if err != nil {
		return err
	}
	if novel.UserID != in.UserID {
		return domain.ErrNovelNotAuthor
	}
	updates := map[string]any{}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if utf8.RuneCountInString(title) < 2 || utf8.RuneCountInString(title) > novelTitleMaxRunes {
			return domain.ErrBadRequest
		}
		updates["title"] = title
	}
	if in.CoverURL != nil {
		cover := strings.TrimSpace(*in.CoverURL)
		if utf8.RuneCountInString(cover) > novelCoverURLMaxRunes {
			return domain.ErrBadRequest
		}
		updates["cover_url"] = cover
	}
	if in.Category != nil {
		category := strings.TrimSpace(*in.Category)
		if utf8.RuneCountInString(category) > novelCategoryMaxRunes {
			return domain.ErrBadRequest
		}
		updates["category"] = category
	}
	if in.Intro != nil {
		intro := strings.TrimSpace(*in.Intro)
		if utf8.RuneCountInString(intro) > novelIntroMaxRunes {
			return domain.ErrBadRequest
		}
		updates["intro"] = intro
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if status != domain.NovelStatusCompleted {
			status = domain.NovelStatusSerializing
		}
		updates["status"] = status
	}
	if len(updates) == 0 {
		return domain.ErrBadRequest
	}
	return s.novels.UpdateNovelFields(ctx, in.NovelID, updates)
}

func (s *novelService) DeleteNovel(ctx context.Context, novelID, userID int) error {
	novel, err := s.novels.FindNovelByID(ctx, novelID)
	if err != nil {
		return err
	}
	if novel.UserID != userID {
		return domain.ErrNovelNotAuthor
	}
	return s.novels.DeleteNovelCascade(ctx, novelID)
}

// ---------------------------------------------------------------------------
// 章节
// ---------------------------------------------------------------------------

func (s *novelService) ListChapters(ctx context.Context, novelID, limit, offset int) ([]*domain.NovelChapter, int64, error) {
	if _, err := s.novels.FindNovelByID(ctx, novelID); err != nil {
		return nil, 0, err
	}
	chapters, err := s.novels.ListChapters(ctx, novelID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.novels.CountChapters(ctx, novelID)
	if err != nil {
		return nil, 0, err
	}
	return chapters, total, nil
}

// GetChapterDetail 阅读页：正文分段（序号与前端一致）+ 每段段评数 + 上下章导航。
func (s *novelService) GetChapterDetail(ctx context.Context, chapterID, viewerID int) (*domain.NovelChapterDetail, error) {
	chapter, err := s.novels.FindChapterByID(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	_ = s.novels.IncrChapterRead(ctx, chapterID)
	_ = s.novels.IncrNovelRead(ctx, chapter.NovelID)
	chapter.ReadNum++

	counts, err := s.novels.ParagraphCommentCounts(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	texts := domain.SplitNovelParagraphs(chapter.Content)
	paragraphs := make([]domain.NovelParagraph, 0, len(texts))
	for i, text := range texts {
		paragraphs = append(paragraphs, domain.NovelParagraph{
			Index:      i,
			Text:       text,
			ImageURL:   domain.NovelParagraphImage(text),
			CommentNum: counts[i],
		})
	}

	detail := &domain.NovelChapterDetail{Chapter: chapter, Paragraphs: paragraphs}
	if novel, err := s.novels.FindNovelByID(ctx, chapter.NovelID); err == nil {
		detail.NovelTitle = novel.Title
		detail.AuthorName = novel.AuthorName
		s.grantReaderUV(ctx, novel, viewerID)
	}
	if prev, err := s.novels.FindAdjacentChapter(ctx, chapter.NovelID, chapter.ChapterNum, false); err != nil {
		return nil, err
	} else if prev != nil {
		detail.Prev = &domain.NovelChapterAdjacent{ID: prev.ID, Num: prev.ChapterNum, Title: prev.Title}
	}
	if next, err := s.novels.FindAdjacentChapter(ctx, chapter.NovelID, chapter.ChapterNum, true); err != nil {
		return nil, err
	} else if next != nil {
		detail.Next = &domain.NovelChapterAdjacent{ID: next.ID, Num: next.ChapterNum, Title: next.Title}
	}
	return detail, nil
}

func (s *novelService) CreateChapter(ctx context.Context, in domain.NovelChapterCreateInput) (*domain.NovelChapter, error) {
	novel, err := s.novels.FindNovelByID(ctx, in.NovelID)
	if err != nil {
		return nil, err
	}
	if novel.UserID != in.UserID {
		return nil, domain.ErrNovelNotAuthor
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || utf8.RuneCountInString(title) > novelChapterTitleMax {
		return nil, domain.ErrBadRequest
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil, domain.ErrBadRequest
	}
	if utf8.RuneCountInString(content) > novelChapterMaxRunes {
		return nil, domain.ErrBadRequest
	}
	note := strings.TrimSpace(in.AuthorNote)
	if utf8.RuneCountInString(note) > novelAuthorNoteMax {
		return nil, domain.ErrBadRequest
	}
	// 章节数不设上限。
	maxNum, err := s.novels.MaxChapterNum(ctx, in.NovelID)
	if err != nil {
		return nil, err
	}

	chapter := &domain.NovelChapter{
		NovelID:    in.NovelID,
		ChapterNum: maxNum + 1,
		Title:      title,
		Content:    in.Content,
		AuthorNote: note,
		// 图片段落不计入字数（图片链接不是「写了多少字」）。
		WordNum:   domain.NovelContentWordCount(in.Content),
		CreatedAt: time.Now(),
	}
	if err := s.novels.CreateChapter(ctx, chapter); err != nil {
		return nil, err
	}
	_ = s.novels.IncrNovelChapterCount(ctx, in.NovelID)
	_ = s.novels.AddNovelWordNum(ctx, in.NovelID, chapter.WordNum)
	_ = s.novels.UpdateNovelLatest(ctx, in.NovelID, chapter.ID, chapter.Title, chapter.CreatedAt)
	// 作者积分：章节更新 +2 积分（每日限 5 章），写作字数里程碑每满 5000 字 +1 积分。
	if s.points != nil {
		_, _ = s.points.GrantChapterPublish(ctx, novel.UserID, in.NovelID, chapter.ID)
		_, _ = s.points.SettleWritingWords(ctx, novel.UserID, in.NovelID, chapter.WordNum)
	}
	return chapter, nil
}

func (s *novelService) DeleteChapter(ctx context.Context, chapterID, userID int) error {
	chapter, err := s.novels.FindChapterByID(ctx, chapterID)
	if err != nil {
		return err
	}
	novel, err := s.novels.FindNovelByID(ctx, chapter.NovelID)
	if err != nil {
		return err
	}
	if novel.UserID != userID {
		return domain.ErrNovelNotAuthor
	}
	if err := s.novels.DeleteChapterCascade(ctx, chapterID, chapter.NovelID); err != nil {
		return err
	}
	return s.novels.RefreshNovelStats(ctx, chapter.NovelID)
}

// ---------------------------------------------------------------------------
// 评论（书评 / 章评 / 段评）
// ---------------------------------------------------------------------------

func (s *novelService) ListComments(ctx context.Context, filter domain.NovelCommentFilter, viewerID int) ([]*domain.NovelComment, map[int]*domain.User, map[int]bool, int64, error) {
	if filter.NovelID <= 0 {
		return nil, nil, nil, 0, nil
	}
	comments, err := s.novels.ListComments(ctx, filter)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	total, err := s.novels.CountComments(ctx, filter)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	if len(comments) == 0 {
		return comments, nil, nil, total, nil
	}
	userIDs := make([]int, 0, len(comments))
	commentIDs := make([]int, 0, len(comments))
	for _, c := range comments {
		userIDs = append(userIDs, c.UserID)
		commentIDs = append(commentIDs, c.ID)
	}
	userMap, err := s.novels.UsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	likedMap, err := s.novels.CommentLikedSet(ctx, viewerID, commentIDs)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	return comments, userMap, likedMap, total, nil
}

func (s *novelService) CreateComment(ctx context.Context, in domain.NovelCommentCreateInput) (*domain.NovelComment, *domain.User, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > novelCommentMaxRunes {
		return nil, nil, domain.ErrBadRequest
	}
	targetType := strings.TrimSpace(in.TargetType)
	if targetType != domain.NovelTargetBook && targetType != domain.NovelTargetChapter && targetType != domain.NovelTargetParagraph {
		return nil, nil, domain.ErrBadRequest
	}
	if _, err := s.novels.FindNovelByID(ctx, in.NovelID); err != nil {
		return nil, nil, err
	}
	author, err := s.novels.FindUserByID(ctx, in.UserID)
	if err != nil {
		return nil, nil, err
	}
	if author.BanEnd.After(time.Now()) {
		return nil, nil, domain.ErrBadRequest
	}

	chapterID := 0
	paragraphIndex := -1
	excerpt := ""

	switch targetType {
	case domain.NovelTargetParagraph:
		chapter, err := s.novels.FindChapterByID(ctx, in.ChapterID)
		if err != nil {
			return nil, nil, err
		}
		if chapter.NovelID != in.NovelID {
			return nil, nil, domain.ErrBadRequest
		}
		paragraphs := domain.SplitNovelParagraphs(chapter.Content)
		if in.ParagraphIndex < 0 || in.ParagraphIndex >= len(paragraphs) {
			return nil, nil, domain.ErrBadRequest
		}
		chapterID = chapter.ID
		paragraphIndex = in.ParagraphIndex
		excerpt = domain.NovelParagraphExcerpt(paragraphs[paragraphIndex], 40)
	case domain.NovelTargetChapter:
		chapter, err := s.novels.FindChapterByID(ctx, in.ChapterID)
		if err != nil {
			return nil, nil, err
		}
		if chapter.NovelID != in.NovelID {
			return nil, nil, domain.ErrBadRequest
		}
		chapterID = chapter.ID
	}

	comment := &domain.NovelComment{
		NovelID:          in.NovelID,
		ChapterID:        chapterID,
		ParagraphIndex:   paragraphIndex,
		TargetType:       targetType,
		ParagraphExcerpt: excerpt,
		UserID:           in.UserID,
		Content:          content,
		CreatedAt:        time.Now(),
	}
	if err := s.novels.CreateComment(ctx, comment); err != nil {
		return nil, nil, err
	}
	_ = s.novels.IncrNovelComment(ctx, in.NovelID, 1)
	if chapterID > 0 {
		_ = s.novels.IncrChapterComment(ctx, chapterID, 1)
	}
	// 读者经验：发表评论 +2 经验（每条首次计入，单用户单日上限防灌水）。
	if s.points != nil {
		_, _ = s.points.GrantCommentExp(ctx, in.UserID, in.NovelID, comment.ID)
	}
	return comment, author, nil
}

func (s *novelService) DeleteComment(ctx context.Context, commentID, userID int) error {
	comment, err := s.novels.FindCommentByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment.UserID != userID {
		return domain.ErrForbidden
	}
	if err := s.novels.DeleteCommentCascade(ctx, commentID); err != nil {
		return err
	}
	_ = s.novels.IncrNovelComment(ctx, comment.NovelID, -1)
	if comment.ChapterID > 0 {
		_ = s.novels.IncrChapterComment(ctx, comment.ChapterID, -1)
	}
	return nil
}

func (s *novelService) ToggleCommentLike(ctx context.Context, commentID, userID int) error {
	comment, err := s.novels.FindCommentByID(ctx, commentID)
	if err != nil {
		return err
	}
	// 先记下当前是否已点赞：只有「新点赞」才给经验；取消点赞不回收（流水只增不减）。
	// 即便这里读到的是旧值，发放侧的幂等键也保证同一人同一评论只会计一次。
	alreadyLiked := false
	if s.points != nil && comment.UserID != userID {
		if set, err := s.novels.CommentLikedSet(ctx, userID, []int{commentID}); err == nil {
			alreadyLiked = set[commentID]
		}
	}
	if err := s.novels.ToggleCommentLike(ctx, commentID, userID); err != nil {
		return err
	}
	if s.points != nil && !alreadyLiked {
		_, _ = s.points.GrantCommentLikeExp(ctx, comment.UserID, comment.NovelID, commentID, userID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 书架与阅读进度
// ---------------------------------------------------------------------------

func (s *novelService) ToggleShelf(ctx context.Context, novelID, userID int) (bool, error) {
	novel, err := s.novels.FindNovelByID(ctx, novelID)
	if err != nil {
		return false, err
	}
	added, err := s.novels.ToggleShelf(ctx, novelID, userID)
	if err != nil {
		return false, err
	}
	// 作者积分：被加入书架每人 +5 积分（同人同书仅首次计入，移出再加不重复给）。
	if added && s.points != nil {
		_, _ = s.points.GrantShelfAdd(ctx, novel.UserID, novelID, userID)
	}
	return added, nil
}

func (s *novelService) IsOnShelf(ctx context.Context, novelID, userID int) (bool, error) {
	return s.novels.IsOnShelf(ctx, novelID, userID)
}

func (s *novelService) ListShelf(ctx context.Context, userID, limit, offset int) ([]*domain.Novel, int64, error) {
	filter := domain.NovelBrowseFilter{ShelfFor: userID, Limit: limit, Offset: offset}
	novels, err := s.novels.ListNovels(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.novels.CountNovels(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return novels, total, nil
}

func (s *novelService) ReportProgress(ctx context.Context, in domain.NovelProgressInput) error {
	chapter, err := s.novels.FindChapterByID(ctx, in.ChapterID)
	if err != nil {
		return err
	}
	if chapter.NovelID != in.NovelID {
		return domain.ErrBadRequest
	}
	percent := in.Percent
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if err := s.novels.UpsertProgress(ctx, &domain.NovelProgress{
		NovelID:        in.NovelID,
		UserID:         in.UserID,
		ChapterID:      chapter.ID,
		ChapterNum:     chapter.ChapterNum,
		ParagraphIndex: in.ParagraphIndex,
		Percent:        percent,
		UpdatedAt:      time.Now(),
	}); err != nil {
		return err
	}
	// 读者经验：阅读心跳秒数累计，每满 1 小时 +1 经验（游标幂等，重复上报不会重复发）。
	if s.points != nil && in.ReadSeconds > 0 {
		_, _ = s.points.SettleReadTime(ctx, in.UserID, in.NovelID, in.ReadSeconds)
	}
	return nil
}

// grantReaderUV 读者第一次进入这本书（详情页或任一章节）时给作者 +0.1 积分。
// 按本独立、UV 去重、作者自己阅读不计。
func (s *novelService) grantReaderUV(ctx context.Context, novel *domain.Novel, viewerID int) {
	if s.points == nil || novel == nil || viewerID <= 0 || viewerID == novel.UserID {
		return
	}
	_, _ = s.points.GrantReaderUV(ctx, novel.UserID, novel.ID, viewerID)
}

func (s *novelService) GetProgress(ctx context.Context, novelID, userID int) (*domain.NovelProgress, error) {
	return s.novels.FindProgress(ctx, novelID, userID)
}

// ---------------------------------------------------------------------------
// 评分
// ---------------------------------------------------------------------------

// MyRating 我给这本书打了几分（0 = 没打过）。
func (s *novelService) MyRating(ctx context.Context, novelID, userID int) (int, error) {
	if s == nil || novelID <= 0 || userID <= 0 {
		return 0, nil
	}
	return s.novels.FindRating(ctx, novelID, userID)
}
