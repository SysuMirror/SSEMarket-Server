package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
)

// RecommendationService 首页推荐流：用户开关、灰度、画像、排序与向量构建。
type RecommendationService interface {
	// Start 启动索引刷新与嵌入构建后台任务（非阻塞）。
	Start(ctx context.Context)
	Stop()

	// Settings / SetMode 用户推荐开关。
	Settings(ctx context.Context, userID int) (*domain.RecommendationSettings, error)
	SetMode(ctx context.Context, userID int, mode string) error

	// EffectiveMode 返回实际推流模式（time / recommend）、是否命中灰度、模型是否可用。
	// 首页 Browse 每个请求都会调用，内部做短 TTL 缓存。
	EffectiveMode(ctx context.Context, userID int) (mode string, grayMatched bool, available bool)

	// RankPostIDs 按「时间 + 画像相似度 + 已读」排序，返回分页后的帖子 ID。
	RankPostIDs(ctx context.Context, userID int, exclude map[int]struct{}, limit, offset int) (ids []int, total int, err error)

	// RecordReadEvent 点进详情标记已读；dwellMS 可选，后续计时器上报时长时更新。
	RecordReadEvent(ctx context.Context, userID, postID, dwellMS int) error

	// RecordFeedImpressions 记录推荐流里实际下发的帖子（看过但没点也算），
	// 下次刷新时做轻微降权，让首页内容自然轮换。
	RecordFeedImpressions(ctx context.Context, userID int, postIDs []int)

	// 管理员：灰度规则与运行状态。
	ListGrayRules(ctx context.Context) ([]domain.RecommendationGrayRule, error)
	UpsertGrayRule(ctx context.Context, in domain.RecommendGrayRuleInput) (*domain.RecommendationGrayRule, error)
	DeleteGrayRule(ctx context.Context, id int) error
	Stats(ctx context.Context) (*domain.RecommendationStats, error)
	Rebuild(ctx context.Context) error
}

// rankEntry 排序中间结构。
type rankEntry struct {
	id    int
	score float64
}

type recIndex struct {
	posts   map[int]domain.RecPostMeta
	text    map[int][]float32
	image   map[int][]float32
	builtAt time.Time
}

// tasteProfile 用户兴趣画像。向量已 L2 归一，点积即余弦。
type tasteProfile struct {
	text          []float32
	image         []float32
	browsed       map[int]struct{}
	partitions    map[string]float64
	tags          map[string]float64
	weight        float64
	strongSignals int
	registeredAt  *time.Time
	builtAt       time.Time
}

type recSettingsCacheEntry struct {
	mode      string
	gray      bool
	available bool
	at        time.Time
}

type recRankCacheEntry struct {
	ids []int
	at  time.Time
}

type recommendationService struct {
	repo   repository.RecommendationRepository
	cfg    recConfig
	client *embeddingClient

	idxMu sync.RWMutex
	idx   *recIndex

	profileMu sync.Mutex
	profiles  map[int]*tasteProfile

	chatMu    sync.Mutex
	chatCache map[int]recChatCacheEntry

	settingsMu    sync.Mutex
	settingsCache map[int]recSettingsCacheEntry

	rankMu    sync.Mutex
	rankCache map[int]recRankCacheEntry

	impMu     sync.Mutex
	impMemory map[int]map[int]recImpression

	statsMu   sync.RWMutex
	lastRunAt time.Time
	lastError string

	started atomic.Bool
	cancel  context.CancelFunc
}

func NewRecommendationService(repo repository.RecommendationRepository) RecommendationService {
	cfg := loadRecConfig()
	return &recommendationService{
		repo:   repo,
		cfg:    cfg,
		client: newEmbeddingClient(cfg),
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

// ---------- 生命周期 ----------

func (s *recommendationService) Start(ctx context.Context) {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go func() {
		s.loadIndex(cctx)
		if s.client.Enabled() {
			logRecf("started provider=%s model=%s dims=%d batch=%d image=%v",
				s.cfg.provider, s.cfg.model, s.cfg.dims, s.cfg.batchSize, s.cfg.imageEnabled)
		} else {
			logRecf("embedding disabled (provider=%s); recommendation falls back to time+quality until RECOMMEND_EMBED_KEY is configured", s.cfg.provider)
		}
		go s.loopIndex(cctx)
		if s.client.Enabled() {
			go s.loopEmbed(cctx)
		}
	}()
}

func (s *recommendationService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *recommendationService) loadIndex(ctx context.Context) {
	posts, err := s.repo.ListPostMeta(ctx)
	if err != nil {
		s.recordError(err)
		logRecf("load post meta failed: %v", err)
		return
	}
	embs, err := s.repo.ListEmbeddings(ctx)
	if err != nil {
		s.recordError(err)
		logRecf("load embeddings failed: %v", err)
		return
	}
	idx := &recIndex{
		posts:   make(map[int]domain.RecPostMeta, len(posts)),
		text:    make(map[int][]float32, len(embs)),
		image:   make(map[int][]float32),
		builtAt: time.Now(),
	}
	for _, p := range posts {
		idx.posts[p.ID] = p
	}
	for _, e := range embs {
		switch e.Kind {
		case "text":
			idx.text[e.PostID] = e.Vector
		case "image":
			idx.image[e.PostID] = e.Vector
		}
	}
	s.idxMu.Lock()
	s.idx = idx
	s.idxMu.Unlock()
	logRecf("index loaded posts=%d text=%d image=%d", len(idx.posts), len(idx.text), len(idx.image))
}

func (s *recommendationService) loopIndex(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.indexRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.loadIndex(ctx)
		}
	}
}

