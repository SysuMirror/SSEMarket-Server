package dto

import "loginTest/domain"

type UserDto struct {
	UserID    int    `json:"userID"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Identity  string `json:"identity"`
}

func ToUserDto(user domain.User) UserDto {
	return UserDto{
		UserID:    user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		AvatarURL: user.AvatarURL,
		Email:     user.Email,
		Identity:  user.Identity,
	}
}
