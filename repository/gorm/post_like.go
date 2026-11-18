package gormrepo

import (
	"context"
	"time"

	"loginTest/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *postRepository) CreateLikeOnce(
	ctx context.Context, userID, postID int, heatDelta float64,
) (bool, error) {
	inserted := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		like := model.Plike{
			UserID:    userID,
			PtargetID: postID,
			Time:      time.Now(),
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Omit("Post", "User").
			Create(&like)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		inserted = true

		if err := tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("like_num", gorm.Expr("like_num + ?", 1)).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("heat", gorm.Expr("heat + ?", heatDelta)).Error
	})
	return inserted, err
}

func (r *postRepository) DeleteLikeOnce(
	ctx context.Context, userID, postID int,
) (bool, error) {
	deleted := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("userID = ? AND ptargetID = ?", userID, postID).
			Delete(&model.Plike{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		deleted = true

		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("like_num", gorm.Expr("GREATEST(like_num - ?, 0)", 1)).Error
	})
	return deleted, err
}
