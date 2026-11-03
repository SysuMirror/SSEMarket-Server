package service

import (
	"context"

	"loginTest/domain"
)

// PostService 帖子领域服务。
type PostService interface {
	CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error)
	Browse(ctx context.Context, filter domain.BrowseFilter) (domain.BrowseResult, error)
	GetDetail(ctx context.Context, postID, viewerUserID int) (*domain.PostDetail, error)
	FindByID(ctx context.Context, postID, viewerUserID int) (*domain.Post, error)
	IncrBrowse(ctx context.Context, postID int) int64
	// Create 创建帖子并返回带 ID 的实体，便于上层（如归档 MQ）取用 post.ID。
	Create(ctx context.Context, in domain.PostCreateInput) (*domain.Post, error)
	// PinPost 给帖子置顶 1 小时(权限校验 + 队列上限 + 同帖不可重复置顶)。
	PinPost(ctx context.Context, postID, userID int) error
	// UnpinPost 取消帖子置顶。
	UnpinPost(ctx context.Context, postID, userID int) error
	// CountActivePinned 当前全站置顶队列长度。
	CountActivePinned(ctx context.Context) (int64, error)
	// ClearExpiredPins 定时清理过期/无效置顶标记。
	ClearExpiredPins(ctx context.Context) (int64, error)
	UserProfilePosts(ctx context.Context, filter domain.UserProfilePostsFilter) ([]domain.BrowseItem, error)
	UserProfilePostNum(ctx context.Context, targetUserID, viewerUserID int) (int64, error)
	UpdatePrivacy(ctx context.Context, postID, userID int, isPrivate bool) error
	ToggleSave(ctx context.Context, userID, postID int) (isSaved bool, err error)
	ToggleWatch(ctx context.Context, userID, postID int) (bool, error)
	ToggleLike(ctx context.Context, userID, postID int) error
	RecordBrowse(ctx context.Context, userID, postID int) error
	Delete(ctx context.Context, postID, userID int) error
	SubmitReport(ctx context.Context, in domain.SubmitReportInput) error
	SubmitFeedback(ctx context.Context, text, attachment string) (*domain.Feedback, error)
	ListAllFeedback(ctx context.Context) ([]*domain.Feedback, error)
	CalculateHeat(ctx context.Context) ([]domain.HeatItem, error)
	ListTags(ctx context.Context) ([]*domain.Tag, error)
}

