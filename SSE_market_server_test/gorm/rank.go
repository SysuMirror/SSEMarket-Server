package gormrepo

import (
	"context"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type rankRepository struct {
	db *gorm.DB
}

func NewRankRepository(db *gorm.DB) repository.RankRepository {
	return &rankRepository{db: db}
}

func (r *rankRepository) AdjustUserReceivedLike(_ context.Context, authorUserID, delta int) {
	if authorUserID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.User{}).Where("userID = ?", authorUserID).
		UpdateColumn("received_like_num", gorm.Expr("GREATEST(received_like_num + ?, 0)", delta))
}

func (r *rankRepository) AdjustUserReceivedVote(_ context.Context, authorUserID, delta int) {
	if authorUserID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.User{}).Where("userID = ?", authorUserID).
		UpdateColumn("received_vote_num", gorm.Expr("GREATEST(received_vote_num + ?, 0)", delta))
}

func (r *rankRepository) AdjustPostVoteNum(_ context.Context, postID, delta int) {
	if postID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("vote_num", gorm.Expr("GREATEST(vote_num + ?, 0)", delta))
}

func (r *rankRepository) OnPostDeleted(_ context.Context, post *domain.Post) {
	if post == nil || post.ID == 0 || post.UserID == 0 {
		return
	}
	var voteCount int64
	r.db.Model(&model.Pvote{}).Where("postID = ?", post.ID).Count(&voteCount)
	r.AdjustUserReceivedLike(nil, post.UserID, -post.LikeNum)
	r.AdjustUserReceivedVote(nil, post.UserID, -int(voteCount))
}

func (r *rankRepository) RebuildRankStats(_ context.Context) {
	r.db.Exec(`
		UPDATE posts p
		LEFT JOIN (
			SELECT postID, COUNT(*) AS cnt FROM pvotes GROUP BY postID
		) v ON p.postID = v.postID
		SET p.vote_num = COALESCE(v.cnt, 0)
	`)

	var users []model.User
	r.db.Select("userID").Find(&users)
	for _, u := range users {
		var likeSum int
		r.db.Model(&model.Post{}).Where("userID = ?", u.UserID).
			Select("COALESCE(SUM(like_num), 0)").Row().Scan(&likeSum)

		var voteSum int64
		r.db.Table("pvotes").
			Joins("INNER JOIN posts ON posts.postID = pvotes.postID").
			Where("posts.userID = ?", u.UserID).
			Count(&voteSum)

		r.db.Model(&model.User{}).Where("userID = ?", u.UserID).Updates(map[string]interface{}{
			"received_like_num": likeSum,
			"received_vote_num": int(voteSum),
		})
	}
}
