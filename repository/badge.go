package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// BadgeRepository 勋章目录 / 发放记录 / 展示偏好的持久化接口。
//
// 勋章目录由管理员直接 INSERT badges 维护，发放在 service/SQL 脚本里完成；
// 这里只负责读和展示偏好写入。
type BadgeRepository interface {
	// ListUserBadges 某用户已获得的全部启用勋章，按目录排序。
	ListUserBadges(ctx context.Context, userID int) ([]domain.AwardedBadge, error)

	// FindPrefs 展示偏好；没有行返回 domain.ErrNotFound。
	FindPrefs(ctx context.Context, userID int) (*domain.UserBadgePrefs, error)

	// SavePrefs 新建或覆盖展示偏好（按 user_id 幂等 upsert）。
	SavePrefs(ctx context.Context, prefs domain.UserBadgePrefs) error

	// WornBadgesByUserIDs 批量取一批用户对外挂载的勋章。
	// 只返回「偏好里填了、且确实拥有、且勋章启用」的记录。
	WornBadgesByUserIDs(ctx context.Context, userIDs []int) (map[int]domain.WornBadge, error)

	// AwardDueAnniversaries 按 users.registered_at 给已满 1..maxYears 年的用户
	// 补发 anniversary-n（目录 code 为 anniversary-n），返回新插入的条数。
	// 依赖 (user_id,badge_id)/(user_id,award_key) 唯一索引保证幂等。
	AwardDueAnniversaries(ctx context.Context, maxYears int, now time.Time) (int64, error)
}
