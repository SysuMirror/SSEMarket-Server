package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	// 积分服务传 nil：这组测试只覆盖改书逻辑，积分挂钩另有测试。
	return service.NewNovelService(repo, nil), repo
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
