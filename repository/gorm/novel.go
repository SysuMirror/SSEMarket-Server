package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type novelRepository struct {
	db *gorm.DB
}

func NewNovelRepository(db *gorm.DB) repository.NovelRepository {
	return &novelRepository{db: db}
}

func (r *novelRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// novelTrimSpace / novelNormalizePage 小说板块内部小工具：
// 分页口径与打分专区一致（默认 15 条，上限 50）。
func novelTrimSpace(s string) string { return strings.TrimSpace(s) }

func novelNormalizePage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ---------------------------------------------------------------------------
// model → domain 映射
// ---------------------------------------------------------------------------

func novelToDomain(n *model.Novel) *domain.Novel {
	if n == nil {
		return nil
	}
	return &domain.Novel{
		ID:                 n.NovelID,
		UserID:             n.UserID,
		Title:              n.Title,
		AuthorName:         n.AuthorName,
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
	}
}

func novelChapterToDomain(c *model.NovelChapter) *domain.NovelChapter {
	if c == nil {
		return nil
	}
	return &domain.NovelChapter{
		ID:         c.ChapterID,
		NovelID:    c.NovelID,
		ChapterNum: c.ChapterNum,
		Title:      c.Title,
		Content:    c.Content,
		AuthorNote: c.AuthorNote,
		WordNum:    c.WordNum,
		CommentNum: c.CommentNum,
		ReadNum:    c.ReadNum,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func novelCommentToDomain(c *model.NovelComment) *domain.NovelComment {
	if c == nil {
		return nil
	}
	return &domain.NovelComment{
		ID:               c.CommentID,
		NovelID:          c.NovelID,
		ChapterID:        c.ChapterID,
		ParagraphIndex:   c.ParagraphIndex,
		TargetType:       c.TargetType,
		ParagraphExcerpt: c.ParagraphExcerpt,
		UserID:           c.UserID,
		Content:          c.Content,
		LikeNum:          c.LikeNum,
		DenyNum:          c.DenyNum,
		CreatedAt:        c.CreatedAt,
	}
}

func novelProgressToDomain(p *model.NovelProgress) *domain.NovelProgress {
	if p == nil {
		return nil
	}
	return &domain.NovelProgress{
		NovelID:        p.NovelID,
		UserID:         p.UserID,
		ChapterID:      p.ChapterID,
		ChapterNum:     p.ChapterNum,
		ParagraphIndex: p.ParagraphIndex,
		Percent:        p.Percent,
		UpdatedAt:      p.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// 书
// ---------------------------------------------------------------------------

// applyNovelFilter 统一列表筛选；列名一律带 novels. 前缀，书架列表会 JOIN 其他表。
func (r *novelRepository) applyNovelFilter(q *gorm.DB, filter domain.NovelBrowseFilter) *gorm.DB {
	if keyword := novelTrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("(novels.title LIKE ? OR novels.intro LIKE ? OR novels.author_name LIKE ?)", like, like, like)
	}
	if category := novelTrimSpace(filter.Category); category != "" {
		q = q.Where("novels.category = ?", category)
	}
	if status := novelTrimSpace(filter.Status); status == domain.NovelStatusSerializing || status == domain.NovelStatusCompleted {
		q = q.Where("novels.status = ?", status)
	}
	if filter.AuthorFor > 0 {
		q = q.Where("novels.user_id = ?", filter.AuthorFor)
	}
	return q
}

func (r *novelRepository) novelQuery(ctx context.Context, filter domain.NovelBrowseFilter) *gorm.DB {
	q := r.session(ctx).Model(&model.Novel{})
	if filter.ShelfFor > 0 {
		q = q.Joins("JOIN novel_bookshelf nb ON nb.novel_id = novels.novel_id AND nb.user_id = ?", filter.ShelfFor)
		q = q.Joins("LEFT JOIN novel_progress np ON np.novel_id = novels.novel_id AND np.user_id = ?", filter.ShelfFor)
	}
	return r.applyNovelFilter(q, filter)
}

func (r *novelRepository) ListNovels(ctx context.Context, filter domain.NovelBrowseFilter) ([]*domain.Novel, error) {
	q := r.novelQuery(ctx, filter)
	limit, offset := novelNormalizePage(filter.Limit, filter.Offset)
	var list []model.Novel
	var err error
	if filter.ShelfFor > 0 {
		// 书架：按最近阅读（无进度记录则按加入时间）倒序
		err = q.Order("COALESCE(np.updated_at, nb.created_at) DESC, novels.novel_id DESC").
			Offset(offset).Limit(limit).Find(&list).Error
	} else {
		err = q.Order("COALESCE(novels.latest_chapter_at, novels.created_at) DESC, novels.novel_id DESC").
			Offset(offset).Limit(limit).Find(&list).Error
	}
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Novel, 0, len(list))
	for i := range list {
		out = append(out, novelToDomain(&list[i]))
	}
	return out, nil
}

func (r *novelRepository) CountNovels(ctx context.Context, filter domain.NovelBrowseFilter) (int64, error) {
	var total int64
	if err := r.novelQuery(ctx, filter).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *novelRepository) FindNovelByID(ctx context.Context, id int) (*domain.Novel, error) {
	var m model.Novel
	err := r.session(ctx).Where("novel_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return novelToDomain(&m), nil
}

func (r *novelRepository) CreateNovel(ctx context.Context, novel *domain.Novel) error {
	now := time.Now()
	m := model.Novel{
		UserID:      novel.UserID,
		Title:       novel.Title,
		AuthorName:  novel.AuthorName,
		CoverURL:    novel.CoverURL,
		Category:    novel.Category,
		Intro:       novel.Intro,
		Status:      novel.Status,
		ChapterNum:  0,
		WordNum:     0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	novel.ID = m.NovelID
	novel.CreatedAt = m.CreatedAt
	novel.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *novelRepository) DeleteNovelCascade(ctx context.Context, novelID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"DELETE l FROM novel_comment_likes l JOIN novel_comments c ON c.comment_id = l.comment_id WHERE c.novel_id = ?",
			novelID,
		).Error; err != nil {
			return err
		}
		if err := tx.Where("novel_id = ?", novelID).Delete(&model.NovelComment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("novel_id = ?", novelID).Delete(&model.NovelChapter{}).Error; err != nil {
			return err
		}
		if err := tx.Where("novel_id = ?", novelID).Delete(&model.NovelBookshelf{}).Error; err != nil {
			return err
		}
		if err := tx.Where("novel_id = ?", novelID).Delete(&model.NovelProgress{}).Error; err != nil {
			return err
		}
		return tx.Where("novel_id = ?", novelID).Delete(&model.Novel{}).Error
	})
}

func (r *novelRepository) CategoriesWithCount(ctx context.Context) ([]domain.NovelCategory, error) {
	var rows []struct {
		Name  string `gorm:"column:category"`
		Count int    `gorm:"column:cnt"`
	}
	err := r.session(ctx).Model(&model.Novel{}).
		Where("category <> ''").
		Group("category").
		Select("category, COUNT(*) AS cnt").
		Order("cnt DESC, category ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.NovelCategory, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.NovelCategory{Name: row.Name, Count: row.Count})
	}
	return out, nil
}

func (r *novelRepository) IncrNovelRead(ctx context.Context, novelID int) error {
	return r.session(ctx).Model(&model.Novel{}).Where("novel_id = ?", novelID).
		UpdateColumn("read_num", gorm.Expr("read_num + 1")).Error
}

func (r *novelRepository) IncrNovelComment(ctx context.Context, novelID int, delta int) error {
	return r.incrColumn(ctx, &model.Novel{}, "novel_id", novelID, "comment_num", delta)
}

func (r *novelRepository) IncrNovelShelf(ctx context.Context, novelID int, delta int) error {
	return r.incrColumn(ctx, &model.Novel{}, "novel_id", novelID, "shelf_num", delta)
}

func (r *novelRepository) IncrNovelChapterCount(ctx context.Context, novelID int) error {
	return r.incrColumn(ctx, &model.Novel{}, "novel_id", novelID, "chapter_num", 1)
}

func (r *novelRepository) AddNovelWordNum(ctx context.Context, novelID, delta int) error {
	return r.incrColumn(ctx, &model.Novel{}, "novel_id", novelID, "word_num", delta)
}

func (r *novelRepository) incrColumn(ctx context.Context, modelPtr any, keyColumn string, keyID int, column string, delta int) error {
	if delta == 0 {
		return nil
	}
	expr := column + " + ?"
	if delta < 0 {
		expr = "GREATEST(" + column + " - ?, 0)"
		delta = -delta
	}
	return r.session(ctx).Model(modelPtr).Where(keyColumn+" = ?", keyID).
		UpdateColumn(column, gorm.Expr(expr, delta)).Error
}

func (r *novelRepository) UpdateNovelLatest(ctx context.Context, novelID, chapterID int, title string, at time.Time) error {
	return r.session(ctx).Model(&model.Novel{}).Where("novel_id = ?", novelID).Updates(map[string]any{
		"latest_chapter_id":    chapterID,
		"latest_chapter_title": title,
		"latest_chapter_at":    at,
		"updated_at":           time.Now(),
	}).Error
}

// RefreshNovelStats 章节增删后重算总章数 / 总字数 / 最新章节。
func (r *novelRepository) RefreshNovelStats(ctx context.Context, novelID int) error {
	var stats struct {
		ChapterNum int `gorm:"column:chapter_num"`
		WordNum    int `gorm:"column:word_num"`
	}
	if err := r.session(ctx).Model(&model.NovelChapter{}).
		Where("novel_id = ?", novelID).
		Select("COUNT(*) AS chapter_num, COALESCE(SUM(word_num), 0) AS word_num").
		Scan(&stats).Error; err != nil {
		return err
	}

	var latest model.NovelChapter
	latestErr := r.session(ctx).Where("novel_id = ?", novelID).
		Order("chapter_num DESC").First(&latest).Error

	updates := map[string]any{
		"chapter_num": stats.ChapterNum,
		"word_num":    stats.WordNum,
		"updated_at":  time.Now(),
	}
	if latestErr == nil {
		updates["latest_chapter_id"] = latest.ChapterID
		updates["latest_chapter_title"] = latest.Title
		updates["latest_chapter_at"] = latest.CreatedAt
	} else if errors.Is(latestErr, gorm.ErrRecordNotFound) {
		updates["latest_chapter_id"] = 0
		updates["latest_chapter_title"] = ""
		updates["latest_chapter_at"] = nil
	} else {
		return latestErr
	}

	return r.session(ctx).Model(&model.Novel{}).Where("novel_id = ?", novelID).Updates(updates).Error
}

// ---------------------------------------------------------------------------
// 章节
// ---------------------------------------------------------------------------

// novelChapterListSelect 目录列表不返回正文，避免整本内容一次性下发。
const novelChapterListSelect = "chapter_id, novel_id, chapter_num, title, word_num, comment_num, read_num, created_at, updated_at"

func (r *novelRepository) ListChapters(ctx context.Context, novelID, limit, offset int) ([]*domain.NovelChapter, error) {
	limit, offset = novelNormalizePage(limit, offset)
	var list []model.NovelChapter
	err := r.session(ctx).Model(&model.NovelChapter{}).
		Select(novelChapterListSelect).
		Where("novel_id = ?", novelID).
		Order("chapter_num ASC").
		Offset(offset).Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.NovelChapter, 0, len(list))
	for i := range list {
		out = append(out, novelChapterToDomain(&list[i]))
	}
	return out, nil
}

func (r *novelRepository) CountChapters(ctx context.Context, novelID int) (int64, error) {
	var total int64
	err := r.session(ctx).Model(&model.NovelChapter{}).Where("novel_id = ?", novelID).Count(&total).Error
	return total, err
}

func (r *novelRepository) MaxChapterNum(ctx context.Context, novelID int) (int, error) {
	var maxNum int
	err := r.session(ctx).Model(&model.NovelChapter{}).
		Where("novel_id = ?", novelID).
		Select("COALESCE(MAX(chapter_num), 0)").Row().Scan(&maxNum)
	if err != nil {
		return 0, err
	}
	return maxNum, nil
}

func (r *novelRepository) FindChapterByID(ctx context.Context, id int) (*domain.NovelChapter, error) {
	var m model.NovelChapter
	err := r.session(ctx).Where("chapter_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return novelChapterToDomain(&m), nil
}

func (r *novelRepository) FindChapterByNum(ctx context.Context, novelID, chapterNum int) (*domain.NovelChapter, error) {
	var m model.NovelChapter
	err := r.session(ctx).Where("novel_id = ? AND chapter_num = ?", novelID, chapterNum).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return novelChapterToDomain(&m), nil
}

func (r *novelRepository) FindAdjacentChapter(ctx context.Context, novelID, chapterNum int, next bool) (*domain.NovelChapter, error) {
	var m model.NovelChapter
	q := r.session(ctx).Model(&model.NovelChapter{}).Select(novelChapterListSelect).Where("novel_id = ?", novelID)
	if next {
		q = q.Where("chapter_num > ?", chapterNum).Order("chapter_num ASC")
	} else {
		q = q.Where("chapter_num < ?", chapterNum).Order("chapter_num DESC")
	}
	err := q.First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return novelChapterToDomain(&m), nil
}

func (r *novelRepository) CreateChapter(ctx context.Context, chapter *domain.NovelChapter) error {
	now := time.Now()
	m := model.NovelChapter{
		NovelID:    chapter.NovelID,
		ChapterNum: chapter.ChapterNum,
		Title:      chapter.Title,
		Content:    chapter.Content,
		AuthorNote: chapter.AuthorNote,
		WordNum:    chapter.WordNum,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	chapter.ID = m.ChapterID
	chapter.CreatedAt = m.CreatedAt
	chapter.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *novelRepository) DeleteChapterCascade(ctx context.Context, chapterID, novelID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			"DELETE l FROM novel_comment_likes l JOIN novel_comments c ON c.comment_id = l.comment_id WHERE c.chapter_id = ?",
			chapterID,
		).Error; err != nil {
			return err
		}
		if err := tx.Where("chapter_id = ?", chapterID).Delete(&model.NovelComment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("chapter_id = ?", chapterID).Delete(&model.NovelProgress{}).Error; err != nil {
			return err
		}
		return tx.Where("chapter_id = ?", chapterID).Delete(&model.NovelChapter{}).Error
	})
}

func (r *novelRepository) IncrChapterRead(ctx context.Context, chapterID int) error {
	return r.session(ctx).Model(&model.NovelChapter{}).Where("chapter_id = ?", chapterID).
		UpdateColumn("read_num", gorm.Expr("read_num + 1")).Error
}

func (r *novelRepository) IncrChapterComment(ctx context.Context, chapterID int, delta int) error {
	return r.incrColumn(ctx, &model.NovelChapter{}, "chapter_id", chapterID, "comment_num", delta)
}

// ---------------------------------------------------------------------------
// 评论
// ---------------------------------------------------------------------------

func (r *novelRepository) applyCommentFilter(q *gorm.DB, filter domain.NovelCommentFilter) *gorm.DB {
	q = q.Where("novel_id = ?", filter.NovelID)
	switch filter.TargetType {
	case domain.NovelTargetParagraph:
		q = q.Where("target_type = ? AND chapter_id = ? AND paragraph_index = ?", domain.NovelTargetParagraph, filter.ChapterID, filter.ParagraphIndex)
	case domain.NovelTargetChapter:
		q = q.Where("target_type = ?", domain.NovelTargetChapter)
		if filter.ChapterID > 0 {
			q = q.Where("chapter_id = ?", filter.ChapterID)
		}
	case domain.NovelTargetBook:
		q = q.Where("target_type = ?", domain.NovelTargetBook)
	}
	return q
}

func (r *novelRepository) ListComments(ctx context.Context, filter domain.NovelCommentFilter) ([]*domain.NovelComment, error) {
	limit, offset := novelNormalizePage(filter.Limit, filter.Offset)
	var list []model.NovelComment
	err := r.applyCommentFilter(r.session(ctx).Model(&model.NovelComment{}), filter).
		Order("comment_id DESC").
		Offset(offset).Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.NovelComment, 0, len(list))
	for i := range list {
		out = append(out, novelCommentToDomain(&list[i]))
	}
	return out, nil
}

func (r *novelRepository) CountComments(ctx context.Context, filter domain.NovelCommentFilter) (int64, error) {
	var total int64
	err := r.applyCommentFilter(r.session(ctx).Model(&model.NovelComment{}), filter).Count(&total).Error
	return total, err
}

// ParagraphCommentCounts 一章内每个段落的段评数量（阅读页段落气泡）。
func (r *novelRepository) ParagraphCommentCounts(ctx context.Context, chapterID int) (map[int]int, error) {
	counts := make(map[int]int)
	var rows []struct {
		ParagraphIndex int `gorm:"column:paragraph_index"`
		Count          int `gorm:"column:cnt"`
	}
	err := r.session(ctx).Model(&model.NovelComment{}).
		Where("chapter_id = ? AND target_type = ?", chapterID, domain.NovelTargetParagraph).
		Group("paragraph_index").
		Select("paragraph_index, COUNT(*) AS cnt").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ParagraphIndex] = row.Count
	}
	return counts, nil
}

