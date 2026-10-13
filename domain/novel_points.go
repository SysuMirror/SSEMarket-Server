package domain

import (
	"errors"
	"strconv"
	"time"
)

// 小说积分与经验体系。
//
// 两种货币：
//   - 经验 experience：读者/用户个人成长，整数；
//   - 积分 novel_points：作者收益，**以「分」为单位记账**（1 积分 = 10 分），
//     这样「阅读人数每人 +0.1 积分」也能用整数精确记账。
//
// 所有发放都写 novel_ledger 流水，靠 dedup_key 唯一索引保证幂等防刷；
// 里程碑类事件（阅读时长、写作字数）额外用 settled_value 记游标，
// amount 记「已发放份数」，重复上报同一秒/同一字不会重复发放。

// 流水事件类型。
const (
	LedgerEventReadTime    = "read_time"    // 阅读时长里程碑（读者经验）
	LedgerEventReadDaily   = "read_daily"   // 当日阅读秒数累计（纯计数，不发放）
	LedgerEventReadStreak  = "read_streak"  // 当日阅读达标奖励
	LedgerEventReadBonus   = "read_bonus"   // 连续阅读加成（昨日也达标）
	LedgerEventComment     = "comment"      // 发表评论（读者经验）
	LedgerEventCommentLike = "comment_like" // 评论被点赞（评论作者经验）
	LedgerEventShelfAdd    = "shelf_add"    // 被加入书架（作者积分）
	LedgerEventReaderUV    = "reader_uv"    // 阅读人数 UV（作者积分）
	LedgerEventWriteWords  = "write_words"  // 写作字数里程碑（作者积分）
	LedgerEventChapterPub  = "chapter_pub"  // 章节更新（作者积分）
	LedgerEventCheckIn     = "check_in"     // 签到（用户经验）
	LedgerEventRewardIn    = "reward_in"    // 收到打赏（作者积分）
	LedgerEventRewardOut   = "reward_out"   // 打赏支出（读者经验，负数）
)

// 发放目标列（白名单，repository 只接受这两个值，避免拼列名注入）。
const (
	RewardTargetExperience = "experience"
	RewardTargetPoints     = "novel_points"
)

// 规则表（数值口径集中在这里，改规则只改这一处）。
const (
	// 阅读：每满 1 小时 +1 经验。
	NovelReadUnitSeconds = 3600
	NovelExpReadHour     = 1
	// 心跳单次最多计入的秒数（防止一次上报灌进几小时）。
	NovelReadHeartbeatMaxSeconds = 120

	// 连续阅读：当日累计阅读满 30 分钟 +2 经验，昨日也达标再 +2。
	NovelReadDailyUnitSeconds = 1800
	NovelExpReadDaily         = 2
	NovelExpReadBonus         = 2

	// 评论：每条首次计入 +2 经验，单用户单日上限 20 经验（防灌水）。
	NovelExpComment      = 2
	NovelCommentDailyCap = 20

	// 评论被点赞：同一人同一评论仅一次 +1 经验，单条评论最多获得 20。
	NovelExpCommentLike   = 1
	NovelCommentLikeTotal = 20

	// 作者：被加入书架每人 +5 积分（同人同书仅首次）。
	NovelPointShelfAdd = 50 // 分

	// 作者：阅读人数每人 +0.1 积分（按本独立、UV 去重、仅首次）。
	NovelPointReaderUV = 1 // 分

	// 作者：写作每满 5000 字 +1 积分。
	NovelWriteUnitWords = 5000
	NovelPointWrite     = 10 // 分

	// 作者：章节更新每章 +2 积分，每日最多计 5 章。
	NovelPointChapter    = 20 // 分
	NovelChapterDailyCap = 100

	// 签到：基础 +2 经验，连续签到每天额外 +1（加成最多 +5）。
	NovelExpCheckIn       = 2
	NovelCheckInStreakMax = 5

	// 打赏：读者用经验换作者积分，同一读者对同一本书每天最多 1 次。
	NovelRewardMinPoints = 10
	NovelRewardMaxPoints = 1000

	// 积分记账单位：1 积分 = 10 分。
	NovelPointUnitPerScore = 10
)

// 积分体系业务错误。
var (
	ErrNovelNotEnoughExperience = errors.New("经验不足，无法打赏")
	ErrNovelRewardSelf          = errors.New("不能给自己打赏")
	ErrNovelRewardDailyLimit    = errors.New("今天已经给这本书打赏过了")
	ErrNovelAlreadyCheckedIn    = errors.New("今天已经签到过了")
)

