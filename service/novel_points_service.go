package service

import (
	"context"
	"time"

	"loginTest/domain"
	"loginTest/repository"
)

// novelPointsService 小说积分与经验体系。
//
// 规则集中在 domain/novel_points.go 的规则表里，本文件只负责「什么时候触发哪条规则」；
// 幂等（重复上报、重复提交）与每日上限全部落在 repository 层（novel_ledger 流水）。
type novelPointsService struct {
	ledger repository.LedgerRepository
	novels repository.NovelRepository
}

func NewNovelPointsService(ledger repository.LedgerRepository, novels repository.NovelRepository) NovelPointsService {
	return &novelPointsService{ledger: ledger, novels: novels}
}

// novelLedgerHiddenEvents 纯计数事件（只推进游标、不发放），不出现在「我的流水」里。
var novelLedgerHiddenEvents = []string{domain.LedgerEventReadDaily}

// ---------------------------------------------------------------------------
// 读者侧：阅读经验
// ---------------------------------------------------------------------------

// SettleReadTime 阅读心跳结算：
//  1. 按「本」累计阅读秒数，每满 3600 秒给读者 +1 经验（游标幂等，重复上报不会重复发）；
//  2. 顺带累计当日阅读秒数，首次跨过 30 分钟时给「连续阅读达标」+2 经验，
//     昨日也达标再给「连续阅读加成」+2 经验。
//
// 单次最多计入 NovelReadHeartbeatMaxSeconds 秒，防止一次上报灌进几小时。
func (s *novelPointsService) SettleReadTime(ctx context.Context, userID, novelID, seconds int) (int, error) {
	if userID <= 0 || novelID <= 0 || seconds <= 0 {
		return 0, nil
	}
	if seconds > domain.NovelReadHeartbeatMaxSeconds {
		seconds = domain.NovelReadHeartbeatMaxSeconds
	}
	granted, err := s.ledger.SettleMilestone(ctx, domain.NovelMilestoneInput{
		UserID:    userID,
		NovelID:   novelID,
		EventType: domain.LedgerEventReadTime,
		DedupKey:  domain.NovelDedupMilestone(domain.LedgerEventReadTime, userID, novelID),
		Delta:     seconds,
		Unit:      domain.NovelReadUnitSeconds,
		Reward:    domain.NovelExpReadHour,
		Target:    domain.RewardTargetExperience,
	})
	if err != nil {
		return 0, err
	}
	_ = s.settleDailyRead(ctx, userID, seconds)
	return granted, nil
}

// settleDailyRead 当日阅读达标判定；失败不影响已经发放的阅读经验。
func (s *novelPointsService) settleDailyRead(ctx context.Context, userID, seconds int) error {
	day := domain.NovelLedgerDay(time.Now())
	_, crossed, err := s.ledger.AccumulateDaily(
		ctx, userID,
		domain.LedgerEventReadDaily,
		domain.NovelDedupDay(domain.LedgerEventReadDaily, userID, day),
		seconds,
		domain.NovelReadDailyUnitSeconds,
	)
	if err != nil || !crossed {
		return err
	}
	if _, _, err := s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    userID,
		EventType: domain.LedgerEventReadStreak,
		DedupKey:  domain.NovelDedupDay(domain.LedgerEventReadStreak, userID, day),
		Amount:    domain.NovelExpReadDaily,
		Target:    domain.RewardTargetExperience,
	}); err != nil {
		return err
	}
	yesterday := domain.NovelLedgerDayShift(day, -1)
	ok, err := s.ledger.ExistsDedup(ctx, domain.NovelDedupDay(domain.LedgerEventReadStreak, userID, yesterday))
	if err != nil || !ok {
		return err
	}
	_, _, err = s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    userID,
		EventType: domain.LedgerEventReadBonus,
		DedupKey:  domain.NovelDedupDay(domain.LedgerEventReadBonus, userID, day),
		Amount:    domain.NovelExpReadBonus,
		Target:    domain.RewardTargetExperience,
	})
	return err
}

// ---------------------------------------------------------------------------
// 读者侧：互动经验
// ---------------------------------------------------------------------------