func (r *novelRepository) FindCommentByID(ctx context.Context, id int) (*domain.NovelComment, error) {
	var m model.NovelComment
	err := r.session(ctx).Where("comment_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return novelCommentToDomain(&m), nil
}

func (r *novelRepository) CreateComment(ctx context.Context, comment *domain.NovelComment) error {
	m := model.NovelComment{
		NovelID:          comment.NovelID,
		ChapterID:        comment.ChapterID,
		ParagraphIndex:   comment.ParagraphIndex,
		TargetType:       comment.TargetType,
		ParagraphExcerpt: comment.ParagraphExcerpt,
		UserID:           comment.UserID,
		Content:          comment.Content,
		CreatedAt:        time.Now(),
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	comment.ID = m.CommentID
	comment.CreatedAt = m.CreatedAt
	return nil
}

func (r *novelRepository) DeleteCommentCascade(ctx context.Context, commentID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("comment_id = ?", commentID).Delete(&model.NovelCommentLike{}).Error; err != nil {
			return err
		}
		return tx.Where("comment_id = ?", commentID).Delete(&model.NovelComment{}).Error
	})
}

func (r *novelRepository) ToggleCommentLike(ctx context.Context, commentID, userID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		var like model.NovelCommentLike
		err := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&like).Error
		if err == nil && like.LikeID != 0 {
			if err := tx.Delete(&like).Error; err != nil {
				return err
			}
			return tx.Model(&model.NovelComment{}).Where("comment_id = ?", commentID).
				UpdateColumn("like_num", gorm.Expr("GREATEST(like_num - 1, 0)")).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&model.NovelCommentLike{
			CommentID: commentID,
			UserID:    userID,
			CreatedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.NovelComment{}).Where("comment_id = ?", commentID).
			UpdateColumn("like_num", gorm.Expr("like_num + 1")).Error
	})
}

