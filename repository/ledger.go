package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

// LedgerRepository 积分/经验流水持久化接口。
//
// 幂等与防刷都在这一层落地：
//   - GrantOnce / GrantCapped 靠 dedup_key 唯一索引，重复提交不会重复发放；
//   - SettleMilestone 用 settled_value 游标，重复上报同一秒/同一字不会重复发放；
//   - SpendExperience 用带条件的 UPDATE，余额不足不会把经验扣成负数。
type LedgerRepository interface {
	// GrantOnce 幂等发放。dedupKey 已存在时返回 granted=0、deduped=true。
	GrantOnce(ctx context.Context, in domain.NovelGrantInput) (granted int, deduped bool, err error)

	// GrantCapped 带「单用户当日上限」的发放：今日该事件已发放 + 本次 > dailyCap 时不发放。
	GrantCapped(ctx context.Context, in domain.NovelGrantInput, dailyCap int) (granted int, skipped bool, err error)

	// SettleMilestone 里程碑结算：把 Delta 累到游标上，每满 Unit 发放 Reward，返回本次实际发放量。
	SettleMilestone(ctx context.Context, in domain.NovelMilestoneInput) (granted int, err error)

	// AccumulateDaily 纯计数（不发放）：把 Delta 累到当日行上，返回累计值与「本次是否首次跨过阈值」。
	AccumulateDaily(ctx context.Context, userID int, eventType, dedupKey string, delta, threshold int) (total int, crossed bool, err error)

	// SpendExperience 扣经验；余额不足返回 spent=false。
	SpendExperience(ctx context.Context, userID, amount int) (spent bool, err error)

	// ExistsDedup 幂等键是否已存在。
	ExistsDedup(ctx context.Context, dedupKey string) (bool, error)

	// SettledByDedup 读某条里程碑流水的 settled_value（该键累计的原始量）；无流水返回 0。
	SettledByDedup(ctx context.Context, dedupKey string) (int, error)

	// CountByDedupPrefix 统计某事件下以 prefix 开头的流水条数（单条评论点赞上限用）。
	CountByDedupPrefix(ctx context.Context, eventType, prefix string) (int64, error)

	// SumToday 该用户今日该事件的发放合计。
	SumToday(ctx context.Context, userID int, eventType string) (int, error)

	// StreakDays 以 endDay 为终点往前连续存在流水的天数（不含 endDay 之后）。
	StreakDays(ctx context.Context, userID int, eventType, endDay string, maxBack int) (int, error)

	// Balance 当前经验与积分（分）。
	Balance(ctx context.Context, userID int) (experience, novelPoints int, err error)

	// TransferExperience 打赏：同一事务内扣打赏者经验、记两条流水、给作者加分。
	// dedupKey 冲突时返回 deduped=true 且不做任何扣减。
	TransferExperience(ctx context.Context, in domain.NovelRewardInput) (deduped bool, err error)

	// SummaryByEvent 按事件汇总发放量（排除纯计数事件）；since 为零值时统计全部时间。
	SummaryByEvent(ctx context.Context, userID int, exclude []string, since time.Time) (map[string]int, error)

	// List 流水分页（按时间倒序），exclude 用于排除纯计数事件。
	List(ctx context.Context, userID, limit, offset int, exclude []string) ([]*domain.NovelLedger, int64, error)
}