// GrantCommentExp 发表评论 +2 经验：每条只计一次（dedup_key = 评论 ID），
// 单用户单日上限 20 经验，防灌水。
func (s *novelPointsService) GrantCommentExp(ctx context.Context, userID, novelID, commentID int) (int, error) {
	if userID <= 0 || commentID <= 0 {
		return 0, nil
	}
	granted, _, err := s.ledger.GrantCapped(ctx, domain.NovelGrantInput{
		UserID:    userID,
		NovelID:   novelID,
		EventType: domain.LedgerEventComment,
		DedupKey:  domain.NovelDedupOnce(domain.LedgerEventComment, commentID),
		Amount:    domain.NovelExpComment,
		Target:    domain.RewardTargetExperience,
	}, domain.NovelCommentDailyCap)
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// GrantCommentLikeExp 评论被点赞：评论作者 +1 经验。
// 自己给自己点赞不给；同一人对同一评论只算一次；单条评论最多获得 20 经验。
func (s *novelPointsService) GrantCommentLikeExp(ctx context.Context, commentAuthorID, novelID, commentID, likerID int) (int, error) {
	if commentAuthorID <= 0 || commentID <= 0 || likerID <= 0 || commentAuthorID == likerID {
		return 0, nil
	}
	count, err := s.ledger.CountByDedupPrefix(ctx, domain.LedgerEventCommentLike, domain.NovelDedupOnce(domain.LedgerEventCommentLike, commentID)+":")
	if err != nil {
		return 0, err
	}
	if count >= domain.NovelCommentLikeTotal {
		return 0, nil
	}
	granted, _, err := s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    commentAuthorID,
		NovelID:   novelID,
		EventType: domain.LedgerEventCommentLike,
		DedupKey:  domain.NovelDedupOnce(domain.LedgerEventCommentLike, commentID, likerID),
		Amount:    domain.NovelExpCommentLike,
		Target:    domain.RewardTargetExperience,
	})
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// ---------------------------------------------------------------------------
// 作者侧：积分
// ---------------------------------------------------------------------------

