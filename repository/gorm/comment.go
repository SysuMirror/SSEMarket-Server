package gormrepo

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"
	"loginTest/util"

	"gorm.io/gorm"
)

type commentRepository struct {
	db *gorm.DB
}

func NewCommentRepository(db *gorm.DB) repository.CommentRepository {
	return &commentRepository{db: db}
}

func (r *commentRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *commentRepository) ListPcommentsByPostID(ctx context.Context, postID int) ([]*domain.Pcomment, error) {
	var rows []model.Pcomment
	err := r.session(ctx).Where("ptargetID = ?", postID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Pcomment, 0, len(rows))
	for i := range rows {
		out = append(out, pcommentToDomain(&rows[i]))
	}
	return out, nil
}

func (r *commentRepository) FindPcommentByID(ctx context.Context, id int) (*domain.Pcomment, error) {
	var row model.Pcomment
	err := r.session(ctx).Where("pcommentID = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return pcommentToDomain(&row), nil
}

func (r *commentRepository) CreatePcomment(ctx context.Context, c *domain.Pcomment) error {
	row := model.Pcomment{
		UserID:    c.UserID,
		PtargetID: c.PostID,
		Pctext:    c.Content,
		Time:      c.CreatedAt,
		LikeNum:   c.LikeNum,
		DenyNum:   c.DenyNum,
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return err
	}
	c.ID = row.PcommentID
	return nil
}

func (r *commentRepository) DeletePcomment(ctx context.Context, id int) error {
	return r.session(ctx).Where("pcommentID = ?", id).Delete(&model.Pcomment{}).Error
}

func (r *commentRepository) CountCcommentsByPcommentID(ctx context.Context, pcommentID int) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Ccomment{}).Where("ctargetID = ?", pcommentID).Count(&count).Error
	return count, err
}

func (r *commentRepository) ListCcommentsByPcommentID(ctx context.Context, pcommentID int) ([]*domain.Ccomment, error) {
	var rows []model.Ccomment
	err := r.session(ctx).Where("ctargetID = ?", pcommentID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Ccomment, 0, len(rows))
	for i := range rows {
		out = append(out, ccommentToDomain(&rows[i]))
	}
	return out, nil
}

func (r *commentRepository) FindCcommentByID(ctx context.Context, id int) (*domain.Ccomment, error) {
	var row model.Ccomment
	err := r.session(ctx).Where("ccommentID = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ccommentToDomain(&row), nil
}

func (r *commentRepository) CreateCcomment(ctx context.Context, c *domain.Ccomment) error {
	row := model.Ccomment{
		UserID:         c.UserID,
		CtargetID:      c.PcommentID,
		Cctext:         c.Content,
		Time:           c.CreatedAt,
		LikeNum:        c.LikeNum,
		DenyNum:        c.DenyNum,
		UserTargetName: c.UserTargetName,
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return err
	}
	c.ID = row.CcommentID
	return nil
}

func (r *commentRepository) DeleteCcomment(ctx context.Context, id int) error {
	return r.session(ctx).Where("ccommentID = ?", id).Delete(&model.Ccomment{}).Error
}

func (r *commentRepository) commentReactionSet(
	ctx context.Context, userID int, targetType, reaction string, targetIDs []int,
) (map[int]bool, error) {
	out := make(map[int]bool)
	if len(targetIDs) == 0 {
		return out, nil
	}
	var rows []model.CommentReaction
	err := r.session(ctx).
		Where("userID = ? AND targetType = ? AND reaction = ? AND targetID IN ?",
			userID, targetType, reaction, targetIDs).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.TargetID] = true
	}
	return out, nil
}

func (r *commentRepository) PcommentLikedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error) {
	return r.commentReactionSet(ctx, userID, model.CommentTargetPcomment, model.ReactionLike, pcommentIDs)
}

func (r *commentRepository) PcommentDeniedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error) {
	return r.commentReactionSet(ctx, userID, model.CommentTargetPcomment, model.ReactionDeny, pcommentIDs)
}

func (r *commentRepository) CcommentLikedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error) {
	return r.commentReactionSet(ctx, userID, model.CommentTargetCcomment, model.ReactionLike, ccommentIDs)
}

