package gormrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type ratingRepository struct {
	db *gorm.DB
}

func NewRatingRepository(db *gorm.DB) repository.RatingRepository {
	return &ratingRepository{db: db}
}

func (r *ratingRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *ratingRepository) ListCategories(ctx context.Context) ([]*domain.RatingCategory, error) {
	var list []model.RatingCategory
	if err := r.session(ctx).Order("created_at DESC, category_id DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.RatingCategory, 0, len(list))
	for i := range list {
		out = append(out, ratingCategoryToDomain(&list[i]))
	}
	return out, nil
}

func (r *ratingRepository) FindCategoryByName(ctx context.Context, name string) (*domain.RatingCategory, error) {
	var m model.RatingCategory
	err := r.session(ctx).Where("name = ?", name).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return ratingCategoryToDomain(&m), nil
}

func (r *ratingRepository) FindCategoryByID(ctx context.Context, id int) (*domain.RatingCategory, error) {
	var m model.RatingCategory
	err := r.session(ctx).Where("category_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return ratingCategoryToDomain(&m), nil
}

func (r *ratingRepository) MaxCategorySortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	err := r.session(ctx).Model(&model.RatingCategory{}).
		Select("COALESCE(MAX(sort_order), 0)").Row().Scan(&maxOrder)
	return maxOrder, err
}

func (r *ratingRepository) CreateCategory(ctx context.Context, cat *domain.RatingCategory) error {
	m := model.RatingCategory{
		Name:        cat.Name,
		Description: cat.Description,
		CreatorID:   cat.CreatorID,
		IsOfficial:  cat.IsOfficial,
		SortOrder:   cat.SortOrder,
		CreatedAt:   cat.CreatedAt,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	cat.ID = m.CategoryID
	return nil
}

func (r *ratingRepository) CreateCategoryWithFeedPost(ctx context.Context, cat *domain.RatingCategory, user *domain.User, feedTitle string) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		m := model.RatingCategory{
			Name:        cat.Name,
			Description: cat.Description,
			CreatorID:   cat.CreatorID,
			IsOfficial:  cat.IsOfficial,
			SortOrder:   cat.SortOrder,
			CreatedAt:   cat.CreatedAt,
		}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		cat.ID = m.CategoryID
		feedContent := ratingCategoryFeedPostContent(cat.Name, cat.Description, cat.ID)
		if err := common.CreateHomeFeedPost(tx, userToModel(user), feedTitle, feedContent); err != nil {
			return domain.ErrBadRequest
		}
		return nil
	})
}

func ratingCategoryFeedPostContent(name, desc string, categoryID int) string {
	link := fmt.Sprintf("%s/rating?category=%d", common.NewFrontendBaseURL, categoryID)
	var b strings.Builder
	b.WriteString("打分专区新增了分区「")
	b.WriteString(name)
	b.WriteString("」，欢迎大家来评分、留言。\n\n")
	b.WriteString("直达链接：\n")
	b.WriteString(link)
	b.WriteString("\n")
	if strings.TrimSpace(desc) != "" {
		b.WriteString("\n分区简介：")
		b.WriteString(strings.TrimSpace(desc))
		b.WriteString("\n")
	}
	return b.String()
}

func (r *ratingRepository) applyBrowseFilter(q *gorm.DB, filter domain.RatingBrowseFilter, catIDs []int) *gorm.DB {
	if filter.CategoryID > 0 {
		q = q.Where("category_id = ?", filter.CategoryID)
	}
	s := strings.TrimSpace(filter.SearchInfo)
	if s == "" {
		return q
	}
	like := "%" + s + "%"
	if filter.CategoryID > 0 {
		return q.Where("title LIKE ? OR content LIKE ?", like, like)
	}
	if len(catIDs) > 0 {
		return q.Where("(title LIKE ? OR content LIKE ?) OR category_id IN (?)", like, like, catIDs)
	}
	return q.Where("title LIKE ? OR content LIKE ?", like, like)
}

func (r *ratingRepository) ListPosts(ctx context.Context, filter domain.RatingBrowseFilter) ([]*domain.RatingPost, error) {
	catIDs, _ := r.categoryIDsForFilter(ctx, filter)
	q := r.applyBrowseFilter(r.session(ctx).Model(&model.RatingPost{}), filter, catIDs)
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	if offset < 0 {
		offset = 0
	}
	var list []model.RatingPost
	err := q.Order("rating_post_id DESC").Offset(offset).Limit(limit).Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.RatingPost, 0, len(list))
	for i := range list {
		out = append(out, ratingPostToDomain(&list[i]))
	}
	return out, nil
}

