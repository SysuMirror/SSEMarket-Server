package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"loginTest/common"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"

	"github.com/brianvoe/gofakeit"
	"golang.org/x/crypto/bcrypt"
)

const maxValidateFail = 5

type userService struct {
	users repository.UserRepository
}

func NewUserService(users repository.UserRepository) UserService {
	return &userService{users: users}
}

func (s *userService) GetByID(ctx context.Context, id int) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}

func (s *userService) Login(ctx context.Context, in domain.LoginInput) (*domain.AuthTokens, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return nil, domain.ErrBadRequest
	}
	if len(in.Password) < 6 {
		return nil, domain.ErrBadRequest
	}

	u, hash, err := s.users.FindAuthByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return nil, domain.ErrUnauthorized
	}

	return s.issueTokens(ctx, u)
}

func (s *userService) Register(ctx context.Context, in domain.RegisterInput) (*domain.AuthTokens, error) {
	name := in.Name
	phone := in.Phone
	password := in.Password
	email := strings.TrimSpace(in.Email)

	if phone == "" {
		for {
			phone = util.GenerateRandomDigits(11)
			ok, err := s.users.ExistsByPhone(ctx, phone)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
		}
	}
	if len(phone) != 11 {
		return nil, domain.ErrBadRequest
	}
	if len(password) < 6 {
		return nil, domain.ErrBadRequest
	}
	if name == "" {
		name = gofakeit.Name()
	}
	if ok, _ := s.users.ExistsByPhone(ctx, phone); ok {
		return nil, domain.ErrConflict
	}
	if !strings.Contains(email, "@") {
		return nil, domain.ErrBadRequest
	}
	if ok, _ := s.users.ExistsByEmail(ctx, email); ok {
		return nil, domain.ErrConflict
	}
	if err := s.verifyEmailCode(ctx, email, in.ValiCode); err != nil {
		return nil, err
	}
	if err := s.users.UseCDKey(ctx, in.CDKey); err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			return nil, domain.ErrBadRequest
		}
		return nil, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.ErrInternal
	}
	u, err := s.users.Create(ctx, name, phone, email, string(hashed))
	if err != nil {
		return nil, err
	}

	tokens, err := s.issueTokens(ctx, u)
	if err != nil {
		return nil, err
	}
	s.clearEmailCode(ctx, email)
	return tokens, nil
}

func (s *userService) issueTokens(ctx context.Context, u *domain.User) (*domain.AuthTokens, error) {
	modelUser := DomainUserToModel(u)
	access, err := common.ReleaseToken(modelUser)
	if err != nil {
		return nil, domain.ErrInternal
	}
	refresh, err := common.ReleaseRefreshToken(modelUser)
	if err != nil {
		return nil, domain.ErrInternal
	}
	expires := time.Now().Add(30 * 24 * time.Hour).Unix()
	if err := s.users.SaveRefreshToken(ctx, u.ID, refresh, expires); err != nil {
		return nil, err
	}
	CacheUserAfterAuth(ctx, modelUser)
	return &domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         u,
	}, nil
}

func (s *userService) verifyEmailCode(ctx context.Context, email, valiCode string) error {
	if core.MyRedis == nil {
		return domain.ErrInternal
	}
	codeKey := "vcode:" + email
	failKey := "vcode_fail:" + email
	correct, err := core.MyRedis.Get(ctx, codeKey).Result()
	if err != nil || correct != valiCode {
		failCountStr, _ := core.MyRedis.Get(ctx, failKey).Result()
		failCount := 0
		if failCountStr != "" {
			if v, convErr := strconv.Atoi(failCountStr); convErr == nil {
				failCount = v
			}
		}
		failCount++
		_ = core.MyRedis.Set(ctx, failKey, strconv.Itoa(failCount), 5*time.Minute).Err()
		if failCount >= maxValidateFail {
			_ = core.MyRedis.Del(ctx, codeKey).Err()
		}
		return domain.ErrUnauthorized
	}
	return nil
}

func (s *userService) clearEmailCode(ctx context.Context, email string) {
	if core.MyRedis == nil {
		return
	}
	_ = core.MyRedis.Del(ctx, "vcode:"+email, "vcode_fail:"+email).Err()
}
