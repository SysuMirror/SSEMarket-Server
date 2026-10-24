package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type coinRepository struct {
	db *gorm.DB
}

func NewCoinRepository(db *gorm.DB) repository.CoinRepository {
	return &coinRepository{db: db}
}

func (r *coinRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// coinDisplayEvents 流水展示口径：纯计数行（当日阅读秒数、写作字数）不露出。
var coinDisplayEvents = []string{
	domain.CoinEventVisitDaily,
	domain.CoinEventPostDaily,
	domain.CoinEventReadDaily,
	domain.CoinEventWriteDaily,
	domain.CoinEventRated,
	domain.CoinEventProgrammersDay2026,
	domain.CoinEventPostTipIncome,
	domain.CoinEventThrow,
	domain.CoinEventRefund,
}

// lockCoinUser 锁住当事人那一行并返回它：同一用户的币收支串行执行，
// 既防并发重复发放，也保证余额不会被扣成负数。
func lockCoinUser(tx *gorm.DB, userID int) (model.User, error) {
	var u model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("userID = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return u, domain.ErrNotFound
		}
		return u, err
	}
	return u, nil
}

func coinLedgerRow(in domain.CoinGrantInput) *model.CoinLedger {
	return &model.CoinLedger{
		UserID:     in.UserID,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		EventType:  in.EventType,
		Amount:     in.Amount,
		DedupKey:   in.DedupKey,
		CreatedAt:  time.Now(),
	}
}

// Grant 发币：写流水 + 加余额，幂等键冲突时整笔不生效（deduped=true）。
func (r *coinRepository) Grant(ctx context.Context, in domain.CoinGrantInput) (float64, bool, error) {
	if in.DedupKey == "" || in.UserID <= 0 || in.Amount == 0 {
		return 0, false, domain.ErrBadRequest
	}
	granted := 0.0
	deduped := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockCoinUser(tx, in.UserID); err != nil {
			return err
		}
		if err := tx.Create(coinLedgerRow(in)).Error; err != nil {
			if isDuplicateKeyErr(err) {
				deduped = true
				return nil
			}
			return err
		}
		if err := tx.Model(&model.User{}).Where("userID = ?", in.UserID).
			UpdateColumn("coins", gorm.Expr("coins + ?", in.Amount)).Error; err != nil {
			return err
		}
		granted = domain.RoundCoin(in.Amount)
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return granted, deduped, nil
}

// AccumulateDaily 纯计数行：amount 恒为 0，只推进 settled_value 游标；
// 返回累计值与「本次是否第一次跨过 threshold」。
func (r *coinRepository) AccumulateDaily(ctx context.Context, userID int, eventType, dedupKey string, delta, threshold int) (int, bool, error) {
	if delta <= 0 || userID <= 0 || dedupKey == "" {
		return 0, false, nil
	}
	total := 0
	crossed := false
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		seed := model.CoinLedger{
			UserID:    userID,
			EventType: eventType,
			DedupKey:  dedupKey,
			Amount:    0,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&seed).Error; err != nil && !isDuplicateKeyErr(err) {
			return err
		}
		var row model.CoinLedger
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("dedup_key = ?", dedupKey).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		total = row.SettledValue + delta
		crossed = threshold > 0 && row.SettledValue < threshold && total >= threshold
		return tx.Model(&model.CoinLedger{}).Where("ledger_id = ?", row.LedgerID).
			UpdateColumn("settled_value", total).Error
	})
	if err != nil {
		return 0, false, err
	}
	return total, crossed, nil
}

