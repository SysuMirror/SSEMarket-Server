package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"loginTest/domain"
	"loginTest/repository"
	"loginTest/service"
)

// novelRepoStub 只实现改书用到的两个方法；其余方法走嵌入接口（测试里不会调用）。
type novelRepoStub struct {
	repository.NovelRepository
	novel   *domain.Novel
	findErr error
	updated map[string]any
}

func (s *novelRepoStub) FindNovelByID(_ context.Context, id int) (*domain.Novel, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	if s.novel == nil || s.novel.ID != id {
		return nil, domain.ErrNotFound
	}
	return s.novel, nil
}

func (s *novelRepoStub) UpdateNovelFields(_ context.Context, novelID int, fields map[string]any) error {
	s.updated = fields
	return nil
}

func newNovelServiceStub(novel *domain.Novel) (service.NovelService, *novelRepoStub) {
	repo := &novelRepoStub{novel: novel}
	// 积分与币都传 nil：这组测试只覆盖改书逻辑，积分挂钩另有测试。
	return service.NewNovelService(repo, nil, nil), repo
}

func sampleNovel() *domain.Novel {
	return &domain.Novel{ID: 7, UserID: 100, Title: "晚自习后的银河", CoverURL: "", Category: "校园"}
}

func TestUpdateNovelOnlyChangesGivenFields(t *testing.T) {
	svc, repo := newNovelServiceStub(sampleNovel())
	cover := "https://cdn.example/cover.png"

	if err := svc.UpdateNovel(context.Background(), domain.NovelUpdateInput{
		NovelID:  7,
		UserID:   100,
		CoverURL: &cover,
	}); err != nil {
		t.Fatalf("改封面失败: %v", err)
	}

	if repo.updated["cover_url"] != cover {
		t.Errorf("cover_url = %v, 期望 %q", repo.updated["cover_url"], cover)
	}
	if _, ok := repo.updated["title"]; ok {
		t.Errorf("没传书名时不应更新 title，实际更新了 %v", repo.updated["title"])
	}
	// updated_at 由 repository 层统一补，service 只负责业务列
	if len(repo.updated) != 1 {
		t.Errorf("只应下发 cover_url 一列，实际 %v", repo.updated)
	}
}

func TestUpdateNovelRejectsNonAuthor(t *testing.T) {
	svc, repo := newNovelServiceStub(sampleNovel())
	cover := "https://cdn.example/cover.png"

	err := svc.UpdateNovel(context.Background(), domain.NovelUpdateInput{
		NovelID:  7,
		UserID:   200, // 不是作者
		CoverURL: &cover,
	})
	if !errors.Is(err, domain.ErrNovelNotAuthor) {
		t.Fatalf("err = %v, 期望 ErrNovelNotAuthor", err)
	}
	if repo.updated != nil {
		t.Errorf("非作者不应落库，实际写入 %v", repo.updated)
	}
}

func TestUpdateNovelValidatesInput(t *testing.T) {
	cases := []struct {
		name  string
		input domain.NovelUpdateInput
	}{
		{"书名过短", domain.NovelUpdateInput{NovelID: 7, UserID: 100, Title: strPtr("单")}},
		{"书名过长", domain.NovelUpdateInput{NovelID: 7, UserID: 100, Title: strPtr(strings.Repeat("书", 61))}},
		{"封面链接过长", domain.NovelUpdateInput{NovelID: 7, UserID: 100, CoverURL: strPtr(strings.Repeat("u", 501))}},
		{"简介过长", domain.NovelUpdateInput{NovelID: 7, UserID: 100, Intro: strPtr(strings.Repeat("内", 1001))}},
		{"分类过长", domain.NovelUpdateInput{NovelID: 7, UserID: 100, Category: strPtr(strings.Repeat("类", 21))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newNovelServiceStub(sampleNovel())
			if err := svc.UpdateNovel(context.Background(), tc.input); !errors.Is(err, domain.ErrBadRequest) {
				t.Fatalf("err = %v, 期望 ErrBadRequest", err)
			}
			if repo.updated != nil {
				t.Errorf("参数非法时不应落库，实际写入 %v", repo.updated)
			}
		})
	}
}