func (s *recommendationService) loopEmbed(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := s.embedTextBatch(ctx)
		if err != nil {
			s.recordError(err)
			logRecf("text embedding batch failed: %v", err)
			if !sleepCtx(ctx, time.Minute) {
				return
			}
			continue
		}
		if n > 0 {
			continue
		}
		if s.cfg.imageEnabled {
			m, imgErr := s.embedImageBatch(ctx)
			if imgErr != nil {
				s.recordError(imgErr)
				logRecf("image embedding batch failed: %v", imgErr)
				if !sleepCtx(ctx, time.Minute) {
					return
				}
				continue
			}
			if m > 0 {
				continue
			}
		}
		if !sleepCtx(ctx, 30*time.Second) {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (s *recommendationService) embedTextBatch(ctx context.Context) (int, error) {
	batch, err := s.repo.ListPendingTextPosts(ctx, s.cfg.batchSize)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}
	texts := make([]string, len(batch))
	for i, c := range batch {
		text := buildPostEmbedText(c)
		if strings.TrimSpace(text) == "" {
			text = "帖子"
		}
		texts[i] = text
	}
	vecs, err := s.client.EmbedTexts(ctx, texts)
	if err != nil {
		return 0, err
	}
	done := 0
	for i, c := range batch {
		if err := s.repo.UpsertEmbedding(ctx, c.PostID, "text", s.cfg.model, len(vecs[i]), contentHash(texts[i]), "", vecs[i]); err != nil {
			return done, err
		}
		s.addIndexVector(c.PostID, "text", vecs[i])
		done++
	}
	s.markRun()
	logRecf("text embeddings built batch=%d", done)
	return done, nil
}

func (s *recommendationService) embedImageBatch(ctx context.Context) (int, error) {
	batch, err := s.repo.ListPendingImagePosts(ctx, 8)
	if err != nil {
		return 0, err
	}
	if len(batch) == 0 {
		return 0, nil
	}
	type imageTask struct {
		postID int
		url    string
	}
	tasks := make([]imageTask, 0, len(batch))
	done := 0
	for _, c := range batch {
		u := extractFirstImageURL(c)
		if u == "" {
			// 标记已处理，避免同一批「疑似有图但解析失败」的帖子反复占住队列。
			if err := s.repo.UpsertEmbedding(ctx, c.PostID, "image", s.cfg.model, 0, contentHash("no-image"), "", nil); err != nil {
				return done, err
			}
			done++
			continue
		}
		tasks = append(tasks, imageTask{postID: c.PostID, url: u})
	}
	if len(tasks) == 0 {
		s.markRun()
		return done, nil
	}
	urls := make([]string, len(tasks))
	for i, t := range tasks {
		urls[i] = t.url
	}
	vecs, err := s.client.EmbedImages(ctx, urls)
	if err != nil {
		// 批量失败常见于个别图片 URL 已失效，整批重试会永远卡在队头。
		// 逐张重试：永久失败写 dims=0 占位跳过，网关抖动/限流则保留待下次重试。
		if len(tasks) == 1 {
			if imageEmbeddingRetryable(err) {
				return done, err
			}
			logRecf("image embedding skipped post=%d: %v", tasks[0].postID, err)
			if markErr := s.repo.UpsertEmbedding(ctx, tasks[0].postID, "image", s.cfg.model, 0, contentHash("image-failed:"+tasks[0].url), tasks[0].url, nil); markErr != nil {
				return done, markErr
			}
			s.markRun()
			return done + 1, nil
		}
		vecs = make([][]float32, len(tasks))
		var retryErr error
		for i, t := range tasks {
			v, singleErr := s.client.EmbedImages(ctx, []string{t.url})
			if singleErr != nil {
				if imageEmbeddingRetryable(singleErr) {
					retryErr = singleErr
					continue
				}
				logRecf("image embedding skipped post=%d: %v", t.postID, singleErr)
				if markErr := s.repo.UpsertEmbedding(ctx, t.postID, "image", s.cfg.model, 0, contentHash("image-failed:"+t.url), t.url, nil); markErr != nil {
					return done, markErr
				}
				continue
			}
			vecs[i] = v[0]
		}
		if retryErr != nil {
			return done, retryErr
		}
	}
	for i, t := range tasks {
		if len(vecs[i]) == 0 {
			// 单张永久失败，上面已写占位行，避免反复占住队头。
			done++
			continue
		}
		if err := s.repo.UpsertEmbedding(ctx, t.postID, "image", s.cfg.model, len(vecs[i]), contentHash(t.url), t.url, vecs[i]); err != nil {
			return done, err
		}
		s.addIndexVector(t.postID, "image", vecs[i])
		done++
	}
	s.markRun()
	logRecf("image embeddings built batch=%d", done)
	return done, nil
}

// imageEmbeddingRetryable 判断图片嵌入错误是否值得稍后重试。
// 网关限流/5xx/网络错误可重试；其余 4xx（图片 404、格式不支持）直接跳过。
func imageEmbeddingRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// 网关会把「取图失败」包成 500，但 body 里保留了上游状态码。
	// 图片本身 4xx（失效/403/格式不支持）重试也没用，直接跳过占位。
	if strings.Contains(msg, "encoding failed") {
		for code := 400; code < 500; code++ {
			if strings.Contains(msg, fmt.Sprintf("%d Client Error", code)) {
				return false
			}
		}
	}
	for _, code := range []string{"status=429", "status=500", "status=502", "status=503", "status=504"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return !strings.Contains(msg, "embed api")
}

func (s *recommendationService) addIndexVector(postID int, kind string, vec []float32) {
	s.idxMu.Lock()
	defer s.idxMu.Unlock()
	if s.idx == nil {
		return
	}
	if kind == "image" {
		s.idx.image[postID] = vec
	} else {
		s.idx.text[postID] = vec
	}
}

func (s *recommendationService) markRun() {
	s.statsMu.Lock()
	s.lastRunAt = time.Now()
	s.lastError = ""
	s.statsMu.Unlock()
}

func (s *recommendationService) recordError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	s.statsMu.Lock()
	s.lastError = msg
	s.statsMu.Unlock()
}

