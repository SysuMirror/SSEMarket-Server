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

func (r *adminRepository) FindByAccount(ctx context.Context, account string) (*domain.Admin, error) {
	var m model.Admin
	err := r.session(ctx).Where("account = ?", account).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return adminToDomain(&m), nil
}

func (r *adminRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.session(ctx).Model(&model.Admin{}).Count(&n).Error
	return n, err
}

func (r *adminRepository) Create(ctx context.Context, admin *domain.Admin) error {
	m := model.Admin{
		Account:  admin.Account,
		Password: admin.Password,
		AdminID:  admin.ID,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	admin.ID = m.AdminID
	return nil
}

func (r *adminRepository) Save(ctx context.Context, admin *domain.Admin) error {
	m := model.Admin{
		AdminID:  admin.ID,
		Account:  admin.Account,
		Password: admin.Password,
	}
	return r.session(ctx).Save(&m).Error
}

func (r *adminRepository) DeleteByAccount(ctx context.Context, account string) error {
	res := r.session(ctx).Where("account = ?", account).Delete(&model.Admin{})
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
