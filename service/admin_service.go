package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"
	"loginTest/util"
)

type adminService struct {
	admins repository.AdminRepository
}

func NewAdminService(admins repository.AdminRepository) AdminService {
	return &adminService{admins: admins}
}

func (s *adminService) Login(ctx context.Context, account, encryptedPassword string) (string, error) {
	admin, err := s.admins.FindByAccount(ctx, account)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrNotFound
		}
		return "", err
	}
	password := util.Decrypt(encryptedPassword)
	if password != admin.Password {
		return "", domain.ErrUnauthorized
	}
	token, err := common.ReleaseToken_admin(model.Admin{
		AdminID:  admin.ID,
		Account:  admin.Account,
		Password: admin.Password,
	})
	if err != nil {
		return "", domain.ErrInternal
	}
	return token, nil
}

func (s *adminService) AddAdmin(ctx context.Context, account, pass1, pass2 string) (*domain.Admin, error) {
	pass1 = util.Decrypt(pass1)
	pass2 = util.Decrypt(pass2)
	if account == "" || pass1 == "" || pass2 == "" {
		return nil, domain.ErrBadRequest
	}
	if pass1 != pass2 {
		return nil, domain.ErrBadRequest
	}
	if len(pass1) < 6 {
		return nil, domain.ErrBadRequest
	}
	if _, err := s.admins.FindByAccount(ctx, account); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	cnt, err := s.admins.Count(ctx)
	if err != nil {
		return nil, err
	}
	admin := &domain.Admin{
		ID:       int(cnt) + 1,
		Account:  account,
		Password: pass1,
	}
	if err := s.admins.Create(ctx, admin); err != nil {
		return nil, err
	}
	return admin, nil
}

func (s *adminService) ChangePassword(ctx context.Context, account, pass1, pass2 string) (*domain.Admin, error) {
	pass1 = util.Decrypt(pass1)
	pass2 = util.Decrypt(pass2)
	if pass1 != pass2 {
		return nil, domain.ErrBadRequest
	}
	admin, err := s.admins.FindByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	admin.Password = pass1
	if err := s.admins.Save(ctx, admin); err != nil {
		return nil, err
	}
	return admin, nil
}

func (s *adminService) DeleteAdmin(ctx context.Context, account string) error {
	return s.admins.DeleteByAccount(ctx, account)
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
