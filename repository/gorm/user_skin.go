package gormrepo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type userSkinRepository struct {
	db *gorm.DB
}

func NewUserSkinRepository(db *gorm.DB) repository.UserSkinRepository {
	return &userSkinRepository{db: db}
}

func newShareToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

type skinRow struct {
	SkinID      int       `gorm:"column:skin_id"`
	OwnerID     int       `gorm:"column:owner_id"`
	OwnerName   string    `gorm:"column:owner_name"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	DesktopURL  string    `gorm:"column:desktop_url"`
	MobileURL   string    `gorm:"column:mobile_url"`
	IsPublic    bool      `gorm:"column:is_public"`
	ShareToken  string    `gorm:"column:share_token"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
	IsOwner     bool      `gorm:"column:is_owner"`
	InLibrary   bool      `gorm:"column:in_library"`
}

func (r skinRow) toDomain() domain.UserSkin {
	return domain.UserSkin{
		ID:          r.SkinID,
		OwnerID:     r.OwnerID,
		OwnerName:   r.OwnerName,
		Name:        r.Name,
		Description: r.Description,
		DesktopURL:  r.DesktopURL,
		MobileURL:   r.MobileURL,
		IsPublic:    r.IsPublic,
		ShareToken:  r.ShareToken,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		IsOwner:     r.IsOwner,
		InLibrary:   r.InLibrary,
	}
}

func (r *userSkinRepository) baseQuery(viewerID int) *gorm.DB {
	if viewerID > 0 {
		return r.db.Table("user_skins AS s").
			Select(`s.skin_id, s.owner_id, COALESCE(u.name, '') AS owner_name,
				s.name, s.description, s.desktop_url, s.mobile_url, s.is_public, s.share_token,
				s.created_at, s.updated_at,
				(s.owner_id = ?) AS is_owner,
				EXISTS(SELECT 1 FROM user_skin_library l WHERE l.skin_id = s.skin_id AND l.user_id = ?) AS in_library`,
				viewerID, viewerID).
			Joins("LEFT JOIN users AS u ON u.userID = s.owner_id")
	}
	return r.db.Table("user_skins AS s").
		Select(`s.skin_id, s.owner_id, COALESCE(u.name, '') AS owner_name,
			s.name, s.description, s.desktop_url, s.mobile_url, s.is_public, s.share_token,
			s.created_at, s.updated_at, 0 AS is_owner, 0 AS in_library`).
		Joins("LEFT JOIN users AS u ON u.userID = s.owner_id")
}

func (r *userSkinRepository) findByID(ctx context.Context, skinID, viewerID int) (*domain.UserSkin, error) {
	var row skinRow
	if err := r.baseQuery(viewerID).Where("s.skin_id = ?", skinID).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.SkinID == 0 {
		return nil, domain.ErrUserSkinNotFound
	}
	skin := row.toDomain()
	return &skin, nil
}

func (r *userSkinRepository) Create(ctx context.Context, in domain.UserSkinInput) (*domain.UserSkin, error) {
	token, err := newShareToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	row := model.UserSkin{
		OwnerID:     in.OwnerID,
		Name:        in.Name,
		Description: in.Description,
		DesktopURL:  in.DesktopURL,
		MobileURL:   in.MobileURL,
		IsPublic:    false,
		ShareToken:  token,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.UserSkinLibrary{
			UserID:  in.OwnerID,
			SkinID:  row.SkinID,
			AddedAt: now,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return r.findByID(ctx, row.SkinID, in.OwnerID)
}

func (r *userSkinRepository) ListLibrary(ctx context.Context, userID int) ([]domain.UserSkin, error) {
	var rows []skinRow
	if err := r.baseQuery(userID).
		Joins("JOIN user_skin_library AS lib ON lib.skin_id = s.skin_id AND lib.user_id = ?", userID).
		Order("lib.added_at DESC, s.skin_id DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.UserSkin, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDomain())
	}
	return result, nil
}

func escapeLike(raw string) string {
	raw = strings.ReplaceAll(raw, `\`, `\\`)
	raw = strings.ReplaceAll(raw, `%`, `\%`)
	raw = strings.ReplaceAll(raw, `_`, `\_`)
	return raw
}

func (r *userSkinRepository) SearchPublic(ctx context.Context, userID int, keyword string, limit int) ([]domain.UserSkin, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := r.baseQuery(userID).Where("s.is_public = ?", true)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + escapeLike(keyword) + "%"
		q = q.Where("(s.name LIKE ? OR u.name LIKE ?)", like, like)
	}
	var rows []skinRow
	if err := q.Order("s.created_at DESC, s.skin_id DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.UserSkin, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDomain())
	}
	return result, nil
}

func (r *userSkinRepository) FindByID(ctx context.Context, skinID int) (*domain.UserSkin, error) {
	return r.findByID(ctx, skinID, 0)
}

func (r *userSkinRepository) FindByShareToken(ctx context.Context, token string) (*domain.UserSkin, error) {
	var row model.UserSkin
	if err := r.db.WithContext(ctx).Where("share_token = ?", token).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrUserSkinNotFound
		}
		return nil, err
	}
	return r.findByID(ctx, row.SkinID, 0)
}

func (r *userSkinRepository) FindForUser(ctx context.Context, userID, skinID int) (*domain.UserSkin, error) {
	if userID <= 0 || skinID <= 0 {
		return nil, domain.ErrUserSkinNotFound
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.UserSkinLibrary{}).
		Where("user_id = ? AND skin_id = ?", userID, skinID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, domain.ErrUserSkinNotInLibrary
	}
	return r.findByID(ctx, skinID, userID)
}

func (r *userSkinRepository) AddToLibrary(ctx context.Context, userID, skinID int) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.UserSkinLibrary{UserID: userID, SkinID: skinID, AddedAt: time.Now()}).Error
}

func (r *userSkinRepository) RemoveFromLibrary(ctx context.Context, userID, skinID int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND skin_id = ?", userID, skinID).
			Delete(&model.UserSkinLibrary{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.UserThemeSetting{}).
			Where("user_id = ? AND custom_skin_id = ?", userID, skinID).
			UpdateColumn("custom_skin_id", 0).Error
	})
}

func (r *userSkinRepository) SetPublic(ctx context.Context, skinID, ownerID int, public bool) (*domain.UserSkin, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.UserSkin{}).
		Where("skin_id = ? AND owner_id = ?", skinID, ownerID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, domain.ErrUserSkinNotOwner
	}
	if err := r.db.WithContext(ctx).Model(&model.UserSkin{}).
		Where("skin_id = ? AND owner_id = ?", skinID, ownerID).
		Updates(map[string]any{"is_public": public, "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	return r.findByID(ctx, skinID, ownerID)
}

func (r *userSkinRepository) Delete(ctx context.Context, skinID, ownerID int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var skin model.UserSkin
		if err := tx.Where("skin_id = ? AND owner_id = ?", skinID, ownerID).First(&skin).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrUserSkinNotFound
			}
			return err
		}
		if err := tx.Where("skin_id = ?", skinID).Delete(&model.UserSkinLibrary{}).Error; err != nil {
			return err
		}
		// 正在使用它的用户回退到皮肤自带背景，避免指向已删除记录。
		if err := tx.Model(&model.UserThemeSetting{}).
			Where("custom_skin_id = ?", skinID).
			UpdateColumn("custom_skin_id", 0).Error; err != nil {
			return err
		}
		return tx.Where("skin_id = ?", skinID).Delete(&model.UserSkin{}).Error
	})
}