func (r *ratingRepository) CountPosts(ctx context.Context, filter domain.RatingBrowseFilter) (int64, error) {
	catIDs, _ := r.categoryIDsForFilter(ctx, filter)
	q := r.applyBrowseFilter(r.session(ctx).Model(&model.RatingPost{}), filter, catIDs)
	var n int64
	err := q.Count(&n).Error
	return n, err
}

func (r *ratingRepository) categoryIDsForFilter(ctx context.Context, filter domain.RatingBrowseFilter) ([]int, error) {
	s := strings.TrimSpace(filter.SearchInfo)
	if s == "" || filter.CategoryID > 0 {
		return nil, nil
	}
	return r.CategoryIDsByNameLike(ctx, "%"+s+"%")
}

func (r *ratingRepository) FindPostByID(ctx context.Context, id int) (*domain.RatingPost, error) {
	var m model.RatingPost
	err := r.session(ctx).Where("rating_post_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return ratingPostToDomain(&m), nil
}

func (r *ratingRepository) CreatePost(ctx context.Context, post *domain.RatingPost) error {
	m := model.RatingPost{
		CategoryID: post.CategoryID,
		UserID:     post.UserID,
		Title:      post.Title,
		Content:    post.Content,
		Photos:     post.Photos,
		CreatedAt:  post.CreatedAt,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	post.ID = m.RatingPostID
	return nil
}

func (r *ratingRepository) IncrCategoryPostCount(ctx context.Context, categoryID, delta int) error {
	expr := "post_count + ?"
	if delta < 0 {
		expr = "GREATEST(post_count - ?, 0)"
		delta = -delta
	}
	return r.session(ctx).Model(&model.RatingCategory{}).Where("category_id = ?", categoryID).
		Update("post_count", gorm.Expr(expr, delta)).Error
}

func (r *ratingRepository) IncrPostBrowse(ctx context.Context, postID int) error {
	return r.session(ctx).Model(&model.RatingPost{}).Where("rating_post_id = ?", postID).
		UpdateColumn("browse_num", gorm.Expr("browse_num + ?", 1)).Error
}

func (r *ratingRepository) DeletePostCascade(ctx context.Context, postID, categoryID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("rating_post_id = ?", postID).Delete(&model.RatingStar{})
		tx.Where("rating_post_id = ?", postID).Delete(&model.RatingComment{})
		tx.Where("rating_post_id = ?", postID).Delete(&model.RatingPostLike{})
		if err := tx.Where("rating_post_id = ?", postID).Delete(&model.RatingPost{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.RatingCategory{}).Where("category_id = ?", categoryID).
			Update("post_count", gorm.Expr("GREATEST(post_count - 1, 0)")).Error
	})
}

func (r *ratingRepository) FindPostLike(ctx context.Context, postID, userID int) (int, error) {
	var like model.RatingPostLike
	err := r.session(ctx).Where("rating_post_id = ? AND user_id = ?", postID, userID).First(&like).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return like.LikeID, nil
}

func (r *ratingRepository) TogglePostLike(ctx context.Context, postID, userID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		var like model.RatingPostLike
		err := tx.Where("rating_post_id = ? AND user_id = ?", postID, userID).First(&like).Error
		if err == nil && like.LikeID != 0 {
			if err := tx.Delete(&like).Error; err != nil {
				return err
			}
			return tx.Model(&model.RatingPost{}).Where("rating_post_id = ?", postID).
				Update("like_num", gorm.Expr("GREATEST(like_num - 1, 0)")).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&model.RatingPostLike{
			RatingPostID: postID,
			UserID:       userID,
			CreatedAt:    time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.RatingPost{}).Where("rating_post_id = ?", postID).
			Update("like_num", gorm.Expr("like_num + 1")).Error
	})
}

func (r *ratingRepository) FindUserStar(ctx context.Context, postID, userID int) (int, error) {
	var star model.RatingStar
	err := r.session(ctx).Where("rating_post_id = ? AND user_id = ?", postID, userID).First(&star).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return star.StarNum, nil
}

func (r *ratingRepository) UpsertStar(ctx context.Context, postID, userID, starNum int) error {
	var star model.RatingStar
	err := r.session(ctx).Where("rating_post_id = ? AND user_id = ?", postID, userID).First(&star).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.session(ctx).Create(&model.RatingStar{
				RatingPostID: postID,
				UserID:       userID,
				StarNum:      starNum,
			}).Error
		}
		return err
	}
	return r.session(ctx).Model(&star).Update("star_num", starNum).Error
}

func (r *ratingRepository) ListCommentsByPost(ctx context.Context, postID int) ([]*domain.RatingComment, error) {
	var list []model.RatingComment
	err := r.session(ctx).Where("rating_post_id = ?", postID).Order("comment_id DESC").Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.RatingComment, 0, len(list))
	for i := range list {
		out = append(out, ratingCommentToDomain(&list[i]))
	}
	return out, nil
}