// UserService 用户领域服务。
type UserService interface {
	GetByID(ctx context.Context, id int) (*domain.User, error)
	Login(ctx context.Context, in domain.LoginInput) (*domain.AuthTokens, error)
	Register(ctx context.Context, in domain.RegisterInput) (*domain.AuthTokens, error)
	DeleteMe(ctx context.Context, in domain.DeleteMeInput) error
	IdentityValidate(ctx context.Context, email, valiCode string) error
	RefreshToken(ctx context.Context, refreshToken string) (*domain.AuthTokens, error)
	ModifyPassword(ctx context.Context, in domain.ModifyPasswordInput) (*domain.User, error)
	ModifyEmail(ctx context.Context, in domain.ModifyEmailInput) (*domain.User, error)
	CheckOAuth2App(ctx context.Context, appID string) (*domain.OAuth2App, error)
	AuthorizeOAuth2App(ctx context.Context, in domain.OAuth2AuthorizeInput) (string, error)
	GetTokenByOAuth2(ctx context.Context, in domain.OAuth2TokenInput) (*domain.OAuth2TokenResult, error)
	ListOAuth2AuthRecords(ctx context.Context, userID, page, pageSize int) (*domain.OAuth2AuthRecordPage, error)
	ValidateEmailExists(ctx context.Context, email string, mode int) error
	UpdateAvatarByPhone(ctx context.Context, phone, avatarURL string) (*domain.User, error)
	GetAvatarByPhone(ctx context.Context, phone string) (*domain.User, error)
	GetInfo(ctx context.Context, phone string, userID int) (*domain.User, error)
	UpdateUserInfo(ctx context.Context, in domain.UpdateUserInput, tokenUserID int) error
	// UpdateProfileBanner 保存本人主页栏板背景（图床绝对地址 + 0-100 不透明度）。
	UpdateProfileBanner(ctx context.Context, userID int, bannerURL string, opacity int) (*domain.User, error)
	SetCustomTitle(ctx context.Context, userID int, title string) error
	ToggleEmailNotification(ctx context.Context, userID int) error
	CalculateAndSaveScores(ctx context.Context)
	GetStatistics(ctx context.Context, userID int) (*domain.UserStatistics, error)

	// UserAPIToken PAT 管理（MCP 等对外调用）
	ListAPITokenScopes(ctx context.Context) []domain.UserAPITokenScope
	CreateAPIToken(ctx context.Context, in domain.UserAPITokenCreateInput) (*domain.UserAPITokenCreateResult, error)
	ListAPITokens(ctx context.Context, userID, page, pageSize int) (*domain.UserAPITokenPage, error)
	RevokeAPIToken(ctx context.Context, tokenID, userID int) error
	DeleteAPIToken(ctx context.Context, tokenID, userID int) error
	VerifyAPIToken(ctx context.Context, plainToken string) (*domain.UserAPITokenVerify, error)
	// IssueTokensForUser 为已存在用户发放令牌（SSO 登录/绑定路径复用，
	// 与邮箱密码登录产物完全一致：JWT + refresh token 落库）。
	IssueTokensForUser(ctx context.Context, userID int) (*domain.AuthTokens, error)
	// PlatformCreateUser 平台中转建号（SM2 验签通过后调用，免验证码/CDKey）。
	PlatformCreateUser(ctx context.Context, in PlatformCreateUserInput) (*domain.User, *domain.AuthTokens, error)
	// MirrorProvision 为已通过 UniSSO 验证的邮箱提供映射账号；已存在则直接发令牌。
	MirrorProvision(ctx context.Context, in MirrorProvisionInput) (*domain.User, *domain.AuthTokens, error)
}

// SsoService UniSSO 单点登录服务。
type SsoService interface {
	// BuildAuthorizeURL 生成 state+PKCE 存 Redis，返回 UniSSO authorize 地址。
	// bindUID>0 表示已登录用户主动绑定；SSO 未启用返回 ErrBadRequest。
	BuildAuthorizeURL(ctx context.Context, bindUID int) (string, error)
	// ConsumeCallback 消费回调：校验 state（GETDEL 一次性）→ 换 token → 取 userinfo
	// → 按绑定关系分派一次性 ticket。错误用 ErrSsoStateInvalid/ErrSsoExchange/ErrSsoConflict 区分。
	ConsumeCallback(ctx context.Context, code, state string) (string, error)
	// ExchangeTicket ticket → 登录令牌（login/bound，一次性）或 bind_needed 信息（可重复读）。
	ExchangeTicket(ctx context.Context, ticket string) (*domain.SsoExchangeResult, error)
	// BindWithCredentials bind_needed ticket + 集市邮箱密码 → 写绑定 + 发令牌（ticket 一次性消费）。
	BindWithCredentials(ctx context.Context, ticket, email, password string) (*domain.AuthTokens, error)
	// GetStatus 查询用户绑定状态（未绑定返回 Bound=false，不报错）。
	GetStatus(ctx context.Context, userID int) (*domain.SsoStatus, error)
	// Unbind 验证密码后删除绑定。
	Unbind(ctx context.Context, userID int, password string) error
}

