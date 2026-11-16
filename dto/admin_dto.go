package dto

import "loginTest/domain"

type AdminDto struct {
	ID          int    `json:"id"`
	Account     string `json:"account"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	SsoSub      string `json:"sso_sub"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at,omitempty"`
	LastLoginAt string `json:"last_login_at,omitempty"`
}

func ToAdminDto(admin domain.Admin) AdminDto {
	out := AdminDto{
		ID:       admin.ID,
		Account:  admin.Account,
		Email:    admin.Email,
		Name:     admin.Name,
		SsoSub:   admin.SsoSub,
		IsActive: admin.IsActive,
	}
	if !admin.CreatedAt.IsZero() {
		out.CreatedAt = admin.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !admin.LastLoginAt.IsZero() {
		out.LastLoginAt = admin.LastLoginAt.Format("2006-01-02 15:04:05")
	}
	return out
}

func ToAdminListDto(items []domain.Admin) []AdminDto {
	out := make([]AdminDto, 0, len(items))
	for _, item := range items {
		out = append(out, ToAdminDto(item))
	}
	return out
}
