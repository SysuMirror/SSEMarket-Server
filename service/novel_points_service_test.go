package service_test

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"loginTest/domain"
	"loginTest/service"
)

// ---------------------------------------------------------------------------
// 内存版流水仓库：幂等与上限口径与 gorm 版一致
// （dedup_key 唯一、当日上限、里程碑游标、余额不足不扣成负数）。
// 用它验证「哪条规则被触发、幂等键怎么拼、上限生效没有」，
// SQL 细节另有 repository 层保证。
// ---------------------------------------------------------------------------

type ledgerStub struct {
	rows   map[string]*domain.NovelLedger
	exp    map[int]int
	points map[int]int
	nextID int
}

func newLedgerStub() *ledgerStub {
	return &ledgerStub{rows: map[string]*domain.NovelLedger{}, exp: map[int]int{}, points: map[int]int{}, nextID: 1}
}

func (s *ledgerStub) credit(userID int, target string, amount int) {
	switch target {
	case domain.RewardTargetExperience:
		s.exp[userID] += amount
	case domain.RewardTargetPoints:
		s.points[userID] += amount
	}
}

func (s *ledgerStub) put(in domain.NovelGrantInput) {
	s.rows[in.DedupKey] = &domain.NovelLedger{
		ID:        s.nextID,
		UserID:    in.UserID,
		NovelID:   in.NovelID,
		EventType: in.EventType,
		Amount:    in.Amount,
		DedupKey:  in.DedupKey,
		CreatedAt: time.Now(),
	}
	s.nextID++
}

func (s *ledgerStub) GrantOnce(_ context.Context, in domain.NovelGrantInput) (int, bool, error) {
	if _, ok := s.rows[in.DedupKey]; ok {
		return 0, true, nil
	}
	s.put(in)
	s.credit(in.UserID, in.Target, in.Amount)
	return in.Amount, false, nil
}

func (s *ledgerStub) GrantCapped(ctx context.Context, in domain.NovelGrantInput, dailyCap int) (int, bool, error) {
	if dailyCap > 0 && s.sumToday(in.UserID, in.EventType)+in.Amount > dailyCap {
		return 0, true, nil
	}
	return s.GrantOnce(ctx, in)
}

func (s *ledgerStub) SettleMilestone(_ context.Context, in domain.NovelMilestoneInput) (int, error) {
	if in.Delta <= 0 || in.Unit <= 0 || in.Reward <= 0 {
		return 0, nil
	}
	row, ok := s.rows[in.DedupKey]
	if !ok {
		row = &domain.NovelLedger{
			ID:        s.nextID,
			UserID:    in.UserID,
			NovelID:   in.NovelID,
			EventType: in.EventType,
			DedupKey:  in.DedupKey,
			CreatedAt: time.Now(),
		}
		s.nextID++
		s.rows[in.DedupKey] = row
	}
	total := row.SettledValue + in.Delta
	expected := (total / in.Unit) * in.Reward
	pending := expected - row.Amount
	if pending < 0 {
		pending = 0
	}
	row.SettledValue = total
	row.Amount += pending
	s.credit(in.UserID, in.Target, pending)
	return pending, nil
}

func (s *ledgerStub) AccumulateDaily(_ context.Context, userID int, eventType, dedupKey string, delta, threshold int) (int, bool, error) {
	if delta <= 0 {
		return 0, false, nil
	}
	row, ok := s.rows[dedupKey]
	if !ok {
		row = &domain.NovelLedger{
			ID:        s.nextID,
			UserID:    userID,
			EventType: eventType,
			DedupKey:  dedupKey,
			CreatedAt: time.Now(),
		}
		s.nextID++
		s.rows[dedupKey] = row
	}
	total := row.SettledValue + delta
	crossed := threshold > 0 && row.SettledValue < threshold && total >= threshold
	row.SettledValue = total
	return total, crossed, nil
}

func (s *ledgerStub) SpendExperience(_ context.Context, userID, amount int) (bool, error) {
	if s.exp[userID] < amount {
		return false, domain.ErrNovelNotEnoughExperience
	}
	s.exp[userID] -= amount
	return true, nil
}

