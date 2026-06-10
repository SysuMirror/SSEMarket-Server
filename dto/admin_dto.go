package dto

import "loginTest/domain"

type AdminDto struct {
	Account string `json:"account"`
}

func ToAdminDto(admin domain.Admin) AdminDto {
	return AdminDto{
		Account: admin.Account,
	}
}
