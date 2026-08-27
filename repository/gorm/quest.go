package gormrepo

import (
	"context"
	"encoding/json"
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
	return `CASE WHEN quest_status IS NOT NULL AND quest_status != '' THEN quest_status WHEN post_kind = 'bounty' THEN 'open' ELSE 'open' END`
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
	priceType := in.PriceType
	if priceType == "" {
		priceType = domain.QuestPriceNegotiable
	}
	attJSON := ""
	if len(in.Attachments) > 0 {
		b, err := json.Marshal(in.Attachments)
		if err != nil {
			return nil, err
		}
		attJSON = string(b)
	}
	var attPtr *string
	if attJSON != "" {
		attPtr = &attJSON
	}
	row := model.Post{
		UserID:           in.UserID,
		Partition:        domain.QuestPartitionHome,
		Title:            in.Title,
		Ptext:            in.Content,
		PostKind:         in.PostKind,
		QuotePrice:       in.QuotePrice,
		PriceType:        priceType,
		QuotePriceMin:    in.QuotePriceMin,
		QuotePriceMax:    in.QuotePriceMax,
		QuestStatus:      domain.QuestStatusOpen,
		QuestDeadline:    in.Deadline,
		QuestAttachments: attPtr,
		PostTime:         now,
		Tag:              "",
		IsPrivate:        false,
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return postToDomain(&row), nil
}

func (r *questRepository) UpdateQuestStatus(ctx context.Context, postID int, status string) error {
	if !domain.IsValidQuestStatus(status) {
		return domain.ErrBadRequest
	}
	res := r.session(ctx).Model(&model.Post{}).
		Where("postID = ? AND post_kind IN ?", postID,
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

func (r *questRepository) UpdateQuestStatusByAuthor(ctx context.Context, postID, userID int, status string) error {
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

func questOrderToDomain(m *model.QuestOrder) *domain.QuestOrder {
	if m == nil {
		return nil
	}
	return &domain.QuestOrder{
		ID:          m.OrderID,
		PostID:      m.PostID,
		PublisherID: m.PublisherID,
		WorkerID:    m.WorkerID,
		Status:      m.Status,
		AcceptedAt:  m.AcceptedAt,
		SubmittedAt: m.SubmittedAt,
		CompletedAt: m.CompletedAt,
		CancelledAt: m.CancelledAt,
		SubmitNote:  m.SubmitNote,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func questOrderToModel(o *domain.QuestOrder) model.QuestOrder {
	if o == nil {
		return model.QuestOrder{}
	}
	return model.QuestOrder{
		OrderID:     o.ID,
		PostID:      o.PostID,
		PublisherID: o.PublisherID,
		WorkerID:    o.WorkerID,
		Status:      o.Status,
		AcceptedAt:  o.AcceptedAt,
		SubmittedAt: o.SubmittedAt,
		CompletedAt: o.CompletedAt,
		CancelledAt: o.CancelledAt,
		SubmitNote:  o.SubmitNote,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   o.UpdatedAt,
	}
}

func (r *questRepository) GetOrderByPostID(ctx context.Context, postID int) (*domain.QuestOrder, error) {
	var row model.QuestOrder
	err := r.session(ctx).Where("post_id = ?", postID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return questOrderToDomain(&row), nil
}

func (r *questRepository) CreateOrder(ctx context.Context, order *domain.QuestOrder) error {
	m := questOrderToModel(order)
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	order.ID = m.OrderID
	return nil
}

func (r *questRepository) UpdateOrder(ctx context.Context, order *domain.QuestOrder) error {
	m := questOrderToModel(order)
	return r.session(ctx).Save(&m).Error
}
