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
	Ratings  RatingService
	Shops    ShopService
	Resumes  ResumeService
	Notices  NoticeService
	Admins   AdminService
	Sues     SueService
	Votes    VoteService
)

// InitServices 组装 Repository → Service 依赖链。
func InitServices(db *gorm.DB) {
	postRepo := gormrepo.NewPostRepository(db)
	userRepo := gormrepo.NewUserRepository(db)
	commentRepo := gormrepo.NewCommentRepository(db)
	chatRepo := gormrepo.NewChatRepository(db)
	ratingRepo := gormrepo.NewRatingRepository(db)
	shopRepo := gormrepo.NewShopRepository(db)
	resumeRepo := gormrepo.NewResumeRepository(db)
	noticeRepo := gormrepo.NewNoticeRepository(db)
	adminRepo := gormrepo.NewAdminRepository(db)
	sueRepo := gormrepo.NewSueRepository(db)
	voteRepo := gormrepo.NewVoteRepository(db)
	rankRepo := gormrepo.NewRankRepository(db)

	Users = NewUserService(userRepo)
	Ranks = NewRankService(rankRepo)
	Posts = NewPostService(postRepo, userRepo, sueRepo, Ranks)
	Comments = NewCommentService(commentRepo)
	Chats = NewChatService(chatRepo)
	Ratings = NewRatingService(ratingRepo)
	Shops = NewShopService(shopRepo, userRepo)
	Resumes = NewResumeService(resumeRepo)
	Notices = NewNoticeService(noticeRepo)
	Admins = NewAdminService(adminRepo)
	Sues = NewSueService(sueRepo)
	Votes = NewVoteService(voteRepo, postRepo, Ranks)
}
