package domain

import (
	"errors"
	"math"
	"strconv"
	"time"
)

// 站点「币」体系（与小说的经验/积分体系完全独立，各记各的流水）。
//
// 币只有两个用途：
//   - 赚币：每日进入网页、每日首次发帖、每日阅读满 30 分钟、每日更新写作满 4000 字、收到评分；
//   - 投币：给帖子或小说投币，每人对每个内容最多 2 币；投币者按每个币 +10 主页经验（users.score）。
//
// 余额记在 users.coins；收支记 coin_ledger 流水（dedup_key 唯一索引保证幂等）；
// 「谁给哪个内容投了几个币」记 coin_throw（唯一索引兜住每人每内容最多 2 币）。
// 投币换来的主页经验既在投币当场加到 score，也被每小时的经验重算重新算一遍，
// 两边口径都是「投币总数 × 10」，所以重算不会把投币经验冲掉。

// 币流水事件类型。
const (
	CoinEventVisitDaily = "visit_daily" // 每日进入网页 +1 币
	CoinEventPostDaily  = "post_daily"  // 每日首次发帖 +1 币
	CoinEventReadDaily  = "read_daily"  // 当日阅读累计满 30 分钟 +1 币
	CoinEventWriteDaily = "write_daily" // 当日更新写作满 4000 字 +1 币
	CoinEventRated      = "rated"       // 收到评分（作者 +1 币）
	// CoinEventProgrammersDay2026 2026 程序员节全站福利（每人 +10.24 币）。
	CoinEventProgrammersDay2026 = "programmers_day_2026"
	// CoinEventPostTipIncome 帖子作者收到的投币分成（每 1 币给作者 +0.1）。
	CoinEventPostTipIncome = "post_tip_income"
	CoinEventThrow         = "coin_throw"  // 投币支出（负数）
	CoinEventRefund        = "coin_refund" // 取消投币退款（正数）

	// 纯计数事件：只推进 settled_value 游标，不发放，也不出现在总览/流水里。
	CoinAccumulateRead  = "acc_read"  // 当日阅读秒数累计
	CoinAccumulateWrite = "acc_write" // 当日更新字数累计
)

// 规则表（数值口径集中在这里，改规则只改这一处）。
const (
	// 每天进入网页给 1 币：前端进站点时上报一次，幂等键含日期，一天只给一次。
	CoinVisitDaily = 1

	// 每日首次发帖 +1 币（同一天第二帖不再给）。
	CoinPostDaily = 1

	// 当日阅读累计满 30 分钟 +1 币。
	CoinReadDailySeconds = 1800
	CoinReadDaily        = 1

	// 当日更新写作满 4000 字 +1 币（「正常更新」= 正常发布章节的正文字数，图片不计）。
	CoinWriteDailyWords = 4000
	CoinWriteDaily      = 1

	// 收到一条评分，作者 +1 币（同一读者对同一本书只算一次）。
	CoinRated = 1

	// 投币：每人对每个内容最多 2 币，每个币给投币者 +10 主页经验。
	CoinMaxPerTarget = 2
	CoinExpPerCoin   = 10

	// CoinAuthorTipPerCoin 帖子每收到 1 币，作者获得 0.1 币分成（小说不走这条规则）。
	CoinAuthorTipPerCoin = 0.1

	// CoinProgrammersDay2026Grant 2026 程序员节全站福利（每人 10.24 币）。
	CoinProgrammersDay2026Grant = 10.24
)

// RoundCoin 币金额统一保留两位小数，避免浮点误差进入余额/流水。
func RoundCoin(value float64) float64 {
	return math.Round(value*100) / 100
}

// 投币目标类型。
const (
	CoinTargetPost  = "post"
	CoinTargetNovel = "novel"
)

// 币体系业务错误。
var (
	ErrCoinNotEnough = errors.New("币不够，无法投币")
	ErrCoinLimit     = errors.New("已经给这个内容投满 2 币了")
	ErrCoinSelf      = errors.New("不能给自己的内容投币")
	ErrCoinNotThrown = errors.New("还没有投过币，无法取消")
	// ErrCoinVoteLegacy 这条内容上的「已投 N 币」全是旧投票折算出来的，
	// 折算票不是真币、没有扣过余额，所以不能撤回（撤回会凭空造币）。
	ErrCoinVoteLegacy = errors.New("旧投票折算的币不能撤回")
)

// CoinLedgerDay 把时间归到「本地日期」，每日任务都按这个口径。
func CoinLedgerDay(t time.Time) string {
	return t.Format("2006-01-02")
}

// CoinLedgerDayStart 某日 0 点（本地），用于「当日累计」查询。
func CoinLedgerDayStart(day string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return time.Now().AddDate(0, 0, -1)
	}
	return t
}

// ---------------------------------------------------------------------------
// dedup_key 构造：所有幂等键集中在这里，避免各处拼字符串口径不一致。
// ---------------------------------------------------------------------------

// CoinDedupDay 每日一次的任务：post_daily / read_daily / write_daily。
func CoinDedupDay(eventType string, userID int, day string) string {
	return eventType + ":" + strconv.Itoa(userID) + ":" + day
}

// CoinDedupAccumulate 当日原始量累计行（阅读秒数、写作字数）。
func CoinDedupAccumulate(eventType string, userID int, day string) string {
	return eventType + ":" + strconv.Itoa(userID) + ":" + day
}