func (s *ledgerStub) TransferExperience(_ context.Context, in domain.NovelRewardInput) (bool, error) {
	if _, ok := s.rows[in.DedupKey]; ok {
		return true, nil
	}
	if s.exp[in.FromUserID] < in.Points {
		return false, domain.ErrNovelNotEnoughExperience
	}
	s.exp[in.FromUserID] -= in.Points
	now := time.Now()
	s.rows[in.DedupKey] = &domain.NovelLedger{
		ID: s.nextID, UserID: in.FromUserID, NovelID: in.NovelID,
		EventType: domain.LedgerEventRewardOut, Amount: -in.Points,
		DedupKey: in.DedupKey, CreatedAt: now,
	}
	s.nextID++
	s.rows[in.DedupKey+":in"] = &domain.NovelLedger{
		ID: s.nextID, UserID: in.AuthorID, NovelID: in.NovelID,
		EventType: domain.LedgerEventRewardIn, Amount: in.Fen,
		DedupKey: in.DedupKey + ":in", CreatedAt: now,
	}
	s.nextID++
	s.points[in.AuthorID] += in.Fen
	return false, nil
}

func (s *ledgerStub) ExistsDedup(_ context.Context, dedupKey string) (bool, error) {
	_, ok := s.rows[dedupKey]
	return ok, nil
}

func (s *ledgerStub) CountByDedupPrefix(_ context.Context, eventType, prefix string) (int64, error) {
	var count int64
	for _, row := range s.rows {
		if row.EventType == eventType && strings.HasPrefix(row.DedupKey, prefix) {
			count++
		}
	}
	return count, nil
}

func (s *ledgerStub) sumToday(userID int, eventType string) int {
	start := domain.NovelLedgerDayStart(domain.NovelLedgerDay(time.Now()))
	total := 0
	for _, row := range s.rows {
		if row.UserID == userID && row.EventType == eventType && !row.CreatedAt.Before(start) {
			total += row.Amount
		}
	}
	return total
}

func (s *ledgerStub) SumToday(ctx context.Context, userID int, eventType string) (int, error) {
	return s.sumToday(userID, eventType), nil
}

func (s *ledgerStub) StreakDays(_ context.Context, userID int, eventType, endDay string, maxBack int) (int, error) {
	days := map[string]struct{}{}
	for _, row := range s.rows {
		if row.UserID == userID && row.EventType == eventType {
			days[domain.NovelLedgerDay(row.CreatedAt)] = struct{}{}
		}
	}
	streak := 0
	for i := 0; i < maxBack; i++ {
		if _, ok := days[domain.NovelLedgerDayShift(endDay, -i)]; !ok {
			break
		}
		streak++
	}
	return streak, nil
}

func (s *ledgerStub) Balance(_ context.Context, userID int) (int, int, error) {
	return s.exp[userID], s.points[userID], nil
}

func (s *ledgerStub) SummaryByEvent(_ context.Context, userID int, exclude []string, since time.Time) (map[string]int, error) {
	out := map[string]int{}
	skipped := map[string]struct{}{}
	for _, e := range exclude {
		skipped[e] = struct{}{}
	}
	for _, row := range s.rows {
		if row.UserID != userID {
			continue
		}
		if _, ok := skipped[row.EventType]; ok {
			continue
		}
		if !since.IsZero() && row.CreatedAt.Before(since) {
			continue
		}
		out[row.EventType] += row.Amount
	}
	return out, nil
}

func (s *ledgerStub) List(_ context.Context, userID, limit, offset int, exclude []string) ([]*domain.NovelLedger, int64, error) {
	skipped := map[string]struct{}{}
	for _, e := range exclude {
		skipped[e] = struct{}{}
	}
	rows := make([]*domain.NovelLedger, 0, len(s.rows))
	for _, row := range s.rows {
		if row.UserID != userID {
			continue
		}
		if _, ok := skipped[row.EventType]; ok {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID > rows[j].ID })
	return rows, int64(len(rows)), nil
}

