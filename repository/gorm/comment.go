package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

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

func (r *commentRepository) PcommentLikedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error) {
	out := make(map[int]bool)
	if len(pcommentIDs) == 0 {
		return out, nil
	}
	var likes []model.Pclike
	err := r.session(ctx).Where("userID = ? AND pctargetID IN ?", userID, pcommentIDs).Find(&likes).Error
	if err != nil {
		return nil, err
	}
	for _, like := range likes {
		out[like.PctargetID] = true
	}
	return out, nil
}

func (r *commentRepository) PcommentDeniedSet(ctx context.Context, userID int, pcommentIDs []int) (map[int]bool, error) {
	out := make(map[int]bool)
	if len(pcommentIDs) == 0 {
		return out, nil
	}
	var denies []model.Pcdeny
	err := r.session(ctx).Where("userID = ? AND pctargetID IN ?", userID, pcommentIDs).Find(&denies).Error
	if err != nil {
		return nil, err
	}
	for _, deny := range denies {
		out[deny.PctargetID] = true
	}
	return out, nil
}

func (r *commentRepository) CcommentLikedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error) {
	out := make(map[int]bool)
	if len(ccommentIDs) == 0 {
		return out, nil
	}
	var likes []model.Cclike
	err := r.session(ctx).Where("userID = ? AND cctargetID IN ?", userID, ccommentIDs).Find(&likes).Error
	if err != nil {
		return nil, err
	}
	for _, like := range likes {
		out[like.CctargetID] = true
	}
	return out, nil
}

func (r *commentRepository) CcommentDeniedSet(ctx context.Context, userID int, ccommentIDs []int) (map[int]bool, error) {
	out := make(map[int]bool)
	if len(ccommentIDs) == 0 {
		return out, nil
	}
	var denies []model.Ccdeny
	err := r.session(ctx).Where("userID = ? AND cctargetID IN ?", userID, ccommentIDs).Find(&denies).Error
	if err != nil {
		return nil, err
	}
	for _, deny := range denies {
		out[deny.CctargetID] = true
	}
	return out, nil
}

func (r *commentRepository) CcommentReaction(ctx context.Context, userID, ccommentID int) (liked, denied bool, err error) {
	likeMap, err := r.CcommentLikedSet(ctx, userID, []int{ccommentID})
	if err != nil {
		return false, false, err
	}
	denyMap, err := r.CcommentDeniedSet(ctx, userID, []int{ccommentID})
	if err != nil {
		return false, false, err
	}
	return likeMap[ccommentID], denyMap[ccommentID], nil
}

func (r *commentRepository) FindPcommentLike(ctx context.Context, userID, pcommentID int) (bool, error) {
	var like model.Pclike
	err := r.session(ctx).Where("userID = ? AND pctargetID = ?", userID, pcommentID).First(&like).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return like.PclikeID != 0, nil
}

func (r *commentRepository) FindPcommentDeny(ctx context.Context, userID, pcommentID int) (bool, error) {
	var deny model.Pcdeny
	err := r.session(ctx).Where("userID = ? AND pctargetID = ?", userID, pcommentID).First(&deny).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return deny.PcdenyID != 0, nil
}

func (r *commentRepository) CreatePcommentLike(ctx context.Context, userID, pcommentID int) error {
	return r.session(ctx).Create(&model.Pclike{UserID: userID, PctargetID: pcommentID}).Error
}

func (r *commentRepository) DeletePcommentLike(ctx context.Context, userID, pcommentID int) error {
	return r.session(ctx).Where("userID = ? AND pctargetID = ?", userID, pcommentID).Delete(&model.Pclike{}).Error
}

func (r *commentRepository) CreatePcommentDeny(ctx context.Context, userID, pcommentID int) error {
	return r.session(ctx).Create(&model.Pcdeny{UserID: userID, PctargetID: pcommentID}).Error
}

func (r *commentRepository) DeletePcommentDeny(ctx context.Context, userID, pcommentID int) error {
	return r.session(ctx).Where("userID = ? AND pctargetID = ?", userID, pcommentID).Delete(&model.Pcdeny{}).Error
}

func (r *commentRepository) IncrPcommentLikeNum(ctx context.Context, pcommentID, delta int) error {
	return r.session(ctx).Model(&model.Pcomment{}).Where("pcommentID = ?", pcommentID).
		UpdateColumn("like_num", gorm.Expr("like_num + ?", delta)).Error
}

func (r *commentRepository) IncrPcommentDenyNum(ctx context.Context, pcommentID, delta int) error {
	return r.session(ctx).Model(&model.Pcomment{}).Where("pcommentID = ?", pcommentID).
		UpdateColumn("deny_num", gorm.Expr("deny_num + ?", delta)).Error
}

func (r *commentRepository) FindCcommentLike(ctx context.Context, userID, ccommentID int) (bool, error) {
	var like model.Cclike
	err := r.session(ctx).Where("userID = ? AND cctargetID = ?", userID, ccommentID).First(&like).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return like.CclikeID != 0, nil
}

func (r *commentRepository) FindCcommentDeny(ctx context.Context, userID, ccommentID int) (bool, error) {
	var deny model.Ccdeny
	err := r.session(ctx).Where("userID = ? AND cctargetID = ?", userID, ccommentID).First(&deny).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return deny.CcdenyID != 0, nil
}

func (r *commentRepository) CreateCcommentLike(ctx context.Context, userID, ccommentID int) error {
	return r.session(ctx).Create(&model.Cclike{UserID: userID, CctargetID: ccommentID}).Error
}

func (r *commentRepository) DeleteCcommentLike(ctx context.Context, userID, ccommentID int) error {
	return r.session(ctx).Where("userID = ? AND cctargetID = ?", userID, ccommentID).Delete(&model.Cclike{}).Error
}

func (r *commentRepository) CreateCcommentDeny(ctx context.Context, userID, ccommentID int) error {
	return r.session(ctx).Create(&model.Ccdeny{UserID: userID, CctargetID: ccommentID}).Error
}

func (r *commentRepository) DeleteCcommentDeny(ctx context.Context, userID, ccommentID int) error {
	return r.session(ctx).Where("userID = ? AND cctargetID = ?", userID, ccommentID).Delete(&model.Ccdeny{}).Error
}

func (r *commentRepository) IncrCcommentLikeNum(ctx context.Context, ccommentID, delta int) error {
	return r.session(ctx).Model(&model.Ccomment{}).Where("ccommentID = ?", ccommentID).
		UpdateColumn("like_num", gorm.Expr("like_num + ?", delta)).Error
}

func (r *commentRepository) IncrCcommentDenyNum(ctx context.Context, ccommentID, delta int) error {
	return r.session(ctx).Model(&model.Ccomment{}).Where("ccommentID = ?", ccommentID).
		UpdateColumn("deny_num", gorm.Expr("deny_num + ?", delta)).Error
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