// Throw 投币：扣币 + 记流水 + 累计投币数 + 加主页经验 + 更新被投内容的投币计数。
//
// 并发口径：
//   - 先锁投币者，保证余额与「每人每内容最多 2 币」串行判定；
//   - 再锁 coin_throws 那一行，累计数不会因并发被算少；
//   - 扣币用带条件的 UPDATE（coins >= ?），余额不足直接失败，不会扣成负数；
//   - 流水幂等键带「第几枚」序号，同一目标上的每一笔都不同，重复提交不会静默合并。
func (r *coinRepository) Throw(ctx context.Context, in domain.CoinThrowInput) (*domain.CoinThrowResult, error) {
	if in.UserID <= 0 || in.TargetID <= 0 || in.Coins <= 0 {
		return nil, domain.ErrBadRequest
	}
	if in.Coins > domain.CoinMaxPerTarget {
		return nil, domain.ErrCoinLimit
	}
	result := &domain.CoinThrowResult{
		Coins:      in.Coins,
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
	}
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockCoinUser(tx, in.UserID)
		if err != nil {
			return err
		}

		// 投币记录：没有就插一条 0 币的占位行，有就锁住读当前累计。
		seed := model.CoinThrow{
			UserID:     in.UserID,
			TargetType: in.TargetType,
			TargetID:   in.TargetID,
			CoinNum:    0,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var throw model.CoinThrow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND target_type = ? AND target_id = ?", in.UserID, in.TargetType, in.TargetID).
			First(&throw).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		legacyVotes, err := legacyVoteCoins(tx, in.UserID, in.TargetType, []int{in.TargetID})
		if err != nil {
			return err
		}
		// coin_num 只记真币（投出去、可撤回、算经验）。旧投票折算不落进这一列，
		// 只在「还能不能投」的上限判定和「已投几枚」的展示口径里加上去——
		// 否则取消投币会把折算票也退成真币。
		held := throw.CoinNum + in.Coins
		if held+legacyVotes[in.TargetID] > domain.CoinMaxPerTarget {
			return domain.ErrCoinLimit
		}

		// 扣币：条件 UPDATE，RowsAffected=0 说明余额不足。
		res := tx.Model(&model.User{}).Where("userID = ? AND coins >= ?", in.UserID, in.Coins).
			UpdateColumn("coins", gorm.Expr("coins - ?", in.Coins))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrCoinNotEnough
		}

		// 流水幂等键序号：这条投币记录上「历史累计投到的第几枚」（含后来取消的）。
		// 用只增不减的 thrown_total，取消后再投也不会和旧流水撞 dedup_key。
		seq := throw.ThrownTotal + in.Coins
		if err := tx.Create(coinLedgerRow(domain.CoinGrantInput{
			UserID:     in.UserID,
			TargetType: in.TargetType,
			TargetID:   in.TargetID,
			EventType:  domain.CoinEventThrow,
			DedupKey:   domain.CoinDedupThrow(throw.ThrowID, seq),
			Amount:     -float64(in.Coins),
		})).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 同一序号已经记过：说明这笔已经投过了，不再重复扣币。
				return domain.ErrCoinLimit
			}
			return err
		}

		// 帖子作者分成：每收到 1 币 +0.1 币（小说不走这条）。
		// 与投币流水用同一事务，作者余额与分成流水要么都成功、要么都不落库。
		if in.TargetType == domain.CoinTargetPost && in.AuthorID > 0 && in.AuthorID != in.UserID {
			tip := domain.RoundCoin(float64(in.Coins) * domain.CoinAuthorTipPerCoin)
			if tip > 0 {
				if err := tx.Model(&model.User{}).Where("userID = ?", in.AuthorID).
					UpdateColumn("coins", gorm.Expr("coins + ?", tip)).Error; err != nil {
					return err
				}
				if err := tx.Create(coinLedgerRow(domain.CoinGrantInput{
					UserID:     in.AuthorID,
					TargetType: in.TargetType,
					TargetID:   in.TargetID,
					EventType:  domain.CoinEventPostTipIncome,
					DedupKey:   domain.CoinDedupPostTip(throw.ThrowID, seq),
					Amount:     tip,
				})).Error; err != nil {
					return err
				}
			}
		}

		if err := tx.Model(&model.CoinThrow{}).Where("throw_id = ?", throw.ThrowID).
			UpdateColumns(map[string]any{
				"coin_num":     held,
				"thrown_total": throw.ThrownTotal + in.Coins,
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return err
		}

		// 投币换来的主页经验：当场加到 score，每小时重算也会按同一口径再加一遍。
		if err := tx.Model(&model.User{}).Where("userID = ?", in.UserID).
			UpdateColumn("score", gorm.Expr("score + ?", in.Coins*domain.CoinExpPerCoin)).Error; err != nil {
			return err
		}

		// 被投内容的投币计数（列表/详情展示用，不是余额，不参与任何结算）。
		if err := adjustTargetCoins(tx, in.TargetType, in.TargetID, in.Coins); err != nil {
			return err
		}

		// 回给前端的 total 是「已投几枚」的展示口径：真币 + 旧投票折算，
		// 与 ThrownByMe（列表/详情里的 myCoins）保持一致。
		result.Total = held + legacyVotes[in.TargetID]
		result.Exp = in.Coins * domain.CoinExpPerCoin
		result.CoinsLeft = domain.RoundCoin(user.Coins - float64(in.Coins))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Cancel 取消投币：把我在该内容上投着的币全部撤回。
//
// 与 Throw 对称的反事务：
//   - 锁投币者 + 锁 coin_throw 行，一个都没投着（coin_num<=0，含只有历史投票折算的
//     情形——折算票不是真币，撤不了）直接返回 ErrCoinNotThrown；
//   - 退币 + 回退主页经验（口径与每小时经验重算一致：coin_throw.coin_num × 10）；
//   - 退款记一条正数流水（幂等键取只增的 thrown_total，反复投了退也不会撞键）；
//   - coin_num 清零，被投内容的投币计数同步减回去。
func (r *coinRepository) Cancel(ctx context.Context, userID int, targetType string, targetID int) (*domain.CoinThrowResult, error) {
	if userID <= 0 || targetID <= 0 {
		return nil, domain.ErrBadRequest
	}
	switch targetType {
	case domain.CoinTargetPost, domain.CoinTargetNovel:
	default:
		return nil, domain.ErrBadRequest
	}
	result := &domain.CoinThrowResult{
		Coins:      0,
		TargetType: targetType,
		TargetID:   targetID,
	}
	err := r.session(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := lockCoinUser(tx, userID)
		if err != nil {
			return err
		}
		var throw model.CoinThrow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND target_type = ? AND target_id = ?", userID, targetType, targetID).
			First(&throw).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCoinNotThrown
			}
			return err
		}
		if throw.CoinNum <= 0 {
			return domain.ErrCoinNotThrown
		}
		// 退款金额以流水为准：coin_num 是「已投几枚」的展示列，老数据里可能混进了
		// 旧投票折算（1 票 = 1 币，没扣过余额），按它退款会凭空造币。
		real, err := realCoinsFromLedger(tx, userID, targetType, targetID)
		if err != nil {
			return err
		}
		if real <= 0 {
			legacy, err := legacyVoteCoins(tx, userID, targetType, []int{targetID})
			if err != nil {
				return err
			}
			if legacy[targetID] > 0 {
				return domain.ErrCoinVoteLegacy
			}
			return domain.ErrCoinNotThrown
		}
		amount := throw.CoinNum
		if amount > real {
			amount = real
		}

		// 退币 + 回退主页经验（GREATEST 兜底，经验不会被回退成负数）。
		if err := tx.Model(&model.User{}).Where("userID = ?", userID).
			UpdateColumn("coins", gorm.Expr("coins + ?", amount)).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("userID = ?", userID).
			UpdateColumn("score", gorm.Expr("GREATEST(score - ?, 0)", amount*domain.CoinExpPerCoin)).Error; err != nil {
			return err
		}

		// 退款流水：正数入账，dedup 取当前 thrown_total（退款不推进它，
		// 但两次退款之间必然隔了一次投币，序号只会更大，不会撞键）。
		if err := tx.Create(coinLedgerRow(domain.CoinGrantInput{
			UserID:     userID,
			TargetType: targetType,
			TargetID:   targetID,
			EventType:  domain.CoinEventRefund,
			DedupKey:   domain.CoinDedupRefund(throw.ThrowID, throw.ThrownTotal),
			Amount:     float64(amount),
		})).Error; err != nil {
			if isDuplicateKeyErr(err) {
				// 同一序号已退过：说明这笔取消已经生效过，不再重复退款。
				return domain.ErrCoinNotThrown
			}
			return err
		}

		if err := tx.Model(&model.CoinThrow{}).Where("throw_id = ?", throw.ThrowID).
			UpdateColumns(map[string]any{"coin_num": 0, "updated_at": time.Now()}).Error; err != nil {
			return err
		}

		// 被投内容的投币计数减回去（GREATEST 已兜住不为负）。
		if err := adjustTargetCoins(tx, targetType, targetID, -amount); err != nil {
			return err
		}

		// 取消后「我投着几个」只剩历史投票折算的币（帖子可能有，小说恒为 0）。
		legacy, err := legacyVoteCoins(tx, userID, targetType, []int{targetID})
		if err != nil {
			return err
		}
		result.Coins = amount
		result.Total = legacy[targetID]
		result.Exp = -amount * domain.CoinExpPerCoin
		result.CoinsLeft = domain.RoundCoin(user.Coins + float64(amount))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// adjustTargetCoins 把投币数累到被投内容上（posts.coin_num / novels.coin_num）。
func adjustTargetCoins(tx *gorm.DB, targetType string, targetID, delta int) error {
	switch targetType {
	case domain.CoinTargetPost:
		return tx.Model(&model.Post{}).Where("postID = ?", targetID).
			UpdateColumn("coin_num", gorm.Expr("GREATEST(coin_num + ?, 0)", delta)).Error
	case domain.CoinTargetNovel:
		return tx.Model(&model.Novel{}).Where("novel_id = ?", targetID).
			UpdateColumn("coin_num", gorm.Expr("GREATEST(coin_num + ?, 0)", delta)).Error
	default:
		return domain.ErrBadRequest
	}
}

// Balance 当前币余额。
func (r *coinRepository) Balance(ctx context.Context, userID int) (float64, error) {
	var u model.User
	if err := r.session(ctx).Select("coins").Where("userID = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	return u.Coins, nil
}

// ExistsDedup 幂等键是否已存在（今天这项任务是否已经发过）。
func (r *coinRepository) ExistsDedup(ctx context.Context, dedupKey string) (bool, error) {
	if dedupKey == "" {
		return false, nil
	}
	var count int64
	if err := r.session(ctx).Model(&model.CoinLedger{}).Where("dedup_key = ?", dedupKey).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// AccumulatedByDedup 某条累计行的游标值（今天读了多少秒、更新了多少字）。
func (r *coinRepository) AccumulatedByDedup(ctx context.Context, dedupKey string) (int, error) {
	if dedupKey == "" {
		return 0, nil
	}
	var row model.CoinLedger
	if err := r.session(ctx).Select("settled_value").Where("dedup_key = ?", dedupKey).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.SettledValue, nil
}

// SumSigned 指定事件的收支合计（带符号）。
func (r *coinRepository) SumSigned(ctx context.Context, userID int, eventTypes ...string) (float64, error) {
	if userID <= 0 || len(eventTypes) == 0 {
		return 0, nil
	}
	var total *float64
	if err := r.session(ctx).Model(&model.CoinLedger{}).
		Where("user_id = ? AND event_type IN ?", userID, eventTypes).
		Select("COALESCE(SUM(amount), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// ThrownByMe 我在这些内容上各投了几个币。
func (r *coinRepository) ThrownByMe(ctx context.Context, userID int, targetType string, targetIDs []int) (map[int]int, error) {
	out := make(map[int]int, len(targetIDs))
	if userID <= 0 || len(targetIDs) == 0 {
		return out, nil
	}
	type row struct {
		TargetID int
		CoinNum  int
	}
	var rows []row
	if err := r.session(ctx).Model(&model.CoinThrow{}).
		Select("target_id", "coin_num").
		Where("user_id = ? AND target_type = ? AND target_id IN ?", userID, targetType, targetIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.TargetID] = x.CoinNum
	}
	// 历史投票折算进来的币也要显示出来，否则前端显示的「已投 N 币」和上限口径不一致。
	legacy, err := legacyVoteCoins(r.session(ctx), userID, targetType, targetIDs)
	if err != nil {
		return nil, err
	}
	for id, n := range legacy {
		out[id] += n
	}
	return out, nil
}

// realCoinsFromLedger 流水口径下我在这条内容上真投着几个币：
// 投币流水（负数）+ 退款流水（正数）取反。流水是唯一不会说谎的地方——
// coin_num 可能被旧数据污染，退款金额必须按流水算，否则取消投币会凭空造币。
func realCoinsFromLedger(tx *gorm.DB, userID int, targetType string, targetID int) (int, error) {
	var net *int
	if err := tx.Model(&model.CoinLedger{}).
		Where("user_id = ? AND target_type = ? AND target_id = ? AND event_type IN ?",
			userID, targetType, targetID, []string{domain.CoinEventThrow, domain.CoinEventRefund}).
		Select("COALESCE(SUM(amount), 0)").Scan(&net).Error; err != nil {
		return 0, err
	}
	if net == nil {
		return 0, nil
	}
	return -*net, nil
}

// legacyVoteCoins 历史投票折算：投票功能已下线，pvotes 表只读不再写入，
// 一人一票折算成一个「已经投过的币」，用于投币上限与「我投了几个」的口径。
// 只有帖子有历史投票，小说恒为空。折算只影响上限与展示，不给投币者补主页经验。
func legacyVoteCoins(db *gorm.DB, userID int, targetType string, targetIDs []int) (map[int]int, error) {
	out := map[int]int{}
	if userID <= 0 || len(targetIDs) == 0 || targetType != domain.CoinTargetPost {
		return out, nil
	}
	type row struct {
		PostID int
		Cnt    int
	}
	var rows []row
	if err := db.Model(&model.Pvote{}).
		Select("postID", "COUNT(*) AS cnt").
		Where("userID = ? AND postID IN ?", userID, targetIDs).
		Group("postID").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.PostID] = x.Cnt
	}
	return out, nil
}

// ThrownForTarget 某个内容被投币总数。
func (r *coinRepository) ThrownForTarget(ctx context.Context, targetType string, targetID int) (int, error) {
	if targetID <= 0 {
		return 0, nil
	}
	var total *int
	if err := r.session(ctx).Model(&model.CoinThrow{}).
		Select("COALESCE(SUM(coin_num), 0)").
		Where("target_type = ? AND target_id = ?", targetType, targetID).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// ThrownTotalByUser 我累计投出去多少币（经验重算用）。
func (r *coinRepository) ThrownTotalByUser(ctx context.Context, userID int) (int, error) {
	if userID <= 0 {
		return 0, nil
	}
	var total *int
	if err := r.session(ctx).Model(&model.CoinThrow{}).
		Select("COALESCE(SUM(coin_num), 0)").
		Where("user_id = ?", userID).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}

// ThrownTotalsAll 全站「用户 → 投币总数」一张表：经验定时重算只查一次，
// 不在按用户循环里逐个查。
func (r *coinRepository) ThrownTotalsAll(ctx context.Context) (map[int]int, error) {
	type row struct {
		UserID  int
		CoinNum int
	}
	var rows []row
	if err := r.session(ctx).Model(&model.CoinThrow{}).
		Select("user_id", "COALESCE(SUM(coin_num), 0) AS coin_num").
		Group("user_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int]int, len(rows))
	for _, x := range rows {
		out[x.UserID] = x.CoinNum
	}
	return out, nil
}

// List 我的币流水（分页，纯计数行不露出）。
func (r *coinRepository) List(ctx context.Context, userID, limit, offset int) ([]*domain.CoinLedger, int64, error) {
	if userID <= 0 {
		return nil, 0, domain.ErrBadRequest
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	base := r.session(ctx).Model(&model.CoinLedger{}).
		Where("user_id = ? AND event_type IN ?", userID, coinDisplayEvents)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.CoinLedger
	if err := base.Order("created_at DESC, ledger_id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*domain.CoinLedger, 0, len(rows))
	for i := range rows {
		out = append(out, coinLedgerToDomain(&rows[i]))
	}
	return out, total, nil
}

func coinLedgerToDomain(l *model.CoinLedger) *domain.CoinLedger {
	return &domain.CoinLedger{
		ID:           l.LedgerID,
		UserID:       l.UserID,
		TargetType:   l.TargetType,
		TargetID:     l.TargetID,
		EventType:    l.EventType,
		Label:        coinLedgerLabel(l),
		Amount:       l.Amount,
		SettledValue: l.SettledValue,
		DedupKey:     l.DedupKey,
		CreatedAt:    l.CreatedAt,
	}
}

// coinLedgerLabel 流水说明：投币/取消投币行带上目标类型，其余用事件名。
func coinLedgerLabel(l *model.CoinLedger) string {
	label := domain.CoinEventLabel(l.EventType)
	if l.TargetType == "" {
		return label
	}
	switch l.EventType {
	case domain.CoinEventThrow:
		return "投币 · " + domain.CoinTargetLabel(l.TargetType)
	case domain.CoinEventRefund:
		return "取消投币 · " + domain.CoinTargetLabel(l.TargetType)
	}
	return label
}
