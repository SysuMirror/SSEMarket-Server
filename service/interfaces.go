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
	CheckOAuth2App(ctx context.Context, appID string) (*domain.OAuth2App, error)
	AuthorizeOAuth2App(ctx context.Context, in domain.OAuth2AuthorizeInput) (string, error)
	GetTokenByOAuth2(ctx context.Context, in domain.OAuth2TokenInput) (*domain.OAuth2TokenResult, error)
	ListOAuth2AuthRecords(ctx context.Context, userID, page, pageSize int) (*domain.OAuth2AuthRecordPage, error)
	ValidateEmailExists(ctx context.Context, email string, mode int) error
	UpdateAvatarByPhone(ctx context.Context, phone, avatarURL string) (*domain.User, error)
	GetAvatarByPhone(ctx context.Context, phone string) (*domain.User, error)
	GetInfo(ctx context.Context, phone string, userID int) (*domain.User, error)
	UpdateUserInfo(ctx context.Context, in domain.UpdateUserInput, tokenUserID int) error
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
	ValidatePcommentAccess(ctx context.Context, pcommentID, viewerUserID int) error
	ValidateCcommentAccess(ctx context.Context, ccommentID, viewerUserID int) error
	ResolveMentionUsers(ctx context.Context, names []string) ([]*domain.User, error)
}

// RankService 排行统计服务。
type RankService interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int)
	AdjustPostVoteNum(ctx context.Context, postID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)
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

// ShopService 商城领域服务。
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

// VoteService 投票领域服务。
type VoteService interface {
	SearchByTitle(ctx context.Context, keyword string, viewerUserID int) ([]domain.VoteSearchItem, error)
	ToggleVote(ctx context.Context, userID, postID int) (bool, error)
	HasVoted(ctx context.Context, userID, postID int) (bool, error)
	GetPostRanking(ctx context.Context) ([]domain.PostRankItem, error)
	GetUserRanking(ctx context.Context) ([]domain.UserRankItem, error)
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
