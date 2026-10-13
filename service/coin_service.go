package service

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/repository"
)

type coinService struct {
	coins repository.CoinRepository
	// posts / novels 只用来解析「被投币内容的作者」和可见性；允许为 nil（单测）。
	posts  repository.PostRepository
	novels repository.NovelRepository
}

func NewCoinService(coins repository.CoinRepository, posts repository.PostRepository, novels repository.NovelRepository) CoinService {
	return &coinService{coins: coins, posts: posts, novels: novels}
}

// coinEarnEvents 赚币的那几条事件（投币支出是负数，不算在内）。
var coinEarnEvents = []string{
	domain.CoinEventVisitDaily,
	domain.CoinEventPostDaily,
	domain.CoinEventReadDaily,
	domain.CoinEventWriteDaily,
	domain.CoinEventRated,
}

// ---------------------------------------------------------------------------
// 赚币：全部走 dedup_key 幂等，同一件事重复触发只会有一条流水
// ---------------------------------------------------------------------------

// GrantDailyVisit 每日进入网页 +1 币。
// 前端进站点时调一次即可：幂等键含日期，重复调用（刷新、多标签页）只会有一条流水。
func (s *coinService) GrantDailyVisit(ctx context.Context, userID int) (int, error) {
	if s == nil || userID <= 0 {
		return 0, nil
	}
	granted, _, err := s.coins.Grant(ctx, domain.CoinGrantInput{
		UserID:    userID,
		EventType: domain.CoinEventVisitDaily,
		DedupKey:  domain.CoinDedupDay(domain.CoinEventVisitDaily, userID, domain.CoinLedgerDay(time.Now())),
		Amount:    domain.CoinVisitDaily,
	})
	return granted, err
}

// GrantFirstPost 每日首次发帖 +1 币。
// 幂等键含日期，所以一天发十帖也只给一次币。
func (s *coinService) GrantFirstPost(ctx context.Context, userID int) (int, error) {
	if s == nil || userID <= 0 {
		return 0, nil
	}
	granted, _, err := s.coins.Grant(ctx, domain.CoinGrantInput{
		UserID:    userID,
		EventType: domain.CoinEventPostDaily,
		DedupKey:  domain.CoinDedupDay(domain.CoinEventPostDaily, userID, domain.CoinLedgerDay(time.Now())),
		Amount:    domain.CoinPostDaily,
	})
	return granted, err
}

// AddReadSeconds 当日阅读秒数累计，满 30 分钟给一次币。
// 秒数累在 acc_read 游标上，只有「第一次跨过阈值」那一次上报会发币。
func (s *coinService) AddReadSeconds(ctx context.Context, userID, seconds int) (int, error) {
	if s == nil || userID <= 0 || seconds <= 0 {
		return 0, nil
	}
	day := domain.CoinLedgerDay(time.Now())
	_, crossed, err := s.coins.AccumulateDaily(ctx, userID,
		domain.CoinAccumulateRead,
		domain.CoinDedupAccumulate(domain.CoinAccumulateRead, userID, day),
		seconds, domain.CoinReadDailySeconds)
	if err != nil || !crossed {
		return 0, err
	}
	granted, _, err := s.coins.Grant(ctx, domain.CoinGrantInput{
		UserID:    userID,
		EventType: domain.CoinEventReadDaily,
		DedupKey:  domain.CoinDedupDay(domain.CoinEventReadDaily, userID, day),
		Amount:    domain.CoinReadDaily,
	})
	return granted, err
}

// AddWriteWords 当日更新写作字数累计，满 4000 字给一次币。
// 字数取正常发布章节的正文字数（图片段落不计入），所以「正常更新」才给币。
func (s *coinService) AddWriteWords(ctx context.Context, authorID, words int) (int, error) {
	if s == nil || authorID <= 0 || words <= 0 {
		return 0, nil
	}
	day := domain.CoinLedgerDay(time.Now())
	_, crossed, err := s.coins.AccumulateDaily(ctx, authorID,
		domain.CoinAccumulateWrite,
		domain.CoinDedupAccumulate(domain.CoinAccumulateWrite, authorID, day),
		words, domain.CoinWriteDailyWords)
	if err != nil || !crossed {
		return 0, err
	}
	granted, _, err := s.coins.Grant(ctx, domain.CoinGrantInput{
		UserID:    authorID,
		EventType: domain.CoinEventWriteDaily,
		DedupKey:  domain.CoinDedupDay(domain.CoinEventWriteDaily, authorID, day),
		Amount:    domain.CoinWriteDaily,
	})
	return granted, err
}

