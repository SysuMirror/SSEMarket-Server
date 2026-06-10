package repository

import (
	"context"

	"loginTest/domain"
)

// RankRepository 排行统计持久化接口。
type RankRepository interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int)
	AdjustPostVoteNum(ctx context.Context, postID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)
}