func (r *novelRepository) CommentLikedSet(ctx context.Context, userID int, commentIDs []int) (map[int]bool, error) {
	liked := make(map[int]bool)
	if userID == 0 || len(commentIDs) == 0 {
		return liked, nil
	}
	var likes []model.NovelCommentLike
	if err := r.session(ctx).Where("user_id = ? AND comment_id IN (?)", userID, commentIDs).Find(&likes).Error; err != nil {
		return nil, err
	}
	for _, l := range likes {
		liked[l.CommentID] = true
	}
	return liked, nil
}

// ---------------------------------------------------------------------------
// 书架与阅读进度
// ---------------------------------------------------------------------------

func (r *novelRepository) ToggleShelf(ctx context.Context, novelID, userID int) (bool, error) {
	added := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		var shelf model.NovelBookshelf
		err := tx.Where("novel_id = ? AND user_id = ?", novelID, userID).First(&shelf).Error
		if err == nil && shelf.ShelfID != 0 {
			if err := tx.Delete(&shelf).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Novel{}).Where("novel_id = ?", novelID).
				UpdateColumn("shelf_num", gorm.Expr("GREATEST(shelf_num - 1, 0)")).Error; err != nil {
				return err
			}
			added = false
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&model.NovelBookshelf{
			NovelID:   novelID,
			UserID:    userID,
			CreatedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Novel{}).Where("novel_id = ?", novelID).
			UpdateColumn("shelf_num", gorm.Expr("shelf_num + 1")).Error; err != nil {
			return err
		}
		added = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return added, nil
}

