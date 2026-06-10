package service

import (
	gormrepo "loginTest/repository/gorm"

	"gorm.io/gorm"
)

// 全局 Service 实例，由 InitServices 注入。
var (
	Posts    PostService
	Users    UserService
	Comments CommentService
	Ranks    RankService
	Chats    ChatService
)

// InitServices 组装 Repository → Service 依赖链。
func InitServices(db *gorm.DB) {
	postRepo := gormrepo.NewPostRepository(db)
	userRepo := gormrepo.NewUserRepository(db)
	commentRepo := gormrepo.NewCommentRepository()
	chatRepo := gormrepo.NewChatRepository()

	Posts = NewPostService(postRepo)
	Users = NewUserService(userRepo)
	Comments = NewCommentService(commentRepo)
	Ranks = NewRankService(db)
	Chats = NewChatService(chatRepo)
}
