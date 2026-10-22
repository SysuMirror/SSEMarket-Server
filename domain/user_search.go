package domain

import (
	"regexp"
	"strings"
	"time"
)

// 用户搜索结果口径。
//
// 搜索页除帖子结果外，还要在顶部展示「可能想找的人」卡片（B 站式）。
// 是否展示由两层判断决定：
//  1. 用户名直接命中（全等 / 包含 / 关键词全部命中）→ 直接展示；
//  2. 用户名/简介与关键词的相关度超过 UserSearchRelevanceThreshold → 展示。
//
// 相关度是本地可解释的字符级相似度（关键词单元覆盖率 + 查询字符覆盖率），
// 不依赖外部 RAG 服务，因此部署后无需额外配置；阈值集中在这里，后续想接
// 向量检索时只需替换 ScoreUserSearch 的实现，接口与前端不受影响。
const (
	// UserSearchRelevanceThreshold 相关度阈值；低于它的候选不展示。
	UserSearchRelevanceThreshold = 0.32
	// UserSearchDefaultLimit 默认返回的用户卡片数量。
	UserSearchDefaultLimit = 3
	// UserSearchMaxLimit 单次最多返回的用户卡片数量。
	UserSearchMaxLimit = 10
	// UserSearchKeywordMaxRunes 关键词长度上限（rune）。
	UserSearchKeywordMaxRunes = 50
)

// 用户卡片的匹配类型。
const (
	UserSearchMatchName     = "name"     // 用户名命中
	UserSearchMatchIntro    = "intro"    // 简介直接包含关键词
	UserSearchMatchSemantic = "semantic" // 相关度超过阈值
)

// UserSearchPostCard 用户卡片里的帖子缩略项（B 站卡片的三格预览）。
type UserSearchPostCard struct {
	PostID    int    `json:"postID"`
	Title     string `json:"title"`
	Partition string `json:"partition"`
	// Photos 帖子配图（"|" 分隔）；前端取第一张做缩略图，没有图时用标题占位。
	Photos    string    `json:"photos"`
	LikeNum   int       `json:"likeNum"`
	BrowseNum int       `json:"browseNum"`
	PostTime  time.Time `json:"postTime"`
}

// UserSearchResult 搜索页用户卡片。
type UserSearchResult struct {
	UserID          int    `json:"userID"`
	Name            string `json:"name"`
	AvatarURL       string `json:"avatarURL"`
	Identity        string `json:"identity"`
	Intro           string `json:"intro"`
	Score           int    `json:"score"`
	CustomTitle     string `json:"customTitle"`
	ReceivedLikeNum int    `json:"receivedLikeNum"`
	// PostCount 该用户公开可见的帖子总数（不含私密/匿名帖）。
	PostCount int `json:"postCount"`
	// Hidden 用户开启了「隐藏历史发帖」；为 true 时 Posts 恒为空，
	// 前端展示「用户隐藏了历史对话」。
	Hidden bool `json:"hidden"`
	// MatchType name / intro / semantic；Relevance 为 0~1 的相关度。
	MatchType string               `json:"matchType"`
	Relevance float64              `json:"relevance"`
	Posts     []UserSearchPostCard `json:"posts"`
}

var (
	userSearchASCIIWordRE = regexp.MustCompile(`[a-z0-9]+`)
	userSearchHanRE       = regexp.MustCompile(`\p{Han}+`)
)

// 搜索意图里的高频虚词/客套词。去掉它们后，剩下的名词/术语才参与匹配，
// 避免「想找保研的学长」这类自然语言被整句当成一个词。
var userSearchIntentWords = []string{
	"想找", "想了解", "有没有", "求推荐", "推荐一下", "请问", "帮我", "谁能",
	"一下", "关于", "如何", "怎么", "什么",
	"的", "吗", "呢", "啊", "吧", "求", "找",
}

