package service

import (
	"context"

	"loginTest/domain"
	"loginTest/model"

	"gorm.io/gorm"
)

type rankService struct {
	db *gorm.DB
}

func NewRankService(db *gorm.DB) RankService {
	return &rankService{db: db}
}

func (s *rankService) AdjustUserReceivedLike(_ context.Context, authorUserID, delta int) {
	AdjustUserReceivedLike(s.db, authorUserID, delta)
}

func (s *rankService) AdjustUserReceivedVote(_ context.Context, authorUserID, delta int) {
	AdjustUserReceivedVote(s.db, authorUserID, delta)
}

func (s *rankService) AdjustPostVoteNum(_ context.Context, postID, delta int) {
	AdjustPostVoteNum(s.db, postID, delta)
}

func (s *rankService) OnPostDeleted(_ context.Context, post *domain.Post) {
	if post == nil {
		return
	}
	OnPostDeleted(s.db, &model.Post{
		PostID:  post.ID,
		UserID:  post.UserID,
		LikeNum: post.LikeNum,
	})
}

func (s *rankService) RebuildRankStats(_ context.Context) {
	RebuildRankStats(s.db)
}
