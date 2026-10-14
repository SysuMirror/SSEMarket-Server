package gormrepo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type staffRepository struct {
	db *gorm.DB
}

func NewStaffRepository(db *gorm.DB) repository.StaffRepository {
	return &staffRepository{db: db}
}

func (r *staffRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func staffAliasToDomain(m *model.StaffAlias) *domain.StaffAlias {
	if m == nil {
		return nil
	}
	return &domain.StaffAlias{
		ID:          m.ID,
		Alias:       m.Alias,
		Title:       m.Title,
		StaffUserID: m.StaffUserID,
		AliasUserID: m.AliasUserID,
	}
}

func (r *staffRepository) ListAliases(ctx context.Context) ([]*domain.StaffAlias, error) {
	var rows []model.StaffAlias
	err := r.session(ctx).Order("id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StaffAlias, 0, len(rows))
	for i := range rows {
		out = append(out, staffAliasToDomain(&rows[i]))
	}
	return out, nil
}

func (r *staffRepository) FindAliasesByNames(ctx context.Context, names []string) ([]*domain.StaffAlias, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var rows []model.StaffAlias
	err := r.session(ctx).Where("alias IN ?", names).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StaffAlias, 0, len(rows))
	for i := range rows {
		out = append(out, staffAliasToDomain(&rows[i]))
	}
	return out, nil
}

func (r *staffRepository) FindAliasesByStaffUserID(ctx context.Context, staffUserID int) ([]*domain.StaffAlias, error) {
	if staffUserID <= 0 {
		return nil, nil
	}
	var rows []model.StaffAlias
	err := r.session(ctx).Where("staff_user_id = ?", staffUserID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StaffAlias, 0, len(rows))
	for i := range rows {
		out = append(out, staffAliasToDomain(&rows[i]))
	}
	return out, nil
}

func (r *staffRepository) FindAliasesByAliasUserIDs(ctx context.Context, aliasUserIDs []int) ([]*domain.StaffAlias, error) {
	if len(aliasUserIDs) == 0 {
		return nil, nil
	}
	var rows []model.StaffAlias
	err := r.session(ctx).Where("alias_user_id IN ?", aliasUserIDs).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StaffAlias, 0, len(rows))
	for i := range rows {
		out = append(out, staffAliasToDomain(&rows[i]))
	}
	return out, nil
}

func (r *staffRepository) UpdateAliasUserID(ctx context.Context, aliasID, aliasUserID int) error {
	return r.session(ctx).Model(&model.StaffAlias{}).Where("id = ?", aliasID).
		UpdateColumn("alias_user_id", aliasUserID).Error
}

func (r *staffRepository) FindShellUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u model.User
	err := r.session(ctx).Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return userToDomain(&u), nil
}

func (r *staffRepository) UsersByNameExcludeEmail(ctx context.Context, name, excludeEmail string) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.User{}).
		Where("name = ? AND (email IS NULL OR email <> ?)", name, excludeEmail).
		Count(&count).Error
	return count, err
}

// staffShellPassword 一段无人知道的随机串的 bcrypt 哈希：职位小号永远无法登录。
func staffShellPassword(aliasID int) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		raw = []byte(fmt.Sprintf("%d:%d", aliasID, time.Now().UnixNano()))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(raw)), bcrypt.DefaultCost)
	if err != nil {
		// 不是合法 bcrypt 哈希的占位串同样无法通过登录校验
		return fmt.Sprintf("staff-alias-unloginnable-%d", aliasID)
	}
	return string(hash)
}

func (r *staffRepository) CreateShellUser(ctx context.Context, aliasID int, alias, title string) (int, error) {
	u := model.User{
		Name:     alias,
		Email:    domain.StaffAliasShellEmail(aliasID),
		Password: staffShellPassword(aliasID),
		// 等级自动满级（前端 Lv8 阈值 score>=5000），称号即职称
		Score:       domain.StaffAliasScore,
		CustomTitle: title,
		// 职位小号不参与邮件通知；没有手机号，无短信/找回路径
		ISEmailNotificationBlocked: true,
	}
	if err := r.session(ctx).Create(&u).Error; err != nil {
		return 0, err
	}
	return u.UserID, nil
}

func (r *staffRepository) UpdateShellUserProfile(ctx context.Context, userID int, alias, title string) error {
	return r.session(ctx).Model(&model.User{}).Where("userID = ?", userID).
		Updates(map[string]any{"name": alias, "custom_title": title}).Error
}

func (r *staffRepository) FindPostContentByID(ctx context.Context, postID int) (string, error) {
	var p model.Post
	err := r.session(ctx).Select("ptext").Where("postID = ?", postID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return p.Ptext, nil
}

func (r *staffRepository) ListPcommentContentsByPostID(ctx context.Context, postID int) ([]string, error) {
	var rows []model.Pcomment
	// 只取正文列，避免拖出整行
	err := r.session(ctx).Select("pctext").Where("ptargetID = ?", postID).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Pctext)
	}
	return out, nil
}

func (r *staffRepository) ListCcommentContentsByPostID(ctx context.Context, postID int) ([]string, error) {
	var texts []string
	// 楼中楼经 ctargetID(=pcommentID) 挂到一级评论，再按帖子过滤
	err := r.session(ctx).Table("ccomments").
		Joins("JOIN pcomments ON pcomments.pcommentID = ccomments.ctargetID").
		Where("pcomments.ptargetID = ?", postID).
		Pluck("ccomments.cctext", &texts).Error
	if err != nil {
		log.Printf("[staff] list ccomment contents postID=%d: %v", postID, err)
		return nil, err
	}
	return texts, nil
}
