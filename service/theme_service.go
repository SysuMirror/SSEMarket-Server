package service

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/repository"
)

type themeService struct {
	themes repository.ThemeRepository
}

func NewThemeService(themes repository.ThemeRepository) ThemeService {
	return &themeService{themes: themes}
}

// sanitizeStoredSettings 兜底清洗库里的历史值：空值补默认、非法值回退，
// 避免一条脏数据让前端背景/换肤整体失效。保留用户的开场/背景开关选择。
func sanitizeStoredSettings(s *domain.ThemeSettings) {
	def := domain.DefaultThemeSettings(s.UserID)
	if s.ThemeMode == "" || !domain.IsKnownThemeMode(s.ThemeMode) {
		s.ThemeMode = def.ThemeMode
	}
	if s.BackgroundSource == "" || !domain.IsKnownThemeBackgroundSource(s.BackgroundSource) {
		s.BackgroundSource = def.BackgroundSource
	}
	url, err := domain.NormalizeThemeBackgroundURL(s.CustomBackgroundURL)
	if err != nil {
		url = ""
	}
	s.CustomBackgroundURL = url
	// 自定义背景地址失效时回退到皮肤自带背景，避免「开着自定义但一片空白」。
	if s.BackgroundSource == domain.ThemeBackgroundSourceCustom && s.CustomBackgroundURL == "" {
		s.BackgroundSource = domain.ThemeBackgroundSourceDefault
	}
}

func (s *themeService) GetUserSettings(ctx context.Context, userID int) (*domain.ThemeSettings, error) {
	if userID <= 0 {
		def := domain.DefaultThemeSettings(userID)
		return &def, nil
	}
	stored, err := s.themes.Find(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			def := domain.DefaultThemeSettings(userID)
			return &def, nil
		}
		return nil, err
	}
	sanitizeStoredSettings(stored)
	return stored, nil
}

func (s *themeService) SaveUserSettings(ctx context.Context, userID int, in domain.ThemeSettings) (*domain.ThemeSettings, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	in.UserID = userID
	if err := domain.NormalizeThemeSettings(&in); err != nil {
		return nil, err
	}
	return s.themes.Save(ctx, in)
}