func (r *commentRepository) CcommentDeniedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error) {
	return r.commentReactionSet(ctx, userID, model.CommentTargetCcomment, model.ReactionDeny, ccommentIDs)
}

func (r *commentRepository) CcommentReaction(ctx context.Context, userID, ccommentID int) (liked, denied bool, err error) {
	var row model.CommentReaction
	err = r.session(ctx).
		Where("userID = ? AND targetType = ? AND targetID = ?",
			userID, model.CommentTargetCcomment, ccommentID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return row.Reaction == model.ReactionLike, row.Reaction == model.ReactionDeny, nil
}

func (r *commentRepository) hasCommentReaction(
	ctx context.Context, userID int, targetType string, targetID int, reaction string,
) (bool, error) {
	var count int64
	err := r.session(ctx).Model(&model.CommentReaction{}).
		Where("userID = ? AND targetType = ? AND targetID = ? AND reaction = ?",
			userID, targetType, targetID, reaction).
		Count(&count).Error
	return count > 0, err
}

func (r *commentRepository) FindPcommentLike(ctx context.Context, userID, pcommentID int) (bool, error) {
	return r.hasCommentReaction(ctx, userID, model.CommentTargetPcomment, pcommentID, model.ReactionLike)
}

func (r *commentRepository) FindPcommentDeny(ctx context.Context, userID, pcommentID int) (bool, error) {
	return r.hasCommentReaction(ctx, userID, model.CommentTargetPcomment, pcommentID, model.ReactionDeny)
}

func (r *commentRepository) FindCcommentLike(ctx context.Context, userID, ccommentID int) (bool, error) {
	return r.hasCommentReaction(ctx, userID, model.CommentTargetCcomment, ccommentID, model.ReactionLike)
}

func (r *commentRepository) FindCcommentDeny(ctx context.Context, userID, ccommentID int) (bool, error) {
	return r.hasCommentReaction(ctx, userID, model.CommentTargetCcomment, ccommentID, model.ReactionDeny)
}

func (r *commentRepository) UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error) {
	out := make(map[int]*domain.User)
	if len(ids) == 0 {
		return out, nil
	}
	var users []model.User
	if err := r.session(ctx).Where("userID IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		out[users[i].UserID] = userToDomain(&users[i])
	}
	return out, nil
}

func (r *commentRepository) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	phone = util.NormalizePhone(phone)
	if !util.IsQueryablePhone(phone) {
		return nil, domain.ErrBadRequest
	}
	var u model.User
	err := r.session(ctx).Where("phone = ?", phone).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return userToDomain(&u), nil
}

func (r *commentRepository) FindUserByID(ctx context.Context, id int) (*domain.User, error) {
	var u model.User
	err := r.session(ctx).Where("userID = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return userToDomain(&u), nil
}

func (r *commentRepository) FindUsersByExactNames(ctx context.Context, names []string) ([]*domain.User, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var users []model.User
	if err := r.session(ctx).Where("name IN ?", names).Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.User, 0, len(users))
	for i := range users {
		out = append(out, userToDomain(&users[i]))
	}
	return out, nil
}

func (r *commentRepository) FindPostByID(ctx context.Context, id int) (*domain.Post, error) {
	var post model.Post
	err := r.session(ctx).Where("postID = ?", id).First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return postToDomain(&post), nil
}

func (r *commentRepository) IncrPostCommentNum(ctx context.Context, postID, delta int) error {
	return r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("comment_num", gorm.Expr("comment_num + ?", delta)).Error
}

func (r *commentRepository) AdjustPostHeat(ctx context.Context, postID int, delta float64) error {
	return r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("heat", gorm.Expr("heat + ?", delta)).Error
}

func (r *commentRepository) CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error {
	// ntext 列 varchar(1000)：@小秋 长回帖会把原文塞爆（MySQL 严格模式下静默插失败=通知丢失）。
	// 一律 900 rune 截断加省略号——顺带修历史隐患（超长评论通知此前也会丢）。
	if utf8.RuneCountInString(text) > 900 {
		text = string([]rune(text)[:900]) + "…"
	}
	notice := model.Notice{
		Receiver: receiver,
		Sender:   sender,
		Type:     noticeType,
		Ntext:    text,
		Time:     time.Now(),
		Read:     false,
		Target:   target,
	}
	return r.session(ctx).Create(&notice).Error
}
