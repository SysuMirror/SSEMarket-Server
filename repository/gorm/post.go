package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/dto"
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

func (r *postRepository) LoadDetailDTO(ctx context.Context, postID int) (*dto.PostDetailCacheDTO, error) {
	var dbPost model.Post
	err := r.session(ctx).Preload("User").Where("postID = ?", postID).First(&dbPost).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	var cacheUser model.User
	if dbPost.UserID == 0 {
		cacheUser = model.User{Name: "管理员", Phone: "11111111111"}
	} else {
		cacheUser = dbPost.User
	}
	var out dto.PostDetailCacheDTO
	out.ConvertFromModel(dbPost, cacheUser)
	return &out, nil
}

func (r *postRepository) ListBrowse(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, error) {
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 {
		limit = 15
	}
	if offset < 0 {
		offset = 0
	}
	searchsort := filter.Searchsort
	if searchsort == "" {
		searchsort = "home"
	}
	viewerID := filter.ViewerUserID

	if searchsort == "save" {
		var saves []model.Psave
		r.session(ctx).Model(&model.Psave{}).
			Where("userID = ?", viewerID).
			Order("psaveID DESC").
			Offset(offset).Limit(limit).
			Find(&saves)
		if len(saves) == 0 {
			return nil, nil
		}
		postIDs := make([]int, 0, len(saves))
		for _, s := range saves {
			postIDs = append(postIDs, s.PtargetID)
		}
		var posts []model.Post
		applyVisibility(r.session(ctx).Model(&model.Post{}).Where("postID IN (?)", postIDs), viewerID).Find(&posts)
		postMap := make(map[int]model.Post, len(posts))
		for _, p := range posts {
			postMap[p.PostID] = p
		}
		out := make([]*domain.Post, 0, len(saves))
		for _, s := range saves {
			if p, ok := postMap[s.PtargetID]; ok && p.PostID != 0 {
				cp := p
				out = append(out, postToDomain(&cp))
			}
		}
		return out, nil
	}

	base := r.session(ctx).Model(&model.Post{})
	base = applyTimeRange(base, filter.TimeFrom, filter.TimeTo)
	base = applyVisibility(base, viewerID)
	if searchsort == "home" {
		base = applyBrowseOrder(base, filter.OrderBy)
	} else {
		base = base.Order("postID DESC")
	}
	if searchsort == "history" {
		var posts []model.Post
		base.Where("userID = ?", viewerID).Offset(offset).Limit(limit).Find(&posts)
		return modelsToDomain(posts), nil
	}

	partition := filter.Partition
	searchinfo := filter.Searchinfo
	tag := filter.Tag
	if partition == "主页" || partition == "" {
		if searchinfo != "" {
			if tag == "" {
				base = base.Where("(title LIKE ? OR ptext LIKE ?)", "%"+searchinfo+"%", "%"+searchinfo+"%")
			} else {
				base = base.Where("(title LIKE ? OR ptext LIKE ?) AND tag = ?", "%"+searchinfo+"%", "%"+searchinfo+"%", tag)
			}
		}
	} else if partition == "优质贴" {
		base = base.Where("is_high_quality = ?", 1)
	} else {
		base = base.Where("`partition` = ?", partition)
		if searchinfo != "" {
			if tag == "" {
				base = base.Where("(title LIKE ? OR ptext LIKE ?)", "%"+searchinfo+"%", "%"+searchinfo+"%")
			} else {
				base = base.Where("(title LIKE ? OR ptext LIKE ?) AND tag = ?", "%"+searchinfo+"%", "%"+searchinfo+"%", tag)
			}
		}
	}
	var posts []model.Post
	base.Offset(offset).Limit(limit).Find(&posts)
	return modelsToDomain(posts), nil
}

func modelsToDomain(posts []model.Post) []*domain.Post {
	out := make([]*domain.Post, 0, len(posts))
	for i := range posts {
		out = append(out, postToDomain(&posts[i]))
	}
	return out
}

func applyBrowseOrder(q *gorm.DB, orderBy string) *gorm.DB {
	switch orderBy {
	case "time_asc":
		return q.Order("post_time ASC")
	case "like_desc":
		return q.Order("like_num DESC, postID DESC")
	case "like_asc":
		return q.Order("like_num ASC, postID DESC")
	case "comment_desc":
		return q.Order("comment_num DESC, postID DESC")
	case "comment_asc":
		return q.Order("comment_num ASC, postID DESC")
	default:
		return q.Order("post_time DESC")
	}
}

func (r *postRepository) LikedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}
	var likes []model.Plike
	r.session(ctx).Where("userID = ? AND ptargetID IN (?)", userID, postIDs).Find(&likes)
	set := make(map[int]struct{}, len(likes))
	for _, l := range likes {
		set[l.PtargetID] = struct{}{}
	}
	return set, nil
}

func (r *postRepository) SavedSet(ctx context.Context, userID int, postIDs []int) (map[int]struct{}, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}
	var saves []model.Psave
	r.session(ctx).Where("userID = ? AND ptargetID IN (?)", userID, postIDs).Find(&saves)
	set := make(map[int]struct{}, len(saves))
	for _, s := range saves {
		set[s.PtargetID] = struct{}{}
	}
	return set, nil
}

func (r *postRepository) AuthorsByIDs(ctx context.Context, userIDs []int) (map[int]*domain.User, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var users []model.User
	r.session(ctx).Where("userID IN (?)", userIDs).Find(&users)
	m := make(map[int]*domain.User, len(users))
	for i := range users {
		m[users[i].UserID] = userToDomain(&users[i])
	}
	return m, nil
}

func (r *postRepository) IsLiked(ctx context.Context, userID, postID int) (bool, error) {
	var like model.Plike
	err := r.session(ctx).Where("userID = ? AND ptargetID = ?", userID, postID).First(&like).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return like.PlikeID != 0, err
}

func (r *postRepository) IsSaved(ctx context.Context, userID, postID int) (bool, error) {
	var save model.Psave
	err := r.session(ctx).Where("userID = ? AND ptargetID = ?", userID, postID).First(&save).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return save.PsaveID != 0, err
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