// StaffService 职位账号（开发人员匿名 @）领域服务。
// 匿名性红线：任何出参都不得携带 domain.StaffAlias.StaffUserID；
// 对前端只给 StaffAliasPublic（alias / title / 职位小号 userID）。
type StaffService interface {
	// ListPublicAliases @ 下拉候选（前端编辑器的 @ 入口用）。
	ListPublicAliases(ctx context.Context) ([]domain.StaffAliasPublic, error)
	// ThreadAliasesForUser 当前用户在该帖子线程内被 @ 到的职位身份
	// （「以职位身份回复」的开关数据源；普通用户恒为空，开销一次索引查询）。
	ThreadAliasesForUser(ctx context.Context, userID, postID int) ([]domain.StaffAliasPublic, error)
	// ShellsForNames 把 @ 解析出的昵称里属于职位的部分映射为职位小号（resolveMentions 渲染用）。
	ShellsForNames(ctx context.Context, names []string) (map[string]*domain.User, error)
	// MatchedAliasesForTokens 正文 @ 解析出的昵称里命中的职位行（小号已确保派生）。
	// 调用方（评论服务）负责据此把通知转发给真实开发者。
	MatchedAliasesForTokens(ctx context.Context, tokens []string) ([]*domain.StaffAlias, error)
	// AuthorizeAliasReply 校验「以职位身份回复」：确为本人职位 + 该线程确实 @ 过这个职位。
	AuthorizeAliasReply(ctx context.Context, userID, postID, asUserID int) (*domain.StaffAlias, error)
	// AliasShellUserIDsForUser 该真实账号名下的职位小号 userID 集合（普通用户恒为空）。
	// 评论列表用它判定「这条对外署名是小号的评论，其实是当前用户发的」，
	// 从而让开发人员能像普通用户一样删除自己的职位身份回复。
	AliasShellUserIDsForUser(ctx context.Context, userID int) (map[int]struct{}, error)
}

// CommentService 评论领域服务。
type CommentService interface {
	ListForPost(ctx context.Context, postID int, postType string, viewerUserID int) ([]*domain.PcommentWithMeta, error)
	DeletePcomment(ctx context.Context, pcommentID int) error
	DeleteCcomment(ctx context.Context, ccommentID int) error
	CreatePcomment(ctx context.Context, in domain.PcommentCreateInput) (*domain.PcommentCreateResult, error)
	CreateCcomment(ctx context.Context, in domain.CcommentCreateInput) (*domain.CcommentCreateResult, error)
	TogglePcommentLike(ctx context.Context, userID, pcommentID int) error
	TogglePcommentDeny(ctx context.Context, userID, pcommentID int) error
	ToggleCcommentLike(ctx context.Context, userID, ccommentID int) error
	ToggleCcommentDeny(ctx context.Context, userID, ccommentID int) error
	FindUserByPhone(ctx context.Context, phone string) (*domain.User, error)
	FindPcomment(ctx context.Context, pcommentID int) (*domain.Pcomment, error)
	FindCcomment(ctx context.Context, ccommentID int) (*domain.Ccomment, error)
	// XqAtThreadDigest @小秋楼中楼上下文摘要（见 commentService 实现）
	XqAtThreadDigest(ctx context.Context, pcommentID, markCcommentID, maxFloors, perFloorRunes int) (string, error)
	ValidatePcommentAccess(ctx context.Context, pcommentID, viewerUserID int) error
	ValidateCcommentAccess(ctx context.Context, ccommentID, viewerUserID int) error
	ResolveMentionUsers(ctx context.Context, names []string) ([]*domain.User, error)
}

// RankService 排行统计服务。
type RankService interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)
	PostCoinRanking(ctx context.Context, limit int) ([]domain.PostCoinRankItem, error)
	UserCoinRanking(ctx context.Context, limit int) ([]domain.UserCoinRankItem, error)
}

// ChatService 聊天领域服务。
type ChatService interface {
	GetHistory(ctx context.Context, senderUserID, targetUserID int, threadAnonymous bool) ([]*domain.ChatHistoryItem, error)
	SaveMessage(ctx context.Context, chatMsgID, senderUserID, targetUserID int, content string, isAnonymous, threadAnonymous bool) (*domain.ChatHistoryItem, error)
	RecallMessage(ctx context.Context, userID, chatMsgID int) (*domain.ChatHistoryItem, error)
	ListRelevantPeers(ctx context.Context, userID int) ([]*domain.ChatPeerSummary, error)
	FindUserByID(ctx context.Context, userID int) (*domain.User, error)
}

