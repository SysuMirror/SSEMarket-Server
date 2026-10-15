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
	Lotteries     LotteryService
	Sso           SsoService
	Quest         QuestService
	Novels        NovelService
	NovelPoints   NovelPointsService
	Coins         CoinService
	Staffs        StaffService
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
	lotteryRepo := gormrepo.NewLotteryRepository(db)
	rankRepo := gormrepo.NewRankRepository(db)
	novelRepo := gormrepo.NewNovelRepository(db)

	// 币体系要先建：投币时要解析被投内容的作者与可见性，所以帖子库和书本库一起注进来；
	// 发帖奖励与主页经验重算也都依赖它，Users / Posts 必须排在后面。
	Coins = NewCoinService(gormrepo.NewCoinRepository(db), postRepo, novelRepo)

	Users = NewUserService(userRepo, Coins)
	Ranks = NewRankService(rankRepo)
	Posts = NewPostService(postRepo, userRepo, sueRepo, Ranks, Coins)
	// 职位账号先建：评论服务要靠它把 @职位 的通知转发给真实开发者、
	// 并校验「以职位身份回复」（Staffs 自身只依赖 staffRepo，无环）。
	Staffs = NewStaffService(gormrepo.NewStaffRepository(db))
	Comments = NewCommentService(commentRepo, postRepo, Staffs)
	Chats = NewChatService(chatRepo)
	Ratings = NewRatingService(ratingRepo)
	Shops = NewShopService(shopRepo, userRepo)
	Resumes = NewResumeService(resumeRepo, userRepo, Posts)
	Notices = NewNoticeService(noticeRepo)
	Admins = NewAdminService(adminRepo)
	WorkbenchApps = NewWorkbenchAppService(gormrepo.NewWorkbenchAppRepository(db))
	Sues = NewSueService(sueRepo)
	Lotteries = NewLotteryService(lotteryRepo, postRepo)

	ssoBindingRepo := gormrepo.NewSsoBindingRepository(db)
	Sso = NewSsoService(ssoBindingRepo, userRepo, Users)

	questRepo := gormrepo.NewQuestRepository(db)
	Quest = NewQuestService(questRepo, postRepo, userRepo)

	ledgerRepo := gormrepo.NewLedgerRepository(db)
	NovelPoints = NewNovelPointsService(ledgerRepo, novelRepo)
	Novels = NewNovelService(novelRepo, NovelPoints, Coins)
}
