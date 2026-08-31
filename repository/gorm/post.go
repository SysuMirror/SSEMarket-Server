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

// browseListSelect 列表只读摘要字段。
// ptext 不做 LEFT 截断，由 controller 层提取图片后统一截断，
// 避免 markdown 图片 URL 被切断导致前端无法渲染预览图。
func browseListSelect(q *gorm.DB) *gorm.DB {
	return q.Select(
		"postID, userID, `partition`, title, ptext, comment_num, like_num, vote_num, browse_num, post_time, heat, photos, tag, post_kind, quote_price, quest_status, is_private, is_anonymous, is_pinned, pinned_at, pinned_expires_at",
	)
}

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
	err := r.session(ctx).Where("postID = ?", postID).First(&dbPost).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	var cacheUser model.User
	if dbPost.UserID == 0 {
		cacheUser = model.User{Name: "管理员", Phone: "11111111111"}
	} else if err := r.session(ctx).Where("userID = ?", dbPost.UserID).First(&cacheUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cacheUser = model.User{UserID: dbPost.UserID}
		} else {
			return nil, err
		}
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
			Joins("INNER JOIN posts ON psaves.ptargetID = posts.postID").
			Where("psaves.userID = ? AND posts.postID != ? AND (posts.is_private = ? OR posts.userID = ?)",
				viewerID, 0, false, viewerID).
			Order("psaves.psaveID DESC").
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
		browseListSelect(r.session(ctx).Model(&model.Post{}).Where("postID IN (?)", postIDs)).Find(&posts)
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
		browseListSelect(base.Where("userID = ?", viewerID)).Offset(offset).Limit(limit).Find(&posts)
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
	q := browseListSelect(base)
	// 排除已置顶帖,避免主页置顶帖重复出现在分页列表中。
	if len(filter.ExcludePostIDs) > 0 {
		q = q.Where("postID NOT IN (?)", filter.ExcludePostIDs)
	}
	q.Offset(offset).Limit(limit).Find(&posts)
	return modelsToDomain(posts), nil
}

// ListPinned 查询主页置顶队列(最多 MaxPinnedQueueSize 条)。
//
// 队列规则:
//   - 仅 is_pinned=1 且 pinned_expires_at > NOW() 且 partition=主页 的帖入队
//   - 按 pinned_at ASC 先进先出:最早置顶的排在最前,与普通帖列表完全分离
//   - 过期/取消置顶由 ClearExpiredPins / UnpinPost 出队(清空 is_pinned 与 pinned_at)
func (r *postRepository) ListPinned(ctx context.Context, viewerUserID int) ([]*domain.Post, error) {
	q := r.session(ctx).Model(&model.Post{}).
		Where("is_pinned = ? AND pinned_expires_at > NOW() AND `partition` = ?", true, "主页").
		Where("is_private = ? OR userID = ?", false, viewerUserID)
	var posts []model.Post
	// 旧排序(后进先出): Order("pinned_at DESC")
	browseListSelect(q).Order("pinned_at ASC, postID ASC").Limit(domain.MaxPinnedQueueSize).Find(&posts)
	return modelsToDomain(posts), nil
}

// CountPinnedByUser 当前用户未过期的置顶帖数量(用于校验每人 1 个上限)。
func (r *postRepository) CountPinnedByUser(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Post{}).
		Where("userID = ? AND is_pinned = ? AND pinned_expires_at > NOW()", userID, true).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CountActivePinned 当前全站置顶队列长度(未过期)。
