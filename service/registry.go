package service

import (
	gormrepo "loginTest/repository/gorm"

	"gorm.io/gorm"
)

// 全局 Service 实例，由 InitServices 注入。
var (
	Posts         PostService
	Users         UserService
	Comments      CommentService
	Ranks         RankService
	Chats         ChatService
	Ratings       RatingService
	Shops         ShopService
	Resumes       ResumeService
	Notices       NoticeService
	Admins        AdminService
	WorkbenchApps WorkbenchAppService
	Sues          SueService
	Votes         VoteService
	Lotteries     LotteryService
	Sso           SsoService
	Quest         QuestService
	Novels        NovelService
	NovelPoints   NovelPointsService
	Coins         CoinService
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
	lotteryRepo := gormrepo.NewLotteryRepository(db)
	rankRepo := gormrepo.NewRankRepository(db)

	Users = NewUserService(userRepo)
	Ranks = NewRankService(rankRepo)
	Posts = NewPostService(postRepo, userRepo, sueRepo, Ranks)
	Comments = NewCommentService(commentRepo, postRepo)
	Chats = NewChatService(chatRepo)
	Ratings = NewRatingService(ratingRepo)
	Shops = NewShopService(shopRepo, userRepo)
	Resumes = NewResumeService(resumeRepo)
	Notices = NewNoticeService(noticeRepo)
	Admins = NewAdminService(adminRepo)
	WorkbenchApps = NewWorkbenchAppService(gormrepo.NewWorkbenchAppRepository(db))
	Sues = NewSueService(sueRepo)
	Votes = NewVoteService(voteRepo, postRepo, Ranks)
	Lotteries = NewLotteryService(lotteryRepo, postRepo)

	ssoBindingRepo := gormrepo.NewSsoBindingRepository(db)
	Sso = NewSsoService(ssoBindingRepo, userRepo, Users)

	questRepo := gormrepo.NewQuestRepository(db)
	Quest = NewQuestService(questRepo, postRepo, userRepo)

	novelRepo := gormrepo.NewNovelRepository(db)
	ledgerRepo := gormrepo.NewLedgerRepository(db)
	NovelPoints = NewNovelPointsService(ledgerRepo, novelRepo)

	// 币体系：投币时要解析被投内容的作者与可见性，所以把帖子库和书本库一起注进来。
	Coins = NewCoinService(gormrepo.NewCoinRepository(db), postRepo, novelRepo)
	Novels = NewNovelService(novelRepo, NovelPoints, Coins)
}