// RatingService 评分专区领域服务。
type RatingService interface {
	ListCategories(ctx context.Context) ([]*domain.RatingCategory, error)
	CreateCategory(ctx context.Context, in domain.RatingCategoryCreateInput) (*domain.RatingCategory, error)
	ListPosts(ctx context.Context, filter domain.RatingBrowseFilter) ([]*domain.RatingPost, error)
	CountPosts(ctx context.Context, filter domain.RatingBrowseFilter) (int64, error)
	CreatePost(ctx context.Context, in domain.RatingPostCreateInput) (int, error)
	GetPostDetail(ctx context.Context, postID, viewerID int) (*domain.RatingPost, int, error)
	DeletePost(ctx context.Context, postID, userID int) error
	TogglePostLike(ctx context.Context, postID, userID int) error
	SubmitStar(ctx context.Context, postID, userID, starNum int) error
	ListComments(ctx context.Context, postID, viewerID int) ([]*domain.RatingComment, map[int]*domain.User, map[int]bool, error)
	CreateComment(ctx context.Context, in domain.RatingCommentCreateInput) (*domain.RatingComment, *domain.User, error)
	ToggleCommentLike(ctx context.Context, commentID, userID int) error
	EnrichPosts(ctx context.Context, posts []*domain.RatingPost, viewerID int) (
		map[int]*domain.RatingCategory,
		map[int]*domain.User,
		map[int]struct{},
		map[int]domain.RatingStarSummary,
		error,
	)
	FindUser(ctx context.Context, id int) (*domain.User, error)
}

// NovelService 小说板块领域服务。
type NovelService interface {
	ListNovels(ctx context.Context, filter domain.NovelBrowseFilter) ([]*domain.Novel, error)
	CountNovels(ctx context.Context, filter domain.NovelBrowseFilter) (int64, error)
	Categories(ctx context.Context) ([]domain.NovelCategory, error)
	GetNovelDetail(ctx context.Context, novelID, viewerID int) (*domain.Novel, *domain.User, bool, *domain.NovelProgress, error)
	EnrichNovels(ctx context.Context, novels []*domain.Novel, viewerID int) (map[int]*domain.User, map[int]bool, map[int]*domain.NovelProgress, error)
	CreateNovel(ctx context.Context, in domain.NovelCreateInput) (int, error)
	UpdateNovel(ctx context.Context, in domain.NovelUpdateInput) error
	DeleteNovel(ctx context.Context, novelID, userID int) error

	ListChapters(ctx context.Context, novelID, limit, offset int) ([]*domain.NovelChapter, int64, error)
	GetChapterDetail(ctx context.Context, chapterID, viewerID int) (*domain.NovelChapterDetail, error)
	CreateChapter(ctx context.Context, in domain.NovelChapterCreateInput) (*domain.NovelChapter, error)
	DeleteChapter(ctx context.Context, chapterID, userID int) error

	ListComments(ctx context.Context, filter domain.NovelCommentFilter, viewerID int) ([]*domain.NovelComment, map[int]*domain.User, map[int]bool, int64, error)
	CreateComment(ctx context.Context, in domain.NovelCommentCreateInput) (*domain.NovelComment, *domain.User, error)
	DeleteComment(ctx context.Context, commentID, userID int) error
	ToggleCommentLike(ctx context.Context, commentID, userID int) error

	ToggleShelf(ctx context.Context, novelID, userID int) (bool, error)
	IsOnShelf(ctx context.Context, novelID, userID int) (bool, error)
	ListShelf(ctx context.Context, userID, limit, offset int) ([]*domain.Novel, int64, error)
	ReportProgress(ctx context.Context, in domain.NovelProgressInput) error
	GetProgress(ctx context.Context, novelID, userID int) (*domain.NovelProgress, error)

	// RateNovel 给书评分（1–5 星，每人每本一条，可改）；作者首次收到这条评分时 +1 币。
	RateNovel(ctx context.Context, novelID, userID, score int) (*domain.NovelRatingResult, error)
	// RatingDistribution 各星级人数：下标 0 = 5 星，依次到下标 4 = 1 星。
	RatingDistribution(ctx context.Context, novelID int) ([5]int, error)
	// MyRating 我给这本书打了几分（0 = 没打过）。
	MyRating(ctx context.Context, novelID, userID int) (int, error)
	// MyRatings 我给这批书各打了几分（列表页按钮态用）。
	MyRatings(ctx context.Context, userID int, novelIDs []int) (map[int]int, error)
	// MyNovelStats 我对这批书各打了几分、各投了几个币（列表/详情按钮态用）。
	MyNovelStats(ctx context.Context, novelIDs []int, viewerID int) (*domain.NovelViewerStats, error)
}

