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
