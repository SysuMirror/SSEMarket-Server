package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// NovelRepository 小说板块持久化接口。
type NovelRepository interface {
	// 书
	ListNovels(ctx context.Context, filter domain.NovelBrowseFilter) ([]*domain.Novel, error)
	CountNovels(ctx context.Context, filter domain.NovelBrowseFilter) (int64, error)
	FindNovelByID(ctx context.Context, id int) (*domain.Novel, error)
	CreateNovel(ctx context.Context, novel *domain.Novel) error
	UpdateNovelFields(ctx context.Context, novelID int, fields map[string]any) error
	DeleteNovelCascade(ctx context.Context, novelID int) error
	CategoriesWithCount(ctx context.Context) ([]domain.NovelCategory, error)

	IncrNovelRead(ctx context.Context, novelID int) error
	IncrNovelComment(ctx context.Context, novelID int, delta int) error
	IncrNovelShelf(ctx context.Context, novelID int, delta int) error
	IncrNovelChapterCount(ctx context.Context, novelID int) error
	AddNovelWordNum(ctx context.Context, novelID, delta int) error
	UpdateNovelLatest(ctx context.Context, novelID, chapterID int, title string, at time.Time) error
	RefreshNovelStats(ctx context.Context, novelID int) error

	// 章节
	ListChapters(ctx context.Context, novelID, limit, offset int) ([]*domain.NovelChapter, error)
	CountChapters(ctx context.Context, novelID int) (int64, error)
	MaxChapterNum(ctx context.Context, novelID int) (int, error)
	FindChapterByID(ctx context.Context, id int) (*domain.NovelChapter, error)
	FindChapterByNum(ctx context.Context, novelID, chapterNum int) (*domain.NovelChapter, error)
	FindAdjacentChapter(ctx context.Context, novelID, chapterNum int, next bool) (*domain.NovelChapter, error)
	CreateChapter(ctx context.Context, chapter *domain.NovelChapter) error
	DeleteChapterCascade(ctx context.Context, chapterID, novelID int) error
	IncrChapterRead(ctx context.Context, chapterID int) error
	IncrChapterComment(ctx context.Context, chapterID int, delta int) error

	// 评论（书评 / 章评 / 段评）
	ListComments(ctx context.Context, filter domain.NovelCommentFilter) ([]*domain.NovelComment, error)
	CountComments(ctx context.Context, filter domain.NovelCommentFilter) (int64, error)
	ParagraphCommentCounts(ctx context.Context, chapterID int) (map[int]int, error)
	FindCommentByID(ctx context.Context, id int) (*domain.NovelComment, error)
	CreateComment(ctx context.Context, comment *domain.NovelComment) error
	DeleteCommentCascade(ctx context.Context, commentID int) error
	ToggleCommentLike(ctx context.Context, commentID, userID int) error
	CommentLikedSet(ctx context.Context, userID int, commentIDs []int) (map[int]bool, error)

	// 评分（1–5 星，每人每本一条，可改）
	// UpsertRating 返回改之前的分数（0 = 首次）与更新后的书本。
	UpsertRating(ctx context.Context, novelID, userID, score int) (prevScore int, novel *domain.Novel, err error)
	FindRating(ctx context.Context, novelID, userID int) (int, error)
	MyRatings(ctx context.Context, userID int, novelIDs []int) (map[int]int, error)

	// 书架与阅读进度
	ToggleShelf(ctx context.Context, novelID, userID int) (bool, error)
	IsOnShelf(ctx context.Context, novelID, userID int) (bool, error)
	OnShelfNovelIDs(ctx context.Context, userID int, novelIDs []int) (map[int]bool, error)
	UpsertProgress(ctx context.Context, p *domain.NovelProgress) error
	FindProgress(ctx context.Context, novelID, userID int) (*domain.NovelProgress, error)
	ProgressByNovelIDs(ctx context.Context, userID int, novelIDs []int) (map[int]*domain.NovelProgress, error)

	// 用户
	UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
	FindUserByID(ctx context.Context, id int) (*domain.User, error)
}
