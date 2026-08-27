package gormrepo

import (
	"context"
	"errors"
	"strings"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type ssoBindingRepository struct {
	db *gorm.DB
}

// NewSsoBindingRepository UniSSO 绑定关系仓储。
func NewSsoBindingRepository(db *gorm.DB) repository.SsoBindingRepository {
	return &ssoBindingRepository{db: db}
}

func (r *ssoBindingRepository) FindBySub(ctx context.Context, sub string) (*domain.SsoBinding, error) {
	var row model.SsoBinding
	err := r.db.WithContext(ctx).Where("sso_sub = ?", sub).First(&row).Error
	return toDomainSsoBinding(row, err)
}

func (r *ssoBindingRepository) FindByUserID(ctx context.Context, userID int) (*domain.SsoBinding, error) {
	var row model.SsoBinding
	err := r.db.WithContext(ctx).Where("local_user_id = ?", userID).First(&row).Error
	return toDomainSsoBinding(row, err)
}

func (r *ssoBindingRepository) Create(ctx context.Context, b *domain.SsoBinding) error {
	row := model.SsoBinding{
		LocalUserID: b.LocalUserID,
		SsoSub:      b.SsoSub,
		SsoUsername: b.SsoUsername,
		CreatedAt:   b.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isSsoDuplicateEntry(err) {
			return domain.ErrConflict
		}
		return err
	}
	b.ID = row.ID
	return nil
}

func (r *ssoBindingRepository) DeleteByUserID(ctx context.Context, userID int) error {
	res := r.db.WithContext(ctx).
		Where("local_user_id = ?", userID).Delete(&model.SsoBinding{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toDomainSsoBinding(row model.SsoBinding, err error) (*domain.SsoBinding, error) {
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.SsoBinding{
		ID:          row.ID,
		LocalUserID: row.LocalUserID,
		SsoSub:      row.SsoSub,
		SsoUsername: row.SsoUsername,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func isSsoDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return strings.Contains(mysqlErr.Message, "sso_bindings")
	}
	return false
}
