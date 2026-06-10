package service

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

type userService struct {
	users repository.UserRepository
}

func NewUserService(users repository.UserRepository) UserService {
	return &userService{users: users}
}

func (s *userService) GetByID(ctx context.Context, id int) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}
