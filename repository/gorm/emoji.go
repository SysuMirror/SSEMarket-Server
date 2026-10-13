package gormrepo

import (
	"context"
	"errors"
	"strings"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type emojiRepository struct {
	db *gorm.DB
}

func NewEmojiRepository(db *gorm.DB) repository.EmojiRepository {
	return &emojiRepository{db: db}
}

func (r *emojiRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// escapeLike 把用户输入里的 LIKE 通配符转义掉，避免搜「%」搜出全世界。
func escapeLike(keyword string) string {
	keyword = strings.ReplaceAll(keyword, `\`, `\\`)
	keyword = strings.ReplaceAll(keyword, `%`, `\%`)
	keyword = strings.ReplaceAll(keyword, `_`, `\_`)
	return "%" + keyword + "%"
}

func (r *emojiRepository) EnsureBuiltinPack(ctx context.Context) (*domain.EmojiPack, error) {
	var row model.EmojiPack
	err := r.session(ctx).Where("ownerUserID = ? AND isDeleted = ?", 0, false).First(&row).Error
	if err == nil {
		return emojiPackToDomain(&row), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	pack := model.EmojiPack{Title: domain.BuiltinPackTitle, OwnerUserID: 0, IsPublic: true}
	if err := r.session(ctx).Create(&pack).Error; err != nil {
		return nil, err
	}
	return emojiPackToDomain(&pack), nil
}

func (r *emojiRepository) EnsureBuiltinStickers(ctx context.Context) error {
	var pack model.EmojiPack
	if err := r.session(ctx).Where("ownerUserID = ? AND isDeleted = ?", 0, false).First(&pack).Error; err != nil {
		return err
	}
	for _, file := range domain.BuiltinEmojiFiles {
		name := domain.BuiltinEmojiName(file)
		var cnt int64
		if err := r.session(ctx).Model(&model.Emoji{}).
			Where("isBuiltin = ? AND name = ?", true, name).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		sticker := model.Emoji{
			PackID:    pack.PackID,
			Name:      name,
			URL:       file,
			IsBuiltin: true,
		}
		if err := r.session(ctx).Create(&sticker).Error; err != nil {
			return err
		}
		if pack.CoverEmojiID == 0 {
			pack.CoverEmojiID = sticker.EmojiID
			if err := r.session(ctx).Model(&model.EmojiPack{}).
				Where("packID = ?", pack.PackID).
				Update("coverEmojiID", sticker.EmojiID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// FindPersonalPack 查用户的个人表情组；还没建过时返回 ErrNotFound（面板据此展示空组，不落库）。
func (r *emojiRepository) FindPersonalPack(ctx context.Context, userID int) (*domain.EmojiPack, error) {
	var row model.EmojiPack
	err := r.session(ctx).
		Where("ownerUserID = ? AND isPersonal = ? AND isDeleted = ?", userID, true, false).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return emojiPackToDomain(&row), nil
}

// EnsurePersonalPack 每个用户一个的个人表情组（isPersonal=true，不公开、不可搜索）。
// 默认不预建：第一次往里放表情时才落库，所以「我的表情」初始就是空的。
func (r *emojiRepository) EnsurePersonalPack(ctx context.Context, userID int) (*domain.EmojiPack, error) {
	var row model.EmojiPack
	err := r.session(ctx).
		Where("ownerUserID = ? AND isPersonal = ? AND isDeleted = ?", userID, true, false).
		First(&row).Error
	if err == nil {
		return emojiPackToDomain(&row), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	pack := model.EmojiPack{Title: domain.PersonalPackTitle, OwnerUserID: userID, IsPersonal: true}
	if err := r.session(ctx).Create(&pack).Error; err != nil {
		return nil, err
	}
	return emojiPackToDomain(&pack), nil
}

func (r *emojiRepository) ListPersonalStickers(ctx context.Context, packID int) ([]*domain.Emoji, error) {
	var rows []model.Emoji
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("packID = ? AND isDeleted = ?", packID, false).
		Order("emojiID DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return emojisToDomain(rows), nil
}

// ExistsStickerInPack 按图床 URL 去重：同一张图重复长按 / 重复上传不重复入库。
func (r *emojiRepository) ExistsStickerInPack(ctx context.Context, packID int, url string) (bool, error) {
	var cnt int64
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("packID = ? AND url = ? AND isDeleted = ?", packID, url, false).
		Count(&cnt).Error
	return cnt > 0, err
}

func (r *emojiRepository) ListBuiltinStickers(ctx context.Context) ([]*domain.Emoji, error) {
	var rows []model.Emoji
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("isBuiltin = ? AND isDeleted = ?", true, false).
		Order("emojiID").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return emojisToDomain(rows), nil
}

func (r *emojiRepository) ListUserPackIDs(ctx context.Context, userID int) ([]int, error) {
	var ids []int
	err := r.session(ctx).Model(&model.UserEmojiPack{}).
		Where("userID = ?", userID).
		Order("id DESC").
		Pluck("packID", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *emojiRepository) ListPackStickers(ctx context.Context, packIDs []int) (map[int][]*domain.Emoji, error) {
	out := make(map[int][]*domain.Emoji, len(packIDs))
	if len(packIDs) == 0 {
		return out, nil
	}
	var rows []model.Emoji
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("packID IN ? AND isDeleted = ?", packIDs, false).
		Order("emojiID").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		e := emojiToDomain(&rows[i])
		out[e.PackID] = append(out[e.PackID], e)
	}
	return out, nil
}

func (r *emojiRepository) CreatePack(ctx context.Context, title string, ownerUserID int, isPublic bool) (*domain.EmojiPack, error) {
	pack := model.EmojiPack{Title: title, OwnerUserID: ownerUserID, IsPublic: isPublic}
	if err := r.session(ctx).Create(&pack).Error; err != nil {
		return nil, err
	}
	return emojiPackToDomain(&pack), nil
}

func (r *emojiRepository) CreateStickers(ctx context.Context, packID, ownerUserID int, items []domain.EmojiStickerInput) ([]*domain.Emoji, error) {
	if len(items) == 0 {
		return nil, nil
	}
	rows := make([]model.Emoji, 0, len(items))
	for _, item := range items {
		rows = append(rows, model.Emoji{
			PackID:      packID,
			Name:        item.Name,
			URL:         item.URL,
			IsBuiltin:   item.IsBuiltin,
			OwnerUserID: ownerUserID,
		})
	}
	if err := r.session(ctx).Create(&rows).Error; err != nil {
		return nil, err
	}
	return emojisToDomain(rows), nil
}

func (r *emojiRepository) AddUserPack(ctx context.Context, userID, packID int) error {
	var cnt int64
	if err := r.session(ctx).Model(&model.UserEmojiPack{}).
		Where("userID = ? AND packID = ?", userID, packID).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	return r.session(ctx).Create(&model.UserEmojiPack{UserID: userID, PackID: packID}).Error
}

func (r *emojiRepository) RemoveUserPack(ctx context.Context, userID, packID int) error {
	return r.session(ctx).Where("userID = ? AND packID = ?", userID, packID).
		Delete(&model.UserEmojiPack{}).Error
}

func (r *emojiRepository) FindPack(ctx context.Context, packID int) (*domain.EmojiPack, error) {
	var row model.EmojiPack
	err := r.session(ctx).Where("packID = ?", packID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return emojiPackToDomain(&row), nil
}

func (r *emojiRepository) FindSticker(ctx context.Context, emojiID int) (*domain.Emoji, error) {
	var row model.Emoji
	err := r.session(ctx).Where("emojiID = ?", emojiID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return emojiToDomain(&row), nil
}

// FindStickersByIDs 不过滤 isDeleted：历史消息里的 token 必须始终能渲染出来。
func (r *emojiRepository) FindStickersByIDs(ctx context.Context, ids []int) ([]*domain.Emoji, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.Emoji
	err := r.session(ctx).Where("emojiID IN ?", ids).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return emojisToDomain(rows), nil
}

func (r *emojiRepository) CountOwnedStickers(ctx context.Context, userID int) (int64, error) {
	var cnt int64
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("ownerUserID = ? AND isDeleted = ?", userID, false).
		Count(&cnt).Error
	return cnt, err
}

func (r *emojiRepository) CountPackStickers(ctx context.Context, packID int) (int64, error) {
	var cnt int64
	err := r.session(ctx).Model(&model.Emoji{}).
		Where("packID = ? AND isDeleted = ?", packID, false).
		Count(&cnt).Error
	return cnt, err
}

func (r *emojiRepository) SoftDeleteSticker(ctx context.Context, emojiID int) error {
	return r.session(ctx).Model(&model.Emoji{}).
		Where("emojiID = ?", emojiID).
		Update("isDeleted", true).Error
}

func (r *emojiRepository) SoftDeletePack(ctx context.Context, packID int) error {
	if err := r.session(ctx).Model(&model.Emoji{}).
		Where("packID = ?", packID).
		Update("isDeleted", true).Error; err != nil {
		return err
	}
	return r.session(ctx).Model(&model.EmojiPack{}).
		Where("packID = ?", packID).
		Update("isDeleted", true).Error
}

// SearchStickers 可见范围：内置 / 自己上传的 / 所在组是公开或内置 / 所在组是自己添加或自建的。
func (r *emojiRepository) SearchStickers(ctx context.Context, userID int, keyword string, limit int) ([]*domain.Emoji, error) {
	kw := escapeLike(keyword)
	addedPacks := r.session(ctx).Model(&model.UserEmojiPack{}).
		Select("packID").Where("userID = ?", userID)
	var rows []model.Emoji
	err := r.session(ctx).
		Joins("LEFT JOIN emoji_packs p ON p.packID = emojis.packID").
		Where("emojis.isDeleted = ?", false).
		Where("emojis.name LIKE ?", kw).
		Where("emojis.isBuiltin = ? OR emojis.ownerUserID = ? OR p.isPublic = ? OR p.ownerUserID = ? OR emojis.packID IN (?)",
			true, userID, true, 0, addedPacks).
		Order("emojiID DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return emojisToDomain(rows), nil
}

func (r *emojiRepository) SearchPacks(ctx context.Context, userID int, keyword string, limit int) ([]*domain.EmojiPack, error) {
	kw := escapeLike(keyword)
	addedPacks := r.session(ctx).Model(&model.UserEmojiPack{}).
		Select("packID").Where("userID = ?", userID)
	// 组命中 = 标题命中，或组里有表情名命中。
	matchingSticker := r.session(ctx).Model(&model.Emoji{}).
		Select("1").
		Where("emojis.packID = emoji_packs.packID").
		Where("emojis.isDeleted = ?", false).
		Where("emojis.name LIKE ?", kw)
	var rows []model.EmojiPack
	err := r.session(ctx).
		Where("emoji_packs.isDeleted = ?", false).
		Where("emoji_packs.isPersonal = ?", false). // 个人表情组不参与搜索
		Where("emoji_packs.isPublic = ? OR emoji_packs.ownerUserID = ? OR emoji_packs.ownerUserID = ? OR emoji_packs.packID IN (?)",
			true, 0, userID, addedPacks).
		Where("emoji_packs.title LIKE ? OR EXISTS (?)", kw, matchingSticker).
		Order("packID DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return emojiPacksToDomain(rows), nil
}
