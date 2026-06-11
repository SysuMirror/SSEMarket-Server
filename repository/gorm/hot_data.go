package gormrepo

import (
	"context"

	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type hotDataRepository struct {
	db *gorm.DB
}

func NewHotDataRepository(db *gorm.DB) repository.HotDataRepository {
	return &hotDataRepository{db: db}
}

func (r *hotDataRepository) GetBrowseNum(_ context.Context, postID int) int64 {
	var p model.Post
	if err := r.db.Select("browse_num").Where("postID = ?", postID).First(&p).Error; err != nil {
		return 0
	}
	return int64(p.BrowseNum)
}

func (r *hotDataRepository) UpdateBrowseNum(_ context.Context, postID int, browseNum int64) error {
	return r.db.Model(&model.Post{}).
		Where("postID = ?", postID).
		Update("browse_num", browseNum).Error
}

func (r *hotDataRepository) ListBrowseNumsForWarmup(_ context.Context, limit int) ([]repository.PostBrowseNum, error) {
	if limit <= 0 {
		limit = 500
	}
	var rows []model.Post
	err := r.db.Select("postID", "browse_num").
		Order("post_time DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]repository.PostBrowseNum, 0, len(rows))
	for _, p := range rows {
		out = append(out, repository.PostBrowseNum{
			PostID:    p.PostID,
			BrowseNum: int64(p.BrowseNum),
		})
	}
	return out, nil
}
