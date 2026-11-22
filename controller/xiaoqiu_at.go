package controller

// @小秋 论坛提及（2026-11-01）：发帖/评论/楼中楼创建成功后，若正文含 "@小秋"，
// 异步把语境+诉求交给小秋 agent（chat 面 /xq-internal/agent/mention，公网不可达，
// 仅 host.docker.internal 直连），拿回排好版的 markdown，以小秋 bot 账号进程内发成回帖。
//
// 语义要点（与方案文档一致）：
//   - XQ_BOT_UID 未配置 = 功能整体静默（灰度开关）；
//   - goroutine 用 context.Background()——绝不能用 c.Request.Context()，响应一返回就取消；
//   - 回帖身份 = bot：帖子@ → CreatePcomment；评论@ → CreateCcomment(带 Target)；
//     楼中楼@ → CreateCcomment(带 ReplyCcommentID)——服务层既有分支会通知到发起人；
//   - bot 回帖不碰 RagNotify（controller 侧不调即天然不进检索库，防自我强化）；
//   - 确定性失败（非 200/解析错/code!=0/空 answer）才发兜底短评；
//     私密帖在 FindByID(bot 视角) 即 ErrNotFound，静默跳过；
//   - 去重双闸：trigger 级 sync.Map（防编辑重放/重启后 Go 侧重发——agent 端 409 兜底）
//     + actor 60s 滑窗（与 agent 端冷却同值，先在这里挡一道省一次往返）。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"loginTest/domain"
	"loginTest/service"
)

const (
	xqAtMarker        = "@小秋"
	xqAtTaskTimeout   = 900 * time.Second // 比 agent 侧 840s 内部超时宽裕，留回程
	xqAtActorCooldown = 60 * time.Second
	xqAtReplyMaxRunes = 2800 // comment_service 对 bot 的放宽上限（DB varchar(4000)）
)

var (
	xqAtURL   = ragEnvStr("XQ_AT_URL", "http://host.docker.internal:8110/xq-internal/agent/mention")
	xqAtToken = os.Getenv("INTERNAL_TOKEN") // 与 agent INTERNAL_TOKEN 同值（已 hash 核对三方一致）
	xqBotUID  = xqEnvInt("XQ_BOT_UID")
	// trigger 去重：kind:triggerID -> 受理时间；actor 滑窗：uid -> 上次受理时间
	xqAtSeen       sync.Map
	xqAtActorSeen  sync.Map
	xqAtCheckCount atomic.Int64
)

func xqEnvInt(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if v < 0 {
		return 0
	}
	return v
}

// xqAtFeatureOn 三要素齐了才开：bot uid、内部 token、mention URL。
func xqAtFeatureOn() bool {
	return xqBotUID > 0 && xqAtToken != "" && xqAtURL != ""
}