func TestUpdateNovelNormalizesStatus(t *testing.T) {
	svc, repo := newNovelServiceStub(sampleNovel())

	if err := svc.UpdateNovel(context.Background(), domain.NovelUpdateInput{
		NovelID:  7,
		UserID:   100,
		Status:   strPtr("完结"), // 非法值应回落成 serializing
	}); err != nil {
		t.Fatalf("改状态失败: %v", err)
	}
	if repo.updated["status"] != domain.NovelStatusSerializing {
		t.Errorf("status = %v, 期望 %q", repo.updated["status"], domain.NovelStatusSerializing)
	}
}

func TestUpdateNovelMissingNovel(t *testing.T) {
	svc, _ := newNovelServiceStub(sampleNovel())
	title := "另一本书名"

	err := svc.UpdateNovel(context.Background(), domain.NovelUpdateInput{
		NovelID: 999, // 不存在
		UserID:  100,
		Title:   &title,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, 期望 ErrNotFound", err)
	}
}

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// 评分自动附带书评（首次新建 / 改分刷新 / 被删重建）
// ---------------------------------------------------------------------------

// ratingRepoStub 只实现评分 + 自动书评用到的方法；其余走嵌入接口（测试里不调用）。
type ratingRepoStub struct {
	repository.NovelRepository
	novel        *domain.Novel
	score        int // 当前存的分数（0 = 没评过）
	commentID    int // 评分行上挂的自动书评 ID
	comments     map[int]*domain.NovelComment
	created      []*domain.NovelComment
	updated      map[int]string
	incrComments int
	nextID       int
}

func (s *ratingRepoStub) FindNovelByID(_ context.Context, id int) (*domain.Novel, error) {
	if s.novel == nil || s.novel.ID != id {
		return nil, domain.ErrNotFound
	}
	return s.novel, nil
}

func (s *ratingRepoStub) UpsertRating(_ context.Context, _, _, score int) (int, *domain.Novel, error) {
	prev := s.score
	s.score = score
	return prev, s.novel, nil
}

func (s *ratingRepoStub) GetRatingCommentID(_ context.Context, _, _ int) (int, error) {
	return s.commentID, nil
}

func (s *ratingRepoStub) SetRatingCommentID(_ context.Context, _, _, commentID int) error {
	s.commentID = commentID
	return nil
}

func (s *ratingRepoStub) UpdateCommentContent(_ context.Context, commentID int, content string) (bool, error) {
	c, ok := s.comments[commentID]
	if !ok {
		return false, nil
	}
	c.Content = content
	s.updated[commentID] = content
	return true, nil
}

func (s *ratingRepoStub) CreateComment(_ context.Context, comment *domain.NovelComment) error {
	comment.ID = s.nextID
	comment.CreatedAt = time.Now()
	s.nextID++
	s.comments[comment.ID] = comment
	s.created = append(s.created, comment)
	return nil
}

func (s *ratingRepoStub) IncrNovelComment(_ context.Context, _ int, _ int) error {
	s.incrComments++
	return nil
}

// readSecondsStub 只覆盖 ReadSecondsFor；其余方法走嵌入接口（测试里不调用）。
type readSecondsStub struct {
	service.NovelPointsService
	seconds int
}

func (s *readSecondsStub) ReadSecondsFor(_ context.Context, _, _ int) (int, error) {
	return s.seconds, nil
}

func newRatingStub(seconds int) (service.NovelService, *ratingRepoStub) {
	repo := &ratingRepoStub{
		novel: &domain.Novel{ID: 7, UserID: 100, Title: "测试书"}, // 作者 100，评分人 42
		comments: map[int]*domain.NovelComment{},
		updated:  map[int]string{},
		nextID:   1,
	}
	return service.NewNovelService(repo, &readSecondsStub{seconds: seconds}, nil), repo
}

func TestRateNovelCreatesRatingComment(t *testing.T) {
	svc, repo := newRatingStub(7500) // 2 小时 5 分钟

	result, err := svc.RateNovel(context.Background(), 7, 42, 5)
	if err != nil {
		t.Fatalf("评分失败: %v", err)
	}
	if !result.CommentCreated {
		t.Fatalf("首次评分应新建自动书评")
	}
	if repo.incrComments != 1 {
		t.Errorf("comment_num 应 +1，实际 %d", repo.incrComments)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %d 条，期望 1", len(repo.created))
	}
	c := repo.created[0]
	if !c.IsRating || c.TargetType != domain.NovelTargetBook {
		t.Errorf("评论应为书评且 isRating=true，得到 type=%q isRating=%v", c.TargetType, c.IsRating)
	}
	want := "★★★★★ 5 星 · 评分时已阅读 2 小时 5 分钟"
	if c.Content != want {
		t.Errorf("content = %q, 期望 %q", c.Content, want)
	}
	if repo.commentID != c.ID {
		t.Errorf("评分行应挂上 comment_id=%d，实际 %d", c.ID, repo.commentID)
	}
}

func TestRateNovelSameScoreDoesNotTouchComment(t *testing.T) {
	svc, repo := newRatingStub(60)

	if _, err := svc.RateNovel(context.Background(), 7, 42, 4); err != nil {
		t.Fatalf("首次评分失败: %v", err)
	}
	result, err := svc.RateNovel(context.Background(), 7, 42, 4)
	if err != nil {
		t.Fatalf("重复评分失败: %v", err)
	}
	if result.CommentCreated || result.Changed {
		t.Fatalf("重复提交同一分数不应新建或改动书评")
	}
	if len(repo.created) != 1 || len(repo.updated) != 0 {
		t.Errorf("created=%d updated=%d，期望 1/0", len(repo.created), len(repo.updated))
	}
}

func TestRateNovelScoreChangeUpdatesComment(t *testing.T) {
	svc, repo := newRatingStub(0) // 没读过

	if _, err := svc.RateNovel(context.Background(), 7, 42, 5); err != nil {
		t.Fatalf("首次评分失败: %v", err)
	}
	if want := "★★★★★ 5 星 · 评分时还没有阅读记录"; repo.created[0].Content != want {
		t.Fatalf("content = %q, 期望 %q", repo.created[0].Content, want)
	}

	result, err := svc.RateNovel(context.Background(), 7, 42, 3)
	if err != nil {
		t.Fatalf("改分失败: %v", err)
	}
	if result.CommentCreated {
		t.Errorf("改分应刷新同一条书评，不该新建")
	}
	if len(repo.created) != 1 {
		t.Errorf("created = %d 条，期望仍是 1", len(repo.created))
	}
	want := "★★★ 3 星 · 评分时还没有阅读记录"
	if got := repo.updated[repo.commentID]; got != want {
		t.Errorf("刷新后 content = %q, 期望 %q", got, want)
	}
}

func TestRateNovelRecreatesDeletedRatingComment(t *testing.T) {
	svc, repo := newRatingStub(120)

	if _, err := svc.RateNovel(context.Background(), 7, 42, 5); err != nil {
		t.Fatalf("首次评分失败: %v", err)
	}
	// 模拟作者将自动书评当垃圾评论删掉
	delete(repo.comments, repo.commentID)

	result, err := svc.RateNovel(context.Background(), 7, 42, 4)
	if err != nil {
		t.Fatalf("改分失败: %v", err)
	}
	if !result.CommentCreated {
		t.Fatalf("书评被删后改分应重建一条")
	}
	if len(repo.created) != 2 {
		t.Errorf("created = %d 条，期望 2", len(repo.created))
	}
	if repo.commentID != repo.created[1].ID {
		t.Errorf("评分行应改挂新评论 %d，实际 %d", repo.created[1].ID, repo.commentID)
	}
}
