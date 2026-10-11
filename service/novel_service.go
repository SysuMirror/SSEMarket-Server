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
	novelChapterLimit     = 200
)

type novelService struct {
	novels repository.NovelRepository
}

func NewNovelService(novels repository.NovelRepository) NovelService {
	return &novelService{novels: novels}
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
	novel := &domain.Novel{
		UserID:     in.UserID,
		Title:      title,
		AuthorName: authorName,
		CoverURL:   strings.TrimSpace(in.CoverURL),
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
			CommentNum: counts[i],
		})
	}

	detail := &domain.NovelChapterDetail{Chapter: chapter, Paragraphs: paragraphs}
	if novel, err := s.novels.FindNovelByID(ctx, chapter.NovelID); err == nil {
		detail.NovelTitle = novel.Title
		detail.AuthorName = novel.AuthorName
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
	total, err := s.novels.CountChapters(ctx, in.NovelID)
	if err != nil {
		return nil, err
	}
	if total >= novelChapterLimit {
		return nil, domain.ErrNovelChapterLimit
	}
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
		WordNum:    domain.NovelWordCount(in.Content),
		CreatedAt:  time.Now(),
	}
	if err := s.novels.CreateChapter(ctx, chapter); err != nil {
		return nil, err
	}
	_ = s.novels.IncrNovelChapterCount(ctx, in.NovelID)
	_ = s.novels.AddNovelWordNum(ctx, in.NovelID, chapter.WordNum)
	_ = s.novels.UpdateNovelLatest(ctx, in.NovelID, chapter.ID, chapter.Title, chapter.CreatedAt)
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
	if _, err := s.novels.FindCommentByID(ctx, commentID); err != nil {
		return err
	}
	return s.novels.ToggleCommentLike(ctx, commentID, userID)
}

// ---------------------------------------------------------------------------
// 书架与阅读进度
// ---------------------------------------------------------------------------

func (s *novelService) ToggleShelf(ctx context.Context, novelID, userID int) (bool, error) {
	if _, err := s.novels.FindNovelByID(ctx, novelID); err != nil {
		return false, err
	}
	return s.novels.ToggleShelf(ctx, novelID, userID)
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
	return s.novels.UpsertProgress(ctx, &domain.NovelProgress{
		NovelID:        in.NovelID,
		UserID:         in.UserID,
		ChapterID:      chapter.ID,
		ChapterNum:     chapter.ChapterNum,
		ParagraphIndex: in.ParagraphIndex,
		Percent:        percent,
		UpdatedAt:      time.Now(),
	})
}

func (s *novelService) GetProgress(ctx context.Context, novelID, userID int) (*domain.NovelProgress, error) {
	return s.novels.FindProgress(ctx, novelID, userID)
}
