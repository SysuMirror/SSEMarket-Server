package gormrepo

import (
	"context"
	"errors"
	"sort"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

const (
	userRankPenaltyID = 278
	userRankPenalty   = 100
)

type voteRepository struct {
	db *gorm.DB
}

func NewVoteRepository(db *gorm.DB) repository.VoteRepository {
	return &voteRepository{db: db}
}

func (r *voteRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *voteRepository) SearchByTitle(ctx context.Context, keyword string, viewerUserID int) ([]domain.VoteSearchItem, error) {
	if keyword == "" {
		return nil, nil
	}
	var posts []model.Post
	err := r.session(ctx).
		Where("title LIKE ? AND (is_private = ? OR userID = ?)", "%"+keyword+"%", false, viewerUserID).
		Order("postID DESC").
		Limit(20).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.VoteSearchItem, 0, len(posts))
	for _, post := range posts {
		out = append(out, domain.VoteSearchItem{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			PostTime:  post.PostTime,
			VoteCount: post.VoteNum,
		})
	}
	return out, nil
}

func (r *voteRepository) SubmitVote(ctx context.Context, userID, postID int) error {
	var existing model.Pvote
	err := r.session(ctx).Where("userID = ? AND postID = ?", userID, postID).First(&existing).Error
	if err == nil && existing.PvoteID != 0 {
		return domain.ErrConflict
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	newVote := model.Pvote{
		PostID: postID,
		UserID: userID,
		Time:   time.Now(),
	}
	return r.session(ctx).Create(&newVote).Error
}

func (r *voteRepository) HasVoted(ctx context.Context, userID, postID int) (bool, error) {
	var existing model.Pvote
	err := r.session(ctx).Where("userID = ? AND postID = ?", userID, postID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return existing.PvoteID != 0, nil
}

func (r *voteRepository) GetPostRanking(ctx context.Context) ([]domain.PostRankItem, error) {
	var posts []model.Post
	err := r.session(ctx).
		Where("vote_num > 0 AND (is_private = ? OR is_private IS NULL)", false).
		Order("vote_num DESC, postID DESC").
		Limit(120).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	items := make([]domain.PostRankItem, 0, len(posts))
	for _, post := range posts {
		if post.IsPrivate {
			continue
		}
		if len(items) >= 100 {
			break
		}
		items = append(items, domain.PostRankItem{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			VoteCount: post.VoteNum,
			PostTime:  post.PostTime,
		})
	}
	return items, nil
}

func effectiveUserRankScore(u model.User) int {
	score := u.ReceivedLikeNum + u.ReceivedVoteNum
	if u.UserID == userRankPenaltyID {
		score -= userRankPenalty
	}
	return score
}

func (r *voteRepository) GetUserRanking(ctx context.Context) ([]domain.UserRankItem, error) {
	var users []model.User
	err := r.session(ctx).
		Where("received_like_num > 0 OR received_vote_num > 0 OR userID = ?", userRankPenaltyID).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	type rankedUser struct {
		user  model.User
		score int
	}
	ranked := make([]rankedUser, 0, len(users))
	for _, u := range users {
		score := effectiveUserRankScore(u)
		if score <= 0 && u.UserID != userRankPenaltyID {
			continue
		}
		ranked = append(ranked, rankedUser{user: u, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].user.UserID < ranked[j].user.UserID
	})
	if len(ranked) > 100 {
		ranked = ranked[:100]
	}
	items := make([]domain.UserRankItem, 0, len(ranked))
	for _, row := range ranked {
		u := row.user
		items = append(items, domain.UserRankItem{
			UserID:          u.UserID,
			UserName:        u.Name,
			AvatarURL:       u.AvatarURL,
			ReceivedLikeNum: u.ReceivedLikeNum,
			ReceivedVoteNum: u.ReceivedVoteNum,
			RankScore:       row.score,
		})
	}
	return items, nil
}