// CoinDedupRated 收到评分：同一读者对同一本书只发一次。
func CoinDedupRated(authorID, novelID, raterID int) string {
	return CoinEventRated + ":" + strconv.Itoa(authorID) + ":" + strconv.Itoa(novelID) + ":" + strconv.Itoa(raterID)
}

// CoinDedupThrow 投币流水：throwID 定位「谁在哪个内容上的投币记录」，
// seq 是这条记录上历史累计投到的第几枚（coin_throw.thrown_total，取消不减）。
//
// 键里带 throwID 是刻意的：旧版键形如 coin_throw:post:12:34:2，序号取自「投完后的
// 累计枚数」；取消投币上线后序号改取只增不减的 thrown_total，老库没回填该列时
// 两者会撞键，表现为「取消投币后再投被误判成已投满 2 币」。换前缀后新键与历史
// 键彻底分开，不依赖回填也能正确工作。
func CoinDedupThrow(throwID, seq int) string {
	return CoinEventThrow + ":" + strconv.Itoa(throwID) + ":" + strconv.Itoa(seq)
}

// CoinDedupPostTip 帖子作者投币分成：同一笔投币（throwID + 第几枚）只给一次。
func CoinDedupPostTip(throwID, seq int) string {
	return CoinEventPostTipIncome + ":" + strconv.Itoa(throwID) + ":" + strconv.Itoa(seq)
}

// CoinDedupRefund 取消投币的退款流水：同一笔投币最多退一次。
// refundSeq 取当前 thrown_total —— 两次退款之间必然隔了一次投币，序号只会更大。
func CoinDedupRefund(throwID, refundSeq int) string {
	return CoinEventRefund + ":" + strconv.Itoa(throwID) + ":" + strconv.Itoa(refundSeq)
}

// CoinTargetLabel 投币目标的中文说明（流水展示用）。
func CoinTargetLabel(targetType string) string {
	switch targetType {
	case CoinTargetPost:
		return "帖子"
	case CoinTargetNovel:
		return "小说"
	default:
		return targetType
	}
}

// CoinEventLabel 币流水事件的中文说明（前端展示用）。
func CoinEventLabel(eventType string) string {
	switch eventType {
	case CoinEventVisitDaily:
		return "每日进入网页"
	case CoinEventPostDaily:
		return "每日首次发帖"
	case CoinEventReadDaily:
		return "当日阅读满 30 分钟"
	case CoinEventWriteDaily:
		return "当日更新满 4000 字"
	case CoinEventRated:
		return "收到评分"
	case CoinEventThrow:
		return "投币"
	case CoinEventRefund:
		return "取消投币"
	case CoinEventProgrammersDay2026:
		return "程序员节发放"
	case CoinEventPostTipIncome:
		return "帖子获投币分成"
	default:
		return eventType
	}
}

// CoinLedger 币流水。
type CoinLedger struct {
	ID           int       `json:"ledgerID"`
	UserID       int       `json:"userID"`
	TargetType   string    `json:"targetType"`
	TargetID     int       `json:"targetID"`
	EventType    string    `json:"event"`
	Label        string    `json:"label"`
	Amount       float64   `json:"amount"`
	SettledValue int       `json:"settledValue"`
	DedupKey     string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

// CoinGrantInput 一次性发币入参（幂等由 DedupKey 保证）。
type CoinGrantInput struct {
	UserID     int
	TargetType string
	TargetID   int
	EventType  string
	DedupKey   string
	Amount     float64
}

// CoinThrowInput 投币入参。
type CoinThrowInput struct {
	UserID int
	// AuthorID 被投币内容的作者：用于帖子分成（每 1 币 +0.1）；自身不能给自己投币由服务层拦截。
	AuthorID   int
	TargetType string
	TargetID   int
	Coins      int
}

// CoinThrowResult 投币结果。
type CoinThrowResult struct {
	Coins      int     `json:"coins"`     // 这次投了几个币
	Total      int     `json:"total"`     // 这个内容上我累计投了几个币
	Exp        int     `json:"exp"`       // 这次换来的主页经验
	CoinsLeft  float64 `json:"coinsLeft"` // 投币后剩余币（保留两位小数）
	TargetType string  `json:"targetType"`
	TargetID   int     `json:"targetID"`
}

// CoinTask 一项「今天怎么赚币」的进度，前端直接渲染成清单。
type CoinTask struct {
	Event     string `json:"event"`
	Label     string `json:"label"`
	Target    int    `json:"target"`    // 达标需要的原始量（1 帖 / 1800 秒 / 4000 字）
	Progress  int    `json:"progress"`  // 今天已经做到多少
	Reward    int    `json:"reward"`    // 达标给几个币
	Done      bool   `json:"done"`      // 今天已经发过
	Available bool   `json:"available"` // 这项任务今天还能不能拿（已发过就 false）
}

// CoinSummary 我的币总览。
type CoinSummary struct {
	Coins       float64    `json:"coins"`
	Day         string     `json:"day"`
	Tasks       []CoinTask `json:"tasks"`
	ThrownTotal int        `json:"thrownTotal"` // 累计投出去多少币
	ThrownExp   int        `json:"thrownExp"`   // 投币换来的主页经验（= 投币数 × 10）
	Received    float64    `json:"received"`    // 累计收到的币（发帖/阅读/写作/评分/投币分成）
}
