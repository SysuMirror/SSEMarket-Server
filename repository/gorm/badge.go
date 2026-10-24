package gormrepo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type badgeRepository struct {
	db *gorm.DB
}

func NewBadgeRepository(db *gorm.DB) repository.BadgeRepository {
	return &badgeRepository{db: db}
}

type awardedBadgeRow struct {
	BadgeID     int       `gorm:"column:badge_id"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	ImageURL    string    `gorm:"column:image_url"`
	Category    string    `gorm:"column:category"`
	SortOrder   int       `gorm:"column:sort_order"`
	Source      string    `gorm:"column:source"`
	AwardedAt   time.Time `gorm:"column:awarded_at"`
}

func (r *badgeRepository) ListUserBadges(ctx context.Context, userID int) ([]domain.AwardedBadge, error) {
	if userID <= 0 {
		return nil, nil
	}
	var rows []awardedBadgeRow
	if err := r.db.WithContext(ctx).
		Table("user_badges AS ub").
		Select("b.badge_id, b.code, b.name, b.description, b.image_url, b.category, b.sort_order, ub.source, ub.awarded_at").
		Joins("JOIN badges AS b ON b.badge_id = ub.badge_id").
		Where("ub.user_id = ? AND b.enabled = ?", userID, true).
		Order("b.sort_order DESC, b.badge_id DESC, ub.awarded_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	badges := make([]domain.AwardedBadge, 0, len(rows))
	for _, row := range rows {
		badges = append(badges, domain.AwardedBadge{
			Badge: domain.Badge{
				ID:          row.BadgeID,
				Code:        row.Code,
				Name:        row.Name,
				Description: row.Description,
				ImageURL:    row.ImageURL,
				Category:    row.Category,
				SortOrder:   row.SortOrder,
				AwardedAt:   row.AwardedAt,
			},
			Source: row.Source,
		})
	}
	return badges, nil
}

// decodeProfileBadgeIDs 解析 profile_badge_ids：
//   - 空串 => nil（默认全部展示）；
//   - "[]" => 空切片（明确不展示）；
//   - 非法 JSON => nil（按默认全部展示兜底，避免一条脏数据让主页空白）。
func decodeProfileBadgeIDs(raw string) []int {
	if raw == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	if ids == nil {
		return []int{}
	}
	return ids
}

func encodeProfileBadgeIDs(ids []int) string {
	if ids == nil {
		return ""
	}
	if len(ids) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (r *badgeRepository) FindPrefs(ctx context.Context, userID int) (*domain.UserBadgePrefs, error) {
	if userID <= 0 {
		return nil, domain.ErrNotFound
	}
	var row model.UserBadgePref
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.UserBadgePrefs{
		UserID:          row.UserID,
		WornBadgeID:     row.WornBadgeID,
		ProfileBadgeIDs: decodeProfileBadgeIDs(row.ProfileBadgeIDs),
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

func (r *badgeRepository) SavePrefs(ctx context.Context, prefs domain.UserBadgePrefs) error {
	if prefs.UserID <= 0 {
		return domain.ErrBadRequest
	}
	now := time.Now()
	row := model.UserBadgePref{
		UserID:          prefs.UserID,
		WornBadgeID:     prefs.WornBadgeID,
		ProfileBadgeIDs: encodeProfileBadgeIDs(prefs.ProfileBadgeIDs),
		UpdatedAt:       now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"worn_badge_id", "profile_badge_ids", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return err
	}
	prefs.UpdatedAt = now
	return nil
}

type wornBadgeRow struct {
	UserID   int    `gorm:"column:user_id"`
	BadgeID  int    `gorm:"column:badge_id"`
	Name     string `gorm:"column:name"`
	ImageURL string `gorm:"column:image_url"`
}

func (r *badgeRepository) WornBadgesByUserIDs(ctx context.Context, userIDs []int) (map[int]domain.WornBadge, error) {
	result := make(map[int]domain.WornBadge, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	var rows []wornBadgeRow
	// 必须 join user_badges：偏好里的 id 可能已被管理员停用/删除，
	// 用户也可能曾经手改库；只有「确实拥有且启用」的勋章才对外展示。
	err := r.db.WithContext(ctx).
		Table("user_badge_prefs AS p").
		Select("p.user_id, b.badge_id, b.name, b.image_url").
		Joins("JOIN badges AS b ON b.badge_id = p.worn_badge_id AND b.enabled = ?", true).
		Joins("JOIN user_badges AS ub ON ub.user_id = p.user_id AND ub.badge_id = p.worn_badge_id").
		Where("p.user_id IN ?", userIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.UserID] = domain.WornBadge{
			UserID:   row.UserID,
			BadgeID:  row.BadgeID,
			Name:     row.Name,
			ImageURL: row.ImageURL,
		}
	}
	return result, nil
}
