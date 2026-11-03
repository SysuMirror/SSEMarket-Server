package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"loginTest/domain"
)

// recConfig 推荐系统的运行时配置，全部来自环境变量，未配置时自动降级为「时间流」。
type recConfig struct {
	enabled          bool
	provider         string // dashscope | spark | none
	baseURL          string
	apiKey           string
	model            string
	dims             int
	batchSize        int
	imageEnabled     bool
	halfLifeHours    float64
	profileTTL       time.Duration
	rankTTL          time.Duration
	indexRefresh     time.Duration
	coldStartDays    int
	coldStartSignals int
	chatBotUID       int
	chatEnabled      bool
	chatBase         string
	chatProfileTTL   time.Duration
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func loadRecConfig() recConfig {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("RECOMMEND_EMBED_PROVIDER")))
	if provider == "" {
		provider = "dashscope"
	}
	if provider == "off" || provider == "none" {
		provider = "none"
	}
	base := strings.TrimSpace(os.Getenv("RECOMMEND_EMBED_BASE"))
	if base == "" {
		if provider == "spark" {
			base = "https://spark.ssemarket.cn/embed/v1"
		} else {
			base = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
	}
	base = strings.TrimRight(base, "/")

	key := strings.TrimSpace(os.Getenv("RECOMMEND_EMBED_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY"))
	}
	modelName := strings.TrimSpace(os.Getenv("RECOMMEND_EMBED_MODEL"))
	if modelName == "" {
		if provider == "spark" {
			modelName = "bge-m3"
		} else {
			modelName = "text-embedding-v4"
		}
	}
	batch := envInt("RECOMMEND_EMBED_BATCH", 10)
	if provider == "spark" && batch < 20 {
		batch = 100
	}
	chatBase := strings.TrimRight(strings.TrimSpace(os.Getenv("RECOMMEND_CHAT_BASE")), "/")
	if chatBase == "" {
		chatBase = "http://host.docker.internal:8110"
	}
	cfg := recConfig{
		enabled:          envBool("RECOMMEND_ENABLED", true) && provider != "none" && key != "",
		provider:         provider,
		baseURL:          base,
		apiKey:           key,
		model:            modelName,
		dims:             envInt("RECOMMEND_EMBED_DIMS", 1024),
		batchSize:        batch,
		imageEnabled:     provider == "spark" && envBool("RECOMMEND_EMBED_IMAGE", true),
		halfLifeHours:    24 * 7,
		profileTTL:       envDuration("RECOMMEND_PROFILE_TTL", 5*time.Minute),
		rankTTL:          envDuration("RECOMMEND_RANK_TTL", 20*time.Minute),
		indexRefresh:     envDuration("RECOMMEND_INDEX_REFRESH", 2*time.Minute),
		coldStartDays:    envInt("RECOMMEND_COLD_START_DAYS", 3),
		coldStartSignals: envInt("RECOMMEND_COLD_START_SIGNALS", 3),
		chatBotUID:       envInt("XQ_BOT_UID", 95317),
		chatEnabled:      envBool("RECOMMEND_CHAT_ENABLED", true),
		chatBase:         chatBase,
		chatProfileTTL:   envDuration("RECOMMEND_CHAT_PROFILE_TTL", 30*time.Minute),
	}
	if v := strings.TrimSpace(os.Getenv("RECOMMEND_TIME_HALF_LIFE")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.halfLifeHours = d.Hours()
		}
	}
	return cfg
}

// embeddingClient 兼容 OpenAI /embeddings 协议的文本嵌入客户端。
// DashScope 与 Spark 网关都走同一套协议；Spark 额外支持 /multimodal-embeddings（图片）。
type embeddingClient struct {
	cfg  recConfig
	http *http.Client
}

func newEmbeddingClient(cfg recConfig) *embeddingClient {
	return &embeddingClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *embeddingClient) Enabled() bool { return c != nil && c.cfg.enabled }

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error any `json:"error"`
}

func (c *embeddingClient) postJSON(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(raw)
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return fmt.Errorf("embed api %s: status=%d body=%s", path, resp.StatusCode, snippet)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("embed api %s: decode: %w", path, err)
	}
	return nil
}