// ---------- 模式与灰度 ----------

func (s *recommendationService) available() bool {
	s.idxMu.RLock()
	defer s.idxMu.RUnlock()
	if s.idx == nil {
		return false
	}
	n := len(s.idx.text)
	total := len(s.idx.posts)
	if n == 0 {
		return false
	}
	if n >= 100 {
		return true
	}
	return n >= 10 && n*2 >= total
}

func (s *recommendationService) effectiveMode(ctx context.Context, userID int) (string, bool, bool) {
	now := time.Now()
	s.settingsMu.Lock()
	if e, ok := s.settingsCache[userID]; ok && now.Sub(e.at) < time.Minute {
		s.settingsMu.Unlock()
		return e.mode, e.gray, e.available
	}
	s.settingsMu.Unlock()

	mode, err := s.repo.GetUserMode(ctx, userID)
	if err != nil {
		mode = domain.RecModeAuto
	}
	gray := false
	if mode == domain.RecModeAuto {
		gray, _ = s.repo.MatchGrayRule(ctx, userID)
	}
	avail := s.available()
	effective := domain.RecEffectiveTime
	if mode == domain.RecModeOff {
		effective = domain.RecEffectiveTime
	} else if (mode == domain.RecModeOn || (mode == domain.RecModeAuto && gray)) && avail {
		effective = domain.RecEffectiveRecommend
	}

	s.settingsMu.Lock()
	s.settingsCache[userID] = recSettingsCacheEntry{mode: effective, gray: gray, available: avail, at: now}
	s.settingsMu.Unlock()
	return effective, gray, avail
}

func (s *recommendationService) EffectiveMode(ctx context.Context, userID int) (string, bool, bool) {
	return s.effectiveMode(ctx, userID)
}

func (s *recommendationService) Settings(ctx context.Context, userID int) (*domain.RecommendationSettings, error) {
	mode, err := s.repo.GetUserMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	effective, gray, avail := s.effectiveMode(ctx, userID)
	reason := ""
	switch {
	case mode == domain.RecModeOff:
		reason = "user_off"
	case (mode == domain.RecModeOn || gray) && !avail:
		reason = "model_building"
	case mode == domain.RecModeAuto && !gray:
		reason = "default_time"
	}
	return &domain.RecommendationSettings{
		Mode:        mode,
		Effective:   effective,
		GrayMatched: gray,
		Available:   avail,
		Reason:      reason,
	}, nil
}

