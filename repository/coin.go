package repository

import (
	"context"

	"loginTest/domain"
)

// CoinRepository 站点「币」持久化接口。
//
// 与 LedgerRepository（小说经验/积分）完全独立：各自的表、各自的幂等键。
// 实现侧的口径：
//   - Grant 靠 dedup_key 唯一索引幂等，重复发放返回 deduped=true；
//   - Throw 在一个事务里「锁用户 → 查已投数 → 条件扣币 → 记流水 → 记投币数 → 加经验」，
//     余额不足不会被扣成负数，每人每内容最多 2 币由唯一索引 + 行锁保证；
//   - AccumulateDaily 只推进 settled_value 游标，用于「当日累计」类任务判定。
type CoinRepository interface {
	// Grant 发币（幂等）。
	Grant(ctx context.Context, in domain.CoinGrantInput) (granted int, deduped bool, err error)

	// AccumulateDaily 把 Delta 累到当日游标上；首次跨过 threshold 时返回 crossed=true。
	AccumulateDaily(ctx context.Context, userID int, eventType, dedupKey string, delta, threshold int) (total int, crossed bool, err error)

	// Throw 投币：扣币 + 加主页经验 + 记流水 + 更新目标投币数。
	// 币不足返回 domain.ErrCoinNotEnough，超过每人每内容上限返回 domain.ErrCoinLimit。
	Throw(ctx context.Context, in domain.CoinThrowInput) (*domain.CoinThrowResult, error)

	// Cancel 取消投币：退币 + 回退主页经验 + 记退款流水 + 把我在该内容上的投币清零。
	// 一个都没投过返回 domain.ErrCoinNotThrown。
	Cancel(ctx context.Context, userID int, targetType string, targetID int) (*domain.CoinThrowResult, error)

	// Balance 当前币余额。
	Balance(ctx context.Context, userID int) (int, error)

	// ExistsDedup 该幂等键是否已存在（判断「今天这项任务已经拿过」）。
	ExistsDedup(ctx context.Context, dedupKey string) (bool, error)

	// AccumulatedByDedup 某条累计行当前的游标值。
	AccumulatedByDedup(ctx context.Context, dedupKey string) (int, error)

	// SumSigned 指定事件的金额合计（含负数）。
	SumSigned(ctx context.Context, userID int, eventTypes ...string) (int, error)

	// ThrownByMe 我在这些内容上各投了几个币。
	ThrownByMe(ctx context.Context, userID int, targetType string, targetIDs []int) (map[int]int, error)

	// ThrownForTarget 某内容被投币总数。
	ThrownForTarget(ctx context.Context, targetType string, targetID int) (int, error)

	// ThrownTotalByUser 某人累计投出去多少币（主页经验里的投币加成 = 该值 × 10）。
	ThrownTotalByUser(ctx context.Context, userID int) (int, error)

	// ThrownTotalsAll 全站每个人的投币总数，一次查询拿全（经验重算用，避免逐用户查询）。
	ThrownTotalsAll(ctx context.Context) (map[int]int, error)

	// List 某人的币流水（分页）。
	List(ctx context.Context, userID, limit, offset int) ([]*domain.CoinLedger, int64, error)
}
