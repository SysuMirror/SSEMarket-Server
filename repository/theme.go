package repository

import (
	"context"

	"loginTest/domain"
)

// ThemeRepository 用户外观设置持久化接口。
//
// 约定：没有行的用户视为全部使用默认设置（见 domain.DefaultThemeSettings），
// Find 查不到时返回 domain.ErrNotFound，由 Service 层翻译成默认值，而不是报错。
type ThemeRepository interface {
	// Find 读取某用户的外观设置；没有行返回 domain.ErrNotFound。
	Find(ctx context.Context, userID int) (*domain.ThemeSettings, error)

	// Save 新建或覆盖某用户的外观设置（按 user_id 幂等 upsert）。
	Save(ctx context.Context, settings domain.ThemeSettings) (*domain.ThemeSettings, error)
}