// GrantShelfAdd 被加入书架：每人 +5 积分。同人同书仅首次计入（移出再加不重复给），
// 自己收藏自己的书不给。
func (s *novelPointsService) GrantShelfAdd(ctx context.Context, authorID, novelID, readerID int) (int, error) {
	if authorID <= 0 || novelID <= 0 || readerID <= 0 || authorID == readerID {
		return 0, nil
	}
	granted, _, err := s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    authorID,
		NovelID:   novelID,
		EventType: domain.LedgerEventShelfAdd,
		DedupKey:  domain.NovelDedupOnce(domain.LedgerEventShelfAdd, readerID, novelID),
		Amount:    domain.NovelPointShelfAdd,
		Target:    domain.RewardTargetPoints,
	})
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// GrantReaderUV 阅读人数：每人 +0.1 积分（=1 分）。按本独立、UV 去重、仅首次。
func (s *novelPointsService) GrantReaderUV(ctx context.Context, authorID, novelID, readerID int) (int, error) {
	if authorID <= 0 || novelID <= 0 || readerID <= 0 {
		return 0, nil
	}
	granted, _, err := s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    authorID,
		NovelID:   novelID,
		EventType: domain.LedgerEventReaderUV,
		DedupKey:  domain.NovelDedupOnce(domain.LedgerEventReaderUV, readerID, novelID),
		Amount:    domain.NovelPointReaderUV,
		Target:    domain.RewardTargetPoints,
	})
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// SettleWritingWords 写作字数里程碑：每满 5000 字作者 +1 积分（按书累计游标，幂等）。
func (s *novelPointsService) SettleWritingWords(ctx context.Context, authorID, novelID, words int) (int, error) {
	if authorID <= 0 || novelID <= 0 || words <= 0 {
		return 0, nil
	}
	granted, err := s.ledger.SettleMilestone(ctx, domain.NovelMilestoneInput{
		UserID:    authorID,
		NovelID:   novelID,
		EventType: domain.LedgerEventWriteWords,
		DedupKey:  domain.NovelDedupMilestone(domain.LedgerEventWriteWords, authorID, novelID),
		Delta:     words,
		Unit:      domain.NovelWriteUnitWords,
		Reward:    domain.NovelPointWrite,
		Target:    domain.RewardTargetPoints,
	})
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// GrantChapterPublish 章节更新：每章 +2 积分，单作者每日最多计 5 章（100 分）。
func (s *novelPointsService) GrantChapterPublish(ctx context.Context, authorID, novelID, chapterID int) (int, error) {
	if authorID <= 0 || novelID <= 0 || chapterID <= 0 {
		return 0, nil
	}
	granted, _, err := s.ledger.GrantCapped(ctx, domain.NovelGrantInput{
		UserID:    authorID,
		NovelID:   novelID,
		EventType: domain.LedgerEventChapterPub,
		DedupKey:  domain.NovelDedupOnce(domain.LedgerEventChapterPub, chapterID),
		Amount:    domain.NovelPointChapter,
		Target:    domain.RewardTargetPoints,
	}, domain.NovelChapterDailyCap)
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// ---------------------------------------------------------------------------
// 签到
// ---------------------------------------------------------------------------

// CheckIn 每日签到：基础 +2 经验，连续签到每天额外 +1（加成最多 +5）。
// 每天一条流水（dedup_key 含日期），重复签到不重复发放。
func (s *novelPointsService) CheckIn(ctx context.Context, userID int) (*domain.NovelCheckInResult, error) {
	if userID <= 0 {
		return nil, domain.ErrBadRequest
	}
	day := domain.NovelLedgerDay(time.Now())
	yesterday := domain.NovelLedgerDayShift(day, -1)
	prevStreak, err := s.ledger.StreakDays(ctx, userID, domain.LedgerEventCheckIn, yesterday, 60)
	if err != nil {
		return nil, err
	}
	streak := prevStreak + 1
	bonus := prevStreak
	if bonus > domain.NovelCheckInStreakMax {
		bonus = domain.NovelCheckInStreakMax
	}
	amount := domain.NovelExpCheckIn + bonus
	granted, deduped, err := s.ledger.GrantOnce(ctx, domain.NovelGrantInput{
		UserID:    userID,
		EventType: domain.LedgerEventCheckIn,
		DedupKey:  domain.NovelDedupDay(domain.LedgerEventCheckIn, userID, day),
		Amount:    amount,
		Target:    domain.RewardTargetExperience,
	})
	if err != nil {
		return nil, err
	}
	if deduped {
		todayStreak, err := s.ledger.StreakDays(ctx, userID, domain.LedgerEventCheckIn, day, 60)
		if err != nil {
			return nil, err
		}
		return &domain.NovelCheckInResult{Amount: 0, Streak: todayStreak, Day: day, Already: true}, nil
	}
	return &domain.NovelCheckInResult{Amount: granted, Streak: streak, Day: day}, nil
}

// ---------------------------------------------------------------------------
// 打赏
// ---------------------------------------------------------------------------

// RewardAuthor 打赏：读者花 N 经验，作者收 N 积分（记账 N×10 分）。
// 同一读者对同一本书每天最多 1 次；不能给自己打赏；经验不足直接拒绝。
func (s *novelPointsService) RewardAuthor(ctx context.Context, fromUserID, novelID, points int) (*domain.NovelRewardResult, error) {
	if fromUserID <= 0 || novelID <= 0 {
		return nil, domain.ErrBadRequest
	}
	if points < domain.NovelRewardMinPoints || points > domain.NovelRewardMaxPoints {
		return nil, domain.ErrBadRequest
	}
	novel, err := s.novels.FindNovelByID(ctx, novelID)
	if err != nil {
		return nil, err
	}
	if novel.UserID == fromUserID {
		return nil, domain.ErrNovelRewardSelf
	}
	day := domain.NovelLedgerDay(time.Now())
	deduped, err := s.ledger.TransferExperience(ctx, domain.NovelRewardInput{
		FromUserID: fromUserID,
		AuthorID:   novel.UserID,
		NovelID:    novelID,
		DedupKey:   domain.NovelDedupReward(fromUserID, novelID, day),
		Points:     points,
		Fen:        points * domain.NovelPointUnitPerScore,
	})
	if err != nil {
		return nil, err
	}
	if deduped {
		return nil, domain.ErrNovelRewardDailyLimit
	}
	experience, _, err := s.ledger.Balance(ctx, fromUserID)
	if err != nil {
		return nil, err
	}
	return &domain.NovelRewardResult{Points: points, Cost: points, Experience: experience}, nil
}

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------

func (s *novelPointsService) Summary(ctx context.Context, userID int) (*domain.NovelPointsSummary, error) {
	if userID <= 0 {
		return nil, domain.ErrBadRequest
	}
	experience, fen, err := s.ledger.Balance(ctx, userID)
	if err != nil {
		return nil, err
	}
	byEvent, err := s.ledger.SummaryByEvent(ctx, userID, novelLedgerHiddenEvents, time.Time{})
	if err != nil {
		return nil, err
	}
	day := domain.NovelLedgerDay(time.Now())
	today, err := s.ledger.SummaryByEvent(ctx, userID, novelLedgerHiddenEvents, domain.NovelLedgerDayStart(day))
	if err != nil {
		return nil, err
	}
	todayExp := 0
	for event, amount := range today {
		if amount > 0 && domain.NovelEventCurrency(event) == "exp" {
			todayExp += amount
		}
	}
	streak, err := s.ledger.StreakDays(ctx, userID, domain.LedgerEventCheckIn, day, 60)
	if err != nil {
		return nil, err
	}
	return &domain.NovelPointsSummary{
		Experience:    experience,
		PointsFen:     fen,
		PointsScore:   domain.NovelPointsScore(fen),
		ByEvent:       byEvent,
		TodayExp:      todayExp,
		CheckInDay:    day,
		CheckInStreak: streak,
	}, nil
}

func (s *novelPointsService) Ledger(ctx context.Context, userID, limit, offset int) ([]*domain.NovelLedger, int64, error) {
	if userID <= 0 {
		return nil, 0, domain.ErrBadRequest
	}
	return s.ledger.List(ctx, userID, limit, offset, novelLedgerHiddenEvents)
}

// ReadSecondsFor 某读者在某本书上累计的阅读秒数：直接读阅读里程碑
// （SettleMilestone 的 settled_value 就是按书累计的原始秒数游标）。
func (s *novelPointsService) ReadSecondsFor(ctx context.Context, userID, novelID int) (int, error) {
	if userID <= 0 || novelID <= 0 {
		return 0, nil
	}
	return s.ledger.SettledByDedup(ctx, domain.NovelDedupMilestone(domain.LedgerEventReadTime, userID, novelID))
}
