package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"loginTest/config"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"

	"golang.org/x/crypto/bcrypt"
)

const (
	ssoStateTTL  = 10 * time.Minute
	ssoTicketTTL = 2 * time.Minute
	ssoScope     = "openid profile email"

	// ticket 类型
	ssoTicketLogin      = "login"
	ssoTicketBindNeeded = "bind_needed"
	ssoTicketBound      = "bound"
)

// 回调阶段错误（controller 映射为 302 ?error=state|exchange|conflict）。
var (
	ErrSsoStateInvalid = errors.New("sso state invalid or replayed")
	ErrSsoExchange     = errors.New("sso token exchange failed")
	ErrSsoConflict     = errors.New("sso binding conflict")
)

// ssoKV state/ticket 的一次性 KV；生产为 Redis，单测注入内存实现。
type ssoKV interface {
	Set(ctx context.Context, key, val string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	GetDel(ctx context.Context, key string) (string, error)
}

type redisSsoKV struct{}

func (redisSsoKV) Set(ctx context.Context, key, val string, ttl time.Duration) error {
	return core.MyRedis.Set(ctx, key, val, ttl).Err()
}

func (redisSsoKV) Get(ctx context.Context, key string) (string, error) {
	return core.MyRedis.Get(ctx, key).Result()
}

func (redisSsoKV) GetDel(ctx context.Context, key string) (string, error) {
	return core.MyRedis.GetDel(ctx, key).Result()
}

// SsoUserAuthRepo 收窄依赖：绑定/解绑验证只需这两个查询（gorm.UserRepository 已满足）。
type SsoUserAuthRepo interface {
	FindByID(ctx context.Context, id int) (*domain.User, error)
	FindAuthByEmail(ctx context.Context, email string) (*domain.User, string, error)
}

// SsoTokenIssuer 发令牌窄依赖，UserService 天然满足。
// 用窄接口而非 UserService，便于单测用最小 fake 注入。
type SsoTokenIssuer interface {
	IssueTokensForUser(ctx context.Context, userID int) (*domain.AuthTokens, error)
}

type ssoService struct {
	bindings repository.SsoBindingRepository
	users    SsoUserAuthRepo
	tokens   SsoTokenIssuer
	kv       ssoKV
	http     *http.Client
}

// NewSsoService 生产装配：Redis KV + 真实 HTTP。
func NewSsoService(bindings repository.SsoBindingRepository, users SsoUserAuthRepo, tokens SsoTokenIssuer) SsoService {
	return &ssoService{
		bindings: bindings,
		users:    users,
		tokens:   tokens,
		kv:       redisSsoKV{},
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// NewSsoServiceForTest 测试装配：注入内存 KV + httptest 客户端。
func NewSsoServiceForTest(bindings repository.SsoBindingRepository, users SsoUserAuthRepo,
	tokens SsoTokenIssuer, kv ssoKV, httpClient *http.Client) SsoService {
	return &ssoService{bindings: bindings, users: users, tokens: tokens, kv: kv, http: httpClient}
}

// SsoPKCEVerifier 生成高熵 code_verifier（RFC 7636 §4.1，64 字节随机 base64url）。
func SsoPKCEVerifier() string {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// SsoPKCEChallenge S256: BASE64URL(SHA256(verifier))，无填充。
func SsoPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func ssoStateKey(state string) string { return "sso:state:" + state }
func ssoTicketKey(t string) string   { return "sso:ticket:" + t }

func (s *ssoService) BuildAuthorizeURL(ctx context.Context, bindUID int) (string, error) {
	if !config.SsoEnabled() {
		return "", domain.ErrBadRequest
	}
	state := util.GenerateRandomString(32)
	verifier := SsoPKCEVerifier()
	payload, err := json.Marshal(domain.SsoStatePayload{Verifier: verifier, BindUID: bindUID})
	if err != nil {
		return "", err
	}
	if err := s.kv.Set(ctx, ssoStateKey(state), string(payload), ssoStateTTL); err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", config.SsoClientID())
	q.Set("redirect_uri", config.SsoRedirectURI())
	q.Set("scope", ssoScope)
	q.Set("state", state)
	q.Set("code_challenge", SsoPKCEChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	return config.SsoAuthorizeURL() + "?" + q.Encode(), nil
}

// exchangeCode POST /api/oauth/token（grant_type=authorization_code + PKCE + client_secret）。
func (s *ssoService) exchangeCode(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", config.SsoClientID())
	form.Set("client_secret", config.SsoClientSecret())
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", config.SsoRedirectURI())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.SsoTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sso token endpoint status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("sso token response invalid: %s", body)
	}
	return out.AccessToken, nil
}

// fetchUserinfo GET /api/oauth/userinfo，Bearer access_token。
func (s *ssoService) fetchUserinfo(ctx context.Context, accessToken string) (*domain.SsoUserinfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.SsoUserinfoURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sso userinfo status %d: %s", resp.StatusCode, body)
	}
	var info domain.SsoUserinfo
	if err := json.Unmarshal(body, &info); err != nil || info.Sub == "" {
		return nil, fmt.Errorf("sso userinfo invalid: %s", body)
	}
	return &info, nil
}

func (s *ssoService) ConsumeCallback(ctx context.Context, code, state string) (string, error) {
	if code == "" || state == "" {
		return "", ErrSsoStateInvalid
	}
	raw, err := s.kv.GetDel(ctx, ssoStateKey(state))
	if err != nil || raw == "" {
		return "", ErrSsoStateInvalid // 不存在/过期/重放
	}
	var st domain.SsoStatePayload
	if err := json.Unmarshal([]byte(raw), &st); err != nil || st.Verifier == "" {
		return "", ErrSsoStateInvalid
	}

	accessToken, err := s.exchangeCode(ctx, code, st.Verifier)
	if err != nil {
		return "", ErrSsoExchange
	}
	info, err := s.fetchUserinfo(ctx, accessToken)
	if err != nil {
		return "", ErrSsoExchange
	}

	// 分派：sub 已绑定 → 直接登录（或绑定场景下的幂等/冲突判定）
	binding, err := s.bindings.FindBySub(ctx, info.Sub)
	switch {
	case err == nil:
		if st.BindUID > 0 && st.BindUID != binding.LocalUserID {
			return "", ErrSsoConflict // 该 SSO 身份已绑在别的集市账号上
		}
		return s.saveTicket(ctx, &domain.SsoTicketPayload{Type: ssoTicketLogin, UID: binding.LocalUserID})
	case errors.Is(err, domain.ErrNotFound):
		// sub 未绑定，继续
	default:
		return "", err
	}

	if st.BindUID > 0 { // 已登录用户主动绑定：校验本账号未绑 → 直接写绑定
		if _, err := s.bindings.FindByUserID(ctx, st.BindUID); err == nil {
			return "", ErrSsoConflict // 本集市账号已有绑定
		}
		if err := s.bindings.Create(ctx, &domain.SsoBinding{
			LocalUserID: st.BindUID,
			SsoSub:      info.Sub,
			SsoUsername: info.PreferredUsername,
			CreatedAt:   time.Now(),
		}); err != nil {
			return "", err
		}
		return s.saveTicket(ctx, &domain.SsoTicketPayload{Type: ssoTicketBound, UID: st.BindUID})
	}

	// 未登录且未绑定 → 待绑定 ticket（前端展示绑定/注册引导）
	return s.saveTicket(ctx, &domain.SsoTicketPayload{
		Type:        ssoTicketBindNeeded,
		Sub:         info.Sub,
		SsoUsername: info.PreferredUsername,
		SsoEmail:    info.Email,
	})
}

func (s *ssoService) saveTicket(ctx context.Context, p *domain.SsoTicketPayload) (string, error) {
	ticket := util.GenerateRandomString(32)
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	if err := s.kv.Set(ctx, ssoTicketKey(ticket), string(raw), ssoTicketTTL); err != nil {
		return "", err
	}
	return ticket, nil
}

func (s *ssoService) ExchangeTicket(ctx context.Context, ticket string) (*domain.SsoExchangeResult, error) {
	p, err := s.peekTicket(ctx, ticket)
	if err != nil {
		return nil, err
	}
	switch p.Type {
	case ssoTicketLogin, ssoTicketBound:
		// 一次性：GETDEL 抢占，并发重放第二次落空
		if _, err := s.kv.GetDel(ctx, ssoTicketKey(ticket)); err != nil {
			return nil, domain.ErrUnauthorized
		}
		tokens, err := s.tokens.IssueTokensForUser(ctx, p.UID)
		if err != nil {
			return nil, err
		}
		return &domain.SsoExchangeResult{
			Type:         p.Type,
			Token:        tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
		}, nil
	case ssoTicketBindNeeded:
		// 不消费：等 POST /api/sso/bind 最终消费
		return &domain.SsoExchangeResult{
			Type:        p.Type,
			SsoUsername: p.SsoUsername,
			SsoEmail:    p.SsoEmail,
		}, nil
	}
	return nil, domain.ErrUnauthorized
}

func (s *ssoService) peekTicket(ctx context.Context, ticket string) (*domain.SsoTicketPayload, error) {
	if ticket == "" {
		return nil, domain.ErrUnauthorized
	}
	raw, err := s.kv.Get(ctx, ssoTicketKey(ticket))
	if err != nil || raw == "" {
		return nil, domain.ErrUnauthorized
	}
	var p domain.SsoTicketPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil || p.Type == "" {
		return nil, domain.ErrUnauthorized
	}
	return &p, nil
}

func (s *ssoService) consumeTicket(ctx context.Context, ticket string) (*domain.SsoTicketPayload, error) {
	if ticket == "" {
		return nil, domain.ErrUnauthorized
	}
	raw, err := s.kv.GetDel(ctx, ssoTicketKey(ticket))
	if err != nil || raw == "" {
		return nil, domain.ErrUnauthorized // 无效/过期/已消费
	}
	var p domain.SsoTicketPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil || p.Type == "" {
		return nil, domain.ErrUnauthorized
	}
	return &p, nil
}

func (s *ssoService) BindWithCredentials(ctx context.Context, ticket, email, password string) (*domain.AuthTokens, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 6 {
		return nil, domain.ErrBadRequest
	}
	p, err := s.consumeTicket(ctx, ticket) // 一次性：无论后续成败，ticket 已消费
	if err != nil {
		return nil, err
	}
	if p.Type != ssoTicketBindNeeded || p.Sub == "" {
		return nil, domain.ErrUnauthorized
	}

	u, hash, err := s.users.FindAuthByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound // 422：账号不存在
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, domain.ErrUnauthorized // 401：密码错误
	}
	if _, err := s.bindings.FindByUserID(ctx, u.ID); err == nil {
		return nil, domain.ErrConflict // 该集市账号已绑定其他 SSO
	}
	if err := s.bindings.Create(ctx, &domain.SsoBinding{
		LocalUserID: u.ID,
		SsoSub:      p.Sub,
		SsoUsername: p.SsoUsername,
		CreatedAt:   time.Now(),
	}); err != nil {
		return nil, err // 含并发下的唯一索引冲突 → ErrConflict
	}
	return s.tokens.IssueTokensForUser(ctx, u.ID)
}

func (s *ssoService) GetStatus(ctx context.Context, userID int) (*domain.SsoStatus, error) {
	b, err := s.bindings.FindByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return &domain.SsoStatus{Bound: false}, nil
		}
		return nil, err
	}
	return &domain.SsoStatus{Bound: true, SsoUsername: b.SsoUsername, BoundAt: b.CreatedAt}, nil
}

func (s *ssoService) Unbind(ctx context.Context, userID int, password string) error {
	if userID <= 0 || len(password) < 6 {
		return domain.ErrBadRequest
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	_, hash, err := s.users.FindAuthByEmail(ctx, u.Email)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return domain.ErrUnauthorized
	}
	return s.bindings.DeleteByUserID(ctx, userID)
}