// seedRow 预置一条历史流水（模拟「昨天已经签到 / 昨天阅读达标」）。
func (s *ledgerStub) seedRow(userID int, eventType, dedupKey string, amount int, dayShift int) {
	s.rows[dedupKey] = &domain.NovelLedger{
		ID:        s.nextID,
		UserID:    userID,
		EventType: eventType,
		Amount:    amount,
		DedupKey:  dedupKey,
		CreatedAt: time.Now().AddDate(0, 0, dayShift),
	}
	s.nextID++
	s.credit(userID, domain.RewardTargetExperience, amount)
}

func newPointsStub() (service.NovelPointsService, *ledgerStub) {
	stub := newLedgerStub()
	return service.NewNovelPointsService(stub, nil), stub
}

// ---------------------------------------------------------------------------
// 阅读经验
// ---------------------------------------------------------------------------

// readTimeRow 取某本书的阅读时长里程碑流水。
func readTimeRow(stub *ledgerStub, userID, novelID int) *domain.NovelLedger {
	return stub.rows[domain.NovelDedupMilestone(domain.LedgerEventReadTime, userID, novelID)]
}

// feedRead 按「单次心跳上限」把 seconds 拆成多次上报，返回累计发放量。
// 服务端对单次上报封顶，测试也必须按真实心跳的口径喂数据。
func feedRead(t *testing.T, svc service.NovelPointsService, userID, novelID, seconds int) int {
	t.Helper()
	total := 0
	for left := seconds; left > 0; {
		chunk := left
		if chunk > domain.NovelReadHeartbeatMaxSeconds {
			chunk = domain.NovelReadHeartbeatMaxSeconds
		}
		granted, err := svc.SettleReadTime(context.Background(), userID, novelID, chunk)
		if err != nil {
			t.Fatalf("结算失败: %v", err)
		}
		total += granted
		left -= chunk
	}
	return total
}

func TestReadTimeMilestoneIsIdempotent(t *testing.T) {
	svc, stub := newPointsStub()

	if got := feedRead(t, svc, 1, 10, 3599); got != 0 {
		t.Fatalf("未满 1 小时不应发放: granted=%d", got)
	}
	if got := feedRead(t, svc, 1, 10, 1); got != 1 {
		t.Fatalf("满 1 小时应 +1 经验: granted=%d", got)
	}
	// 同一本书再读满一小时，再 +1。
	if got := feedRead(t, svc, 1, 10, 3600); got != 1 {
		t.Fatalf("再满 1 小时应 +1 经验: granted=%d", got)
	}
	row := readTimeRow(stub, 1, 10)
	if row == nil {
		t.Fatalf("缺少 read_time 流水")
	}
	// 游标记总秒数，amount 记已发放份数 —— 重复上报同一秒不会重复发。
	if row.SettledValue != 3599+1+3600 || row.Amount != 2 {
		t.Errorf("read_time 流水 = %+v, 期望 settled_value=%d amount=2", row, 3599+1+3600)
	}
	// 2 点阅读经验 + 2 点「当日阅读满 30 分钟」达标经验。
	if exp := stub.exp[1]; exp != 4 {
		t.Errorf("experience = %d, 期望 4", exp)
	}
}

func TestReadHeartbeatIsCapped(t *testing.T) {
	svc, stub := newPointsStub()
	// 一次上报灌进几小时：只按上限计入，不会凭空发经验。
	if got, err := svc.SettleReadTime(context.Background(), 1, 10, 100000); err != nil || got != 0 {
		t.Fatalf("granted=%d err=%v, 期望 0", got, err)
	}
	row := stub.rows[domain.NovelDedupMilestone(domain.LedgerEventReadTime, 1, 10)]
	if row == nil || row.SettledValue != domain.NovelReadHeartbeatMaxSeconds {
		t.Errorf("游标 = %+v, 期望 settled_value=%d", row, domain.NovelReadHeartbeatMaxSeconds)
	}
}

func TestReadTimeIsPerBook(t *testing.T) {
	svc, stub := newPointsStub()
	for _, novelID := range []int{10, 11} {
		if got := feedRead(t, svc, 1, novelID, 3600); got != 1 {
			t.Fatalf("书 %d: granted=%d, 期望 1", novelID, got)
		}
	}
	// 每本书一条独立游标，互不影响。
	for _, novelID := range []int{10, 11} {
		row := readTimeRow(stub, 1, novelID)
		if row == nil || row.Amount != 1 || row.SettledValue != 3600 {
			t.Errorf("书 %d 的 read_time 流水 = %+v, 期望 settled_value=3600 amount=1", novelID, row)
		}
	}
}

