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

type resumeRepository struct {
	db *gorm.DB
}

func NewResumeRepository(db *gorm.DB) repository.ResumeRepository {
	return &resumeRepository{db: db}
}

func (r *resumeRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *resumeRepository) CountByUser(ctx context.Context, userID int) (int64, error) {
	var n int64
	err := r.session(ctx).Model(&model.Resume{}).
		Where("userID = ? AND is_delete = ?", userID, false).
		Count(&n).Error
	return n, err
}

func (r *resumeRepository) Create(ctx context.Context, resume *domain.Resume) error {
	now := time.Now()
	m := model.Resume{
		UserID:      resume.UserID,
		Title:       resume.Title,
		Description: resume.Description,
		FileName:    resume.FileName,
		CosKey:      resume.CosKey,
		FileSize:    resume.FileSize,
		MimeType:    resume.MimeType,
		IsAnonymous: resume.IsAnonymous,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	resume.ID = m.ResumeID
	resume.CreatedAt = m.CreatedAt
	resume.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *resumeRepository) List(ctx context.Context, filter domain.ResumeFilter) ([]*domain.Resume, error) {
	q := r.session(ctx).Model(&model.Resume{}).Where("is_delete = ?", false)
	if filter.SearchSort == "my" {
		q = q.Where("userID = ?", filter.UserID)
	}
	if s := strings.TrimSpace(filter.SearchInfo); s != "" {
		like := "%" + s + "%"
		q = q.Where("(title LIKE ? OR description LIKE ? OR file_name LIKE ?)", like, like, like)
	}
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var list []model.Resume
	err := q.Order("resumeID DESC").Offset(offset).Limit(limit).Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Resume, 0, len(list))
	for i := range list {
		out = append(out, resumeToDomain(&list[i]))
	}
	return out, nil
}

func (r *resumeRepository) FindByID(ctx context.Context, id int) (*domain.Resume, error) {
	var m model.Resume
	err := r.session(ctx).Where("resumeID = ? AND is_delete = ?", id, false).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return resumeToDomain(&m), nil
}

func (r *resumeRepository) IncrViewNum(ctx context.Context, id int) error {
	return r.session(ctx).Model(&model.Resume{}).Where("resumeID = ?", id).
		UpdateColumn("view_num", gorm.Expr("view_num + ?", 1)).Error
}

func (r *resumeRepository) IncrDownloadNum(ctx context.Context, id int) error {
	return r.session(ctx).Model(&model.Resume{}).Where("resumeID = ?", id).
		UpdateColumn("download_num", gorm.Expr("download_num + ?", 1)).Error
}

func (r *resumeRepository) UpdateMeta(ctx context.Context, id int, title, description string) error {
	now := time.Now()
	res := r.session(ctx).Model(&model.Resume{}).Where("resumeID = ? AND is_delete = ?", id, false).
		Updates(map[string]interface{}{
			"title":       title,
			"description": description,
			"updated_at":  now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *resumeRepository) SoftDelete(ctx context.Context, id int) error {
	res := r.session(ctx).Model(&model.Resume{}).Where("resumeID = ?", id).Update("is_delete", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *resumeRepository) UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error) {
	m := make(map[int]*domain.User)
	if len(ids) == 0 {
		return m, nil
	}
	var users []model.User
	if err := r.session(ctx).Where("userID IN (?)", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		m[users[i].UserID] = userToDomain(&users[i])
	}
	return m, nil
}
