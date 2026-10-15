package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/repository"
)

const (
	resumeMaxPerUser = 5
	// 官方发帖账号与参数：新简历上架后以该账号在「实习就业」分区自动发帖公告。
	officialAccountEmail  = "ssemarket@163.com"
	resumePostPartition   = "实习就业"
	resumePostTitlePrefix = "【新简历】"
	resumePostURLBase     = "https://ssemarket.cn/new/resume/"
)

type resumeService struct {
	resumes repository.ResumeRepository
	users   repository.UserRepository
	posts   PostService
}

func NewResumeService(resumes repository.ResumeRepository, users repository.UserRepository, posts PostService) ResumeService {
	return &resumeService{resumes: resumes, users: users, posts: posts}
}

func (s *resumeService) CanUpload(ctx context.Context, userID int) error {
	count, err := s.resumes.CountByUser(ctx, userID)
	if err != nil {
		return err
	}
	if count >= resumeMaxPerUser {
		return domain.ErrBadRequest
	}
	return nil
}

func (s *resumeService) Create(ctx context.Context, in domain.ResumeCreateInput) (int, error) {
	if err := s.CanUpload(ctx, in.UserID); err != nil {
		return 0, err
	}
	title, description, err := validateResumeMeta(in.Title, in.Description)
	if err != nil {
		return 0, err
	}
	r := &domain.Resume{
		UserID:      in.UserID,
		Title:       title,
		Description: description,
		FileName:    in.FileName,
		CosKey:      in.CosKey,
		FileSize:    in.FileSize,
		MimeType:    in.MimeType,
		IsAnonymous: in.IsAnonymous,
	}
	if err := s.resumes.Create(ctx, r); err != nil {
		return 0, err
	}
	go s.announceNewResume(r)
	return r.ID, nil
}

// announceNewResume 以官方账号发帖公告新简历；best-effort，任何失败只记日志不影响上传。
func (s *resumeService) announceNewResume(resume *domain.Resume) {
	defer func() { _ = recover() }()
	if resume == nil || resume.ID == 0 {
		return
	}
	ctx := context.Background()
	official, err := s.users.FindByEmail(ctx, officialAccountEmail)
	if err != nil || official == nil || official.ID == 0 {
		fmt.Printf("[ResumeAnnounce] official account %s not found: %v\n", officialAccountEmail, err)
		return
	}

	title := resume.Title
	if utf8.RuneCountInString(title) > 60 {
		title = string([]rune(title)[:60]) + "…"
	}
	var b strings.Builder
	b.WriteString("有同学发布了一份新简历，欢迎大家围观交流。\n\n")
	b.WriteString("简历标题：" + resume.Title + "\n")
	if desc := strings.TrimSpace(resume.Description); desc != "" {
		b.WriteString("\n" + desc + "\n")
	}
	b.WriteString("\n查看简历：" + resumePostURLBase + strconv.Itoa(resume.ID))

	if _, err := s.posts.Create(ctx, domain.PostCreateInput{
		UserID:    official.ID,
		Title:     resumePostTitlePrefix + title,
		Content:   b.String(),
		Partition: resumePostPartition,
	}); err != nil {
		fmt.Printf("[ResumeAnnounce] create post for resume %d failed: %v\n", resume.ID, err)
	}
}

func (s *resumeService) List(ctx context.Context, filter domain.ResumeFilter) ([]*domain.Resume, error) {
	return s.resumes.List(ctx, filter)
}

func (s *resumeService) GetByID(ctx context.Context, id int) (*domain.Resume, error) {
	return s.resumes.FindByID(ctx, id)
}

func (s *resumeService) Detail(ctx context.Context, id int) (*domain.Resume, error) {
	r, err := s.resumes.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	_ = s.resumes.IncrViewNum(ctx, id)
	r.ViewNum++
	return r, nil
}

func (s *resumeService) RecordDownload(ctx context.Context, id int) error {
	if _, err := s.resumes.FindByID(ctx, id); err != nil {
		return err
	}
	return s.resumes.IncrDownloadNum(ctx, id)
}

func (s *resumeService) Update(ctx context.Context, in domain.ResumeUpdateInput) (*domain.Resume, error) {
	title, description, err := validateResumeMeta(in.Title, in.Description)
	if err != nil {
		return nil, err
	}
	r, err := s.resumes.FindByID(ctx, in.ResumeID)
	if err != nil {
		return nil, err
	}
	if r.UserID != in.UserID {
		return nil, domain.ErrForbidden
	}
	if err := s.resumes.UpdateMeta(ctx, in.ResumeID, title, description); err != nil {
		return nil, err
	}
	r.Title = title
	r.Description = description
	return r, nil
}

func (s *resumeService) Delete(ctx context.Context, id, userID int) (string, error) {
	r, err := s.resumes.FindByID(ctx, id)
	if err != nil {
		return "", err
	}
	if r.UserID != userID {
		return "", domain.ErrForbidden
	}
	if err := s.resumes.SoftDelete(ctx, id); err != nil {
		return "", err
	}
	return r.CosKey, nil
}

func (s *resumeService) UploadersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error) {
	return s.resumes.UsersByIDs(ctx, ids)
}

func validateResumeMeta(title, description string) (string, string, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" {
		return "", "", domain.ErrBadRequest
	}
	if utf8.RuneCountInString(title) > 50 {
		return "", "", domain.ErrBadRequest
	}
	if utf8.RuneCountInString(description) > 500 {
		return "", "", domain.ErrBadRequest
	}
	return title, description, nil
}
