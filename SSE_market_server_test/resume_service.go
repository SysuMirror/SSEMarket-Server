package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/repository"
)

const (
	resumeMaxPerUser = 5
)

type resumeService struct {
	resumes repository.ResumeRepository
}

func NewResumeService(resumes repository.ResumeRepository) ResumeService {
	return &resumeService{resumes: resumes}
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
	return r.ID, nil
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
