package gormrepo

import (
	"context"

	"loginTest/domain"
	"loginTest/model"

	"gorm.io/gorm"
)

func (r *commentRepository) CreatePcommentAndAdjustPost(
	ctx context.Context, c *domain.Pcomment, commentDelta int, heatDelta float64,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.Pcomment{
			UserID:    c.UserID,
			PtargetID: c.PostID,
			Pctext:    c.Content,
			Time:      c.CreatedAt,
			LikeNum:   c.LikeNum,
			DenyNum:   c.DenyNum,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		c.ID = row.PcommentID

		if err := tx.Model(&model.Post{}).Where("postID = ?", c.PostID).
			UpdateColumn("comment_num", gorm.Expr("comment_num + ?", commentDelta)).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("postID = ?", c.PostID).
			UpdateColumn("heat", gorm.Expr("heat + ?", heatDelta)).Error
	})
}

func (r *commentRepository) CreateCcommentAndAdjustPost(
	ctx context.Context, c *domain.Ccomment, postID, commentDelta int, heatDelta float64,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.Ccomment{
			UserID:         c.UserID,
			CtargetID:      c.PcommentID,
			Cctext:         c.Content,
			Time:           c.CreatedAt,
			LikeNum:        c.LikeNum,
			DenyNum:        c.DenyNum,
			UserTargetName: c.UserTargetName,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		c.ID = row.CcommentID

		if err := tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("comment_num", gorm.Expr("comment_num + ?", commentDelta)).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("heat", gorm.Expr("heat + ?", heatDelta)).Error
	})
}

func (r *commentRepository) DeletePcommentAndAdjustPost(
	ctx context.Context, id, postID, commentDelta int, heatDelta float64,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("pcommentID = ?", id).Delete(&model.Pcomment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		if err := tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("comment_num", gorm.Expr("GREATEST(comment_num + ?, 0)", commentDelta)).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("heat", gorm.Expr("GREATEST(heat + ?, 0)", heatDelta)).Error
	})
}

func (r *commentRepository) DeleteCcommentAndAdjustPost(
	ctx context.Context, id, postID, commentDelta int, heatDelta float64,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("ccommentID = ?", id).Delete(&model.Ccomment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}

		if err := tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("comment_num", gorm.Expr("GREATEST(comment_num + ?, 0)", commentDelta)).Error; err != nil {
			return err
		}
		return tx.Model(&model.Post{}).Where("postID = ?", postID).
			UpdateColumn("heat", gorm.Expr("GREATEST(heat + ?, 0)", heatDelta)).Error
	})
}
