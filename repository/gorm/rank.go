package gormrepo

import (
	"context"
	"sort"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

// 投币榜默认/最大条数。
const (
	defaultRankLimit = 100
	maxRankLimit     = 100
)

type rankRepository struct {
	db *gorm.DB
}

func NewRankRepository(db *gorm.DB) repository.RankRepository {
	return &rankRepository{db: db}
}

func (r *rankRepository) session(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}

func rankLimit(limit int) int {
	if limit <= 0 {
		return defaultRankLimit
	}
	if limit > maxRankLimit {
		return maxRankLimit
	}
	return limit
}

func (r *rankRepository) AdjustUserReceivedLike(_ context.Context, authorUserID, delta int) {
	if authorUserID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.User{}).Where("userID = ?", authorUserID).
		UpdateColumn("received_like_num", gorm.Expr("GREATEST(received_like_num + ?, 0)", delta))
}

// OnPostDeleted 删帖时回退作者的获赞数。
// 历史票数不再回退 users.received_vote_num：该字段已退出榜单，只留作历史数据。
func (r *rankRepository) OnPostDeleted(_ context.Context, post *domain.Post) {
	if post == nil || post.ID == 0 || post.UserID == 0 {
		return
	}
	r.AdjustUserReceivedLike(nil, post.UserID, -post.LikeNum)
}

// RebuildRankStats 重建获赞数。投币榜是实时从 posts/novels 聚合的，不需要预计算列。
func (r *rankRepository) RebuildRankStats(_ context.Context) {
	var users []model.User
	r.db.Select("userID").Find(&users)
	for _, u := range users {
		var likeSum int
		r.db.Model(&model.Post{}).Where("userID = ?", u.UserID).
			Select("COALESCE(SUM(like_num), 0)").Row().Scan(&likeSum)

		r.db.Model(&model.User{}).Where("userID = ?", u.UserID).
			UpdateColumn("received_like_num", likeSum)
	}
}

// PostCoinRanking 帖子投币榜：按「真实投币数 + 历史投票数」倒序。
// vote_num 已经没有写入路径（只有删帖会连带消失），所以这个排序值刷不动。
func (r *rankRepository) PostCoinRanking(ctx context.Context, limit int) ([]domain.PostCoinRankItem, error) {
	n := rankLimit(limit)
	var posts []model.Post
	err := r.session(ctx).
		Select("postID", "title", "`partition`", "post_time", "coin_num", "vote_num").
		Where("(is_private = ? OR is_private IS NULL)", false).
		Order("(coin_num + vote_num) DESC, postID DESC").
		Limit(n).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}
	items := make([]domain.PostCoinRankItem, 0, len(posts))
	for _, post := range posts {
		coins := post.CoinNum + post.VoteNum
		if coins <= 0 {
			continue // 一个币都没有的不进榜
		}
		items = append(items, domain.PostCoinRankItem{
			PostID:    post.PostID,
			Title:     post.Title,
			Partition: post.Partition,
			CoinNum:   coins,
			PostTime:  post.PostTime,
		})
	}
	return items, nil
}

// userCoinRow 聚合用的中间行。
type userCoinRow struct {
	UserID int
	Coins  int
}

// UserCoinRanking 作者投币榜：某人所有内容收到的币数（帖子含历史票数 + 小说）。
func (r *rankRepository) UserCoinRanking(ctx context.Context, limit int) ([]domain.UserCoinRankItem, error) {
	n := rankLimit(limit)

	var postRows []userCoinRow
	if err := r.session(ctx).Table("posts").
		Select("userID", "COALESCE(SUM(coin_num + vote_num), 0) AS coins").
		Where("(is_private = ? OR is_private IS NULL)", false).
		Group("userID").
		Find(&postRows).Error; err != nil {
		return nil, err
	}

	var novelRows []userCoinRow
	if err := r.session(ctx).Table("novels").
		Select("user_id AS user_id", "COALESCE(SUM(coin_num), 0) AS coins").
		Group("user_id").
		Find(&novelRows).Error; err != nil {
		return nil, err
	}

	totals := make(map[int]int, len(postRows)+len(novelRows))
	for _, row := range postRows {
		if row.UserID != 0 {
			totals[row.UserID] += row.Coins
		}
	}
	for _, row := range novelRows {
		if row.UserID != 0 {
			totals[row.UserID] += row.Coins
		}
	}

	ranked := make([]userCoinRow, 0, len(totals))
	for userID, coins := range totals {
		if coins <= 0 {
			continue // 没收到过币的不进榜
		}
		ranked = append(ranked, userCoinRow{UserID: userID, Coins: coins})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Coins != ranked[j].Coins {
			return ranked[i].Coins > ranked[j].Coins
		}
		return ranked[i].UserID < ranked[j].UserID
	})
	if len(ranked) > n {
		ranked = ranked[:n]
	}

	items := make([]domain.UserCoinRankItem, 0, len(ranked))
	for _, row := range ranked {
		item := domain.UserCoinRankItem{UserID: row.UserID, ReceivedCoins: row.Coins}
		var u model.User
		if err := r.session(ctx).Select("userID", "name", "avatar_url").
			Where("userID = ?", row.UserID).First(&u).Error; err == nil {
			item.UserName = u.Name
			item.AvatarURL = u.AvatarURL
		}
		items = append(items, item)
	}
	return items, nil
}