// GrantRated 收到评分：作者 +1 币（同一读者对同一本书只算一次）。
func (s *coinService) GrantRated(ctx context.Context, authorID, novelID, raterID int) (int, error) {
	if s == nil || authorID <= 0 || novelID <= 0 || raterID <= 0 || authorID == raterID {
		return 0, nil
	}
	granted, _, err := s.coins.Grant(ctx, domain.CoinGrantInput{
		UserID:     authorID,
		TargetType: domain.CoinTargetNovel,
		TargetID:   novelID,
		EventType:  domain.CoinEventRated,
		DedupKey:   domain.CoinDedupRated(authorID, novelID, raterID),
		Amount:     domain.CoinRated,
	})
	return granted, err
}

// ---------------------------------------------------------------------------
// 投币
// ---------------------------------------------------------------------------

// Throw 给帖子/小说投币：扣币、加主页经验、记流水，一个事务里完成。
// 目标不存在或不可见返回 ErrNotFound；给自己投币返回 ErrCoinSelf。
func (s *coinService) Throw(ctx context.Context, userID int, targetType string, targetID, coins int) (*domain.CoinThrowResult, error) {
	if s == nil {
		return nil, domain.ErrBadRequest
	}
	if targetID <= 0 || coins <= 0 {
		return nil, domain.ErrBadRequest
	}
	switch targetType {
	case domain.CoinTargetPost:
		if s.posts == nil {
			return nil, domain.ErrNotFound
		}
		// FindPublicByID 只返回对他人可见的帖子：私密帖不能被投币。
		post, err := s.posts.FindPublicByID(ctx, targetID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, domain.ErrNotFound
			}
			return nil, err
		}
		if post.UserID == userID {
			return nil, domain.ErrCoinSelf
		}
	case domain.CoinTargetNovel:
		if s.novels == nil {
			return nil, domain.ErrNotFound
		}
		novel, err := s.novels.FindNovelByID(ctx, targetID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, domain.ErrNotFound
			}
			return nil, err
		}
		if novel.UserID == userID {
			return nil, domain.ErrCoinSelf
		}
	default:
		return nil, domain.ErrBadRequest
	}
	result, err := s.coins.Throw(ctx, domain.CoinThrowInput{
		UserID:     userID,
		TargetType: targetType,
		TargetID:   targetID,
		Coins:      coins,
	})
	if err != nil {
		return nil, err
	}
	// 帖子详情缓存与主页 Feed 缓存里带着 coin_num，投币后必须失效，否则币数停在旧值。
	// （小说列表/详情没有缓存，novels.coin_num 直接读库，不需要处理。）
	if targetType == domain.CoinTargetPost {
		InvalidatePostDetail(targetID)
		InvalidateHomeBrowse()
	}
	return result, nil
}

