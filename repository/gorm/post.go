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

type postRepository struct {
	db *gorm.DB
}

func NewPostRepository(db *gorm.DB) repository.PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *postRepository) FindByID(ctx context.Context, id int) (*domain.Post, error) {
	var post model.Post
	err := r.session(ctx).Where("postID = ?", id).First(&post).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return postToDomain(&post), nil
}

func (r *postRepository) List(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, int64, error) {
	q := r.buildBrowseQuery(ctx, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 15
	}
	var rows []model.Post
	err := q.Order("post_time DESC").Offset(filter.Offset).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Post, 0, len(rows))
	for i := range rows {
		out = append(out, postToDomain(&rows[i]))
	}
	return out, total, nil
}

func (r *postRepository) Count(ctx context.Context, filter domain.BrowseFilter) (int64, error) {
	q := r.buildBrowseQuery(ctx, filter)
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *postRepository) UpdateBrowseNum(ctx context.Context, id int, browseNum int64) error {
	return r.session(ctx).Model(&model.Post{}).
		Where("postID = ?", id).
		Update("browse_num", browseNum).Error
}

func (r *postRepository) buildBrowseQuery(ctx context.Context, filter domain.BrowseFilter) *gorm.DB {
	searchsort := filter.Searchsort
	if searchsort == "" {
		searchsort = "home"
	}
	viewerUserID := filter.ViewerUserID

	if searchsort == "save" {
		return r.session(ctx).Model(&model.Psave{}).
			Joins("INNER JOIN posts ON psaves.ptargetID = posts.postID").
			Where("psaves.userID = ? AND posts.postID != ? AND (posts.is_private = ? OR posts.userID = ?)",
				viewerUserID, 0, false, viewerUserID)
	}
	if searchsort == "history" {
		return r.session(ctx).Model(&model.Post{}).
			Where("userID = ?", viewerUserID).
			Where("postId != ?", 0)
	}

	partition := filter.Partition
	searchinfo := filter.Searchinfo
	if searchsort == "home" {
		if partition == "主页" || partition == "" {
			if searchinfo == "" && filter.TimeFrom == "" && filter.TimeTo == "" {
				return applyVisibility(r.session(ctx).Model(&model.Post{}), viewerUserID)
			}
			q := r.session(ctx).Model(&model.Post{}).Where("postId != ?", 0)
			q = applyVisibility(q, viewerUserID)
			q = applyTimeRange(q, filter.TimeFrom, filter.TimeTo)
			if searchinfo != "" {
				q = q.Where("title LIKE ? OR ptext LIKE ? OR tag LIKE ?",
					"%"+searchinfo+"%", "%"+searchinfo+"%", "%"+searchinfo+"%")
			}
			return q
		}
		if partition == "优质贴" {
			q := applyVisibility(r.session(ctx).Model(&model.Post{}).Where("is_high_quality = ?", 1), viewerUserID)
			return applyTimeRange(q, filter.TimeFrom, filter.TimeTo)
		}
		q := applyVisibility(r.session(ctx).Model(&model.Post{}).Where("`partition` = ?", partition), viewerUserID)
		q = applyTimeRange(q, filter.TimeFrom, filter.TimeTo)
		if searchinfo != "" {
			q = q.Where("title LIKE ? OR ptext LIKE ? OR tag LIKE ?",
				"%"+searchinfo+"%", "%"+searchinfo+"%", "%"+searchinfo+"%")
		}
		return q
	}
	return applyVisibility(r.session(ctx).Model(&model.Post{}), viewerUserID)
}

func applyVisibility(q *gorm.DB, viewerUserID int) *gorm.DB {
	return q.Where("(is_private = ? OR userID = ?)", false, viewerUserID)
}

func applyTimeRange(q *gorm.DB, timeFrom, timeTo string) *gorm.DB {
	if t, ok := parseBrowseTime(timeFrom, false); ok {
		q = q.Where("post_time >= ?", t)
	}
	if t, ok := parseBrowseTime(timeTo, true); ok {
		q = q.Where("post_time <= ?", t)
	}
	return q
}

func parseBrowseTime(s string, endOfDay bool) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, s, time.Local)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && endOfDay {
			t = t.Add(24*time.Hour - time.Nanosecond)
		}
		return t, true
	}
	return time.Time{}, false
}