func (s *recommendationService) SetMode(ctx context.Context, userID int, mode string) error {
	switch mode {
	case domain.RecModeAuto, domain.RecModeOn, domain.RecModeOff:
	default:
		return domain.ErrBadRequest
	}
	if userID <= 0 {
		return domain.ErrBadRequest
	}
	if err := s.repo.SaveUserMode(ctx, userID, mode); err != nil {
		return err
	}
	s.invalidateUser(ctx, userID)
	return nil
}

func (s *recommendationService) invalidateUser(ctx context.Context, userID int) {
	s.settingsMu.Lock()
	delete(s.settingsCache, userID)
	s.settingsMu.Unlock()
	s.profileMu.Lock()
	delete(s.profiles, userID)
	s.profileMu.Unlock()
	s.rankMu.Lock()
	delete(s.rankCache, userID)
	s.rankMu.Unlock()
	if core.MyRedis != nil {
		_ = core.MyRedis.Del(ctx, recRankKey(userID)).Err()
	}
}

func (s *recommendationService) invalidateAllSettings() {
	s.settingsMu.Lock()
	s.settingsCache = map[int]recSettingsCacheEntry{}
	s.settingsMu.Unlock()
}

func (s *recommendationService) ListGrayRules(ctx context.Context) ([]domain.RecommendationGrayRule, error) {
	return s.repo.ListGrayRules(ctx)
}

func (s *recommendationService) UpsertGrayRule(ctx context.Context, in domain.RecommendGrayRuleInput) (*domain.RecommendationGrayRule, error) {
	if in.UIDMin < 0 || in.UIDMax < in.UIDMin || (in.UIDMin == 0 && in.UIDMax == 0) {
		return nil, domain.ErrBadRequest
	}
	in.Note = strings.TrimSpace(in.Note)
	if runes := []rune(in.Note); len(runes) > 255 {
		in.Note = string(runes[:255])
	}
	rule, err := s.repo.UpsertGrayRule(ctx, in)
	if err != nil {
		return nil, err
	}
	s.invalidateAllSettings()
	return rule, nil
}

func (s *recommendationService) DeleteGrayRule(ctx context.Context, id int) error {
	if id <= 0 {
		return domain.ErrBadRequest
	}
	if err := s.repo.DeleteGrayRule(ctx, id); err != nil {
		return err
	}
	s.invalidateAllSettings()
	return nil
}

func (s *recommendationService) Stats(ctx context.Context) (*domain.RecommendationStats, error) {
	text, image, err := s.repo.CountEmbeddings(ctx)
	if err != nil {
		return nil, err
	}
	pendingText, pendingImage, err := s.repo.CountPending(ctx)
	if err != nil {
		return nil, err
	}
	posts, err := s.repo.CountPosts(ctx)
	if err != nil {
		return nil, err
	}
	s.idxMu.RLock()
	indexSize, indexText, indexImage := 0, 0, 0
	var loadedAt time.Time
	if s.idx != nil {
		indexSize = len(s.idx.posts)
		indexText = len(s.idx.text)
		indexImage = len(s.idx.image)
		loadedAt = s.idx.builtAt
	}
	s.idxMu.RUnlock()
	s.statsMu.RLock()
	lastRun, lastErr := s.lastRunAt, s.lastError
	s.statsMu.RUnlock()
	provider := s.cfg.provider
	if !s.cfg.enabled {
		provider = "disabled"
	}
	return &domain.RecommendationStats{
		Provider:      provider,
		Model:         s.cfg.model,
		Dims:          s.cfg.dims,
		ImageEnabled:  s.cfg.imageEnabled,
		Available:     s.available(),
		PostsTotal:    posts,
		TextEmbedded:  text,
		ImageEmbedded: image,
		PendingText:   pendingText,
		PendingImage:  pendingImage,
		IndexSize:     indexSize,
		IndexText:     indexText,
		IndexImage:    indexImage,
		IndexLoadedAt: loadedAt,
		LastRunAt:     lastRun,
		LastError:     lastErr,
	}, nil
}

func (s *recommendationService) Rebuild(ctx context.Context) error {
	if err := s.repo.DeleteAllEmbeddings(ctx); err != nil {
		return err
	}
	s.idxMu.Lock()
	s.idx = &recIndex{
		posts:   s.idx.posts,
		text:    map[int][]float32{},
		image:   map[int][]float32{},
		builtAt: time.Now(),
	}
	s.idxMu.Unlock()
	s.profileMu.Lock()
	s.profiles = map[int]*tasteProfile{}
	s.profileMu.Unlock()
	s.invalidateAllSettings()
	return nil
}

