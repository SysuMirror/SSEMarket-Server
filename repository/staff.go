package repository

import (
	"context"

	"loginTest/domain"
)

// StaffRepository 职位账号（开发人员匿名 @）持久化接口。
type StaffRepository interface {
	// ---- 职位行 ----
	ListAliases(ctx context.Context) ([]*domain.StaffAlias, error)
	FindAliasesByNames(ctx context.Context, names []string) ([]*domain.StaffAlias, error)
	FindAliasesByStaffUserID(ctx context.Context, staffUserID int) ([]*domain.StaffAlias, error)
	FindAliasesByAliasUserIDs(ctx context.Context, aliasUserIDs []int) ([]*domain.StaffAlias, error)
	UpdateAliasUserID(ctx context.Context, aliasID, aliasUserID int) error

	// ---- 职位小号（users 表派生行）----
	// FindShellUserByEmail 只按职位小号固定邮箱查（staff-<id>@staff.internal）。
	FindShellUserByEmail(ctx context.Context, email string) (*domain.User, error)
	// UsersByNameExcludeEmail 查同名用户（排除职位小号邮箱），用于 alias 与普通昵称冲突检测。
	UsersByNameExcludeEmail(ctx context.Context, name, excludeEmail string) (int64, error)
	// CreateShellUser 建职位小号（满级 + 职称 + 不可登录的随机口令），返回 userID。
	CreateShellUser(ctx context.Context, aliasID int, alias, title string) (int, error)
	// UpdateShellUserProfile 同步小号的昵称 / 称号（管理员改了 alias/title 行之后）。
	UpdateShellUserProfile(ctx context.Context, userID int, alias, title string) error

	// ---- 线程正文扫描（「以职位身份回复」的资格判定用）----
	FindPostContentByID(ctx context.Context, postID int) (string, error)
	ListPcommentContentsByPostID(ctx context.Context, postID int) ([]string, error)
	ListCcommentContentsByPostID(ctx context.Context, postID int) ([]string, error)
}