func TestDailyReadStreakAndBonus(t *testing.T) {
	svc, stub := newPointsStub()
	day := domain.NovelLedgerDay(time.Now())

	// 昨日已达标 → 今日达标时额外给加成。
	stub.seedRow(1, domain.LedgerEventReadStreak, domain.NovelDedupDay(domain.LedgerEventReadStreak, 1, domain.NovelLedgerDayShift(day, -1)), 2, -1)
	before := stub.exp[1]

	if feedRead(t, svc, 1, 10, 1799) != 0 {
		t.Fatalf("未满 30 分钟不应发放")
	}
	if got := stub.exp[1] - before; got != 0 {
		t.Errorf("未满 30 分钟不应给达标经验，实际 %d", got)
	}
	feedRead(t, svc, 1, 10, 1)
	got := stub.exp[1] - before
	if got != domain.NovelExpReadDaily+domain.NovelExpReadBonus {
		t.Errorf("达标经验 = %d, 期望 %d", got, domain.NovelExpReadDaily+domain.NovelExpReadBonus)
	}
	// 同一天再达标不重复给（只多 1 点阅读满一小时的经验）。
	feedRead(t, svc, 1, 10, 3600)
	got = stub.exp[1] - before
	if got != domain.NovelExpReadDaily+domain.NovelExpReadBonus+1 {
		t.Errorf("达标经验 = %d, 期望只多 1 点阅读经验（%d）", got, domain.NovelExpReadDaily+domain.NovelExpReadBonus+1)
	}
}

// ---------------------------------------------------------------------------
// 互动经验
// ---------------------------------------------------------------------------

func TestCommentExpCountsOncePerCommentWithDailyCap(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	for i := 1; i <= 10; i++ {
		if got, err := svc.GrantCommentExp(ctx, 7, 10, i); err != nil || got != domain.NovelExpComment {
			t.Fatalf("第 %d 条评论: granted=%d err=%v", i, got, err)
		}
	}
	// 同一条评论重复发不重复计。
	if got, err := svc.GrantCommentExp(ctx, 7, 10, 1); err != nil || got != 0 {
		t.Fatalf("重复评论: granted=%d err=%v, 期望 0", got, err)
	}
	if got, err := svc.GrantCommentExp(ctx, 7, 10, 11); err != nil || got != 0 {
		t.Fatalf("第 11 条: granted=%d err=%v, 期望 0（已达单日上限）", got, err)
	}
	if exp := stub.exp[7]; exp != domain.NovelCommentDailyCap {
		t.Errorf("experience = %d, 期望 %d", exp, domain.NovelCommentDailyCap)
	}
}

func TestCommentLikeSkipsSelfAndCapsPerComment(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	const author = 100 // 评论作者，不能和点赞者编号重叠

	// 自己给自己点赞不给经验。
	if got, err := svc.GrantCommentLikeExp(ctx, author, 10, 99, author); err != nil || got != 0 {
		t.Fatalf("自赞: granted=%d err=%v, 期望 0", got, err)
	}
	for liker := 1; liker <= domain.NovelCommentLikeTotal; liker++ {
		if got, err := svc.GrantCommentLikeExp(ctx, author, 10, 99, liker); err != nil || got != domain.NovelExpCommentLike {
			t.Fatalf("点赞者 %d: granted=%d err=%v", liker, got, err)
		}
	}
	// 同一人重复点赞不重复计。
	if got, err := svc.GrantCommentLikeExp(ctx, author, 10, 99, 1); err != nil || got != 0 {
		t.Fatalf("重复点赞: granted=%d err=%v, 期望 0", got, err)
	}
	// 单条评论达到上限后不再给。
	if got, err := svc.GrantCommentLikeExp(ctx, author, 10, 99, 999); err != nil || got != 0 {
		t.Fatalf("超限点赞: granted=%d err=%v, 期望 0", got, err)
	}
	if exp := stub.exp[author]; exp != domain.NovelCommentLikeTotal {
		t.Errorf("experience = %d, 期望 %d", exp, domain.NovelCommentLikeTotal)
	}
}