// NovelLedger 积分/经验流水。
type NovelLedger struct {
	ID           int       `json:"ledgerID"`
	UserID       int       `json:"userID"`
	NovelID      int       `json:"novelID"`
	EventType    string    `json:"eventType"`
	Amount       int       `json:"amount"`
	SettledValue int       `json:"settledValue"`
	DedupKey     string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

// NovelGrantInput 一次性发放入参（幂等由 DedupKey 保证）。
type NovelGrantInput struct {
	UserID    int
	NovelID   int
	EventType string
	DedupKey  string
	Amount    int
	Target    string // RewardTargetExperience / RewardTargetPoints
}

// NovelMilestoneInput 里程碑结算入参：Delta 是本次新增的原始量（秒/字），
// 每满 Unit 发放 Reward 份。
type NovelMilestoneInput struct {
	UserID    int
	NovelID   int
	EventType string
	DedupKey  string
	Delta     int
	Unit      int
	Reward    int
	Target    string
}

// NovelRewardInput 打赏入参：Points 是作者收到的积分，Fen 是对应的「分」（Points × 10），
// 打赏者扣掉的经验数值等于 Points。
type NovelRewardInput struct {
	FromUserID int
	AuthorID   int
	NovelID    int
	DedupKey   string
	Points     int
	Fen        int
}

// NovelPointsSummary 我的积分/经验总览。
type NovelPointsSummary struct {
	Experience    int            `json:"experience"`
	PointsFen     int            `json:"pointsFen"`   // 以「分」计
	PointsScore   float64        `json:"pointsScore"` // 换算成「积分」，展示用
	ByEvent       map[string]int `json:"byEvent"`
	TodayExp      int            `json:"todayExp"`
	CheckInDay    string         `json:"checkInDay"`
	CheckInStreak int            `json:"checkInStreak"`
}

// NovelCheckInResult 签到结果。
type NovelCheckInResult struct {
	Amount  int    `json:"amount"`
	Streak  int    `json:"streak"`
	Day     string `json:"day"`
	Already bool   `json:"already"`
}

// NovelRewardResult 打赏结果。
type NovelRewardResult struct {
	Points     int `json:"points"`     // 作者收到的积分
	Cost       int `json:"cost"`       // 打赏者扣掉的等量经验
	Experience int `json:"experience"` // 打赏者剩余经验
}

// NovelLedgerDay 把时间归到「本地日期」，签到/每日上限都按这个口径。
func NovelLedgerDay(t time.Time) string {
	return t.Format("2006-01-02")
}

// NovelLedgerDayShift 取某日往前 n 天的日期（n 可为负）。
func NovelLedgerDayShift(day string, n int) string {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return day
	}
	return NovelLedgerDay(t.AddDate(0, 0, n))
}

// NovelLedgerDayStart 某日 0 点（本地），用于「今日已发放」统计。
func NovelLedgerDayStart(day string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return time.Now().AddDate(0, 0, -1)
	}
	return t
}

// ---------------------------------------------------------------------------
// dedup_key 构造：所有幂等键集中在这里，避免各处拼字符串口径不一致。
// ---------------------------------------------------------------------------

func NovelDedupMilestone(eventType string, userID, novelID int) string {
	return eventType + ":" + strconv.Itoa(userID) + ":" + strconv.Itoa(novelID)
}

func NovelDedupOnce(eventType string, ids ...int) string {
	key := eventType
	for _, id := range ids {
		key += ":" + strconv.Itoa(id)
	}
	return key
}

func NovelDedupDay(eventType string, userID int, day string) string {
	return eventType + ":" + strconv.Itoa(userID) + ":" + day
}

// NovelDedupReward 打赏幂等键：同一读者对同一本书每天最多一笔。
func NovelDedupReward(fromUserID, novelID int, day string) string {
	return LedgerEventRewardOut + ":" + strconv.Itoa(fromUserID) + ":" + strconv.Itoa(novelID) + ":" + day
}

// NovelPointsScore 把「分」换算成展示用的「积分」。
func NovelPointsScore(fen int) float64 {
	return float64(fen) / float64(NovelPointUnitPerScore)
}

// NovelEventLabel 流水事件的中文说明（前端展示用）。
func NovelEventLabel(eventType string) string {
	switch eventType {
	case LedgerEventReadTime:
		return "阅读满 1 小时"
	case LedgerEventReadDaily:
		return "当日阅读累计"
	case LedgerEventReadStreak:
		return "连续阅读达标"
	case LedgerEventReadBonus:
		return "连续阅读加成"
	case LedgerEventComment:
		return "发表评论"
	case LedgerEventCommentLike:
		return "评论被点赞"
	case LedgerEventShelfAdd:
		return "被加入书架"
	case LedgerEventReaderUV:
		return "新增在读读者"
	case LedgerEventWriteWords:
		return "写作字数里程碑"
	case LedgerEventChapterPub:
		return "章节更新"
	case LedgerEventCheckIn:
		return "签到"
	case LedgerEventRewardIn:
		return "收到打赏"
	case LedgerEventRewardOut:
		return "打赏支出"
	default:
		return eventType
	}
}

// NovelEventCurrency 该事件发放的货币。
func NovelEventCurrency(eventType string) string {
	switch eventType {
	case LedgerEventShelfAdd, LedgerEventReaderUV, LedgerEventWriteWords, LedgerEventChapterPub, LedgerEventRewardIn:
		return "point"
	default:
		return "exp"
	}
}