// EmbedTexts 批量文本嵌入。返回顺序与入参一致。
func (c *embeddingClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("embedding disabled")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	payload := map[string]any{
		"input":      texts,
		"dimensions": c.cfg.dims,
	}
	if c.cfg.provider == "spark" {
		payload["mode"] = "document"
		payload["model"] = c.cfg.model
	} else {
		payload["model"] = c.cfg.model
		payload["encoding_format"] = "float"
	}
	var out embedResponse
	if err := c.postJSON(ctx, "/embeddings", payload, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embed api: expected %d vectors, got %d", len(texts), len(out.Data))
	}
	vecs := make([][]float32, len(texts))
	for i, item := range out.Data {
		idx := item.Index
		if idx < 0 || idx >= len(vecs) {
			idx = i
		}
		vecs[idx] = normalizeVec(item.Embedding)
	}
	for i := range vecs {
		if len(vecs[i]) == 0 {
			return nil, fmt.Errorf("embed api: empty vector at index %d", i)
		}
	}
	return vecs, nil
}

// EmbedImages 图片嵌入（仅 Spark 网关支持）。urls 需为公网可访问图片。
func (c *embeddingClient) EmbedImages(ctx context.Context, urls []string) ([][]float32, error) {
	if !c.Enabled() || !c.cfg.imageEnabled {
		return nil, fmt.Errorf("image embedding disabled")
	}
	if len(urls) == 0 {
		return nil, nil
	}
	inputs := make([]map[string]string, 0, len(urls))
	for _, u := range urls {
		inputs = append(inputs, map[string]string{"image": u})
	}
	payload := map[string]any{
		"inputs":     inputs,
		"mode":       "document",
		"dimensions": c.cfg.dims,
	}
	var out embedResponse
	if err := c.postJSON(ctx, "/multimodal-embeddings", payload, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(urls) {
		return nil, fmt.Errorf("multimodal embed api: expected %d vectors, got %d", len(urls), len(out.Data))
	}
	vecs := make([][]float32, len(urls))
	for i, item := range out.Data {
		idx := item.Index
		if idx < 0 || idx >= len(vecs) {
			idx = i
		}
		vecs[idx] = normalizeVec(item.Embedding)
	}
	return vecs, nil
}

func normalizeVec(v []float32) []float32 {
	if len(v) == 0 {
		return nil
	}
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum <= 0 {
		return v
	}
	norm := float32(math.Sqrt(sum))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func dotVec(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

var (
	recMDImageRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	recMDLinkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]+\)`)
	recURLRe     = regexp.MustCompile(`https?://\S+`)
	recHTMLRe    = regexp.MustCompile(`<[^>]+>`)
	recSpaceRe   = regexp.MustCompile(`\s+`)
)

// buildPostEmbedText 把帖子压缩成适合嵌入的文本：标题 + 分区 + 标签 + 去噪正文。
func buildPostEmbedText(c domain.RecPostContent) string {
	body := c.Ptext
	body = recMDImageRe.ReplaceAllString(body, " ")
	body = recMDLinkRe.ReplaceAllString(body, "$1")
	body = recURLRe.ReplaceAllString(body, " ")
	body = recHTMLRe.ReplaceAllString(body, " ")
	body = strings.ReplaceAll(body, "```", " ")
	body = recSpaceRe.ReplaceAllString(body, " ")
	body = strings.TrimSpace(body)
	runes := []rune(body)
	if len(runes) > 1600 {
		body = string(runes[:1600])
	}
	parts := make([]string, 0, 5)
	if t := strings.TrimSpace(c.Title); t != "" {
		parts = append(parts, "标题："+t)
	}
	if p := strings.TrimSpace(c.Partition); p != "" {
		parts = append(parts, "分区："+p)
	}
	if t := strings.TrimSpace(c.Tag); t != "" {
		parts = append(parts, "标签："+t)
	}
	if body != "" {
		parts = append(parts, "正文："+body)
	}
	if len(parts) == 0 {
		parts = append(parts, c.Title)
	}
	return strings.Join(parts, "\n")
}

func contentHash(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func isAllowedRecImageURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "image.ssemarket.cn"
}

// extractFirstImageURL 取帖子首图：优先 photos 列，其次正文 markdown 图片。
func extractFirstImageURL(c domain.RecPostContent) string {
	if u := strings.TrimSpace(c.Photos); u != "" {
		if u = strings.Fields(u)[0]; isAllowedRecImageURL(u) {
			return u
		}
	}
	for _, m := range recMDImageRe.FindAllStringSubmatch(c.Ptext, -1) {
		if len(m) >= 2 && isAllowedRecImageURL(m[1]) {
			return m[1]
		}
	}
	return ""
}

// logRecf 统一日志前缀，避免把 key 等敏感信息带进日志。
func logRecf(format string, args ...any) {
	log.Printf("[recommend] "+format, args...)
}