// ---------------------------------------------------------------------------
// 作者积分
// ---------------------------------------------------------------------------

func TestShelfAddCountsOncePerReader(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	if got, err := svc.GrantShelfAdd(ctx, 100, 10, 7); err != nil || got != domain.NovelPointShelfAdd {
		t.Fatalf("首次收藏: granted=%d err=%v", got, err)
	}
	// 移出后再加，不重复给（同人同书仅首次）。
	if got, err := svc.GrantShelfAdd(ctx, 100, 10, 7); err != nil || got != 0 {
		t.Fatalf("重复收藏: granted=%d err=%v, 期望 0", got, err)
	}
	if got, err := svc.GrantShelfAdd(ctx, 100, 10, 8); err != nil || got != domain.NovelPointShelfAdd {
		t.Fatalf("另一位读者: granted=%d err=%v", got, err)
	}
	// 自己收藏自己的书不给。
	if got, err := svc.GrantShelfAdd(ctx, 100, 10, 100); err != nil || got != 0 {
		t.Fatalf("自己收藏: granted=%d err=%v, 期望 0", got, err)
	}
	if points := stub.points[100]; points != 2*domain.NovelPointShelfAdd {
		t.Errorf("novel_points = %d, 期望 %d", points, 2*domain.NovelPointShelfAdd)
	}
}

func TestReaderUVIsPerBookAndOncePerReader(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	if got, err := svc.GrantReaderUV(ctx, 100, 10, 7); err != nil || got != domain.NovelPointReaderUV {
		t.Fatalf("首次在读: granted=%d err=%v", got, err)
	}
	if got, err := svc.GrantReaderUV(ctx, 100, 10, 7); err != nil || got != 0 {
		t.Fatalf("重复在读: granted=%d err=%v, 期望 0", got, err)
	}
	if got, err := svc.GrantReaderUV(ctx, 100, 11, 7); err != nil || got != domain.NovelPointReaderUV {
		t.Fatalf("另一本书: granted=%d err=%v", got, err)
	}
	if points := stub.points[100]; points != 2*domain.NovelPointReaderUV {
		t.Errorf("novel_points = %d, 期望 %d", points, 2*domain.NovelPointReaderUV)
	}
}

func TestWritingWordsMilestone(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	if got, err := svc.SettleWritingWords(ctx, 100, 10, 4999); err != nil || got != 0 {
		t.Fatalf("未满 5000 字: granted=%d err=%v", got, err)
	}
	if got, err := svc.SettleWritingWords(ctx, 100, 10, 1); err != nil || got != domain.NovelPointWrite {
		t.Fatalf("满 5000 字: granted=%d err=%v", got, err)
	}
	// 同一本书重复结算同一批字数不重复给。
	if got, err := svc.SettleWritingWords(ctx, 100, 10, 0); err != nil || got != 0 {
		t.Fatalf("空结算: granted=%d err=%v", got, err)
	}
	if points := stub.points[100]; points != domain.NovelPointWrite {
		t.Errorf("novel_points = %d, 期望 %d", points, domain.NovelPointWrite)
	}
}

func TestChapterPublishDailyCap(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		if got, err := svc.GrantChapterPublish(ctx, 100, 10, i); err != nil || got != domain.NovelPointChapter {
			t.Fatalf("第 %d 章: granted=%d err=%v", i, got, err)
		}
	}
	if got, err := svc.GrantChapterPublish(ctx, 100, 10, 6); err != nil || got != 0 {
		t.Fatalf("第 6 章: granted=%d err=%v, 期望 0（每日上限）", got, err)
	}
	// 同一章重复发不重复计。
	if got, err := svc.GrantChapterPublish(ctx, 100, 10, 1); err != nil || got != 0 {
		t.Fatalf("重复章节: granted=%d err=%v, 期望 0", got, err)
	}
	if points := stub.points[100]; points != domain.NovelChapterDailyCap {
		t.Errorf("novel_points = %d, 期望 %d", points, domain.NovelChapterDailyCap)
	}
}