func (s *recommendationService) RecordReadEvent(ctx context.Context, userID, postID, dwellMS int) error {
	if userID <= 0 || postID <= 0 || dwellMS < 0 || dwellMS > 10*60*1000 {
		return domain.ErrBadRequest
	}
	if err := s.repo.CreateReadEvent(ctx, userID, postID, dwellMS); err != nil {
		return err
	}
	s.invalidateUser(ctx, userID)
	return nil
}

// ---------- 画像 ----------

func (s *recommendationService) profileFor(ctx context.Context, userID int) (*tasteProfile, error) {
	s.profileMu.Lock()
	if p, ok := s.profiles[userID]; ok && time.Since(p.builtAt) < s.cfg.profileTTL {
		s.profileMu.Unlock()
		return p, nil
	}
	s.profileMu.Unlock()

	browses, err := s.repo.ListUserBrowses(ctx, userID, 300)
	if err != nil {
		return nil, err
	}
	likes, _ := s.repo.ListUserLikes(ctx, userID, 200)
	comments, _ := s.repo.ListUserComments(ctx, userID, 200)
	saves, _ := s.repo.ListUserSaves(ctx, userID, 200)
	coins, _ := s.repo.ListUserCoinThrows(ctx, userID, 200)
	reads, _ := s.repo.ListUserReadEvents(ctx, userID, 200)
	authored, _ := s.repo.ListUserAuthoredPosts(ctx, userID, 100)
	chats := s.chatMessagesFor(ctx, userID)
	// 已读只认「点进详情」接口写入的 post_read_events，默认全部未读。
	// 不拿 pbrowse（feed 曝光/误触）当已读，避免不公平。
	browsed := make(map[int]struct{}, len(reads))
	registeredAt, _ := s.repo.GetUserRegisteredAt(ctx, userID)

	// 强兴趣信号：点赞/评论/收藏/投币 + 有效阅读（≥15s）+ 自己发帖 + 与小秋聊天。
	// 单纯刷 feed 产生的浏览不算，避免把「划过」误判成喜欢。
	strongSignals := len(likes) + len(comments) + len(saves) + len(coins)
	for _, ev := range reads {
		if ev.DwellMS >= 15000 {
			strongSignals++
		}
	}
	strongSignals += minInt(len(authored), 5)
	strongSignals += minInt(len(chats), 5)

	now := time.Now()
	weights := make(map[int]float64, 512)
	add := func(items []domain.RecInteraction, base float64) {
		for _, it := range items {
			if it.PostID <= 0 {
				continue
			}
			weights[it.PostID] += base * s.decay(now, it.At)
		}
	}
	// 浏览记录只作弱信号：feed 展示/误触很多，权重压到点赞的 1/15。
	add(browses, 0.2)
	add(likes, 3.0)
	add(comments, 4.0)
	add(saves, 4.5)
	add(coins, 5.0)
	// 自己发的帖子是最明确的兴趣表达之一，权重略低于主动互动。
	add(authored, 2.5)
	for _, ev := range reads {
		if ev.PostID <= 0 {
			continue
		}
		browsed[ev.PostID] = struct{}{}
		// dwell=0 只是「点进详情」的已读标记，不代表喜欢，不参与画像加权。
		if ev.DwellMS > 0 {
			weights[ev.PostID] += dwellWeight(ev.DwellMS) * s.decay(now, ev.At)
		}
	}

	p := &tasteProfile{
		browsed:       browsed,
		partitions:    map[string]float64{},
		tags:          map[string]float64{},
		strongSignals: strongSignals,
		registeredAt:  registeredAt,
		builtAt:       now,
	}

	// 小秋聊天不是帖子，单独把最近对话拼成一段文本做向量，再按条数给权重。
	// 嵌入在后台进行，不阻塞首页；算好后画像会失效，下次刷新自动带上聊天兴趣。
	chatVec := s.chatVectorFor(userID, chats)
	chatWeight := 0.0
	if len(chatVec) > 0 {
		chatWeight = math.Min(6.0, 1.0+0.25*float64(len(chats)))
	}

	s.idxMu.RLock()
	idx := s.idx
	var textAcc, imageAcc []float64
	if idx != nil {
		for id, w := range weights {
			if v, ok := idx.text[id]; ok && len(v) > 0 {
				if textAcc == nil {
					textAcc = make([]float64, len(v))
				}
				if len(v) == len(textAcc) {
					for i, x := range v {
						textAcc[i] += w * float64(x)
					}
					p.weight += w
				}
			}
			if v, ok := idx.image[id]; ok && len(v) > 0 {
				if imageAcc == nil {
					imageAcc = make([]float64, len(v))
				}
				if len(v) == len(imageAcc) {
					for i, x := range v {
						imageAcc[i] += w * 0.5 * float64(x)
					}
				}
			}
			if meta, ok := idx.posts[id]; ok {
				if meta.Partition != "" {
					p.partitions[meta.Partition] += w
				}
				if meta.Tag != "" {
					p.tags[meta.Tag] += w
				}
			}
		}
	}
	s.idxMu.RUnlock()

	if len(chatVec) > 0 {
		if textAcc == nil {
			textAcc = make([]float64, len(chatVec))
		}
		if len(chatVec) == len(textAcc) {
			for i, x := range chatVec {
				textAcc[i] += chatWeight * float64(x)
			}
			p.weight += chatWeight
		}
	}
	p.text = unitVector(textAcc)
	p.image = unitVector(imageAcc)
	normalizePrefMap(p.partitions)
	normalizePrefMap(p.tags)

	s.profileMu.Lock()
	if len(s.profiles) >= 2048 {
		s.profiles = map[int]*tasteProfile{}
	}
	s.profiles[userID] = p
	s.profileMu.Unlock()
	return p, nil
}

