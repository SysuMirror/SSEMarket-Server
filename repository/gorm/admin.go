package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type adminRepository struct {
	db *gorm.DB
}

func NewAdminRepository(db *gorm.DB) repository.AdminRepository {
	return &adminRepository{db: db}
}

func (r *adminRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *adminRepository) FindByID(ctx context.Context, id int) (*domain.Admin, error) {
	var m model.Admin
	err := r.session(ctx).Where("adminID = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return adminToDomain(&m), nil
}

func (r *adminRepository) FindBySsoSub(ctx context.Context, sub string) (*domain.Admin, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, domain.ErrNotFound
	}
	var m model.Admin
	err := r.session(ctx).Where("sso_sub = ?", sub).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return adminToDomain(&m), nil
}

func (r *adminRepository) FindByEmail(ctx context.Context, email string) (*domain.Admin, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, domain.ErrNotFound
	}
	var m model.Admin
	err := r.session(ctx).Where("LOWER(email) = ?", email).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return adminToDomain(&m), nil
}

func (r *adminRepository) List(ctx context.Context) ([]domain.Admin, error) {
	var rows []model.Admin
	if err := r.session(ctx).Order("adminID ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Admin, 0, len(rows))
	for i := range rows {
		out = append(out, *adminToDomain(&rows[i]))
	}
	return out, nil
}

func (r *adminRepository) CountActive(ctx context.Context) (int64, error) {
	var n int64
	err := r.session(ctx).Model(&model.Admin{}).Where("is_active = ?", true).Count(&n).Error
	return n, err
}

func (r *adminRepository) Create(ctx context.Context, admin *domain.Admin) error {
	m := model.Admin{
		Account:     admin.Account,
		AdminID:     admin.ID,
		SsoSub:      admin.SsoSub,
		Email:       admin.Email,
		Name:        admin.Name,
		IsActive:    admin.IsActive,
		CreatedAt:   timePtr(admin.CreatedAt),
		LastLoginAt: timePtr(admin.LastLoginAt),
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	admin.ID = m.AdminID
	return nil
}

func (r *adminRepository) UpdateLogin(ctx context.Context, id int, name, email, ssoSub string, at time.Time) error {
	updates := map[string]interface{}{
		"last_login_at": at,
		"name":          name,
		"email":         email,
		"is_active":     true,
	}
	if strings.TrimSpace(ssoSub) != "" {
		updates["sso_sub"] = ssoSub
	}
	return r.session(ctx).Model(&model.Admin{}).Where("adminID = ?", id).Updates(updates).Error
}

func (r *adminRepository) SetActive(ctx context.Context, id int, active bool) error {
	return r.session(ctx).Model(&model.Admin{}).Where("adminID = ?", id).
		Update("is_active", active).Error
}

func (r *adminRepository) DeleteByID(ctx context.Context, id int) error {
	res := r.session(ctx).Where("adminID = ?", id).Delete(&model.Admin{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *adminRepository) FilterUsers(ctx context.Context, filter domain.AdminUserFilter) ([]domain.AdminUserItem, error) {
	q := r.session(ctx).Model(&model.User{})
	if filter.Phone != "" {
		q = q.Where("phone = ?", filter.Phone)
	}
	if filter.Name != "" {
		q = q.Where("name LIKE ?", filter.Name+"%")
	}
	if filter.IDPass != -1 {
		q = q.Where("idPass = ?", filter.IDPass)
	}
	var users []model.User
	if err := q.Where("name <> ?", "用户已注销").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]domain.AdminUserItem, 0, len(users))
	for _, u := range users {
		out = append(out, domain.AdminUserItem{
			Phone:     u.Phone,
			Email:     u.Email,
			Name:      u.Name,
			PunishNum: u.Punishnum,
		})
	}
	return out, nil
}

func (r *adminRepository) DeleteUserByName(ctx context.Context, name string) error {
	var u model.User
	if err := r.session(ctx).Where("name = ?", name).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	return r.session(ctx).Delete(&u).Error
}

func (r *adminRepository) CreateAdminPost(ctx context.Context, in domain.AdminPostInput) (int, error) {
	p := model.Post{
		UserID:     0,
		Partition:  in.Partition,
		Title:      in.Title,
		Ptext:      in.Content,
		LikeNum:    0,
		CommentNum: 0,
		BrowseNum:  0,
		Heat:       0,
		PostTime:   time.Now(),
		Photos:     in.Photos,
		Tag:        in.Tags,
	}
	if err := r.session(ctx).Create(&p).Error; err != nil {
		return 0, err
	}
	return p.PostID, nil
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
