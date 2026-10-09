package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// UserRepository 用户持久化接口。
type UserRepository interface {
	FindByID(ctx context.Context, id int) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByPhone(ctx context.Context, phone string) (*domain.User, error)
	FindByPhoneOrID(ctx context.Context, phone string, userID int) (*domain.User, error)
	FindAuthByEmail(ctx context.Context, email string) (*domain.User, string, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	Create(ctx context.Context, name, phone, email, hashedPassword string) (*domain.User, error)
	Update(ctx context.Context, u *domain.User) error
	SoftDeleteAccount(ctx context.Context, userID int) error
	UpdateEmailWithHistory(ctx context.Context, userID int, oldEmail, newEmail string) error
	ExistsUsedEmail(ctx context.Context, userID int, email string) (bool, error)
	UpdatePassword(ctx context.Context, userID int, hashedPassword string) error
	UpdateCustomTitle(ctx context.Context, userID int, title string) error
	UpdateAvatarURL(ctx context.Context, userID int, avatarURL string) error
	ToggleEmailNotificationBlocked(ctx context.Context, userID int) error
	UpdateScore(ctx context.Context, userID int, score int) error
	ListAllUsers(ctx context.Context) ([]*domain.User, error)
	SaveRefreshToken(ctx context.Context, userID int, token string, expiresAt int64) error
	FindValidRefreshToken(ctx context.Context, token string, userID int) (*domain.RefreshTokenRecord, error)
	MarkRefreshTokenUsed(ctx context.Context, id int) error
	UseCDKey(ctx context.Context, content string) error

	FindActiveOAuth2App(ctx context.Context, appID string) (*domain.OAuth2App, error)
	FindActiveOAuth2AppByRedirectURI(ctx context.Context, appID, redirectURI string) (*domain.OAuth2App, error)
	FindActiveOAuth2AppBySecret(ctx context.Context, appID, appSecret string) (*domain.OAuth2App, error)
	CreateOAuth2Code(ctx context.Context, code string, appID string, userID int, scope, redirectURI, state string, expiresAt time.Time) error
	CreateOAuth2AuthRecord(ctx context.Context, record *domain.OAuth2AuthRecord) error
	FindUnusedOAuth2Code(ctx context.Context, code, appID string) (*domain.OAuth2Code, error)
	MarkOAuth2CodeUsed(ctx context.Context, codeID int) error
	CreateOAuth2Token(ctx context.Context, accessToken, appID string, userID int, scope string, expiresAt time.Time) error
	CountOAuth2AuthRecords(ctx context.Context, userID int) (int64, error)
	ListOAuth2AuthRecords(ctx context.Context, userID, offset, limit int) ([]domain.OAuth2AuthRecord, error)

	// UserAPIToken PAT 持久化
	CreateUserAPIToken(ctx context.Context, userID int, tokenHash, tokenPrefix, name, scopes string) (int, error)
	FindUserAPITokenByHash(ctx context.Context, tokenHash string) (*domain.UserAPITokenVerify, error)
	TouchUserAPITokenLastUsed(ctx context.Context, tokenID int, lastUsedAt time.Time) error
	CountUserAPITokens(ctx context.Context, userID int) (int64, error)
	ListUserAPITokens(ctx context.Context, userID, offset, limit int) ([]domain.UserAPITokenItem, error)
	RevokeUserAPIToken(ctx context.Context, tokenID, userID int) error
	DeleteUserAPIToken(ctx context.Context, tokenID, userID int) error

	ListPostIDsByUserID(ctx context.Context, userID int) ([]int, error)
	GetPostLikeNum(ctx context.Context, postID int) (int, error)
	ListPcommentIDsByUserID(ctx context.Context, userID int) ([]int, error)
	GetPcommentLikeNum(ctx context.Context, pcommentID int) (int, error)
	CountPsavesByPostIDs(ctx context.Context, postIDs []int) (int64, error)
	CountPcommentsOnPostsExcludingUser(ctx context.Context, postIDs []int, userID int) (int64, error)
	CountCcommentsOnPcommentsExcludingUser(ctx context.Context, pcommentIDs []int, userID int) (int64, error)
	ListCcommentIDsByUserID(ctx context.Context, userID int) ([]int, error)
	GetCcommentLikeNum(ctx context.Context, ccommentID int) (int, error)
	CountSuccessfulSuesByUser(ctx context.Context, userID int) (int64, error)

	ListPostSummariesByUserID(ctx context.Context, userID int) ([]domain.PostSummary, error)
	CountPsavesByUserID(ctx context.Context, userID int) (int64, error)
	ListChatMessagesForUser(ctx context.Context, userID int) ([]domain.ChatMessagePeer, error)

	// ListTopActive 按活跃度（获赞+获票）取 Top N，供缓存预热。
	ListTopActive(ctx context.Context, limit int) ([]*domain.User, error)
}