// CancelThrow 取消投币：把我在该内容上投着的币全部撤回（退币 + 回退经验）。
//
// 与 Throw 不同，这里不校验目标是否存在/可见——coin_throw 行本身就是投过币的凭证，
// 内容被删后也应该能撤币拿回余额；帖子侧照常失效详情与 Feed 缓存。
func (s *coinService) CancelThrow(ctx context.Context, userID int, targetType string, targetID int) (*domain.CoinThrowResult, error) {
	if s == nil {
		return nil, domain.ErrBadRequest
	}
	result, err := s.coins.Cancel(ctx, userID, targetType, targetID)
	if err != nil {
		return nil, err
	}
	if targetType == domain.CoinTargetPost {
		InvalidatePostDetail(targetID)
		InvalidateHomeBrowse()
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------

// Summary 币总览：余额、今天各项任务的进度、累计投币与经验加成。
func (s *coinService) Summary(ctx context.Context, userID int) (*domain.CoinSummary, error) {
	if s == nil {
		return nil, domain.ErrBadRequest
	}
	day := domain.CoinLedgerDay(time.Now())
	coins, err := s.coins.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}
	thrown, err := s.coins.ThrownTotalByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	received, err := s.coins.SumSigned(ctx, userID, coinEarnEvents...)
	if err != nil {
		return nil, err
	}
	// 「收到评分」这一项只看 rated，不能把发帖/阅读/写作的币也算成评分收益。
	ratedCoins, err := s.coins.SumSigned(ctx, userID, domain.CoinEventRated)
	if err != nil {
		return nil, err
	}

	visitDone, err := s.coins.ExistsDedup(ctx, domain.CoinDedupDay(domain.CoinEventVisitDaily, userID, day))
	if err != nil {
		return nil, err
	}
	postDone, err := s.coins.ExistsDedup(ctx, domain.CoinDedupDay(domain.CoinEventPostDaily, userID, day))
	if err != nil {
		return nil, err
	}
	readDone, err := s.coins.ExistsDedup(ctx, domain.CoinDedupDay(domain.CoinEventReadDaily, userID, day))
	if err != nil {
		return nil, err
	}
	writeDone, err := s.coins.ExistsDedup(ctx, domain.CoinDedupDay(domain.CoinEventWriteDaily, userID, day))
	if err != nil {
		return nil, err
	}
	readSeconds, err := s.coins.AccumulatedByDedup(ctx, domain.CoinDedupAccumulate(domain.CoinAccumulateRead, userID, day))
	if err != nil {
		return nil, err
	}
	writeWords, err := s.coins.AccumulatedByDedup(ctx, domain.CoinDedupAccumulate(domain.CoinAccumulateWrite, userID, day))
	if err != nil {
		return nil, err
	}

	// 进入网页与发帖这两项没有计数器：当天做过就是 1，没做过就 0，达标线 1 次。
	visitProgress := 0
	if visitDone {
		visitProgress = 1
	}
	postProgress := 0
	if postDone {
		postProgress = 1
	}

	tasks := []domain.CoinTask{
		{
			Event: domain.CoinEventVisitDaily, Label: "进入网页 1 次",
			Target: 1, Progress: visitProgress, Reward: domain.CoinVisitDaily,
			Done: visitDone, Available: !visitDone,
		},
		{
			Event: domain.CoinEventPostDaily, Label: "发 1 篇帖子",
			Target: 1, Progress: postProgress, Reward: domain.CoinPostDaily,
			Done: postDone, Available: !postDone,
		},
		{
			Event: domain.CoinEventReadDaily, Label: "阅读满 30 分钟",
			Target: domain.CoinReadDailySeconds, Progress: readSeconds, Reward: domain.CoinReadDaily,
			Done: readDone, Available: !readDone,
		},
		{
			Event: domain.CoinEventWriteDaily, Label: "更新写作满 4000 字",
			Target: domain.CoinWriteDailyWords, Progress: writeWords, Reward: domain.CoinWriteDaily,
			Done: writeDone, Available: !writeDone,
		},
		{
			Event: domain.CoinEventRated, Label: "作品收到评分（作者收益）",
			Target: domain.CoinRated, Progress: ratedCoins, Reward: domain.CoinRated,
			Done: false, Available: true,
		},
	}

	return &domain.CoinSummary{
		Coins:       coins,
		Day:         day,
		Tasks:       tasks,
		ThrownTotal: thrown,
		ThrownExp:   thrown * domain.CoinExpPerCoin,
		Received:    received,
	}, nil
}

// Ledger 我的币流水（分页）。
func (s *coinService) Ledger(ctx context.Context, userID, limit, offset int) ([]*domain.CoinLedger, int64, error) {
	if s == nil {
		return nil, 0, domain.ErrBadRequest
	}
	return s.coins.List(ctx, userID, limit, offset)
}

// MyThrows 我在这些内容上各投了几个币（卡片/详情页按钮态用）。
func (s *coinService) MyThrows(ctx context.Context, userID int, targetType string, targetIDs []int) (map[int]int, error) {
	if s == nil || userID <= 0 || len(targetIDs) == 0 {
		return map[int]int{}, nil
	}
	return s.coins.ThrownByMe(ctx, userID, targetType, targetIDs)
}

// ThrownForTarget 某个内容被投币总数（详情页展示）。
func (s *coinService) ThrownForTarget(ctx context.Context, targetType string, targetID int) (int, error) {
	if s == nil || targetID <= 0 {
		return 0, nil
	}
	return s.coins.ThrownForTarget(ctx, targetType, targetID)
}

// ThrownTotalsAll 全站每个人的投币总数，一次查询拿全。
// 主页经验每小时重算用它，避免在按用户的循环里逐个查（N 个用户 N 次查询）。
func (s *coinService) ThrownTotalsAll(ctx context.Context) (map[int]int, error) {
	if s == nil {
		return map[int]int{}, nil
	}
	return s.coins.ThrownTotalsAll(ctx)
}
