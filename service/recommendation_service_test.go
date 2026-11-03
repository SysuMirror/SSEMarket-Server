package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"loginTest/domain"
	"loginTest/repository"
)

// fakeRecRepo 只覆盖测试用得到的方法；未实现的方法会落到嵌入的 nil 接口上。
type fakeRecRepo struct {
	repository.RecommendationRepository
	mode     string
	gray     bool
	authored []domain.RecInteraction
	chats    []domain.RecChatMessage
}

func (f *fakeRecRepo) GetUserMode(context.Context, int) (string, error) { return f.mode, nil }
func (f *fakeRecRepo) MatchGrayRule(context.Context, int) (bool, error) { return f.gray, nil }
func (f *fakeRecRepo) ListUserBrowses(context.Context, int, int) ([]domain.RecInteraction, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserLikes(context.Context, int, int) ([]domain.RecInteraction, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserComments(context.Context, int, int) ([]domain.RecInteraction, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserSaves(context.Context, int, int) ([]domain.RecInteraction, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserCoinThrows(context.Context, int, int) ([]domain.RecInteraction, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserReadEvents(context.Context, int, int) ([]domain.RecReadEvent, error) {
	return nil, nil
}
func (f *fakeRecRepo) ListUserAuthoredPosts(context.Context, int, int) ([]domain.RecInteraction, error) {
	return f.authored, nil
}
func (f *fakeRecRepo) ListUserChatMessages(context.Context, int, int, int) ([]domain.RecChatMessage, error) {
	return f.chats, nil
}
func (f *fakeRecRepo) BrowsedPostIDs(context.Context, int, int) (map[int]struct{}, error) {
	return map[int]struct{}{}, nil
}
func (f *fakeRecRepo) GetUserRegisteredAt(context.Context, int) (*time.Time, error) {
	return nil, nil
}

func newTestRecService(repo repository.RecommendationRepository) *recommendationService {
	return &recommendationService{
		repo: repo,
		cfg: recConfig{
			halfLifeHours:    24 * 7,
			profileTTL:       time.Minute,
			rankTTL:          time.Minute,
			indexRefresh:     time.Minute,
			coldStartDays:    3,
			coldStartSignals: 3,
			chatBotUID:       95317,
		},
		idx: &recIndex{
			posts: map[int]domain.RecPostMeta{},
			text:  map[int][]float32{},
			image: map[int][]float32{},
		},
		profiles:      map[int]*tasteProfile{},
		chatCache:     map[int]recChatCacheEntry{},
		settingsCache: map[int]recSettingsCacheEntry{},
		rankCache:     map[int]recRankCacheEntry{},
		impMemory:     map[int]map[int]recImpression{},
	}
}

func TestScorePostPrefersSimilarAndPenalizesRead(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	now := time.Now()
	idx := &recIndex{
		posts: map[int]domain.RecPostMeta{},
		text: map[int][]float32{
			1: {1, 0},
			2: {0, 1},
		},
		image: map[int][]float32{},
	}
	taste := &tasteProfile{text: []float32{1, 0}, browsed: map[int]struct{}{}}

	similarOld := domain.RecPostMeta{ID: 1, PostTime: now.Add(-72 * time.Hour)}
	unrelatedFresh := domain.RecPostMeta{ID: 2, PostTime: now}

	oldScore := s.scorePost(similarOld, idx, taste, now, false, 1, recScoreFresh)
	freshScore := s.scorePost(unrelatedFresh, idx, taste, now, false, 1, recScoreFresh)
	if oldScore <= freshScore {
		t.Fatalf("similar post should outrank unrelated fresh post: old=%.4f fresh=%.4f", oldScore, freshScore)
	}

	readScore := s.scorePost(similarOld, idx, taste, now, true, 1, recScoreFresh)
	if readScore >= oldScore {
		t.Fatalf("read post should be penalized: read=%.4f unread=%.4f", readScore, oldScore)
	}
}

func TestColdStartPrefersQualityOverRecency(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	now := time.Now()
	idx := &recIndex{posts: map[int]domain.RecPostMeta{}, text: map[int][]float32{}, image: map[int][]float32{}}
	fresh := domain.RecPostMeta{ID: 1, PostTime: now}
	oldHighQuality := domain.RecPostMeta{ID: 2, PostTime: now.Add(-2 * 365 * 24 * time.Hour), LikeNum: 30, CommentNum: 20, CoinNum: 5}
	freshScore := s.scorePost(fresh, idx, nil, now, false, 1, recScoreCold)
	oldScore := s.scorePost(oldHighQuality, idx, nil, now, false, 1, recScoreCold)
	if freshScore >= oldScore {
		t.Fatalf("cold start should rank old quality above fresh empty: fresh=%.4f old=%.4f", freshScore, oldScore)
	}
}

func TestQualityIgnoresBrowseNum(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	now := time.Now()
	idx := &recIndex{posts: map[int]domain.RecPostMeta{}, text: map[int][]float32{}, image: map[int][]float32{}}
	lowBrowse := domain.RecPostMeta{ID: 1, PostTime: now, LikeNum: 5, CommentNum: 2, CoinNum: 1, BrowseNum: 10}
	highBrowse := lowBrowse
	highBrowse.ID = 2
	highBrowse.BrowseNum = 999999
	if s.scorePost(lowBrowse, idx, nil, now, false, 1, recScoreCold) != s.scorePost(highBrowse, idx, nil, now, false, 1, recScoreCold) {
		t.Fatal("browse count must not affect quality score")
	}
}

func TestBucketPatternsExposeOldPosts(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	now := time.Now()
	addBucket := func(base int, age time.Duration) {
		for i := 1; i <= 30; i++ {
			id := base + i
			s.idx.posts[id] = domain.RecPostMeta{ID: id, PostTime: now.Add(-age), LikeNum: i, CommentNum: i / 2}
			s.idx.text[id] = []float32{1, 0}
		}
	}
	addBucket(0, 2*24*time.Hour)     // 1..30 fresh
	addBucket(100, 180*24*time.Hour) // 101..130 mid
	addBucket(200, 900*24*time.Hour) // 201..230 old
	countBuckets := func(ids []int) (fresh, mid, old int) {
		for _, id := range ids {
			switch {
			case id <= 30:
				fresh++
			case id <= 130:
				mid++
			default:
				old++
			}
		}
		return
	}

	// 冷启动：无画像 -> 第一页应以老/中为主。
	cold, _, err := s.RankPostIDs(context.Background(), 9, nil, 15, 0)
	if err != nil {
		t.Fatal(err)
	}
	cf, cm, co := countBuckets(cold)
	if co < 8 || cm < 3 || cf < 2 {
		t.Fatalf("cold start mix mismatch: fresh=%d mid=%d old=%d", cf, cm, co)
	}

	// 有画像：仍保留固定老帖配额。
	registered := now.Add(-30 * 24 * time.Hour)
	s.profiles[7] = &tasteProfile{
		text: []float32{1, 0}, browsed: map[int]struct{}{},
		strongSignals: 10, registeredAt: &registered, builtAt: now,
	}
	personal, _, err := s.RankPostIDs(context.Background(), 7, nil, 15, 0)
	if err != nil {
		t.Fatal(err)
	}
	pf, pm, po := countBuckets(personal)
	if po < 4 || pm < 4 || pf < 7 {
		t.Fatalf("personalized mix mismatch: fresh=%d mid=%d old=%d", pf, pm, po)
	}
}

func TestProfileUsesAuthoredPosts(t *testing.T) {
	now := time.Now()
	repo := &fakeRecRepo{authored: []domain.RecInteraction{{PostID: 1, At: now}}}
	s := newTestRecService(repo)
	s.idx.posts[1] = domain.RecPostMeta{ID: 1, Partition: "技术", Tag: "Go", PostTime: now}
	s.idx.text[1] = []float32{1, 0}
	p, err := s.profileFor(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.text) == 0 || p.partitions["技术"] <= 0 || p.tags["Go"] <= 0 {
		t.Fatalf("authored post should shape profile: text=%v partitions=%v tags=%v", p.text, p.partitions, p.tags)
	}
	if p.strongSignals < 1 {
		t.Fatal("authored post should count as a strong signal")
	}
}

func TestBuildChatProfileText(t *testing.T) {
	if buildChatProfileText(nil) != "" {
		t.Fatal("empty chat should produce empty text")
	}
	text := buildChatProfileText([]domain.RecChatMessage{
		{Content: "最近有什么好看的小说"},
		{Content: " "},
		{Content: "我想学 Go 和分布式"},
	})
	if !strings.Contains(text, "我想学 Go 和分布式") || !strings.Contains(text, "最近有什么好看的小说") {
		t.Fatalf("unexpected chat profile text: %q", text)
	}
	// 仓库返回新消息在前；拼接后应按时间正序（旧 -> 新）出现。
	if strings.Index(text, "最近有什么好看的小说") < strings.Index(text, "我想学 Go 和分布式") {
		t.Fatalf("chat messages should be chronological: %q", text)
	}
	long := strings.Repeat("很长的聊天内容", 1000)
	text = buildChatProfileText([]domain.RecChatMessage{{Content: long}})
	if len([]rune(text)) > 3100 {
		t.Fatalf("chat profile text should be capped, got %d runes", len([]rune(text)))
	}
}

func TestFetchAgentChatMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/v1/agent/conversations":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"conversations":[{"session_id":"s1","title":"标题兜底","updated_at":"2026-11-03T10:00:00Z"}]}}`)
		case "/api/v1/agent/conversations/s1/messages":
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"messages":[{"role":"user","content":"我想学 Go"},{"role":"assistant","content":"好的"},{"role":"user","content":"推荐点分布式资料"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := newTestRecService(&fakeRecRepo{})
	s.cfg.chatEnabled = true
	s.cfg.chatBase = srv.URL
	msgs, err := s.fetchAgentChatMessages(context.Background(), "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Content != "我想学 Go" || msgs[1].Content != "推荐点分布式资料" {
		t.Fatalf("unexpected chat messages: %+v", msgs)
	}
}

func TestEffectiveModeGrayRules(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		gray      bool
		available bool
		want      string
	}{
		{"auto gray available", domain.RecModeAuto, true, true, domain.RecEffectiveRecommend},
		{"auto no gray", domain.RecModeAuto, false, true, domain.RecEffectiveTime},
		{"explicit on available", domain.RecModeOn, false, true, domain.RecEffectiveRecommend},
		{"explicit on unavailable", domain.RecModeOn, false, false, domain.RecEffectiveTime},
		{"explicit off beats gray", domain.RecModeOff, true, true, domain.RecEffectiveTime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestRecService(&fakeRecRepo{mode: tc.mode, gray: tc.gray})
			for i := 0; i < 150; i++ {
				s.idx.posts[i] = domain.RecPostMeta{ID: i}
				if tc.available {
					s.idx.text[i] = []float32{1, 0}
				}
			}
			got, _, _ := s.effectiveMode(context.Background(), 1)
			if got != tc.want {
				t.Fatalf("mode=%q gray=%v available=%v: got %q want %q", tc.mode, tc.gray, tc.available, got, tc.want)
			}
		})
	}
}

func TestRankPostIDsPaginationStable(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{mode: domain.RecModeOn})
	now := time.Now()
	for i := 1; i <= 30; i++ {
		s.idx.posts[i] = domain.RecPostMeta{ID: i, PostTime: now.Add(-time.Duration(i) * time.Hour)}
		s.idx.text[i] = []float32{float32(i), 1}
	}
	ctx := context.Background()
	first, total, err := s.RankPostIDs(ctx, 7, nil, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 30 || len(first) != 10 {
		t.Fatalf("first page: total=%d len=%d", total, len(first))
	}
	second, _, err := s.RankPostIDs(ctx, 7, nil, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]struct{}{}
	for _, id := range append(append([]int{}, first...), second...) {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate post %d across pages", id)
		}
		seen[id] = struct{}{}
	}
}

func TestFeedImpressionsRotateNextRefresh(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{mode: domain.RecModeOn})
	now := time.Now()
	for i := 1; i <= 45; i++ {
		s.idx.posts[i] = domain.RecPostMeta{ID: i, PostTime: now}
		s.idx.text[i] = []float32{1, 0}
	}
	ctx := context.Background()
	first, _, err := s.RankPostIDs(ctx, 7, nil, recFeedPageSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != recFeedPageSize {
		t.Fatalf("first page len=%d", len(first))
	}

	// 看了一页但一个都没点：记曝光，下一次刷新应该换一批。
	s.RecordFeedImpressions(ctx, 7, first)
	second, _, err := s.RankPostIDs(ctx, 7, nil, recFeedPageSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[int]struct{}, len(first))
	for _, id := range first {
		seen[id] = struct{}{}
	}
	overlap := 0
	for _, id := range second {
		if _, ok := seen[id]; ok {
			overlap++
		}
	}
	if overlap != 0 {
		t.Fatalf("refresh should rotate seen posts down, overlap=%d first=%v second=%v", overlap, first, second)
	}
}

func TestImpressionPenaltyDecays(t *testing.T) {
	now := time.Now()
	fresh := impressionPenalty(recImpression{count: 1, lastAt: now}, now)
	old := impressionPenalty(recImpression{count: 1, lastAt: now.Add(-48 * time.Hour)}, now)
	repeat := impressionPenalty(recImpression{count: 4, lastAt: now}, now)
	if fresh <= 0 || old >= fresh || repeat <= fresh {
		t.Fatalf("unexpected penalties fresh=%.4f old=%.4f repeat=%.4f", fresh, old, repeat)
	}
	if impressionPenalty(recImpression{}, now) != 0 {
		t.Fatal("no impression should have no penalty")
	}
}

func TestExplorationSlotsPickOffInterestHighQuality(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{mode: domain.RecModeOn})
	s.cfg.explorePerPage = 2
	s.cfg.exploreSimPercentile = 0.20
	s.cfg.exploreQualityPercentile = 0.60
	s.cfg.exploreCooldown = 7 * 24 * time.Hour

	now := time.Now()
	for i := 1; i <= 70; i++ {
		s.idx.posts[i] = domain.RecPostMeta{ID: i, PostTime: now, Partition: "技术", Tag: "Go"}
		s.idx.text[i] = []float32{1, 0}
	}
	offInterest := make(map[int]struct{})
	for i := 101; i <= 120; i++ {
		s.idx.posts[i] = domain.RecPostMeta{
			ID: i, PostTime: now, Partition: "摄影", Tag: "摄影",
			LikeNum: 50, CommentNum: 20,
		}
		s.idx.text[i] = []float32{0, 1}
		offInterest[i] = struct{}{}
	}
	registered := now.Add(-30 * 24 * time.Hour)
	s.profiles[7] = &tasteProfile{
		text: []float32{1, 0}, browsed: map[int]struct{}{},
		partitions: map[string]float64{"技术": 1}, tags: map[string]float64{"Go": 1},
		strongSignals: 10, registeredAt: &registered, builtAt: now,
	}

	ctx := context.Background()
	first, total, err := s.RankPostIDs(ctx, 7, nil, recFeedPageSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 90 || len(first) != recFeedPageSize {
		t.Fatalf("unexpected first page total=%d len=%d", total, len(first))
	}
	// 默认探索位落在第 5、第 12 条（0-based 4、11）。
	for _, pos := range []int{4, 11} {
		if _, ok := offInterest[first[pos]]; !ok {
			t.Fatalf("position %d should be exploration, got post %d: %v", pos+1, first[pos], first)
		}
	}
	offCount := 0
	for _, id := range first {
		if _, ok := offInterest[id]; ok {
			offCount++
		}
	}
	if offCount != 2 {
		t.Fatalf("first page should have exactly 2 exploration posts, got %d: %v", offCount, first)
	}

	// 探索位按页重复：第二页同样应有两处，且分页不重不漏。
	seen := make(map[int]struct{}, total)
	for _, id := range first {
		seen[id] = struct{}{}
	}
	for offset := recFeedPageSize; offset < total; offset += recFeedPageSize {
		page, _, err := s.RankPostIDs(ctx, 7, nil, recFeedPageSize, offset)
		if err != nil {
			t.Fatal(err)
		}
		for i, id := range page {
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate post %d across pages", id)
			}
			seen[id] = struct{}{}
			if offset == recFeedPageSize && (i == 4 || i == 11) {
				if _, ok := offInterest[id]; !ok {
					t.Fatalf("second page position %d should be exploration, got %d", i+1, id)
				}
			}
		}
	}
	if len(seen) != total {
		t.Fatalf("pagination should cover all posts: got %d want %d", len(seen), total)
	}
}

func TestExplorationSkipsReadAndRecentlySeen(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	s.cfg.explorePerPage = 2
	s.cfg.exploreSimPercentile = 1
	s.cfg.exploreQualityPercentile = 0
	s.cfg.exploreCooldown = 7 * 24 * time.Hour

	now := time.Now()
	entries := make([]rankEntry, 0, 10)
	for i := 1; i <= 10; i++ {
		entries = append(entries, rankEntry{
			id: i, sim: 0, quality: 0.9, hasVector: true, tag: fmt.Sprintf("t%d", i),
		})
	}
	entries[0].read = true
	impressions := map[int]recImpression{
		2: {count: 1, lastAt: now},
		3: {count: 1, lastAt: now.Add(-8 * 24 * time.Hour)},
	}
	selected := s.selectExploration(entries, 8, impressions, now)
	if len(selected) != 8 {
		t.Fatalf("expected 8 exploration posts, got %d", len(selected))
	}
	chosen := make(map[int]struct{}, len(selected))
	for _, e := range selected {
		if e.id == 1 {
			t.Fatal("read post must not be used for exploration")
		}
		if e.id == 2 {
			t.Fatal("recently seen post must not be used for exploration")
		}
		if _, dup := chosen[e.id]; dup {
			t.Fatalf("duplicate exploration post %d", e.id)
		}
		chosen[e.id] = struct{}{}
	}
	if _, ok := chosen[3]; !ok {
		t.Fatal("impression older than cooldown should be allowed again")
	}
}

func TestExplorationDisabled(t *testing.T) {
	s := newTestRecService(&fakeRecRepo{})
	s.cfg.explorePerPage = 0
	entries := []rankEntry{{id: 1, hasVector: true, quality: 1}}
	if got := s.selectExploration(entries, 2, nil, time.Now()); len(got) != 0 {
		t.Fatalf("exploration should be disabled, got %+v", got)
	}
}

func TestUnitVectorNormalizes(t *testing.T) {
	v := unitVector([]float64{3, 4})
	if v == nil || len(v) != 2 {
		t.Fatalf("unexpected vector %v", v)
	}
	if dotVec(v, v) < 0.9999 || dotVec(v, v) > 1.0001 {
		t.Fatalf("vector should be unit length, got %f", dotVec(v, v))
	}
	if unitVector(nil) != nil {
		t.Fatal("nil accumulator should produce nil vector")
	}
}

func TestDwellWeight(t *testing.T) {
	if dwellWeight(1000) >= 0 {
		t.Fatal("very short dwell should be negative")
	}
	if dwellWeight(5000) <= 0 {
		t.Fatal("short dwell should be positive")
	}
	if dwellWeight(200000) <= dwellWeight(60000) {
		t.Fatal("longer dwell should weigh more")
	}
}

func TestBuildPostEmbedTextStripsImageURLs(t *testing.T) {
	text := buildPostEmbedText(domain.RecPostContent{
		Title:     "测试标题",
		Partition: "技术",
		Tag:       "Go",
		Ptext:     "正文 ![图片](https://image.ssemarket.cn/a.png) 还有 https://example.com/x 内容",
	})
	for _, bad := range []string{"image.ssemarket.cn", "example.com", "![", "]("} {
		if strings.Contains(text, bad) {
			t.Fatalf("embed text should not contain %q: %q", bad, text)
		}
	}
	if !strings.Contains(text, "标题：测试标题") || !strings.Contains(text, "正文：") {
		t.Fatalf("embed text missing expected sections: %q", text)
	}
}

func TestImageEmbeddingRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("embed api /multimodal-embeddings: status=500 body={\"detail\":\"encoding failed: 404 Client Error: Not Found\"}"), false},
		{fmt.Errorf("embed api /multimodal-embeddings: status=500 body={\"detail\":\"encoding failed: 403 Client Error: Forbidden\"}"), false},
		{fmt.Errorf("embed api /multimodal-embeddings: status=500 body={\"detail\":\"internal error\"}"), true},
		{fmt.Errorf("embed api /multimodal-embeddings: status=503"), true},
		{fmt.Errorf("embed api /multimodal-embeddings: status=429"), true},
	}
	for _, tc := range cases {
		if got := imageEmbeddingRetryable(tc.err); got != tc.want {
			t.Fatalf("imageEmbeddingRetryable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestExtractFirstImageURLOnlyAllowlistedHost(t *testing.T) {
	got := extractFirstImageURL(domain.RecPostContent{Ptext: "![a](https://image.ssemarket.cn/ok.png) ![b](https://evil.example.com/no.png)"})
	if got != "https://image.ssemarket.cn/ok.png" {
		t.Fatalf("got %q", got)
	}
	if extractFirstImageURL(domain.RecPostContent{Ptext: "![a](http://image.ssemarket.cn/ok.png)"}) != "" {
		t.Fatal("http image should be rejected")
	}
}

func TestRecEligible(t *testing.T) {
	base := domain.BrowseFilter{ViewerUserID: 1, Searchsort: "home", Partition: "主页", OrderBy: "time_desc"}
	if !recEligible(base) {
		t.Fatal("default home feed should be eligible")
	}
	for _, mutate := range []func(*domain.BrowseFilter){
		func(f *domain.BrowseFilter) { f.Searchinfo = "go" },
		func(f *domain.BrowseFilter) { f.Tag = "go" },
		func(f *domain.BrowseFilter) { f.Partition = "课程交流" },
		func(f *domain.BrowseFilter) { f.OrderBy = "like_desc" },
		func(f *domain.BrowseFilter) { f.Searchsort = "save" },
		func(f *domain.BrowseFilter) { f.TimeFrom = "2026-01-01" },
		func(f *domain.BrowseFilter) { f.ViewerUserID = 0 },
	} {
		f := base
		mutate(&f)
		if recEligible(f) {
			t.Fatalf("filter should not be eligible: %+v", f)
		}
	}
}