func (r *ratingRepository) FindCommentByID(ctx context.Context, id int) (*domain.RatingComment, error) {
	var m model.RatingComment
	err := r.session(ctx).Where("comment_id = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return ratingCommentToDomain(&m), nil
}

func (r *ratingRepository) CreateComment(ctx context.Context, c *domain.RatingComment) error {
	m := model.RatingComment{
		RatingPostID: c.RatingPostID,
		UserID:       c.UserID,
		Content:      c.Content,
		StarNum:      c.StarNum,
		CreatedAt:    c.CreatedAt,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	c.ID = m.CommentID
	return nil
}

func (r *ratingRepository) IncrPostCommentNum(ctx context.Context, postID int) error {
	return r.session(ctx).Model(&model.RatingPost{}).Where("rating_post_id = ?", postID).
		Update("comment_num", gorm.Expr("comment_num + 1")).Error
}

func (r *ratingRepository) ToggleCommentLike(ctx context.Context, commentID, userID int) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		var like model.RatingCommentLike
		err := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&like).Error
		if err == nil && like.LikeID != 0 {
			if err := tx.Delete(&like).Error; err != nil {
				return err
			}
			return tx.Model(&model.RatingComment{}).Where("comment_id = ?", commentID).
				Update("like_num", gorm.Expr("GREATEST(like_num - 1, 0)")).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&model.RatingCommentLike{
			CommentID: commentID,
			UserID:    userID,
			CreatedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.RatingComment{}).Where("comment_id = ?", commentID).
			Update("like_num", gorm.Expr("like_num + 1")).Error
	})
}

func (r *ratingRepository) CategoriesByIDs(ctx context.Context, ids []int) (map[int]*domain.RatingCategory, error) {
	m := make(map[int]*domain.RatingCategory)
	if len(ids) == 0 {
		return m, nil
	}
	var list []model.RatingCategory
	if err := r.session(ctx).Where("category_id IN (?)", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	for i := range list {
		m[list[i].CategoryID] = ratingCategoryToDomain(&list[i])
	}
	return m, nil
}

func (r *ratingRepository) UsersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error) {
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

func (r *ratingRepository) PostLikedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error) {
	set := make(map[int]struct{})
	if userID == 0 || len(postIDs) == 0 {
		return set, nil
	}
	var likes []model.RatingPostLike
	if err := r.session(ctx).Where("user_id = ? AND rating_post_id IN (?)", userID, postIDs).Find(&likes).Error; err != nil {
		return nil, err
	}
	for _, l := range likes {
		set[l.RatingPostID] = struct{}{}
	}
	return set, nil
}

func (r *ratingRepository) StarsByPostIDs(ctx context.Context, postIDs []int) (map[int][]int, error) {
	m := make(map[int][]int)
	if len(postIDs) == 0 {
		return m, nil
	}
	var list []model.RatingStar
	if err := r.session(ctx).Where("rating_post_id IN (?)", postIDs).Find(&list).Error; err != nil {
		return nil, err
	}
	for _, s := range list {
		m[s.RatingPostID] = append(m[s.RatingPostID], s.StarNum)
	}
	return m, nil
}

func (r *ratingRepository) CommentLikedSet(ctx context.Context, userID int, commentIDs []int) (map[int]bool, error) {
	m := make(map[int]bool)
	if userID == 0 || len(commentIDs) == 0 {
		return m, nil
	}
	var likes []model.RatingCommentLike
	if err := r.session(ctx).Where("user_id = ? AND comment_id IN (?)", userID, commentIDs).Find(&likes).Error; err != nil {
		return nil, err
	}
	for _, l := range likes {
		m[l.CommentID] = true
	}
	return m, nil
}

func (r *ratingRepository) CategoryIDsByNameLike(ctx context.Context, pattern string) ([]int, error) {
	var ids []int
	err := r.session(ctx).Model(&model.RatingCategory{}).Where("name LIKE ?", pattern).Pluck("category_id", &ids).Error
	return ids, err
}

func (r *ratingRepository) CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error {
	n := model.Notice{
		Receiver: receiver,
		Sender:   sender,
		Type:     noticeType,
		Ntext:    text,
		Time:     time.Now(),
		Read:     false,
		Target:   target,
	}
	return r.session(ctx).Create(&n).Error
}

func (r *ratingRepository) FindUserByID(ctx context.Context, id int) (*domain.User, error) {
	var u model.User
	err := r.session(ctx).Where("userID = ?", id).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&u), nil
}
