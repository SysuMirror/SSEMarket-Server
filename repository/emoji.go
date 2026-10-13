package repository

import (
	"context"

	"loginTest/domain"
)

// EmojiRepository 表情包持久化接口。
type EmojiRepository interface {
	// 内置组 / 内置表情种子（幂等）。
	EnsureBuiltinPack(ctx context.Context) (*domain.EmojiPack, error)
	EnsureBuiltinStickers(ctx context.Context) error

	// 个人表情组：每用户一个（isPersonal=true），默认空，首次放表情时才落库。
	FindPersonalPack(ctx context.Context, userID int) (*domain.EmojiPack, error)
	EnsurePersonalPack(ctx context.Context, userID int) (*domain.EmojiPack, error)
	ListPersonalStickers(ctx context.Context, packID int) ([]*domain.Emoji, error)
	ExistsStickerInPack(ctx context.Context, packID int, url string) (bool, error)

	ListBuiltinStickers(ctx context.Context) ([]*domain.Emoji, error)
	ListUserPackIDs(ctx context.Context, userID int) ([]int, error)
	ListPackStickers(ctx context.Context, packIDs []int) (map[int][]*domain.Emoji, error)

	CreatePack(ctx context.Context, title string, ownerUserID int, isPublic bool) (*domain.EmojiPack, error)
	CreateStickers(ctx context.Context, packID, ownerUserID int, items []domain.EmojiStickerInput) ([]*domain.Emoji, error)

	AddUserPack(ctx context.Context, userID, packID int) error
	RemoveUserPack(ctx context.Context, userID, packID int) error

	FindPack(ctx context.Context, packID int) (*domain.EmojiPack, error)
	FindSticker(ctx context.Context, emojiID int) (*domain.Emoji, error)
	FindStickersByIDs(ctx context.Context, ids []int) ([]*domain.Emoji, error)

	CountOwnedStickers(ctx context.Context, userID int) (int64, error)
	CountPackStickers(ctx context.Context, packID int) (int64, error)

	SoftDeleteSticker(ctx context.Context, emojiID int) error
	SoftDeletePack(ctx context.Context, packID int) error

	SearchStickers(ctx context.Context, userID int, keyword string, limit int) ([]*domain.Emoji, error)
	SearchPacks(ctx context.Context, userID int, keyword string, limit int) ([]*domain.EmojiPack, error)
}
