package gormrepo

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ledgerRepository struct {
	db *gorm.DB
}

func NewLedgerRepository(db *gorm.DB) repository.LedgerRepository {
	return &ledgerRepository{db: db}
}

func (r *ledgerRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// ledgerRewardColumns 发放目标列白名单：只有这两列能被 gorm.Expr 拼进 UPDATE，
// 事件类型/金额都来自服务端规则表，不接受客户端传入的列名。
var ledgerRewardColumns = map[string]string{
	domain.RewardTargetExperience: "experience",
	domain.RewardTargetPoints:     "novel_points",
}

func ledgerTargetColumn(target string) (string, bool) {
	col, ok := ledgerRewardColumns[target]
	return col, ok
}

// isDuplicateKeyErr 幂等键冲突判定：GORM 未开 TranslateError，所以同时认
// 驱动错误码 1062 与错误文本，避免换驱动时静默失效。
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") || strings.Contains(msg, "unique constraint")
}

// lockUser 锁住受益人那一行：同一用户的发放串行执行，
// 既防并发重复发放，也保证余额不会被扣成负数。
func lockUser(tx *gorm.DB, userID int) error {
	var u model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("userID = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	return nil
}

func addReward(tx *gorm.DB, userID int, column string, amount int) error {
	if amount == 0 {
		return nil
	}
	return tx.Model(&model.User{}).Where("userID = ?", userID).
		UpdateColumn(column, gorm.Expr(column+" + ?", amount)).Error
}

func ledgerRow(in domain.NovelGrantInput) *model.NovelLedger {
	return &model.NovelLedger{
		UserID:    in.UserID,
		NovelID:   in.NovelID,
		EventType: in.EventType,
		Amount:    in.Amount,
		DedupKey:  in.DedupKey,
		CreatedAt: time.Now(),
	}
}

// ---------------------------------------------------------------------------
// 一次性发放
// ---------------------------------------------------------------------------

func (r *ledgerRepository) GrantOnce(ctx context.Context, in domain.NovelGrantInput) (int, bool, error) {
	column, ok := ledgerTargetColumn(in.Target)
	if !ok || in.DedupKey == "" || in.Amount == 0 {
		return 0, false, domain.ErrBadRequest
	}
	granted := 0
	deduped := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, in.UserID); err != nil {
			return err
		}
		if err := tx.Create(ledgerRow(in)).Error; err != nil {
			if isDuplicateKeyErr(err) {
				deduped = true
				return nil
			}
			return err
		}
		if err := addReward(tx, in.UserID, column, in.Amount); err != nil {
			return err
		}
		granted = in.Amount
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return granted, deduped, nil
}

func (r *ledgerRepository) GrantCapped(ctx context.Context, in domain.NovelGrantInput, dailyCap int) (int, bool, error) {
	column, ok := ledgerTargetColumn(in.Target)
	if !ok || in.DedupKey == "" || in.Amount <= 0 {
		return 0, false, domain.ErrBadRequest
	}
	granted := 0
	skipped := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, in.UserID); err != nil {
			return err
		}
		today, err := sumTodayTx(tx, in.UserID, in.EventType)
		if err != nil {
			return err
		}
		if dailyCap > 0 && today+in.Amount > dailyCap {
			skipped = true
			return nil
		}
		if err := tx.Create(ledgerRow(in)).Error; err != nil {
			if isDuplicateKeyErr(err) {
				skipped = true
				return nil
			}
			return err
		}
		if err := addReward(tx, in.UserID, column, in.Amount); err != nil {
			return err
		}
		granted = in.Amount
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return granted, skipped, nil
}

// ---------------------------------------------------------------------------
// 里程碑结算
// ---------------------------------------------------------------------------