// ---------------------------------------------------------------------------
// 签到
// ---------------------------------------------------------------------------

func TestCheckInOncePerDayWithStreakBonus(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()
	day := domain.NovelLedgerDay(time.Now())

	// 昨天签过 → 今天连续加成 +1。
	stub.seedRow(1, domain.LedgerEventCheckIn, domain.NovelDedupDay(domain.LedgerEventCheckIn, 1, domain.NovelLedgerDayShift(day, -1)), 2, -1)

	result, err := svc.CheckIn(ctx, 1)
	if err != nil {
		t.Fatalf("签到失败: %v", err)
	}
	if result.Already {
		t.Fatalf("首次签到不应被判为已签到")
	}
	if result.Amount != domain.NovelExpCheckIn+1 {
		t.Errorf("签到经验 = %d, 期望 %d", result.Amount, domain.NovelExpCheckIn+1)
	}
	if result.Streak != 2 {
		t.Errorf("连续天数 = %d, 期望 2", result.Streak)
	}
	// 重复签到不重复发放。
	again, err := svc.CheckIn(ctx, 1)
	if err != nil {
		t.Fatalf("重复签到出错: %v", err)
	}
	if !again.Already || again.Amount != 0 {
		t.Errorf("重复签到 = %+v, 期望 already=true amount=0", again)
	}
}

func TestCheckInStreakBonusIsCapped(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()
	day := domain.NovelLedgerDay(time.Now())
	// 连续 10 天历史 → 加成封顶 5。
	for i := 1; i <= 10; i++ {
		stub.seedRow(2, domain.LedgerEventCheckIn, domain.NovelDedupDay(domain.LedgerEventCheckIn, 2, domain.NovelLedgerDayShift(day, -i)), 2, -i)
	}
	result, err := svc.CheckIn(ctx, 2)
	if err != nil {
		t.Fatalf("签到失败: %v", err)
	}
	if result.Amount != domain.NovelExpCheckIn+domain.NovelCheckInStreakMax {
		t.Errorf("签到经验 = %d, 期望 %d", result.Amount, domain.NovelExpCheckIn+domain.NovelCheckInStreakMax)
	}
}

// 打赏已下线（被投币体系替换），对应的 RewardAuthor 测试随之移除。
// ---------------------------------------------------------------------------
// 总览与流水
// ---------------------------------------------------------------------------

func TestSummaryAndLedger(t *testing.T) {
	svc, stub := newPointsStub()
	ctx := context.Background()

	if got := feedRead(t, svc, 1, 10, 3600); got != 1 {
		t.Fatalf("阅读经验 = %d, 期望 1", got)
	}
	if _, err := svc.GrantShelfAdd(ctx, 100, 10, 7); err != nil {
		t.Fatalf("收藏失败: %v", err)
	}
	summary, err := svc.Summary(ctx, 1)
	if err != nil {
		t.Fatalf("总览失败: %v", err)
	}
	// 1 点阅读经验 + 2 点当日阅读达标经验。
	if summary.Experience != 3 {
		t.Errorf("experience = %d, 期望 3", summary.Experience)
	}
	if summary.ByEvent[domain.LedgerEventReadTime] != 1 {
		t.Errorf("byEvent = %+v, 期望含 read_time=1", summary.ByEvent)
	}
	if summary.ByEvent[domain.LedgerEventReadStreak] != 2 {
		t.Errorf("byEvent = %+v, 期望含 read_streak=2", summary.ByEvent)
	}
	// 纯计数事件（当日阅读累计）不出现在总览与流水里。
	if _, ok := summary.ByEvent[domain.LedgerEventReadDaily]; ok {
		t.Errorf("byEvent 不应包含纯计数事件 read_daily: %+v", summary.ByEvent)
	}
	items, total, err := svc.Ledger(ctx, 1, 20, 0)
	if err != nil {
		t.Fatalf("流水失败: %v", err)
	}
	if total == 0 || len(items) == 0 {
		t.Fatalf("流水为空")
	}
	for _, item := range items {
		if item.EventType == domain.LedgerEventReadDaily {
			t.Errorf("流水里不应出现纯计数事件: %+v", item)
		}
	}
	_ = stub
}