// buildChatProfileText 把最近的用户提问拼成一段画像文本，限制长度避免请求过大。
func buildChatProfileText(msgs []domain.RecChatMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	const maxRunes = 3000
	var b strings.Builder
	b.WriteString("用户与小秋的聊天内容：\n")
	used := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		runes := []rune(strings.TrimSpace(msgs[i].Content))
		if len(runes) < 2 {
			continue
		}
		if room := maxRunes - used; len(runes) > room {
			runes = runes[:room]
		}
		if len(runes) == 0 {
			break
		}
		b.WriteString(string(runes))
		b.WriteByte('\n')
		used += len(runes)
		if used >= maxRunes {
			break
		}
	}
	if used == 0 {
		return ""
	}
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *recommendationService) decay(now, at time.Time) float64 {
	if at.IsZero() {
		return 0.5
	}
	age := now.Sub(at).Hours()
	if age < 0 {
		age = 0
	}
	// 兴趣半衰期 45 天；下限 0.1 避免很久以前的行为完全失效。
	return math.Max(0.1, math.Pow(0.5, age/(24*45)))
}

func dwellWeight(dwellMS int) float64 {
	switch {
	case dwellMS < 3000:
		return -0.4
	case dwellMS < 15000:
		return 0.4
	case dwellMS < 60000:
		return 1.0
	case dwellMS < 180000:
		return 1.8
	default:
		return 2.5
	}
}

func unitVector(acc []float64) []float32 {
	if len(acc) == 0 {
		return nil
	}
	var sum float64
	for _, v := range acc {
		sum += v * v
	}
	if sum <= 1e-12 {
		return nil
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(acc))
	for i, v := range acc {
		out[i] = float32(v / norm)
	}
	return out
}

func normalizePrefMap(m map[string]float64) {
	if len(m) == 0 {
		return
	}
	var max float64
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		for k := range m {
			delete(m, k)
		}
		return
	}
	for k, v := range m {
		m[k] = v / max
	}
}

// ---------- 排序 ----------