// NovelPointsService 小说积分与经验体系（流水见 novel_ledger）。
type NovelPointsService interface {
	// 读者侧
	SettleReadTime(ctx context.Context, userID, novelID, seconds int) (int, error)
	GrantCommentExp(ctx context.Context, userID, novelID, commentID int) (int, error)
	GrantCommentLikeExp(ctx context.Context, commentAuthorID, novelID, commentID, likerID int) (int, error)
	CheckIn(ctx context.Context, userID int) (*domain.NovelCheckInResult, error)

	// 作者侧
	GrantShelfAdd(ctx context.Context, authorID, novelID, readerID int) (int, error)
	GrantReaderUV(ctx context.Context, authorID, novelID, readerID int) (int, error)
	SettleWritingWords(ctx context.Context, authorID, novelID, words int) (int, error)
	GrantChapterPublish(ctx context.Context, authorID, novelID, chapterID int) (int, error)

	// 查询
	Summary(ctx context.Context, userID int) (*domain.NovelPointsSummary, error)
	Ledger(ctx context.Context, userID, limit, offset int) ([]*domain.NovelLedger, int64, error)

	// ReadSecondsFor 某读者在某本书上累计的阅读秒数（阅读里程碑游标）；没读过返回 0。
	ReadSecondsFor(ctx context.Context, userID, novelID int) (int, error)
}

// ShopService 商城领域服务。
// CoinService 站内币体系（与上面的经验/积分互不相通，各记各的流水）。
//
// 赚币的四条规则都在服务端触发，调用方不需要判断「今天是不是第一次」——
// 幂等键由 domain/coin.go 统一拼，重复调用只会有一条流水。
type CoinService interface {
	// 赚币
	GrantDailyVisit(ctx context.Context, userID int) (int, error)
	GrantFirstPost(ctx context.Context, userID int) (int, error)
	AddReadSeconds(ctx context.Context, userID, seconds int) (int, error)
	AddWriteWords(ctx context.Context, authorID, words int) (int, error)
	GrantRated(ctx context.Context, authorID, novelID, raterID int) (int, error)

	// 投币：每人每个内容最多 2 币，每个币给投币者 +10 主页经验
	Throw(ctx context.Context, userID int, targetType string, targetID, coins int) (*domain.CoinThrowResult, error)
	// CancelThrow 取消投币：撤回我在该内容上投着的全部币，退币并回退经验
	CancelThrow(ctx context.Context, userID int, targetType string, targetID int) (*domain.CoinThrowResult, error)

	// 查询
	Summary(ctx context.Context, userID int) (*domain.CoinSummary, error)
	Ledger(ctx context.Context, userID, limit, offset int) ([]*domain.CoinLedger, int64, error)
	MyThrows(ctx context.Context, userID int, targetType string, targetIDs []int) (map[int]int, error)
	ThrownForTarget(ctx context.Context, targetType string, targetID int) (int, error)
	// ThrownTotalsAll 全站投币总数（主页经验重算用，一次查询）。
	ThrownTotalsAll(ctx context.Context) (map[int]int, error)
}

