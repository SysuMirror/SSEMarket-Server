package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// RecommendationRepository 推荐系统持久化接口。
type RecommendationRepository interface {
	// 用户设置
	GetUserMode(ctx context.Context, userID int) (string, error)
	SaveUserMode(ctx context.Context, userID int, mode string) error

	// 灰度规则
	ListGrayRules(ctx context.Context) ([]domain.RecommendationGrayRule, error)
	UpsertGrayRule(ctx context.Context, in domain.RecommendGrayRuleInput) (*domain.RecommendationGrayRule, error)
	DeleteGrayRule(ctx context.Context, id int) error
	MatchGrayRule(ctx context.Context, userID int) (bool, error)

	// 帖子索引与向量
	ListPostMeta(ctx context.Context) ([]domain.RecPostMeta, error)
	ListEmbeddings(ctx context.Context) ([]domain.RecEmbedding, error)
	ListPendingTextPosts(ctx context.Context, limit int) ([]domain.RecPostContent, error)
	ListPendingImagePosts(ctx context.Context, limit int) ([]domain.RecPostContent, error)
	UpsertEmbedding(ctx context.Context, postID int, kind, model string, dims int, contentHash, imageURL string, vector []float32) error
	DeleteAllEmbeddings(ctx context.Context) error
	CountPosts(ctx context.Context) (int64, error)
	CountEmbeddings(ctx context.Context) (text, image int64, err error)
	CountPending(ctx context.Context) (text, image int64, err error)

	// 用户行为
	ListUserBrowses(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserLikes(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserComments(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserSaves(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserCoinThrows(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserReadEvents(ctx context.Context, userID, limit int) ([]domain.RecReadEvent, error)
	ListUserAuthoredPosts(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error)
	ListUserChatMessages(ctx context.Context, userID, botUID, limit int) ([]domain.RecChatMessage, error)
	BrowsedPostIDs(ctx context.Context, userID, limit int) (map[int]struct{}, error)
	GetUserRegisteredAt(ctx context.Context, userID int) (*time.Time, error)
	CreateReadEvent(ctx context.Context, userID, postID, dwellMS int) error
}