func (s *recommendationService) RankPostIDs(ctx context.Context, userID int, exclude map[int]struct{}, limit, offset int) ([]int, int, error) {
	if limit <= 0 {
		limit = 15
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	// 翻页优先复用第一页算出的完整排序，保证分页稳定、不重不漏。
	if offset > 0 {
		if ids, ok := s.getRankCache(ctx, userID); ok {
			ids = filterExcludedIDs(ids, exclude)
			total := len(ids)
			if offset >= total {
				return nil, total, nil
			}
			end := offset + limit
			if end > total {
				end = total
			}
			return ids[offset:end], total, nil
		}
	}
	// 非首页信息流（如新帖轮询 limit=20、置顶刷新 limit=1）只借用现有排序，
	// 不重新计算、也不覆盖分页缓存；真正的 feed 每页固定 15 条。
	if offset == 0 && limit != recFeedPageSize {
		if ids, ok := s.getRankCache(ctx, userID); ok {
			ids = filterExcludedIDs(ids, exclude)
			total := len(ids)
			if total == 0 {
				return nil, 0, nil
			}
			end := limit
			if end > total {
				end = total
			}
			return ids[:end], total, nil
		}
	}

	taste, err := s.profileFor(ctx, userID)
	if err != nil {
		logRecf("profile build failed user=%d: %v", userID, err)
		taste = nil
	}
	now := time.Now()
	coldStart := s.isColdStart(taste, now)
	pattern := recPatternPersonalized
	if coldStart {
		pattern = recPatternColdStart
	}
	impressions := s.impressionsFor(ctx, userID)

	s.idxMu.RLock()
	idx := s.idx
	buckets := [3][]rankEntry{}
	for id, meta := range idx.posts {
		if meta.IsPrivate && meta.UserID != userID {
			continue
		}
		if _, skip := exclude[id]; skip {
			continue
		}
		if meta.IsPinned && meta.PinnedExpiresAt != nil && meta.PinnedExpiresAt.After(now) {
			continue
		}
		read := false
		if taste != nil {
			_, read = taste.browsed[id]
		}
		bucket := recBucketFor(meta.PostTime, now)
		mode := recScoreResurface
		switch {
		case coldStart:
			mode = recScoreCold
		case bucket == recBucketFresh:
			mode = recScoreFresh
		}
		score := s.scorePost(meta, idx, taste, now, read, userID, mode)
		// 看过但没点开的帖子轻微降权；次数越多扣得越多，48 小时半衰。
		score -= impressionPenalty(impressions[id], now)
		buckets[bucket] = append(buckets[bucket], rankEntry{
			id:    id,
			score: score,
		})
	}
	s.idxMu.RUnlock()

	for i := range buckets {
		bucket := buckets[i]
		sort.Slice(bucket, func(a, b int) bool {
			if bucket[a].score == bucket[b].score {
				return bucket[a].id > bucket[b].id
			}
			return bucket[a].score > bucket[b].score
		})
		buckets[i] = bucket
	}
	ids := interleaveRecBuckets(buckets, pattern)
	s.setRankCache(ctx, userID, ids)

	total := len(ids)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return ids[offset:end], total, nil
}

// 首页按年龄分三桶，再用固定配额交错，保证老帖稳定获得曝光，
// 而不是被时间衰减永远压在新帖后面。
// recFeedPageSize 与前端 FEED_PAGE_SIZE 对齐：只有真正的首页信息流分页
// 才记录曝光并触发刷新重排，后台轮询/置顶探测不污染推荐信号。
const recFeedPageSize = 15

const (
	recBucketFresh = iota // ≤90 天
	recBucketMid          // 90~365 天
	recBucketOld          // >365 天
)

// 有画像用户：每 15 条 7 新 / 4 中 / 4 老。
var recPatternPersonalized = []int{
	recBucketFresh, recBucketMid, recBucketOld, recBucketFresh, recBucketFresh,
	recBucketMid, recBucketOld, recBucketFresh, recBucketMid, recBucketFresh,
	recBucketOld, recBucketFresh, recBucketMid, recBucketOld, recBucketFresh,
}

// 冷启动用户（新账号或强信号不足）：每 15 条 9 老 / 3 中 / 3 新，
// 优先把历史优质帖刷出来，等账号满 3 天且积累 3 个强兴趣信号后再切画像。
var recPatternColdStart = []int{
	recBucketOld, recBucketOld, recBucketOld, recBucketMid, recBucketOld,
	recBucketOld, recBucketFresh, recBucketOld, recBucketMid, recBucketOld,
	recBucketOld, recBucketFresh, recBucketMid, recBucketOld, recBucketFresh,
}

type recScoreMode int

const (
	recScoreFresh recScoreMode = iota
	recScoreResurface
	recScoreCold
)

func recBucketFor(postTime, now time.Time) int {
	age := now.Sub(postTime)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= 90*24*time.Hour:
		return recBucketFresh
	case age <= 365*24*time.Hour:
		return recBucketMid
	default:
		return recBucketOld
	}
}

func (s *recommendationService) isColdStart(t *tasteProfile, now time.Time) bool {
	if t == nil {
		return true
	}
	if len(t.text) == 0 && len(t.image) == 0 {
		return true
	}
	if t.strongSignals < s.cfg.coldStartSignals {
		return true
	}
	if t.registeredAt != nil && now.Sub(*t.registeredAt) < time.Duration(s.cfg.coldStartDays)*24*time.Hour {
		return true
	}
	return false
}

// interleaveRecBuckets 按配额交错三个年龄桶；某个桶耗尽时从其余桶补位，
// 保证全量帖子只出现一次且分页稳定。
func interleaveRecBuckets(buckets [3][]rankEntry, pattern []int) []int {
	pos := [3]int{}
	total := len(buckets[0]) + len(buckets[1]) + len(buckets[2])
	out := make([]int, 0, total)
	take := func(b int) bool {
		if pos[b] >= len(buckets[b]) {
			return false
		}
		out = append(out, buckets[b][pos[b]].id)
		pos[b]++
		return true
	}
	for _, want := range pattern {
		if take(want) {
			continue
		}
		for _, alt := range [3]int{recBucketFresh, recBucketMid, recBucketOld} {
			if take(alt) {
				break
			}
		}
	}
	for {
		moved := false
		for _, b := range [3]int{recBucketFresh, recBucketMid, recBucketOld} {
			if take(b) {
				moved = true
				break
			}
		}
		if !moved {
			break
		}
	}
	return out
}

