package repository

import (
	"context"

	"loginTest/domain"
)

// RankRepository 排行统计持久化接口。
//
// 投票功能已下线：AdjustPostVoteNum / AdjustUserReceivedVote 随之删除，
// posts.vote_num 与 users.received_vote_num 只作为历史数据保留，不再有写入路径。
// 两个榜单（帖子榜 / 作者榜）现在都按投币数排。
type RankRepository interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)

	// PostCoinRanking 帖子投币榜：投币数 = 真实投币数 + 历史投票数。
	PostCoinRanking(ctx context.Context, limit int) ([]domain.PostCoinRankItem, error)

	// UserCoinRanking 作者投币榜：按「自己帖子 + 小说收到的币数」排名。
	UserCoinRanking(ctx context.Context, limit int) ([]domain.UserCoinRankItem, error)
}
