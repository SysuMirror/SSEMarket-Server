package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/repository"
)

type userSkinService struct {
	skins repository.UserSkinRepository
}

func NewUserSkinService(skins repository.UserSkinRepository) UserSkinService {
	return &userSkinService{skins: skins}
}

func normalizeSkinURL(raw string) (string, error) {
	normalized, err := domain.NormalizeThemeBackgroundURL(raw)
	if err != nil || normalized == "" {
		return "", domain.ErrUserSkinImageInvalid
	}
	return normalized, nil
}

func (s *userSkinService) Create(ctx context.Context, ownerID int, in domain.UserSkinInput) (*domain.UserSkin, error) {
	if ownerID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	in.OwnerID = ownerID
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 30 {
		return nil, domain.ErrUserSkinNameInvalid
	}
	in.Description = strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(in.Description) > 120 {
		in.Description = string([]rune(in.Description)[:120])
	}
	desktop, err := normalizeSkinURL(in.DesktopURL)
	if err != nil {
		return nil, err
	}
	mobile, err := normalizeSkinURL(in.MobileURL)
	if err != nil {
		return nil, err
	}
	in.DesktopURL = desktop
	in.MobileURL = mobile
	return s.skins.Create(ctx, in)
}

func (s *userSkinService) ListLibrary(ctx context.Context, userID int) ([]domain.UserSkin, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	return s.skins.ListLibrary(ctx, userID)
}

func (s *userSkinService) SearchPublic(ctx context.Context, userID int, keyword string, limit int) ([]domain.UserSkin, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	return s.skins.SearchPublic(ctx, userID, keyword, limit)
}

func (s *userSkinService) AddToLibrary(ctx context.Context, userID, skinID int) (*domain.UserSkin, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	skin, err := s.skins.FindByID(ctx, skinID)
	if err != nil {
		return nil, err
	}
	if skin.OwnerID != userID && !skin.IsPublic {
		return nil, domain.ErrUserSkinNotPublic
	}
	if err := s.skins.AddToLibrary(ctx, userID, skinID); err != nil {
		return nil, err
	}
	return s.skins.FindForUser(ctx, userID, skinID)
}

func (s *userSkinService) AddByShareToken(ctx context.Context, userID int, token string) (*domain.UserSkin, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrUserSkinNotFound
	}
	skin, err := s.skins.FindByShareToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !skin.IsPublic {
		return nil, domain.ErrUserSkinNotPublic
	}
	if err := s.skins.AddToLibrary(ctx, userID, skin.ID); err != nil {
		return nil, err
	}
	return s.skins.FindForUser(ctx, userID, skin.ID)
}

func (s *userSkinService) RemoveFromLibrary(ctx context.Context, userID, skinID int) error {
	if userID <= 0 {
		return domain.ErrUnauthorized
	}
	skin, err := s.skins.FindByID(ctx, skinID)
	if err != nil {
		return err
	}
	if skin.OwnerID == userID {
		return domain.ErrUserSkinAlreadyOwned
	}
	return s.skins.RemoveFromLibrary(ctx, userID, skinID)
}

func (s *userSkinService) SetPublic(ctx context.Context, userID, skinID int, public bool) (*domain.UserSkin, error) {
	if userID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	skin, err := s.skins.FindByID(ctx, skinID)
	if err != nil {
		return nil, err
	}
	if skin.OwnerID != userID {
		return nil, domain.ErrUserSkinNotOwner
	}
	return s.skins.SetPublic(ctx, skinID, userID, public)
}

func (s *userSkinService) Delete(ctx context.Context, userID, skinID int) error {
	if userID <= 0 {
		return domain.ErrUnauthorized
	}
	skin, err := s.skins.FindByID(ctx, skinID)
	if err != nil {
		return err
	}
	if skin.OwnerID != userID {
		return domain.ErrUserSkinNotOwner
	}
	return s.skins.Delete(ctx, skinID, userID)
}

func (s *userSkinService) ResolveShareToken(ctx context.Context, token string) (*domain.UserSkin, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, domain.ErrUserSkinNotFound
	}
	skin, err := s.skins.FindByShareToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !skin.IsPublic {
		return nil, domain.ErrUserSkinNotPublic
	}
	return skin, nil
}

func (s *userSkinService) GetActiveForUser(ctx context.Context, userID, skinID int) (*domain.UserSkin, error) {
	if userID <= 0 || skinID <= 0 {
		return nil, errors.New("invalid skin")
	}
	return s.skins.FindForUser(ctx, userID, skinID)
}