func (r *ledgerRepository) SettleMilestone(ctx context.Context, in domain.NovelMilestoneInput) (int, error) {
	column, ok := ledgerTargetColumn(in.Target)
	if !ok || in.DedupKey == "" {
		return 0, domain.ErrBadRequest
	}
	if in.Delta <= 0 || in.Unit <= 0 || in.Reward <= 0 {
		return 0, nil
	}
	granted := 0
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, in.UserID); err != nil {
			return err
		}
		// 先保证游标行存在（重复插入直接忽略）。
		seed := model.NovelLedger{
			UserID:    in.UserID,
			NovelID:   in.NovelID,
			EventType: in.EventType,
			DedupKey:  in.DedupKey,
			Amount:    0,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&seed).Error; err != nil && !isDuplicateKeyErr(err) {
			return err
		}
		var row model.NovelLedger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dedup_key = ?", in.DedupKey).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		total := row.SettledValue + in.Delta
		// amount 记「已发放货币量」，expected 记「按游标应发放货币量」，差额就是本次发放。
		expected := (total / in.Unit) * in.Reward
		pending := expected - row.Amount
		if pending < 0 {
			pending = 0
		}
		if err := tx.Model(&model.NovelLedger{}).Where("ledger_id = ?", row.LedgerID).
			Updates(map[string]any{"settled_value": total, "amount": row.Amount + pending}).Error; err != nil {
			return err
		}
		if err := addReward(tx, in.UserID, column, pending); err != nil {
			return err
		}
		granted = pending
		return nil
	})
	if err != nil {
		return 0, err
	}
	return granted, nil
}

// AccumulateDaily 纯计数行：amount 恒为 0，只推进 settled_value 游标。
func (r *ledgerRepository) AccumulateDaily(ctx context.Context, userID int, eventType, dedupKey string, delta, threshold int) (int, bool, error) {
	if delta <= 0 {
		return 0, false, nil
	}
	total := 0
	crossed := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		seed := model.NovelLedger{
			UserID:    userID,
			EventType: eventType,
			DedupKey:  dedupKey,
			Amount:    0,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&seed).Error; err != nil && !isDuplicateKeyErr(err) {
			return err
		}
		var row model.NovelLedger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dedup_key = ?", dedupKey).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		total = row.SettledValue + delta
		crossed = threshold > 0 && row.SettledValue < threshold && total >= threshold
		return tx.Model(&model.NovelLedger{}).Where("ledger_id = ?", row.LedgerID).
			UpdateColumn("settled_value", total).Error
	})
	if err != nil {
		return 0, false, err
	}
	return total, crossed, nil
}

// ---------------------------------------------------------------------------
// 扣减与查询
// ---------------------------------------------------------------------------

func (r *ledgerRepository) SpendExperience(ctx context.Context, userID, amount int) (bool, error) {
	if amount <= 0 {
		return false, domain.ErrBadRequest
	}
	spent := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUser(tx, userID); err != nil {
			return err
		}
		res := tx.Model(&model.User{}).Where("userID = ? AND experience >= ?", userID, amount).
			UpdateColumn("experience", gorm.Expr("experience - ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNovelNotEnoughExperience
		}
		spent = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return spent, nil
}

// lockUsers 按 userID 升序加锁：两个用户互相打赏时不会交叉持锁死锁。
func lockUsers(tx *gorm.DB, userIDs ...int) error {
	ids := make([]int, 0, len(userIDs))
	seen := make(map[int]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if err := lockUser(tx, id); err != nil {
			return err
		}
	}
	return nil
}

// TransferExperience 打赏：扣打赏者经验 + 记两条流水 + 给作者加分，全在一个事务里。
// 幂等键冲突时整笔回滚（不扣任何经验），返回 deduped=true。
func (r *ledgerRepository) TransferExperience(ctx context.Context, in domain.NovelRewardInput) (bool, error) {
	if in.DedupKey == "" || in.Points <= 0 || in.Fen <= 0 || in.FromUserID <= 0 || in.AuthorID <= 0 {
		return false, domain.ErrBadRequest
	}
	pointColumn, ok := ledgerTargetColumn(domain.RewardTargetPoints)
	if !ok {
		return false, domain.ErrBadRequest
	}
	deduped := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUsers(tx, in.FromUserID, in.AuthorID); err != nil {
			return err
		}
		var existing int64
		if err := tx.Model(&model.NovelLedger{}).Where("dedup_key = ?", in.DedupKey).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			deduped = true
			return nil
		}
		res := tx.Model(&model.User{}).Where("userID = ? AND experience >= ?", in.FromUserID, in.Points).
			UpdateColumn("experience", gorm.Expr("experience - ?", in.Points))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNovelNotEnoughExperience
		}
		now := time.Now()
		if err := tx.Create(&model.NovelLedger{
			UserID:    in.FromUserID,
			NovelID:   in.NovelID,
			EventType: domain.LedgerEventRewardOut,
			Amount:    -in.Points,
			DedupKey:  in.DedupKey,
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.NovelLedger{
			UserID:    in.AuthorID,
			NovelID:   in.NovelID,
			EventType: domain.LedgerEventRewardIn,
			Amount:    in.Fen,
			DedupKey:  in.DedupKey + ":in",
			CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		return addReward(tx, in.AuthorID, pointColumn, in.Fen)
	})
	if err != nil {
		return false, err
	}
	return deduped, nil
}

