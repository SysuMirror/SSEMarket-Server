package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"loginTest/config"
	"loginTest/domain"
	"loginTest/repository"
)

type adminService struct {
	admins repository.AdminRepository
}

func NewAdminService(admins repository.AdminRepository) AdminService {
	return &adminService{admins: admins}
}

func (s *adminService) GetByID(ctx context.Context, id int) (*domain.Admin, error) {
	return s.admins.FindByID(ctx, id)
}

func (s *adminService) FilterUsers(ctx context.Context, filter domain.AdminUserFilter) ([]domain.AdminUserItem, error) {
	return s.admins.FilterUsers(ctx, filter)
}

func (s *adminService) DeleteUser(ctx context.Context, name string) error {
	return s.admins.DeleteUserByName(ctx, name)
}

func (s *adminService) CreatePost(ctx context.Context, in domain.AdminPostInput) error {
	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	partition := strings.TrimSpace(in.Partition)
	if title == "" || content == "" || partition == "" {
		return domain.ErrBadRequest
	}
	if utf8.RuneCountInString(title) > 15 {
		return domain.ErrBadRequest
	}
	if utf8.RuneCountInString(content) > config.PostContentMaxRunes {
		return domain.ErrBadRequest
	}
	_, err := s.admins.CreateAdminPost(ctx, in)
	return err
}
