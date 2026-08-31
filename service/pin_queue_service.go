package service

import (
	"context"
	"time"

	"loginTest/domain"
	"loginTest/repository"
)

// PinQueueService Scheme B 置顶队列领域服务（草案）。
// 下一步接线时：PostService.PinPost/Browse/ClearExpiredPins 改为委托本服务；
// posts 表内联字段停止写入，仅作回滚窗口只读兼容。
type PinQueueService interface {
	ListHomePinnedPostIDs(ctx context.Context) ([]int, error)
	EnqueueUserPin(ctx context.Context, postID, userID int, ttl time.Duration) error
	EnqueueAdminPin(ctx context.Context, postID, adminUserID int, ttl time.Duration) error
	Dequeue(ctx context.Context, postID, actorUserID int) error
	ClearExpired(ctx context.Context) (int64, error)
	IsPostPinned(ctx context.Context, postID int) (bool, *domain.PinQueueEntry, error)
	CountActive(ctx context.Context) (int64, error)
}

type pinQueueService struct {
	pins  repository.PinQueueRepository
	posts repository.PostRepository
}

func NewPinQueueService(pins repository.PinQueueRepository, posts repository.PostRepository) PinQueueService {
	return &pinQueueService{pins: pins, posts: posts}
}

func (s *pinQueueService) ListHomePinnedPostIDs(ctx context.Context) ([]int, error) {
	entries, err := s.pins.ListActive(ctx, domain.PinScopeHome, domain.MaxPinnedQueueSize)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.PostID)
	}
	return ids, nil
}

func (s *pinQueueService) EnqueueUserPin(ctx context.Context, postID, userID int, ttl time.Duration) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	if post.Partition != "主页" {
		return domain.ErrPinnedWrongPartition
	}
	if _, err := s.pins.FindActiveByPostID(ctx, domain.PinScopeHome, postID); err == nil {
		return domain.ErrAlreadyPinned
	} else if err != domain.ErrNotFound {
		return err
	}
	n, err := s.pins.CountActiveByOwner(ctx, domain.PinScopeHome, userID)
	if err != nil {
		return err
	}
	if n >= 1 {
		return domain.ErrPinnedLimitReached
	}
	global, err := s.pins.CountActive(ctx, domain.PinScopeHome)
	if err != nil {
		return err
	}
	if global >= int64(domain.MaxPinnedQueueSize) {
		return domain.ErrGlobalPinnedLimitReached
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	_, err = s.pins.Enqueue(ctx, domain.PinEnqueueInput{
		Scope:       domain.PinScopeHome,
		PostID:      postID,
		OwnerUserID: userID,
		ActorUserID: userID,
		Source:      domain.PinSourceUser,
		ExpireAt:    time.Now().Add(ttl),
	})
	if err != nil {
		return err
	}
	InvalidateHomeBrowse()
	return nil
}

func (s *pinQueueService) EnqueueAdminPin(ctx context.Context, postID, adminUserID int, ttl time.Duration) error {
	if _, err := s.posts.FindByID(ctx, postID); err != nil {
		return err
	}
	global, err := s.pins.CountActive(ctx, domain.PinScopeHome)
	if err != nil {
		return err
	}
	if global >= int64(domain.MaxPinnedQueueSize) {
		return domain.ErrGlobalPinnedLimitReached
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	_, err = s.pins.Enqueue(ctx, domain.PinEnqueueInput{
		Scope:       domain.PinScopeHome,
		PostID:      postID,
		OwnerUserID: adminUserID,
		ActorUserID: adminUserID,
		Source:      domain.PinSourceAdmin,
		ExpireAt:    time.Now().Add(ttl),
	})
	if err != nil {
		return err
	}
	InvalidateHomeBrowse()
	return nil
}

func (s *pinQueueService) Dequeue(ctx context.Context, postID, actorUserID int) error {
	entry, err := s.pins.FindActiveByPostID(ctx, domain.PinScopeHome, postID)
	if err != nil {
		if err == domain.ErrNotFound {
			return domain.ErrNotPinned
		}
		return err
	}
	// 作者可取消；管理员取消走独立入口时可放宽（后续接线）
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != actorUserID && entry.OwnerUserID != actorUserID {
		return domain.ErrForbidden
	}
	if err := s.pins.DequeueByPostID(ctx, domain.PinScopeHome, postID); err != nil {
		return err
	}
	InvalidateHomeBrowse()
	return nil
}

func (s *pinQueueService) ClearExpired(ctx context.Context) (int64, error) {
	n, err := s.pins.ClearExpired(ctx, domain.PinScopeHome, time.Now())
	if err != nil {
		return 0, err
	}
	if n > 0 {
		InvalidateHomeBrowse()
	}
	return n, nil
}

func (s *pinQueueService) IsPostPinned(ctx context.Context, postID int) (bool, *domain.PinQueueEntry, error) {
	e, err := s.pins.FindActiveByPostID(ctx, domain.PinScopeHome, postID)
	if err == domain.ErrNotFound {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return true, e, nil
}

func (s *pinQueueService) CountActive(ctx context.Context) (int64, error) {
	return s.pins.CountActive(ctx, domain.PinScopeHome)
}