func (r *novelRepository) IsOnShelf(ctx context.Context, novelID, userID int) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	var shelf model.NovelBookshelf
	err := r.session(ctx).Where("novel_id = ? AND user_id = ?", novelID, userID).First(&shelf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return shelf.ShelfID != 0, nil
}

func (r *novelRepository) OnShelfNovelIDs(ctx context.Context, userID int, novelIDs []int) (map[int]bool, error) {
	onShelf := make(map[int]bool)
	if userID == 0 || len(novelIDs) == 0 {
		return onShelf, nil
	}
	var ids []int
	err := r.session(ctx).Model(&model.NovelBookshelf{}).
		Where("user_id = ? AND novel_id IN (?)", userID, novelIDs).
		Pluck("novel_id", &ids).Error
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		onShelf[id] = true
	}
	return onShelf, nil
}

func (r *novelRepository) UpsertProgress(ctx context.Context, p *domain.NovelProgress) error {
	now := time.Now()
	var m model.NovelProgress
	err := r.session(ctx).Where("novel_id = ? AND user_id = ?", p.NovelID, p.UserID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		m = model.NovelProgress{
			NovelID:        p.NovelID,
			UserID:         p.UserID,
			ChapterID:      p.ChapterID,
			ChapterNum:     p.ChapterNum,
			ParagraphIndex: p.ParagraphIndex,
			Percent:        p.Percent,
			UpdatedAt:      now,
		}
		return r.session(ctx).Create(&m).Error
	}
	if err != nil {
		return err
	}
	return r.session(ctx).Model(&m).Updates(map[string]any{
		"chapter_id":      p.ChapterID,
		"chapter_num":     p.ChapterNum,
		"paragraph_index": p.ParagraphIndex,
		"percent":         p.Percent,
		"updated_at":      now,
	}).Error
}