func (s *recommendationService) scorePost(p domain.RecPostMeta, idx *recIndex, taste *tasteProfile, now time.Time, read bool, viewerID int, mode recScoreMode) float64 {
	ageHours := now.Sub(p.PostTime).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	timeScore := math.Exp(-ageHours / s.cfg.halfLifeHours)
	// 浏览量来自详情页曝光，不能代表「喜欢」；质量只用主动互动。
	engagement := float64(2*p.LikeNum + 3*p.CommentNum + 2*p.CoinNum)
	quality := clamp01(math.Log1p(engagement) / 6.0)
	readAdj := 0.05
	if read {
		readAdj = -0.25
	}

	simScore := s.similarityScore(p, idx, taste)
	affinity := 0.0
	if taste != nil {
		if v, ok := taste.partitions[p.Partition]; ok {
			affinity += 0.04 * v
		}
		if v, ok := taste.tags[p.Tag]; ok {
			affinity += 0.03 * v
		}
	}

	var score float64
	switch mode {
	case recScoreCold:
		// 冷启动：质量优先，历史优质帖排最前；有弱画像时只做轻量修正。
		score = 0.70*quality + 0.20*simScore + readAdj + affinity
	case recScoreFresh:
		score = 0.45*timeScore + 0.35*simScore + 0.15*quality + readAdj + affinity
	default:
		// 中/老帖不再乘时间衰减，按「像不像 + 好不好」排，
		// 靠年龄桶配额而不是时间分保证曝光。
		score = 0.60*simScore + 0.30*quality + readAdj + affinity
	}
	if p.UserID == viewerID {
		score -= 0.08
	}
	return score
}

func (s *recommendationService) similarityScore(p domain.RecPostMeta, idx *recIndex, taste *tasteProfile) float64 {
	if taste == nil {
		return 0
	}
	sim, weight := 0.0, 0.0
	if len(taste.text) > 0 {
		if v, ok := idx.text[p.ID]; ok && len(v) > 0 {
			sim += 0.85 * dotVec(v, taste.text)
			weight += 0.85
		}
	}
	if len(taste.image) > 0 {
		if v, ok := idx.image[p.ID]; ok && len(v) > 0 {
			sim += 0.15 * dotVec(v, taste.image)
			weight += 0.15
		}
	}
	if weight == 0 {
		return 0
	}
	return clamp01((sim/weight - 0.10) / 0.65)
}

func recRankKey(userID int) string {
	return fmt.Sprintf("rec:feed:v2:%d", userID)
}

// filterExcludedIDs 缓存排序生成后置顶队列可能变化，翻页时再剔除一次，
// 避免置顶帖同时出现在置顶区和普通列表。
func filterExcludedIDs(ids []int, exclude map[int]struct{}) []int {
	if len(exclude) == 0 {
		return ids
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, skip := exclude[id]; !skip {
			out = append(out, id)
		}
	}
	return out
}

func (s *recommendationService) getRankCache(ctx context.Context, userID int) ([]int, bool) {
	if core.MyRedis != nil {
		if raw, err := core.MyRedis.Get(ctx, recRankKey(userID)).Result(); err == nil && raw != "" {
			var ids []int
			if json.Unmarshal([]byte(raw), &ids) == nil && len(ids) > 0 {
				return ids, true
			}
		}
	}
	s.rankMu.Lock()
	defer s.rankMu.Unlock()
	if e, ok := s.rankCache[userID]; ok && time.Since(e.at) < s.cfg.rankTTL {
		return e.ids, true
	}
	return nil, false
}

func (s *recommendationService) setRankCache(ctx context.Context, userID int, ids []int) {
	if len(ids) == 0 {
		return
	}
	if core.MyRedis != nil {
		if raw, err := json.Marshal(ids); err == nil {
			_ = core.MyRedis.Set(ctx, recRankKey(userID), raw, s.cfg.rankTTL).Err()
		}
	}
	s.rankMu.Lock()
	if len(s.rankCache) >= 1024 {
		s.rankCache = map[int]recRankCacheEntry{}
	}
	s.rankCache[userID] = recRankCacheEntry{ids: ids, at: time.Now()}
	s.rankMu.Unlock()
}