func normalizeUserSearchText(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func stripUserSearchIntent(s string) string {
	for _, w := range userSearchIntentWords {
		s = strings.ReplaceAll(s, w, " ")
	}
	return strings.TrimSpace(s)
}

// userSearchQueryUnits 把查询拆成可匹配单元：ASCII 单词 + 中文二元组。
// 中文按二元组而不是整段，是为了「保研学长」能和「保研经验分享」产生交集。
func userSearchQueryUnits(query string) []string {
	q := normalizeUserSearchText(stripUserSearchIntent(query))
	if q == "" {
		q = normalizeUserSearchText(query)
	}
	seen := make(map[string]struct{})
	units := make([]string, 0, 8)
	add := func(u string) {
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		units = append(units, u)
	}
	for _, w := range userSearchASCIIWordRE.FindAllString(q, -1) {
		add(w)
	}
	for _, run := range userSearchHanRE.FindAllString(q, -1) {
		runes := []rune(run)
		if len(runes) <= 2 {
			add(run)
			continue
		}
		for i := 0; i+1 < len(runes); i++ {
			add(string(runes[i : i+2]))
		}
	}
	return units
}

// userSearchTextRelevance 查询与一段文本的相关度（0~1）。
// 定义：
//   - 文本直接包含整句查询 → 1；
//   - 否则 = 0.7 * 查询单元命中率 + 0.3 * 查询字符覆盖率。
//
// 用「查询字符覆盖率」而不是 Jaccard，避免长简介把分数稀释掉。
func userSearchTextRelevance(query, text string) float64 {
	q := normalizeUserSearchText(stripUserSearchIntent(query))
	if q == "" {
		// 整个查询都是「想找/求/的」这类意图词，没有可匹配的实体。
		return 0
	}
	t := normalizeUserSearchText(text)
	if q == "" || t == "" {
		return 0
	}
	if strings.Contains(t, q) {
		return 1
	}
	units := userSearchQueryUnits(query)
	if len(units) == 0 {
		return 0
	}
	matched := 0
	for _, u := range units {
		if strings.Contains(t, u) {
			matched++
		}
	}
	unitCoverage := float64(matched) / float64(len(units))

	qChars := make(map[rune]struct{})
	for _, r := range q {
		if r == ' ' {
			continue
		}
		qChars[r] = struct{}{}
	}
	overlap := 0
	if len(qChars) > 0 {
		for r := range qChars {
			if strings.ContainsRune(t, r) {
				overlap++
			}
		}
	}
	charCoverage := float64(overlap) / float64(len(qChars))
	score := 0.7*unitCoverage + 0.3*charCoverage
	if score > 1 {
		score = 1
	}
	return score
}

// ScoreUserSearch 计算用户与关键词的匹配分与匹配类型。
// 返回 score <= 0 表示不匹配。
func ScoreUserSearch(query string, user *User) (float64, string) {
	if user == nil {
		return 0, ""
	}
	q := normalizeUserSearchText(stripUserSearchIntent(query))
	if q == "" {
		// 整个查询都是「想找/求/的」这类意图词，没有可匹配的实体。
		return 0, ""
	}
	name := normalizeUserSearchText(user.Name)
	if name != "" {
		switch {
		case name == q:
			return 1, UserSearchMatchName
		case strings.Contains(name, q):
			return 0.96, UserSearchMatchName
		case len([]rune(q)) >= 2 && strings.Contains(q, name):
			// 搜索词包含用户名（如搜「磊哥游戏」命中「磊哥」）。
			return 0.86, UserSearchMatchName
		}
		if units := userSearchQueryUnits(query); len(units) > 0 {
			all := true
			for _, u := range units {
				if !strings.Contains(name, u) {
					all = false
					break
				}
			}
			if all {
				return 0.82, UserSearchMatchName
			}
		}
	}

	intro := normalizeUserSearchText(user.Intro)
	if intro != "" && strings.Contains(intro, q) {
		return 0.76, UserSearchMatchIntro
	}

	best := userSearchTextRelevance(query, user.Name)
	if v := userSearchTextRelevance(query, user.Intro); v > best {
		best = v
	}
	if best >= UserSearchRelevanceThreshold {
		return best, UserSearchMatchSemantic
	}
	return best, ""
}

// EscapeUserSearchLike 转义 LIKE 通配符，避免用户输入 `%`/`_` 扩大匹配范围。
// 用 `!` 作为转义符（SQL 里写 ESCAPE '!'），不依赖 MySQL 对反斜杠的 sql_mode。
func EscapeUserSearchLike(s string) string {
	r := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`)
	return r.Replace(s)
}

// UserSearchTerms 从关键词中提取用于 SQL 预筛的候选单元（去重、限制数量）。
// 直接用中文二元组可能让 SQL 条件过多，因此最多取 8 个。
func UserSearchTerms(query string) []string {
	const maxTerms = 8
	units := userSearchQueryUnits(query)
	if len(units) <= maxTerms {
		return units
	}
	return units[:maxTerms]
}