func (r *postRepository) CountActivePinned(ctx context.Context) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Post{}).
		Where("is_pinned = ? AND pinned_expires_at > NOW() AND `partition` = ?", true, "主页").
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// PinPost 将帖子入队置顶:写入 pinned_at(入队时刻)与过期时间。
// 调用方需保证权限/上限/分区等校验已完成。
func (r *postRepository) PinPost(ctx context.Context, postID, userID int, expiresAt time.Time) error {
	now := time.Now()
	result := r.session(ctx).Model(&model.Post{}).
		Where("postID = ? AND userID = ?", postID, userID).
		Updates(map[string]any{
			"is_pinned":         1,
			"pinned_at":         now, // 入队时刻,决定 FIFO 顺序
			"pinned_expires_at": expiresAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UnpinPost 手动出队:清空置顶标记与入队时间。
func (r *postRepository) UnpinPost(ctx context.Context, postID, userID int) error {
	result := r.session(ctx).Model(&model.Post{}).
		Where("postID = ? AND userID = ?", postID, userID).
		Updates(map[string]any{
			"is_pinned":          0,
			"pinned_at":          nil,
			"pinned_expires_at":  nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ClearExpiredPins 定时出队:清理过期或无效置顶标记。
func (r *postRepository) ClearExpiredPins(ctx context.Context) (int64, error) {
	result := r.session(ctx).Model(&model.Post{}).
		Where("is_pinned = ? AND (pinned_expires_at IS NULL OR pinned_expires_at <= NOW())", true).
		Updates(map[string]any{
			"is_pinned":         0,
			"pinned_at":         nil,
			"pinned_expires_at": nil,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// FindHotPcommentsByPostIDs 批量查询每个帖子的热门一级评论。
// 排序规则:like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC
// 然后按 ptargetID 分组取首条,得到每个帖子的 Top1。
func (r *postRepository) FindHotPcommentsByPostIDs(ctx context.Context, postIDs []int) (map[int]*domain.Pcomment, error) {
	out := make(map[int]*domain.Pcomment, len(postIDs))
	if len(postIDs) == 0 {
		return out, nil
	}
	var rows []model.Pcomment
	err := r.session(ctx).
		Where("ptargetID IN ?", postIDs).
		Order("ptargetID, like_num DESC, CHAR_LENGTH(pctext) DESC, pcommentID ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		pid := rows[i].PtargetID
		if _, exists := out[pid]; exists {
			continue
		}
		out[pid] = pcommentToDomain(&rows[i])
	}
	return out, nil
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

func (r *postRepository) IsWatched(ctx context.Context, userID, postID int) (bool, error) {
	var watch model.PostWatch
	err := r.session(ctx).Where("userID = ? AND postID = ?", userID, postID).First(&watch).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return watch.PostWatchID != 0, err
}

func (r *postRepository) CreateWatch(ctx context.Context, userID, postID int) error {
	w := model.PostWatch{
		UserID:    userID,
		PostID:    postID,
		CreatedAt: time.Now(),
	}
	return r.session(ctx).Create(&w).Error
}

func (r *postRepository) DeleteWatch(ctx context.Context, userID, postID int) error {
	return r.session(ctx).Where("userID = ? AND postID = ?", userID, postID).Delete(&model.PostWatch{}).Error
}

func (r *postRepository) ListWatcherUserIDsByPostID(ctx context.Context, postID int) ([]int, error) {
	var rows []model.PostWatch
	if err := r.session(ctx).Where("postID = ?", postID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rows))
	for _, row := range rows {
		if row.UserID != 0 {
			out = append(out, row.UserID)
		}
	}
	return out, nil
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

func (r *postRepository) Create(ctx context.Context, post *domain.Post) error {
	m := postToModel(post)
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	post.ID = m.PostID
	return nil
}

func (r *postRepository) Delete(ctx context.Context, postID int) error {
	return r.session(ctx).Where("postID = ?", postID).Delete(&model.Post{}).Error
}

func (r *postRepository) ListViewerPrivatePostsRecent(ctx context.Context, userID, limit int) ([]*domain.Post, error) {
	if userID <= 0 || limit <= 0 {
		return nil, nil
	}
	var posts []model.Post
	err := browseListSelect(r.session(ctx).Model(&model.Post{}).
		Where("userID = ? AND is_private = ?", userID, true).
		Order("post_time DESC").
		Limit(limit)).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return modelsToDomain(posts), nil
}

func (r *postRepository) UpdatePrivacy(ctx context.Context, postID int, isPrivate bool) error {
	return r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).
		Update("is_private", isPrivate).Error
}

func (r *postRepository) UpdateLikeNum(ctx context.Context, postID int, delta int) error {
	return r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("like_num", gorm.Expr("like_num + ?", delta)).Error
}

func (r *postRepository) UpdateHeat(ctx context.Context, postID int, delta float64) error {
	return r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("heat", gorm.Expr("heat + ?", delta)).Error
}

func (r *postRepository) HasBrowsed(ctx context.Context, userID, postID int) (bool, error) {
	var browsed model.Pbrowse
	err := r.session(ctx).Where("userID = ? AND ptargetID = ?", userID, postID).First(&browsed).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return browsed.PbrowseID != 0, err
}

func (r *postRepository) CreateBrowse(ctx context.Context, userID, postID int) error {
	b := model.Pbrowse{
		UserID:    userID,
		PtargetID: postID,
		Time:      time.Now(),
	}
	return r.session(ctx).Create(&b).Error
}

func (r *postRepository) DeleteSave(ctx context.Context, userID, postID int) error {
	return r.session(ctx).Where("userID = ? AND ptargetID = ?", userID, postID).Delete(&model.Psave{}).Error
}

func (r *postRepository) CreateSave(ctx context.Context, userID, postID int) error {
	exists, err := r.IsSaved(ctx, userID, postID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	s := model.Psave{
		UserID:    userID,
		PtargetID: postID,
	}
	return r.session(ctx).Create(&s).Error
}

func (r *postRepository) DeleteLike(ctx context.Context, userID, postID int) error {
	return r.session(ctx).Where("userID = ? AND ptargetID = ?", userID, postID).Delete(&model.Plike{}).Error
}

func (r *postRepository) CreateLike(ctx context.Context, userID, postID int) error {
	like := model.Plike{
		UserID:    userID,
		PtargetID: postID,
		Time:      time.Now(),
	}
	return r.session(ctx).Create(&like).Error
}

func (r *postRepository) ListUserProfile(ctx context.Context, targetUserID, viewerUserID, limit, offset int) ([]*domain.Post, error) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	if offset < 0 {
		offset = 0
	}
	query := r.session(ctx).Where("userID = ?", targetUserID)
	if viewerUserID != targetUserID {
		// 他人视角：排除私密帖与匿名帖，避免通过个人主页关联出匿名帖作者
		query = query.Where("is_private = ? AND is_anonymous = ?", false, false)
	}
	var posts []model.Post
	err := query.Order("postID DESC").Offset(offset).Limit(limit).Find(&posts).Error
	if err != nil {
		return nil, err
	}
	return modelsToDomain(posts), nil
}

func (r *postRepository) CountUserProfile(ctx context.Context, targetUserID, viewerUserID int) (int64, error) {
	query := r.session(ctx).Model(&model.Post{}).Where("userID = ?", targetUserID)
	if viewerUserID != targetUserID {
		// 他人视角：计数也排除私密帖与匿名帖
		query = query.Where("is_private = ? AND is_anonymous = ?", false, false)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *postRepository) FindPublicByID(ctx context.Context, postID int) (*domain.Post, error) {
	var post model.Post
	err := r.session(ctx).Where("postID = ? AND is_private = ?", postID, false).First(&post).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return postToDomain(&post), nil
}

func (r *postRepository) ListAllTags(ctx context.Context) ([]*domain.Tag, error) {
	var tags []model.Tag
	if err := r.session(ctx).Find(&tags).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Tag, 0, len(tags))
	for i := range tags {
		out = append(out, tagToDomain(&tags[i]))
	}
	return out, nil
}

func (r *postRepository) FindTagByName(ctx context.Context, name string) (*domain.Tag, error) {
	var tag model.Tag
	err := r.session(ctx).Where("name = ?", name).First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return tagToDomain(&tag), nil
}

func (r *postRepository) CreatePostNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error {
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

func (r *postRepository) CreateFeedback(ctx context.Context, text, attachment string) (*domain.Feedback, error) {
	f := model.Feedback{
		Ftext:      text,
		Attachment: attachment,
		Time:       time.Now(),
		Status:     "wait",
	}
	if err := r.session(ctx).Create(&f).Error; err != nil {
		return nil, err
	}
	if f.FeedbackID == 0 {
		return nil, domain.ErrInternal
	}
	return feedbackToDomain(&f), nil
}

func (r *postRepository) ListAllFeedback(ctx context.Context) ([]*domain.Feedback, error) {
	var list []model.Feedback
	if err := r.session(ctx).Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Feedback, 0, len(list))
	for i := range list {
		out = append(out, feedbackToDomain(&list[i]))
	}
	return out, nil
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