func (r *novelRepository) FindProgress(ctx context.Context, novelID, userID int) (*domain.NovelProgress, error) {
	var m model.NovelProgress
	err := r.session(ctx).Where("novel_id = ? AND user_id = ?", novelID, userID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return novelProgressToDomain(&m), nil
}

func (r *novelRepository) ProgressByNovelIDs(ctx context.Context, userID int, novelIDs []int) (map[int]*domain.NovelProgress, error) {
	out := make(map[int]*domain.NovelProgress)
	if userID == 0 || len(novelIDs) == 0 {
		return out, nil
	}
	var list []model.NovelProgress
	if err := r.session(ctx).Where("user_id = ? AND novel_id IN (?)", userID, novelIDs).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		out[list[i].NovelID] = novelProgressToDomain(&list[i])
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 用户
// ---------------------------------------------------------------------------

func (r *novelRepository) UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error) {
	users := make(map[int]*domain.User)
	if len(ids) == 0 {
		return users, nil
	}
	var list []model.User
	if err := r.session(ctx).Where("userID IN (?)", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		users[list[i].UserID] = userToDomain(&list[i])
	}
	return users, nil
}

func (r *novelRepository) FindUserByID(ctx context.Context, id int) (*domain.User, error) {
	var u model.User
	err := r.session(ctx).Where("userID = ?", id).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&u), nil
}
