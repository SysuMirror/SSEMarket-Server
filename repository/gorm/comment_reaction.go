package gormrepo

import (
	"context"
	"errors"
	"fmt"

	"loginTest/domain"
	"loginTest/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockPcommentForUpdate 锁住一级评论行，串行化同一条评论上的反应切换。
func lockPcommentForUpdate(tx *gorm.DB, pcommentID int) error {
	var id int
	if err := tx.Raw(
		"SELECT pcommentID FROM pcomments WHERE pcommentID = ? FOR UPDATE", pcommentID,
	).Scan(&id).Error; err != nil {
		return err
	}
	if id == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// lockCcommentForUpdate 锁住楼中楼行，串行化同一条楼中楼上的反应切换。
func lockCcommentForUpdate(tx *gorm.DB, ccommentID int) error {
	var id int
	if err := tx.Raw(
		"SELECT ccommentID FROM ccomments WHERE ccommentID = ? FOR UPDATE", ccommentID,
	).Scan(&id).Error; err != nil {
		return err
	}
	if id == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// toggleCommentReaction 在同一个事务内完成评论反应切换：
// 已是指定反应则取消；是相反反应则改到指定反应；没有反应则新建。
func (r *commentRepository) toggleCommentReaction(
	ctx context.Context,
	userID, targetID int,
	targetType, desired string,
	lock func(*gorm.DB) error,
	adjust func(*gorm.DB, string, int) error,
) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}

		var current model.CommentReaction
		err := tx.Where(
			"userID = ? AND targetType = ? AND targetID = ?",
			userID, targetType, targetID,
		).First(&current).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&model.CommentReaction{
					UserID:     userID,
					TargetType: targetType,
					TargetID:   targetID,
					Reaction:   desired,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			return adjust(tx, desired, 1)
		case err != nil:
			return err
		}

		if current.Reaction == desired {
			res := tx.Where("reactionID = ?", current.ReactionID).
				Delete(&model.CommentReaction{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return nil
			}
			return adjust(tx, desired, -1)
		}

		res := tx.Model(&model.CommentReaction{}).
			Where("reactionID = ? AND reaction = ?", current.ReactionID, current.Reaction).
			UpdateColumn("reaction", desired)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrConflict
		}
		if err := adjust(tx, current.Reaction, -1); err != nil {
			return err
		}
		return adjust(tx, desired, 1)
	})
}

func adjustPcommentReactionCount(tx *gorm.DB, pcommentID int, reaction string, delta int) error {
	column, err := reactionColumn(reaction)
	if err != nil {
		return err
	}
	return tx.Model(&model.Pcomment{}).Where("pcommentID = ?", pcommentID).
		UpdateColumn(column, gorm.Expr("GREATEST("+column+" + ?, 0)", delta)).Error
}

func adjustCcommentReactionCount(tx *gorm.DB, ccommentID int, reaction string, delta int) error {
	column, err := reactionColumn(reaction)
	if err != nil {
		return err
	}
	return tx.Model(&model.Ccomment{}).Where("ccommentID = ?", ccommentID).
		UpdateColumn(column, gorm.Expr("GREATEST("+column+" + ?, 0)", delta)).Error
}

func reactionColumn(reaction string) (string, error) {
	switch reaction {
	case model.ReactionLike:
		return "like_num", nil
	case model.ReactionDeny:
		return "deny_num", nil
	default:
		return "", fmt.Errorf("unknown reaction %q", reaction)
	}
}

func (r *commentRepository) TogglePcommentLikeOnce(
	ctx context.Context, userID, pcommentID int,
) error {
	return r.toggleCommentReaction(
		ctx, userID, pcommentID,
		model.CommentTargetPcomment, model.ReactionLike,
		func(tx *gorm.DB) error { return lockPcommentForUpdate(tx, pcommentID) },
		func(tx *gorm.DB, reaction string, delta int) error {
			return adjustPcommentReactionCount(tx, pcommentID, reaction, delta)
		},
	)
}

func (r *commentRepository) TogglePcommentDenyOnce(
	ctx context.Context, userID, pcommentID int,
) error {
	return r.toggleCommentReaction(
		ctx, userID, pcommentID,
		model.CommentTargetPcomment, model.ReactionDeny,
		func(tx *gorm.DB) error { return lockPcommentForUpdate(tx, pcommentID) },
		func(tx *gorm.DB, reaction string, delta int) error {
			return adjustPcommentReactionCount(tx, pcommentID, reaction, delta)
		},
	)
}

func (r *commentRepository) ToggleCcommentLikeOnce(
	ctx context.Context, userID, ccommentID int,
) error {
	return r.toggleCommentReaction(
		ctx, userID, ccommentID,
		model.CommentTargetCcomment, model.ReactionLike,
		func(tx *gorm.DB) error { return lockCcommentForUpdate(tx, ccommentID) },
		func(tx *gorm.DB, reaction string, delta int) error {
			return adjustCcommentReactionCount(tx, ccommentID, reaction, delta)
		},
	)
}

func (r *commentRepository) ToggleCcommentDenyOnce(
	ctx context.Context, userID, ccommentID int,
) error {
	return r.toggleCommentReaction(
		ctx, userID, ccommentID,
		model.CommentTargetCcomment, model.ReactionDeny,
		func(tx *gorm.DB) error { return lockCcommentForUpdate(tx, ccommentID) },
		func(tx *gorm.DB, reaction string, delta int) error {
			return adjustCcommentReactionCount(tx, ccommentID, reaction, delta)
		},
	)
}
