package domain

import (
	"errors"
	"time"
)

// 勋章类别（仅用于前端分组与排序，不参与权限判断）。
const (
	BadgeCategoryAnniversary = "anniversary" // 注册周年
	BadgeCategoryFestival    = "festival"    // 节日限定
	BadgeCategorySpecial     = "special"     // 其它特殊成就
)

const (
	// BadgeMaxWornQuery 批量查询「对外挂载」时一次最多接受多少个用户。
	BadgeMaxWornQuery = 100
	// BadgeMaxProfileSelection 个人主页最多同时展示多少枚勋章。
	BadgeMaxProfileSelection = 30
)

var (
	ErrBadgeNotOwned    = errors.New("只能展示自己已获得的勋章")
	ErrBadgePrefsTooBig = errors.New("最多选择 30 枚勋章")
)

// Badge 勋章目录实体。
type Badge struct {
	ID          int
	Code        string
	Name        string
	Description string
	ImageURL    string
	Category    string
	SortOrder   int
	AwardedAt   time.Time
}

// AwardedBadge 用户已获得的一枚勋章（目录信息 + 发放时间）。
type AwardedBadge struct {
	Badge
	Source string
}

// UserBadgePrefs 展示偏好。
//
// ProfileBadgeIDs 为 nil 表示「默认全部展示」；空切片表示「明确不展示」。
type UserBadgePrefs struct {
	UserID          int
	WornBadgeID     int
	ProfileBadgeIDs []int
	UpdatedAt       time.Time
}

// UserBadgesView 我的勋章：全部已获得 + 当前展示偏好。
type UserBadgesView struct {
	UserID          int
	Badges          []AwardedBadge
	WornBadgeID     int
	ProfileBadgeIDs []int
}

// PublicUserBadges 他人主页可见的勋章：只返回主页展示范围内的勋章。
type PublicUserBadges struct {
	UserID          int
	Badges          []AwardedBadge
	WornBadgeID     int
	ProfileBadgeIDs []int
}

// WornBadge 对外挂载的勋章（帖子/评论作者昵称旁）。
type WornBadge struct {
	UserID   int
	BadgeID  int
	Name     string
	ImageURL string
}

// BadgePrefsInput 保存展示偏好的入参。
//
// ProfileBadgeIDs 为 nil 表示恢复「默认全部展示」，空切片表示不展示任何勋章。
type BadgePrefsInput struct {
	WornBadgeID     int
	ProfileBadgeIDs []int
}

// FilterBadgesByProfileSelection 按主页展示偏好过滤勋章列表，保持原有顺序。
//
//   - selected == nil：默认全部展示；
//   - selected 为空：不展示；
//   - 其它：只保留选中的 badge ID（忽略已失效/未拥有 ID）。
func FilterBadgesByProfileSelection(badges []AwardedBadge, selected []int) []AwardedBadge {
	if selected == nil {
		return badges
	}
	keep := make(map[int]struct{}, len(selected))
	for _, id := range selected {
		keep[id] = struct{}{}
	}
	filtered := make([]AwardedBadge, 0, len(badges))
	for _, b := range badges {
		if _, ok := keep[b.ID]; ok {
			filtered = append(filtered, b)
		}
	}
	return filtered
}
