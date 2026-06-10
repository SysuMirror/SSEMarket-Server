package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *userRepository) FindByID(ctx context.Context, id int) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("userID = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindAuthByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	var user model.User
	err := r.session(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", domain.ErrNotFound
		}
		return nil, "", err
	}
	return userToDomain(&user), user.Password, nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var n int64
	err := r.session(ctx).Model(&model.User{}).Where("email = ?", email).Count(&n).Error
	return n > 0, err
}

func (r *userRepository) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var n int64
	err := r.session(ctx).Model(&model.User{}).Where("phone = ?", phone).Count(&n).Error
	return n > 0, err
}

func (r *userRepository) Create(ctx context.Context, name, phone, email, hashedPassword string) (*domain.User, error) {
	user := model.User{
		Name:     name,
		Phone:    phone,
		Email:    email,
		Password: hashedPassword,
		Banend:   time.Now(),
	}
	if err := r.session(ctx).Create(&user).Error; err != nil {
		return nil, err
	}
	if user.UserID == 0 {
		return nil, domain.ErrInternal
	}
	return userToDomain(&user), nil
}

func (r *userRepository) SaveRefreshToken(ctx context.Context, userID int, token string, expiresAt int64) error {
	rec := model.RefreshToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Unix(expiresAt, 0),
		IsUsed:    false,
		CreatedAt: time.Now(),
	}
	return r.session(ctx).Create(&rec).Error
}

func (r *userRepository) UseCDKey(ctx context.Context, content string) error {
	var key model.CDKey
	err := r.session(ctx).Where("content = ? AND used = 0", content).First(&key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrBadRequest
		}
		return err
	}
	return r.session(ctx).Model(&key).Updates(model.CDKey{
		Used:     true,
		UsedTime: time.Now(),
	}).Error
}