func (r *ledgerRepository) ExistsDedup(ctx context.Context, dedupKey string) (bool, error) {
	var count int64
	if err := r.session(ctx).Model(&model.NovelLedger{}).Where("dedup_key = ?", dedupKey).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ledgerRepository) CountByDedupPrefix(ctx context.Context, eventType, prefix string) (int64, error) {
	var count int64
	if err := r.session(ctx).Model(&model.NovelLedger{}).
		Where("event_type = ? AND dedup_key LIKE ?", eventType, prefix+"%").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func sumTodayTx(tx *gorm.DB, userID int, eventType string) (int, error) {
	var total int64
	err := tx.Model(&model.NovelLedger{}).
		Where("user_id = ? AND event_type = ? AND created_at >= ?", userID, eventType, domain.NovelLedgerDayStart(domain.NovelLedgerDay(time.Now()))).
		Select("COALESCE(SUM(amount), 0)").Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return int(total), nil
}

func (r *ledgerRepository) SumToday(ctx context.Context, userID int, eventType string) (int, error) {
	return sumTodayTx(r.session(ctx), userID, eventType)
}

func (r *ledgerRepository) StreakDays(ctx context.Context, userID int, eventType, endDay string, maxBack int) (int, error) {
	if maxBack <= 0 {
		maxBack = 30
	}
	from := domain.NovelLedgerDayStart(domain.NovelLedgerDayShift(endDay, -(maxBack - 1)))
	var rows []model.NovelLedger
	if err := r.session(ctx).Where("user_id = ? AND event_type = ? AND created_at >= ?", userID, eventType, from).
		Select("created_at").Find(&rows).Error; err != nil {
		return 0, err
	}
	days := make(map[string]struct{}, len(rows))
	for i := range rows {
		days[domain.NovelLedgerDay(rows[i].CreatedAt)] = struct{}{}
	}
	streak := 0
	for i := 0; i < maxBack; i++ {
		day := domain.NovelLedgerDayShift(endDay, -i)
		if _, ok := days[day]; !ok {
			break
		}
		streak++
	}
	return streak, nil
}

func (r *ledgerRepository) Balance(ctx context.Context, userID int) (int, int, error) {
	var u model.User
	if err := r.session(ctx).Select("experience", "novel_points").Where("userID = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, domain.ErrNotFound
		}
		return 0, 0, err
	}
	return u.Experience, u.NovelPoints, nil
}

func (r *ledgerRepository) SummaryByEvent(ctx context.Context, userID int, exclude []string, since time.Time) (map[string]int, error) {
	type row struct {
		EventType string
		Total     int
	}
	q := r.session(ctx).Model(&model.NovelLedger{}).Where("user_id = ?", userID)
	if len(exclude) > 0 {
		q = q.Where("event_type NOT IN (?)", exclude)
	}
	if !since.IsZero() {
		q = q.Where("created_at >= ?", since)
	}
	var rows []row
	if err := q.Select("event_type, COALESCE(SUM(amount), 0) AS total").Group("event_type").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, x := range rows {
		out[x.EventType] = x.Total
	}
	return out, nil
}

func (r *ledgerRepository) List(ctx context.Context, userID, limit, offset int, exclude []string) ([]*domain.NovelLedger, int64, error) {
	limit, offset = novelNormalizePage(limit, offset)
	q := r.session(ctx).Model(&model.NovelLedger{}).Where("user_id = ?", userID)
	if len(exclude) > 0 {
		q = q.Where("event_type NOT IN (?)", exclude)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.NovelLedger
	if err := q.Order("created_at DESC, ledger_id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*domain.NovelLedger, 0, len(rows))
	for i := range rows {
		out = append(out, ledgerToDomain(&rows[i]))
	}
	return out, total, nil
}

func ledgerToDomain(l *model.NovelLedger) *domain.NovelLedger {
	if l == nil {
		return nil
	}
	return &domain.NovelLedger{
		ID:           l.LedgerID,
		UserID:       l.UserID,
		NovelID:      l.NovelID,
		EventType:    l.EventType,
		Amount:       l.Amount,
		SettledValue: l.SettledValue,
		DedupKey:     l.DedupKey,
		CreatedAt:    l.CreatedAt,
	}
}
