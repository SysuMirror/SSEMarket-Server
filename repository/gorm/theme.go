package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type themeRepository struct {
	db *gorm.DB
}

func NewThemeRepository(db *gorm.DB) repository.ThemeRepository {
	return &themeRepository{db: db}
}

func themeSettingsFromRow(row *model.UserThemeSetting) domain.ThemeSettings {
	return domain.ThemeSettings{
		UserID:              row.UserID,
		ThemeMode:           row.ThemeMode,
		SplashEnabled:       row.SplashEnabled,
		BackgroundEnabled:   row.BackgroundEnabled,
		BackgroundSource:    row.BackgroundSource,
		CustomBackgroundURL: row.CustomBackgroundURL,
		CustomSkinID:        row.CustomSkinID,
		PostCardOpacity:     row.PostCardOpacity,
		UpdatedAt:           row.UpdatedAt,
	}
}

func (r *themeRepository) Find(ctx context.Context, userID int) (*domain.ThemeSettings, error) {
	if userID <= 0 {
		return nil, domain.ErrNotFound
	}
	var row model.UserThemeSetting
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	settings := themeSettingsFromRow(&row)
	return &settings, nil
}

// themeEventFromRow 把表里的 DATETIME 墙钟解释成北京时间（+08:00），
// 与服务器本地时区无关；管理员按北京时间插入即可。
func themeEventFromRow(row *model.ThemeEvent) domain.ThemeEvent {
	return domain.ThemeEvent{
		ThemeID:  row.ThemeID,
		Name:     row.Name,
		StartsAt: domain.ThemeWallClock(row.StartsAt),
		EndsAt:   domain.ThemeWallClock(row.EndsAt),
	}
}

func (r *themeRepository) ListEvents(ctx context.Context) ([]domain.ThemeEvent, error) {
	var rows []model.ThemeEvent
	if err := r.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("starts_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	events := make([]domain.ThemeEvent, 0, len(rows))
	for i := range rows {
		events = append(events, themeEventFromRow(&rows[i]))
	}
	return events, nil
}

func (r *themeRepository) Save(ctx context.Context, settings domain.ThemeSettings) (*domain.ThemeSettings, error) {
	if settings.UserID <= 0 {
		return nil, domain.ErrBadRequest
	}
	now := time.Now()
	row := model.UserThemeSetting{
		UserID:              settings.UserID,
		ThemeMode:           settings.ThemeMode,
		SplashEnabled:       settings.SplashEnabled,
		BackgroundEnabled:   settings.BackgroundEnabled,
		BackgroundSource:    settings.BackgroundSource,
		CustomBackgroundURL: settings.CustomBackgroundURL,
		CustomSkinID:        settings.CustomSkinID,
		PostCardOpacity:     settings.PostCardOpacity,
		UpdatedAt:           now,
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"theme_mode",
			"splash_enabled",
			"background_enabled",
			"background_source",
			"custom_background_url",
			"custom_skin_id",
			"post_card_opacity",
			"updated_at",
		}),
	}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	settings.UpdatedAt = now
	return &settings, nil
}