func xqAtRuneTrunc(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// xqAtExtractAsk 取 "@小秋" 之后到文本尾作为诉求；去首尾标点空白；空诉求回整条正文。
func xqAtExtractAsk(content string) string {
	idx := strings.Index(content, xqAtMarker)
	if idx < 0 {
		return strings.TrimSpace(content)
	}
	ask := strings.Trim(content[idx+len(xqAtMarker):], " \t\r\n")
	ask = strings.Trim(ask, "：:，,.。；;、！!？?")
	ask = strings.TrimSpace(ask)
	if ask == "" {
		return strings.TrimSpace(content)
	}
	return ask
}

// xqAtSweep 懒清扫：每 8192 次检查把两张表里超过 1h 的条目删掉（表极小，够用）。
func xqAtSweep() {
	if xqAtCheckCount.Add(1)%8192 != 0 {
		return
	}
	now := time.Now()
	exp := 1 * time.Hour
	xqAtSeen.Range(func(k, v any) bool {
		if t, ok := v.(time.Time); ok && now.Sub(t) > exp {
			xqAtSeen.Delete(k)
		}
		return true
	})
	xqAtActorSeen.Range(func(k, v any) bool {
		if t, ok := v.(time.Time); ok && now.Sub(t) > exp {
			xqAtActorSeen.Delete(k)
		}
		return true
	})
}

// TryHandleAtMention 是三个创建成功路径的统一挂钩点，同步部分只做廉价门控，
// 其余全在 goroutine 里。kind: "post" | "pcomment" | "ccomment"。
func TryHandleAtMention(kind string, triggerID, postID, pcommentID, mentionerUID int, mentionerName, triggerContent string) {
	if !xqAtFeatureOn() || triggerID == 0 || postID == 0 {
		return
	}
	if mentionerUID == xqBotUID { // bot 自己的回帖不再触发
		return
	}
	if !strings.Contains(triggerContent, xqAtMarker) {
		return
	}
	xqAtSweep()
	// 先查 actor 滑窗再消费 trigger 键：反过来会把冷却期内的新评论永久标记成"已处理"
	if last, ok := xqAtActorSeen.Load(mentionerUID); ok {
		if t, isTime := last.(time.Time); isTime && time.Since(t) < xqAtActorCooldown {
			return // 该用户 60s 内已触发过
		}
	}
	key := fmt.Sprintf("%s:%d", kind, triggerID)
	if _, loaded := xqAtSeen.LoadOrStore(key, time.Now()); loaded {
		return // 同一条评论只跑一次（重放/重启兜底）
	}
	xqAtActorSeen.Store(mentionerUID, time.Now())

	switch kind {
	case "post", "pcomment", "ccomment":
	default:
		return
	}
	if kind != "post" && pcommentID == 0 {
		return
	}

	go xqAtRun(kind, triggerID, postID, pcommentID, mentionerUID, mentionerName, triggerContent)
}

func xqAtRun(kind string, triggerID, postID, pcommentID, mentionerUID int, mentionerName, triggerContent string) {
	ctx, cancel := context.WithTimeout(context.Background(), xqAtTaskTimeout)
	defer cancel()

	// bot 视角取帖：私密帖/已删帖 → ErrNotFound → 静默跳过（不发兜底，避免暴露存在性）
	post, err := service.Posts.FindByID(ctx, postID, xqBotUID)
	if err != nil || post == nil {
		if err != nil && err != domain.ErrNotFound {
			log.Printf("[xq-at] FindByID post=%d failed: %v", postID, err)
		}
		return
	}

	var where, ctxExcerpt, ctxThread, ctxTrigger string
	switch kind {
	case "post":
		where = fmt.Sprintf("帖子《%s》（postID=%d，发起人即楼主）", post.Title, post.ID)
	case "pcomment":
		where = fmt.Sprintf("帖子《%s》（postID=%d）的一级评论（pcommentID=%d）下", post.Title, post.ID, pcommentID)
		pc, e := service.Comments.FindPcomment(ctx, pcommentID)
		if e == nil && pc != nil {
			ctxExcerpt = xqAtRuneTrunc(pc.Content, 500)
		}
		// 楼内讨论上下文：小秋要能接住"上面大家聊了啥"式的追问
		ctxThread, _ = service.Comments.XqAtThreadDigest(ctx, pcommentID, 0, 15, 200)
	case "ccomment":
		where = fmt.Sprintf("帖子《%s》（postID=%d）的楼中楼（ccommentID=%d）中", post.Title, post.ID, triggerID)
		pc, e := service.Comments.FindPcomment(ctx, pcommentID)
		if e == nil && pc != nil {
			ctxExcerpt = xqAtRuneTrunc(pc.Content, 500)
		}
		// 触发的那条楼中楼全文（诉求提取只截 @小秋 之后，前半句也是话）
		if tc, e := service.Comments.FindCcomment(ctx, triggerID); e == nil && tc != nil {
			ctxTrigger = xqAtRuneTrunc(tc.Content, 500)
		}
		ctxThread, _ = service.Comments.XqAtThreadDigest(ctx, pcommentID, triggerID, 15, 200)
	}

	ask := xqAtExtractAsk(triggerContent)
	var b strings.Builder
	fmt.Fprintf(&b, "【集市@小秋】有人在论坛里 @了你。\n位置：%s\n", where)
	fmt.Fprintf(&b, "帖子内容节选：%s\n", xqAtRuneTrunc(post.Content, 2000))
	if ctxExcerpt != "" {
		fmt.Fprintf(&b, "所在一级评论内容节选：%s\n", ctxExcerpt)
	}
	if ctxTrigger != "" {
		fmt.Fprintf(&b, "喊你的这条评论全文：%s\n", ctxTrigger)
	}
	if ctxThread != "" {
		fmt.Fprintf(&b, "所在楼讨论上下文（按时间序）：\n%s", ctxThread)
	}
	if mentionerName != "" && !post.IsAnonymous {
		fmt.Fprintf(&b, "发起人：%s\n", mentionerName)
	}
	fmt.Fprintf(&b, "诉求：%s", xqAtRuneTrunc(ask, 2000))
	task := b.String()

	sessionID := fmt.Sprintf("xqat-%s-%d", kind, triggerID)
	answer, fail := xqAtCallAgent(ctx, sessionID, mentionerUID, task)
	if fail != "" {
		log.Printf("[xq-at] %s trigger=%d 未成功：%s", kind, triggerID, fail)
	}

	content := answer
	if fail != "" {
		content = "小秋开小差了，这次没接住～稍后再 @小秋 试试。"
	}
	content = xqAtRuneTrunc(content, xqAtReplyMaxRunes-1)
	if utf8.RuneCountInString(answer) > xqAtReplyMaxRunes && fail == "" {
		content += "…"
	}

	switch kind {
	case "post":
		if _, err := service.Comments.CreatePcomment(ctx, domain.PcommentCreateInput{
			UserID:  xqBotUID,
			PostID:  postID,
			Content: content,
		}); err != nil {
			log.Printf("[xq-at] CreatePcomment bot reply post=%d failed: %v", postID, err)
		}
	default:
		// 匿名帖：回帖元数据不带发起人身份（结构通知照发：pc 作者/ReplyCcommentID 分支）
		targetName, targetUID := mentionerName, mentionerUID
		if post.IsAnonymous {
			targetName, targetUID = "", 0
		}
		in := domain.CcommentCreateInput{
			UserID:         xqBotUID,
			PostID:         postID,
			PcommentID:     pcommentID,
			Content:        content,
			UserTargetName: xqAtRuneTrunc(targetName, 50),
			TargetUserID:   targetUID,
		}
		if kind == "ccomment" {
			in.ReplyCcommentID = triggerID
		}
		if _, err := service.Comments.CreateCcomment(ctx, in); err != nil {
			log.Printf("[xq-at] CreateCcomment bot reply pc=%d failed: %v", pcommentID, err)
		}
	}
	log.Printf("[xq-at] replied kind=%s trigger=%d bot=%d fail=%q len=%d", kind, triggerID, xqBotUID, fail, utf8.RuneCountInString(content))
}

// xqAtCallAgent POST mention 端点；返回 (answer, failReason)，failReason 非空即失败。
func xqAtCallAgent(ctx context.Context, sessionID string, actorUID int, task string) (string, string) {
	payload, err := json.Marshal(map[string]any{
		"task":          task,
		"session_id":    sessionID,
		"actor_user_id": strconv.Itoa(actorUID),
	})
	if err != nil {
		return "", "marshal body: " + err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, xqAtURL, bytes.NewReader(payload))
	if err != nil {
		return "", "build request: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", xqAtToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "http: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Sprintf("agent status %d", resp.StatusCode)
	}
	var out struct {
		Code int `json:"code"`
		Data struct {
			Answer string `json:"answer"`
		} `json:"data"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return "", "decode: " + err.Error()
	}
	if out.Code != 0 || strings.TrimSpace(out.Data.Answer) == "" {
		return "", fmt.Sprintf("agent code=%d empty answer", out.Code)
	}
	return out.Data.Answer, ""
}
