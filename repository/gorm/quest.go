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

type questRepository struct {
	db *gorm.DB
}

func NewQuestRepository(db *gorm.DB) repository.QuestRepository {
	return &questRepository{db: db}
}

func (r *questRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func questEffectiveStatusSQL() string {
	return `CASE WHEN quest_status IS NOT NULL AND quest_status != '' THEN quest_status WHEN post_kind = 'bounty' THEN 'in_progress' ELSE 'open' END`
}

func (r *questRepository) baseQuestQuery(ctx context.Context, filter domain.QuestListFilter) *gorm.DB {
	q := r.session(ctx).Model(&model.Post{}).
		Where("post_kind IN ?", []string{domain.QuestPostKindBounty, domain.QuestPostKindInquiry}).
		Where("is_private = ? OR userID = ?", false, filter.ViewerUserID)

	kind := strings.TrimSpace(filter.Kind)
	if kind == domain.QuestPostKindBounty || kind == domain.QuestPostKindInquiry {
		q = q.Where("post_kind = ?", kind)
	}
	status := strings.TrimSpace(filter.Status)
	if status != "" && status != "all" && domain.IsValidQuestStatus(status) {
		q = q.Where(questEffectiveStatusSQL()+" = ?", status)
	} else {
		// 大厅默认不展示已取消；删除帖走 DeletePost，不再用 cancelled 占位
		q = q.Where(questEffectiveStatusSQL()+" <> ?", domain.QuestStatusCancelled)
	}
	if kw := strings.TrimSpace(filter.Keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR ptext LIKE ?", like, like)
	}
	if filter.AuthorUserID > 0 {
		q = q.Where("userID = ?", filter.AuthorUserID)
	}
	if filter.SavedOnly && filter.ViewerUserID > 0 {
		q = q.Where("postID IN (SELECT ptargetID FROM psaves WHERE userID = ?)", filter.ViewerUserID)
	}
	return q
}

func questOrderClause(sort string) string {
	switch strings.TrimSpace(sort) {
	case "price_asc":
		return "quote_price IS NULL, quote_price ASC, post_time DESC"
	case "price_desc":
		return "quote_price DESC, post_time DESC"
	default:
		return "post_time DESC"
	}
}

func (r *questRepository) List(ctx context.Context, filter domain.QuestListFilter) ([]*domain.Post, int64, error) {
	q := r.baseQuestQuery(ctx, filter)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 15
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var rows []model.Post
	err := q.Order(questOrderClause(filter.Sort)).Offset(offset).Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Post, 0, len(rows))
	for i := range rows {
		out = append(out, postToDomain(&rows[i]))
	}
	return out, total, nil
}

func (r *questRepository) GetQuestPost(ctx context.Context, postID int) (*domain.Post, error) {
	var row model.Post
	err := r.session(ctx).Where("postID = ? AND post_kind IN ?", postID,
		[]string{domain.QuestPostKindBounty, domain.QuestPostKindInquiry}).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return postToDomain(&row), nil
}

func (r *questRepository) CreateQuestPost(ctx context.Context, in domain.QuestPostCreateInput) (*domain.Post, error) {
	now := time.Now()
	status := domain.QuestStatusOpen
	if in.PostKind == domain.QuestPostKindBounty {
		status = domain.QuestStatusInProgress
	}
	row := model.Post{
		UserID:      in.UserID,
		Partition:   domain.QuestPartitionHome,
		Title:       in.Title,
		Ptext:       in.Content,
		PostKind:    in.PostKind,
		QuotePrice:  in.QuotePrice,
		QuestStatus: status,
		PostTime:    now,
		Tag:         "",
		IsPrivate:   false,
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return postToDomain(&row), nil
}

func (r *questRepository) UpdateQuestStatus(ctx context.Context, postID, userID int, status string) error {
	if !domain.IsValidQuestStatus(status) {
		return domain.ErrBadRequest
	}
	res := r.session(ctx).Model(&model.Post{}).
		Where("postID = ? AND userID = ? AND post_kind IN ?", postID, userID,
			[]string{domain.QuestPostKindBounty, domain.QuestPostKindInquiry}).
		Update("quest_status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
