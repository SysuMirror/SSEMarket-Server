package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/util"
)

// 管理台 UniSSO 准入错误。controller 映射为 403 并按 msg 区分提示。
var (
	ErrAdminNotInternal    = errors.New("unisso identity is not internal")
	ErrAdminNotWhitelisted = errors.New("unisso identity is not in admin whitelist")
	ErrAdminInactive       = errors.New("admin whitelist entry disabled")
	ErrAdminLastActive     = errors.New("cannot remove the last active admin")
)

// LoginWithSso 管理台主登录：unisso 邮箱密码校验 → 学院成员与白名单准入 → 管理台 JWT。
func (s *adminService) LoginWithSso(ctx context.Context, email, encryptedPassword string) (*domain.Admin, string, *domain.SsoSession, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	password := util.Decrypt(encryptedPassword)
	if email == "" || len(password) < 6 {
		return nil, "", nil, domain.ErrBadRequest
	}
	u, sess, err := s.verifyPasswordIdentity(ctx, email, password)
	if err != nil {
		return nil, "", nil, err
	}
	admin, err := s.admitAdmin(ctx, u, email)
	if err != nil {
		return nil, "", nil, err
	}
	token, err := common.ReleaseToken_admin(*admin)
	if err != nil {
		return nil, "", nil, domain.ErrInternal
	}
	return admin, token, sess, nil
}

// LoginWithSession 管理台静默登录：共享 Cookie 会话 → unisso 校验 → 白名单准入 → 管理台 JWT。
func (s *adminService) LoginWithSession(ctx context.Context, session string) (*domain.Admin, string, error) {
	if strings.TrimSpace(session) == "" {
		return nil, "", ErrSsoInvalidSession
	}
	u, _, err := s.verifySessionIdentity(ctx, session)
	if err != nil {
		return nil, "", err
	}
	admin, err := s.admitAdmin(ctx, u, u.Email)
	if err != nil {
		return nil, "", err
	}
	token, err := common.ReleaseToken_admin(*admin)
	if err != nil {
		return nil, "", domain.ErrInternal
	}
	return admin, token, nil
}

// admitAdmin 管理台准入：必须是 unisso 学院成员（internal），且已在白名单内并处于启用状态。
// 绑定关系缺失时按邮箱匹配并回填 sso_sub，后续登录走 sub 精确匹配。
func (s *adminService) admitAdmin(ctx context.Context, u *domain.SsoPasswordVerifyUser, fallbackEmail string) (*domain.Admin, error) {
	if u == nil || u.Sub == "" {
		return nil, ErrSsoInvalidSession
	}
	if !bool(u.Internal) {
		return nil, ErrAdminNotInternal
	}
	admin, err := s.admins.FindBySsoSub(ctx, u.Sub)
	if errors.Is(err, domain.ErrNotFound) {
		admin, err = s.admins.FindByEmail(ctx, fallbackEmail)
	}
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrAdminNotWhitelisted
		}
		return nil, err
	}
	if !admin.IsActive {
		return nil, ErrAdminInactive
	}

	name := strings.TrimSpace(u.Name)
	if name == "" {
		name = strings.TrimSpace(u.PreferredUsername)
	}
	email := strings.ToLower(strings.TrimSpace(fallbackEmail))
	now := time.Now()
	if err := s.admins.UpdateLogin(ctx, admin.ID, name, email, u.Sub, now); err != nil {
		return nil, err
	}
	admin.Name = name
	admin.Email = email
	admin.SsoSub = u.Sub
	admin.IsActive = true
	admin.LastLoginAt = now
	return admin, nil
}

// ListAdmins 返回管理台白名单（不含密码）。历史本地密码账号没有统一身份认证邮箱，不再展示。
func (s *adminService) ListAdmins(ctx context.Context) ([]domain.Admin, error) {
	items, err := s.admins.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Admin, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Email) == "" && strings.TrimSpace(item.SsoSub) == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// AddAdminByEmail 按邮箱加入白名单。对方须是 unisso 学院成员，首次登录时回填 sso_sub。
func (s *adminService) AddAdminByEmail(ctx context.Context, email, name string) (*domain.Admin, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") || len(email) > 128 {
		return nil, domain.ErrBadRequest
	}
	if _, err := s.admins.FindByEmail(ctx, email); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	admin := &domain.Admin{
		Account:   email,
		Email:     email,
		Name:      strings.TrimSpace(name),
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	if err := s.admins.Create(ctx, admin); err != nil {
		return nil, err
	}
	return admin, nil
}

// SetAdminActive 启用/停用白名单条目。停用最后一个启用中的管理员会被拒绝。
func (s *adminService) SetAdminActive(ctx context.Context, id int, active bool) error {
	admin, err := s.admins.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !active && admin.IsActive {
		n, err := s.admins.CountActive(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrAdminLastActive
		}
	}
	return s.admins.SetActive(ctx, id, active)
}

// DeleteAdminByID 删除白名单条目。最后一个启用中的管理员不可删除。
func (s *adminService) DeleteAdminByID(ctx context.Context, id int) error {
	admin, err := s.admins.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if admin.IsActive {
		n, err := s.admins.CountActive(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrAdminLastActive
		}
	}
	return s.admins.DeleteByID(ctx, id)
}

func (s *adminService) verifyPasswordIdentity(ctx context.Context, email, password string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	if Sso == nil {
		return nil, nil, ErrSsoUnavailable
	}
	return Sso.VerifyPasswordIdentity(ctx, email, password)
}

func (s *adminService) verifySessionIdentity(ctx context.Context, session string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	if Sso == nil {
		return nil, nil, ErrSsoUnavailable
	}
	return Sso.VerifySessionIdentity(ctx, session)
}
