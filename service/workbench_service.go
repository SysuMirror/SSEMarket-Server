package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/repository"
)

// WorkbenchAppService 工作台应用领域服务。
type WorkbenchAppService interface {
	CreateApp(ctx context.Context, in domain.WorkbenchAppCreateInput) (*domain.WorkbenchApp, error)
	ListVisible(ctx context.Context) ([]*domain.WorkbenchApp, error)
	ListAll(ctx context.Context) ([]*domain.WorkbenchApp, error)
}

type workbenchAppService struct {
	apps repository.WorkbenchAppRepository
}

func NewWorkbenchAppService(apps repository.WorkbenchAppRepository) WorkbenchAppService {
	return &workbenchAppService{apps: apps}
}

const (
	workbenchAppNameMaxRunes = 20
	workbenchAppTargetMaxLen = 500
)

func validateWorkbenchAppInput(in *domain.WorkbenchAppCreateInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return errors.New("应用名称不能为空")
	}
	if utf8.RuneCountInString(in.Name) > workbenchAppNameMaxRunes {
		return errors.New("应用名称不能超过 20 字符")
	}
	in.Icon = strings.TrimSpace(in.Icon)
	if in.Icon == "" {
		return errors.New("图标不能为空")
	}
	in.TargetType = strings.ToLower(strings.TrimSpace(in.TargetType))
	switch in.TargetType {
	case "route":
		in.Target = strings.TrimSpace(in.Target)
		if !strings.HasPrefix(in.Target, "/") {
			return errors.New("站内路由必须以 / 开头")
		}
	case "link":
		in.Target = strings.TrimSpace(in.Target)
		u, err := url.Parse(in.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("站外链接必须是合法的 http(s) 地址")
		}
	default:
		return errors.New("targetType 必须为 route 或 link")
	}
	if utf8.RuneCountInString(in.Target) > workbenchAppTargetMaxLen {
		return errors.New("跳转地址过长")
	}
	in.GroupName = strings.TrimSpace(in.GroupName)
	if in.GroupName == "" {
		in.GroupName = "common"
	}
	in.InvokerSlug = strings.TrimSpace(in.InvokerSlug)
	if utf8.RuneCountInString(in.InvokerSlug) > 100 {
		return errors.New("调用方标识过长")
	}
	return nil
}

func (s *workbenchAppService) CreateApp(ctx context.Context, in domain.WorkbenchAppCreateInput) (*domain.WorkbenchApp, error) {
	if err := validateWorkbenchAppInput(&in); err != nil {
		return nil, err
	}
	if _, err := s.apps.FindByName(ctx, in.Name); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	sortOrder := in.SortOrder
	if sortOrder <= 0 {
		maxOrder, err := s.apps.MaxSortOrder(ctx)
		if err != nil {
			return nil, err
		}
		sortOrder = maxOrder + 10
	}

	source := "local"
	if in.InvokerSlug != "" {
		source = "platform"
	}

	now := time.Now()
	app := &domain.WorkbenchApp{
		Name:        in.Name,
		Icon:        in.Icon,
		TargetType:  in.TargetType,
		Target:      in.Target,
		GroupName:   in.GroupName,
		ColorBg:     strings.TrimSpace(in.ColorBg),
		ColorFg:     strings.TrimSpace(in.ColorFg),
		Source:      source,
		InvokerSlug: in.InvokerSlug,
		SortOrder:   sortOrder,
		IsVisible:   in.IsVisible,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.apps.Create(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

func (s *workbenchAppService) ListVisible(ctx context.Context) ([]*domain.WorkbenchApp, error) {
	return s.apps.ListVisible(ctx)
}

func (s *workbenchAppService) ListAll(ctx context.Context) ([]*domain.WorkbenchApp, error) {
	return s.apps.ListAll(ctx)
}
