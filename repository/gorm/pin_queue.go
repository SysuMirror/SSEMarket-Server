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

type pinQueueRepository struct {
	db *gorm.DB
}

func NewPinQueueRepository(db *gorm.DB) repository.PinQueueRepository {
	return &pinQueueRepository{db: db}
}

func (r *pinQueueRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *pinQueueRepository) ListActive(ctx context.Context, scope string, limit int) ([]*domain.PinQueueEntry, error) {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	if limit <= 0 {
		limit = domain.MaxPinnedQueueSize
	}
	var rows []model.PinQueue
	err := r.session(ctx).
		Where("scope = ? AND expire_at > NOW()", scope).
		Order("enqueue_at ASC, post_id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.PinQueueEntry, 0, len(rows))
	for i := range rows {
		out = append(out, pinQueueToDomain(&rows[i]))
	}
	return out, nil
}

func (r *pinQueueRepository) FindActiveByPostID(ctx context.Context, scope string, postID int) (*domain.PinQueueEntry, error) {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	var row model.PinQueue
	err := r.session(ctx).
		Where("scope = ? AND post_id = ? AND expire_at > NOW()", scope, postID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return pinQueueToDomain(&row), nil
}

func (r *pinQueueRepository) CountActive(ctx context.Context, scope string) (int64, error) {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	var n int64
	err := r.session(ctx).Model(&model.PinQueue{}).
		Where("scope = ? AND expire_at > NOW()", scope).
		Count(&n).Error
	return n, err
}

func (r *pinQueueRepository) CountActiveByOwner(ctx context.Context, scope string, ownerUserID int) (int64, error) {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	var n int64
	err := r.session(ctx).Model(&model.PinQueue{}).
		Where("scope = ? AND owner_user_id = ? AND expire_at > NOW()", scope, ownerUserID).
		Count(&n).Error
	return n, err
}

func (r *pinQueueRepository) Enqueue(ctx context.Context, in domain.PinEnqueueInput) (*domain.PinQueueEntry, error) {
	if in.Scope == "" {
		in.Scope = domain.PinScopeHome
	}
	if in.Source == "" {
		in.Source = domain.PinSourceUser
	}
	now := time.Now()
	row := model.PinQueue{
		Scope:       in.Scope,
		PostID:      in.PostID,
		OwnerUserID: in.OwnerUserID,
		Source:      in.Source,
		EnqueueAt:   now,
		ExpireAt:    in.ExpireAt,
	}
	// 同帖重复入队：更新入队时间（排到队尾）并续期；业务层通常会先 Find 再拒绝。
	err := r.session(ctx).
		Where("scope = ? AND post_id = ?", in.Scope, in.PostID).
		Assign(map[string]any{
			"owner_user_id": in.OwnerUserID,
			"source":        in.Source,
			"enqueue_at":    now,
			"expire_at":     in.ExpireAt,
		}).
		FirstOrCreate(&row).Error
	if err != nil {
		return nil, err
	}
	_ = r.AppendEvent(ctx, domain.PinEvent{
		Scope:       in.Scope,
		PostID:      in.PostID,
		ActorUserID: in.ActorUserID,
		Action:      domain.PinActionEnqueue,
		Source:      in.Source,
		EnqueueAt:   &row.EnqueueAt,
		ExpireAt:    &row.ExpireAt,
	})
	return pinQueueToDomain(&row), nil
}

func (r *pinQueueRepository) DequeueByPostID(ctx context.Context, scope string, postID int) error {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	res := r.session(ctx).Where("scope = ? AND post_id = ?", scope, postID).Delete(&model.PinQueue{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	_ = r.AppendEvent(ctx, domain.PinEvent{
		Scope:  scope,
		PostID: postID,
		Action: domain.PinActionDequeue,
		Source: domain.PinSourceUser,
	})
	return nil
}

func (r *pinQueueRepository) ClearExpired(ctx context.Context, scope string, now time.Time) (int64, error) {
	if scope == "" {
		scope = domain.PinScopeHome
	}
	if now.IsZero() {
		now = time.Now()
	}
	var expired []model.PinQueue
	if err := r.session(ctx).Where("scope = ? AND expire_at <= ?", scope, now).Find(&expired).Error; err != nil {
		return 0, err
	}
	if len(expired) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(expired))
	for _, e := range expired {
		ids = append(ids, e.PinID)
		enq, exp := e.EnqueueAt, e.ExpireAt
		_ = r.AppendEvent(ctx, domain.PinEvent{
			Scope:       e.Scope,
			PostID:      e.PostID,
			ActorUserID: 0,
			Action:      domain.PinActionExpire,
			Source:      domain.PinSourceSystem,
			Detail:      "ttl_expired",
			EnqueueAt:   &enq,
			ExpireAt:    &exp,
		})
	}
	res := r.session(ctx).Where("pin_id IN ?", ids).Delete(&model.PinQueue{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (r *pinQueueRepository) AppendEvent(ctx context.Context, ev domain.PinEvent) error {
	if ev.Scope == "" {
		ev.Scope = domain.PinScopeHome
	}
	row := model.PinEvent{
		Scope:       ev.Scope,
		PostID:      ev.PostID,
		ActorUserID: ev.ActorUserID,
		Action:      ev.Action,
		Source:      ev.Source,
		Detail:      ev.Detail,
		EnqueueAt:   ev.EnqueueAt,
		ExpireAt:    ev.ExpireAt,
	}
	return r.session(ctx).Create(&row).Error
}

func pinQueueToDomain(m *model.PinQueue) *domain.PinQueueEntry {
	if m == nil {
		return nil
	}
	return &domain.PinQueueEntry{
		PinID:       m.PinID,
		Scope:       m.Scope,
		PostID:      m.PostID,
		OwnerUserID: m.OwnerUserID,
		Source:      m.Source,
		EnqueueAt:   m.EnqueueAt,
		ExpireAt:    m.ExpireAt,
	}
}