type ShopService interface {
	Create(ctx context.Context, in domain.ProductCreateInput) (int, error)
	List(ctx context.Context, filter domain.ProductFilter) ([]*domain.Product, error)
	Count(ctx context.Context, filter domain.ProductFilter) (int64, error)
	GetByID(ctx context.Context, id int) (*domain.Product, error)
	GetRawByID(ctx context.Context, id int) (*domain.Product, error)
	Delete(ctx context.Context, id, userID int) error
	MarkSold(ctx context.Context, id, userID int) error
	CarouselPhotos(ctx context.Context) ([]string, error)
	SellerInfo(ctx context.Context, userID int) (*domain.User, error)
}

// ResumeService 简历领域服务。
type ResumeService interface {
	CanUpload(ctx context.Context, userID int) error
	Create(ctx context.Context, in domain.ResumeCreateInput) (int, error)
	List(ctx context.Context, filter domain.ResumeFilter) ([]*domain.Resume, error)
	GetByID(ctx context.Context, id int) (*domain.Resume, error)
	Detail(ctx context.Context, id int) (*domain.Resume, error)
	RecordDownload(ctx context.Context, id int) error
	Update(ctx context.Context, in domain.ResumeUpdateInput) (*domain.Resume, error)
	Delete(ctx context.Context, id, userID int) (string, error)
	UploadersByIDs(ctx context.Context, ids []int) (map[int]*domain.User, error)
}

// NoticeService 通知领域服务。
type NoticeService interface {
	List(ctx context.Context, receiverID, read, requireID, pageSize int, receiverName string) (*domain.NoticePage, error)
	GetCounts(ctx context.Context, receiverID int) (*domain.NoticeCounts, error)
	MarkRead(ctx context.Context, noticeID int) error
}

// AdminService 管理端领域服务。
type AdminService interface {
	GetByID(ctx context.Context, id int) (*domain.Admin, error)
	Login(ctx context.Context, account, encryptedPassword string) (string, error)
	AddAdmin(ctx context.Context, account, pass1, pass2 string) (*domain.Admin, error)
	ChangePassword(ctx context.Context, account, pass1, pass2 string) (*domain.Admin, error)
	DeleteAdmin(ctx context.Context, account string) error
	FilterUsers(ctx context.Context, filter domain.AdminUserFilter) ([]domain.AdminUserItem, error)
	DeleteUser(ctx context.Context, name string) error
	CreatePost(ctx context.Context, in domain.AdminPostInput) error
}

// SueService 举报领域服务。
type SueService interface {
	ListPending(ctx context.Context) ([]domain.SueListItem, error)
	MarkNoViolation(ctx context.Context, sueID int) error
	MarkViolation(ctx context.Context, sueID int) error
}

// LotteryService 抽奖帖领域服务。
type LotteryService interface {
	Create(ctx context.Context, in domain.LotteryCreateInput) (int, error)
	GetDetail(ctx context.Context, postID, viewerUserID int) (*domain.LotteryPost, error)
	ParticipateButton(ctx context.Context, userID, postID int) error
	ParticipateByComment(ctx context.Context, in domain.LotteryParticipateInput) error
	HasParticipated(ctx context.Context, userID, postID int) (bool, error)
	Draw(ctx context.Context, postID int) error
	AutoDrawDue(ctx context.Context)
	IsLotteryPost(ctx context.Context, postID int) (bool, error)
}

