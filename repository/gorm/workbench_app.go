package gormrepo

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type workbenchAppRepository struct {
	db *gorm.DB
}

func NewWorkbenchAppRepository(db *gorm.DB) repository.WorkbenchAppRepository {
	return &workbenchAppRepository{db: db}
}

func (r *workbenchAppRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func workbenchAppToDomain(m *model.WorkbenchApp) *domain.WorkbenchApp {
	return &domain.WorkbenchApp{
		AppID:       m.AppID,
		Name:        m.Name,
		Icon:        m.Icon,
		TargetType:  m.TargetType,
		Target:      m.Target,
		GroupName:   m.GroupName,
		ColorBg:     m.ColorBg,
		ColorFg:     m.ColorFg,
		Source:      m.Source,
		InvokerSlug: m.InvokerSlug,
		SortOrder:   m.SortOrder,
		IsVisible:   m.IsVisible,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func (r *workbenchAppRepository) FindByName(ctx context.Context, name string) (*domain.WorkbenchApp, error) {
	var m model.WorkbenchApp
	err := r.session(ctx).Where("name = ?", name).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return workbenchAppToDomain(&m), nil
}

func (r *workbenchAppRepository) MaxSortOrder(ctx context.Context) (int, error) {
	var maxOrder int
	err := r.session(ctx).Model(&model.WorkbenchApp{}).
		Select("COALESCE(MAX(sort_order), 0)").Row().Scan(&maxOrder)
	return maxOrder, err
}

func (r *workbenchAppRepository) Create(ctx context.Context, app *domain.WorkbenchApp) error {
	m := model.WorkbenchApp{
		Name:        app.Name,
		Icon:        app.Icon,
		TargetType:  app.TargetType,
		Target:      app.Target,
		GroupName:   app.GroupName,
		ColorBg:     app.ColorBg,
		ColorFg:     app.ColorFg,
		Source:      app.Source,
		InvokerSlug: app.InvokerSlug,
		SortOrder:   app.SortOrder,
		IsVisible:   app.IsVisible,
		CreatedAt:   app.CreatedAt,
		UpdatedAt:   app.UpdatedAt,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	app.AppID = m.AppID
	return nil
}

func (r *workbenchAppRepository) ListVisible(ctx context.Context) ([]*domain.WorkbenchApp, error) {
	var list []model.WorkbenchApp
	if err := r.session(ctx).
		Where("is_visible = ?", true).
		Order("sort_order ASC, app_id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.WorkbenchApp, 0, len(list))
	for i := range list {
		out = append(out, workbenchAppToDomain(&list[i]))
	}
	return out, nil
}

func (r *workbenchAppRepository) ListAll(ctx context.Context) ([]*domain.WorkbenchApp, error) {
	var list []model.WorkbenchApp
	if err := r.session(ctx).
		Order("sort_order ASC, app_id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.WorkbenchApp, 0, len(list))
	for i := range list {
		out = append(out, workbenchAppToDomain(&list[i]))
	}
	return out, nil
}
