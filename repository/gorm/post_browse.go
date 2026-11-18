package gormrepo

import (
	"context"
	"time"

	"loginTest/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *postRepository) CreateBrowseOnce(
	ctx context.Context, userID, postID int, heatDelta float64,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Omit(clause.Associations).
			Create(&model.Pbrowse{
				UserID:    userID,
				PtargetID: postID,
				Time:      time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("heat", gorm.Expr("heat + ?", heatDelta)).Error
	})
}