// PresenceService 在线状态：站点与按内容聚合的实时在线人数。
// 实现基于 Redis ZSET 滑动窗口，不存在于数据库。
type PresenceService interface {
	// Heartbeat 记录一次用户心跳（站点 + 可选场景/内容），返回最新人数。
	Heartbeat(ctx context.Context, userID int, scene string, targetID int) domain.PresenceSnapshot
	// Snapshot 只读取当前人数，不写入活跃记录。
	Snapshot(ctx context.Context, scene string, targetID int) domain.PresenceSnapshot
}

// UserSearchService 搜索页「用户卡片」：按用户名/简介匹配，并在相关度超过阈值时展示。
type UserSearchService interface {
	// Search 返回最多 limit 个用户卡片；viewerUserID 用于判断「隐藏历史发帖」
	// 是否对当前浏览者生效（本人仍可看到自己的帖子）。
	Search(ctx context.Context, keyword string, limit, viewerUserID int) ([]domain.UserSearchResult, error)
}

// UserSkinService 用户自定义皮肤：上传、皮肤库、公开搜索、分享链接添加。
type UserSkinService interface {
	Create(ctx context.Context, ownerID int, in domain.UserSkinInput) (*domain.UserSkin, error)
	ListLibrary(ctx context.Context, userID int) ([]domain.UserSkin, error)
	SearchPublic(ctx context.Context, userID int, keyword string, limit int) ([]domain.UserSkin, error)
	AddToLibrary(ctx context.Context, userID, skinID int) (*domain.UserSkin, error)
	AddByShareToken(ctx context.Context, userID int, token string) (*domain.UserSkin, error)
	RemoveFromLibrary(ctx context.Context, userID, skinID int) error
	SetPublic(ctx context.Context, userID, skinID int, public bool) (*domain.UserSkin, error)
	Delete(ctx context.Context, userID, skinID int) error
	ResolveShareToken(ctx context.Context, token string) (*domain.UserSkin, error)
	GetActiveForUser(ctx context.Context, userID, skinID int) (*domain.UserSkin, error)
}

// BadgeService 勋章系统：用户已获得的勋章、主页展示范围、对外挂载。
// 勋章目录由管理员直接 INSERT badges 维护，发放在批量 SQL 脚本里完成。
type BadgeService interface {
	// GetMyBadges 当前用户全部已获得勋章 + 展示偏好。
	GetMyBadges(ctx context.Context, userID int) (*domain.UserBadgesView, error)
	// GetPublicBadges 他人主页可见的勋章（按用户的主页展示偏好过滤）。
	GetPublicBadges(ctx context.Context, userID int) (*domain.PublicUserBadges, error)
	// SavePrefs 保存对外挂载（0=不挂）与主页展示勋章列表（nil=全部）。
	SavePrefs(ctx context.Context, userID int, in domain.BadgePrefsInput) (*domain.UserBadgesView, error)
	// WornBadges 批量查询一批用户对外挂载的勋章，供帖子/评论昵称旁展示。
	WornBadges(ctx context.Context, userIDs []int) (map[int]domain.WornBadge, error)
	// AwardDueAnniversaries 扫描 registered_at，给今天/此前已到期的用户补发周年勋章。
	// 每天由 cron 调用；也可手动触发，返回新插入条数。
	AwardDueAnniversaries(ctx context.Context) (int64, error)
}

// ThemeService 全站皮肤/外观设置。
// 活动时间窗由 domain 的静态注册表计算，本服务只负责用户选择的读写与校验。
type ThemeService interface {
	// GetUserSettings 读取某用户的外观设置；没有保存过时返回默认值。
	GetUserSettings(ctx context.Context, userID int) (*domain.ThemeSettings, error)
	// SaveUserSettings 校验并保存外观设置（theme_mode / 开场画面 / 背景）。
	SaveUserSettings(ctx context.Context, userID int, in domain.ThemeSettings) (*domain.ThemeSettings, error)
	// ListThemeEvents 读取 theme_events 表里所有启用的活动时间窗。
	ListThemeEvents(ctx context.Context) ([]domain.ThemeEvent, error)
}
