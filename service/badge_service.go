package service

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/repository"
)

type badgeService struct {
	badges repository.BadgeRepository
}

func NewBadgeService(badges repository.BadgeRepository) BadgeService {
	return &badgeService{badges: badges}
}

// prefsOrDefault 读取展示偏好；没有行时返回全默认（不挂载、主页全部展示）。
func (s *badgeService) prefsOrDefault(ctx context.Context, userID int) (*domain.UserBadgePrefs, error) {
	prefs, err := s.badges.FindPrefs(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return &domain.UserBadgePrefs{UserID: userID}, nil
		}
		return nil, err
	}
	return prefs, nil
}

func ownedBadgeSet(badges []domain.AwardedBadge) map[int]struct{} {
	owned := make(map[int]struct{}, len(badges))
	for _, b := range badges {
		owned[b.ID] = struct{}{}
	}
	return owned
}

// sanitizeProfileSelection 只保留仍然存在且启用、并且确实拥有的勋章 ID，
// 保持用户原顺序并去重；nil 仍表示「全部展示」。
func sanitizeProfileSelection(selected []int, owned map[int]struct{}) []int {
	if selected == nil {
		return nil
	}
	seen := make(map[int]struct{}, len(selected))
	clean := make([]int, 0, len(selected))
	for _, id := range selected {
		if id <= 0 {
			continue
		}
		if _, ok := owned[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	return clean
}

func (s *badgeService) GetMyBadges(ctx context.Context, userID int) (*domain.UserBadgesView, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	badges, err := s.badges.ListUserBadges(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs, err := s.prefsOrDefault(ctx, userID)
	if err != nil {
		return nil, err
	}
	owned := ownedBadgeSet(badges)
	worn := prefs.WornBadgeID
	if _, ok := owned[worn]; !ok {
		worn = 0
	}
	return &domain.UserBadgesView{
		UserID:          userID,
		Badges:          badges,
		WornBadgeID:     worn,
		ProfileBadgeIDs: sanitizeProfileSelection(prefs.ProfileBadgeIDs, owned),
	}, nil
}

func (s *badgeService) GetPublicBadges(ctx context.Context, userID int) (*domain.PublicUserBadges, error) {
	if userID <= 0 {
		return nil, domain.ErrBadRequest
	}
	badges, err := s.badges.ListUserBadges(ctx, userID)
	if err != nil {
		return nil, err
	}
	prefs, err := s.prefsOrDefault(ctx, userID)
	if err != nil {
		return nil, err
	}
	owned := ownedBadgeSet(badges)
	worn := prefs.WornBadgeID
	if _, ok := owned[worn]; !ok {
		worn = 0
	}
	selection := sanitizeProfileSelection(prefs.ProfileBadgeIDs, owned)
	return &domain.PublicUserBadges{
		UserID:          userID,
		Badges:          domain.FilterBadgesByProfileSelection(badges, selection),
		WornBadgeID:     worn,
		ProfileBadgeIDs: selection,
	}, nil
}

func (s *badgeService) SavePrefs(ctx context.Context, userID int, in domain.BadgePrefsInput) (*domain.UserBadgesView, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	badges, err := s.badges.ListUserBadges(ctx, userID)
	if err != nil {
		return nil, err
	}
	owned := ownedBadgeSet(badges)

	worn := in.WornBadgeID
	if worn != 0 {
		if _, ok := owned[worn]; !ok {
			return nil, domain.ErrBadgeNotOwned
		}
	}

	profile := in.ProfileBadgeIDs
	if profile != nil {
		if len(profile) > domain.BadgeMaxProfileSelection {
			return nil, domain.ErrBadgePrefsTooBig
		}
		for _, id := range profile {
			if id <= 0 {
				continue
			}
			if _, ok := owned[id]; !ok {
				return nil, domain.ErrBadgeNotOwned
			}
		}
		profile = sanitizeProfileSelection(profile, owned)
	}

	if err := s.badges.SavePrefs(ctx, domain.UserBadgePrefs{
		UserID:          userID,
		WornBadgeID:     worn,
		ProfileBadgeIDs: profile,
	}); err != nil {
		return nil, err
	}
	return s.GetMyBadges(ctx, userID)
}

func (s *badgeService) WornBadges(ctx context.Context, userIDs []int) (map[int]domain.WornBadge, error) {
	clean := make([]int, 0, len(userIDs))
	seen := make(map[int]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return map[int]domain.WornBadge{}, nil
	}
	return s.badges.WornBadgesByUserIDs(ctx, clean)
}
