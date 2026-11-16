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
	"strconv"
	"strings"
	"time"

	"loginTest/config"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"
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

// 回调阶段错误（controller 映射为 302 ?error=state|exchange|conflict|internal）。
var (
	ErrSsoStateInvalid = errors.New("sso state invalid or replayed")
	ErrSsoExchange     = errors.New("sso token exchange failed")
	ErrSsoConflict     = errors.New("sso binding conflict")
	ErrSsoForbidden    = errors.New("sso user is not internal")
)

// M2M 密码校验（/api/auth/login 主路径）错误，controller 映射为明确提示。
var (
	ErrSsoBadCredentials  = errors.New("sso invalid email or password")
	ErrSsoUnavailable     = errors.New("sso service unavailable")
	ErrSsoNoLocalAccount  = errors.New("sso identity has no local market account")
	ErrSsoVerifyForbidden = errors.New("sso password verify forbidden")
	ErrSsoTooManyAttempts = errors.New("sso password verify too many attempts")
	ErrSsoInvalidSession  = errors.New("sso session invalid or expired")
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
func ssoTicketKey(t string) string    { return "sso:ticket:" + t }

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
	// 准入红线：只有 unisso 判定为院内（internal=true）的账号才允许进入集市，
	// 登录与绑定两条路径共用此校验。
	if !bool(info.Internal) {
		return "", ErrSsoForbidden
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

// verifyPassword 调 unisso M2M POST /api/oauth/password/verify 校验邮箱密码（登录主路径，无跳转）。
// 同时返回 unisso 签发的共享 SSO 会话元数据，供登录响应种父域 Cookie。
func (s *ssoService) verifyPassword(ctx context.Context, email, password string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	form := url.Values{}
	form.Set("client_id", config.SsoClientID())
	form.Set("client_secret", config.SsoClientSecret())
	form.Set("email", email)
	form.Set("password", password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.SsoPasswordVerifyURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, ErrSsoUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, nil, ErrSsoUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)
		switch {
		case resp.StatusCode == http.StatusUnauthorized && strings.Contains(bodyStr, "invalid_credentials"):
			return nil, nil, ErrSsoBadCredentials
		case resp.StatusCode == http.StatusForbidden && strings.Contains(bodyStr, "password_verify_forbidden"):
			return nil, nil, ErrSsoVerifyForbidden
		case resp.StatusCode == http.StatusTooManyRequests:
			// unisso 按 IP+email 锁 15 分钟，契约要求平台侧不得自动重试
			return nil, nil, ErrSsoTooManyAttempts
		default:
			// 400 invalid_request / 401 invalid_client / 5xx：客户端或服务端故障
			return nil, nil, ErrSsoUnavailable
		}
	}
	var out struct {
		OK   bool                          `json:"ok"`
		User *domain.SsoPasswordVerifyUser `json:"user"`
		domain.SsoSession
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.OK || out.User == nil || out.User.Sub == "" {
		return nil, nil, ErrSsoUnavailable
	}
	return out.User, &out.SsoSession, nil
}

// ObtainPasswordSession 注册成功后补取 unisso 共享会话：unisso 尚无该账号时返回错误，
// 调用方必须忽略，不得影响注册结果。
func (s *ssoService) ObtainPasswordSession(ctx context.Context, email, password string) (*domain.SsoSession, error) {
	if config.SsoPasswordVerifyURL() == "" {
		return nil, ErrSsoUnavailable
	}
	_, sess, err := s.verifyPassword(ctx, strings.TrimSpace(strings.ToLower(email)), password)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// VerifySession 调 unisso M2M POST /api/oauth/session/verify 校验共享 Cookie 里的会话指针。
// 成功会滑动续期；401 会话失效、其余状态一律按不可用处理，前端静默回落登录表单。
func (s *ssoService) VerifySession(ctx context.Context, session string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	if session == "" || config.SsoSessionVerifyURL() == "" {
		return nil, nil, ErrSsoInvalidSession
	}
	form := url.Values{}
	form.Set("client_id", config.SsoClientID())
	form.Set("client_secret", config.SsoClientSecret())
	form.Set("session", session)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.SsoSessionVerifyURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, ErrSsoUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, nil, ErrSsoUnavailable
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, ErrSsoInvalidSession
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, ErrSsoUnavailable
	}
	var out struct {
		OK   bool                          `json:"ok"`
		User *domain.SsoPasswordVerifyUser `json:"user"`
		domain.SsoSession
	}
	if err := json.Unmarshal(body, &out); err != nil || !out.OK || out.User == nil || out.User.Sub == "" {
		return nil, nil, ErrSsoUnavailable
	}
	return out.User, &out.SsoSession, nil
}

// VerifyPasswordIdentity 对外暴露纯 unisso 密码校验（管理台登录复用）。
func (s *ssoService) VerifyPasswordIdentity(ctx context.Context, email, password string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return nil, nil, domain.ErrBadRequest
	}
	return s.verifyPassword(ctx, email, password)
}

// VerifySessionIdentity 对外暴露纯共享会话校验（管理台静默登录复用）。
func (s *ssoService) VerifySessionIdentity(ctx context.Context, session string) (*domain.SsoPasswordVerifyUser, *domain.SsoSession, error) {
	return s.VerifySession(ctx, session)
}

// RevokeSession 调 unisso 吊销共享会话（best-effort：平台登出无论如何都要清本地会话与 Cookie）。
func (s *ssoService) RevokeSession(ctx context.Context, session string) {
	if session == "" || config.SsoSessionRevokeURL() == "" {
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	form := url.Values{}
	form.Set("client_id", config.SsoClientID())
	form.Set("client_secret", config.SsoClientSecret())
	form.Set("session", session)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, config.SsoSessionRevokeURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
}

// LoginViaSession 静默 SSO：共享 Cookie → unisso session/verify → 本地账号定位 → 本地 JWT。
func (s *ssoService) LoginViaSession(ctx context.Context, session string) (*domain.AuthTokens, error) {
	u, _, err := s.VerifySession(ctx, session)
	if err != nil {
		return nil, err
	}
	return s.loginUnissoUser(ctx, u, u.Email)
}

// LoginViaUnisso 主模式登录：unisso 密码校验 → internal 准入 → 定位集市账号（sse_sub 优先，
// 缺失按邮箱匹配）→ 幂等补绑定 → 签发本地 JWT。身份来源是 unisso，本地密码不参与。
// 同时返回 SSO 会话元数据，由登录响应种父域 Cookie。
func (s *ssoService) LoginViaUnisso(ctx context.Context, email, password string) (*domain.AuthTokens, *domain.SsoSession, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 6 {
		return nil, nil, domain.ErrBadRequest
	}
	if config.SsoPasswordVerifyURL() == "" {
		return nil, nil, ErrSsoUnavailable
	}
	u, sess, err := s.verifyPassword(ctx, email, password)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := s.loginUnissoUser(ctx, u, email)
	if err != nil {
		return nil, nil, err
	}
	return tokens, sess, nil
}

// loginUnissoUser 已通过 unisso 校验的身份 → 本地账号。密码登录与静默 SSO 共用同一套
// 准入、定位与绑定逻辑，保证两条路径行为一致。
func (s *ssoService) loginUnissoUser(ctx context.Context, u *domain.SsoPasswordVerifyUser, fallbackEmail string) (*domain.AuthTokens, error) {
	if !bool(u.Internal) {
		return nil, ErrSsoForbidden
	}

	uid := 0
	if u.SseSub != "" {
		if v, convErr := strconv.Atoi(u.SseSub); convErr == nil {
			uid = v
		}
	}
	if uid <= 0 { // 无 sse 身份（unisso 新注册号）：按邮箱匹配集市账号
		lu, _, err := s.users.FindAuthByEmail(ctx, strings.TrimSpace(strings.ToLower(fallbackEmail)))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, ErrSsoNoLocalAccount
			}
			return nil, err
		}
		uid = lu.ID
	}
	if _, err := s.users.FindByID(ctx, uid); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrSsoNoLocalAccount
		}
		return nil, err
	}

	// 绑定一致性：sub 已绑他人 → 冲突；未绑 → 幂等补绑定（保证 sso_bindings 与登录路径一致）
	if b, err := s.bindings.FindBySub(ctx, u.Sub); err == nil {
		if b.LocalUserID != uid {
			return nil, ErrSsoConflict
		}
	} else if errors.Is(err, domain.ErrNotFound) {
		if err := s.bindings.Create(ctx, &domain.SsoBinding{
			LocalUserID: uid,
			SsoSub:      u.Sub,
			SsoUsername: u.PreferredUsername,
			CreatedAt:   time.Now(),
		}); err != nil && !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return s.tokens.IssueTokensForUser(ctx, uid)
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

	u, _, err := s.users.FindAuthByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound // 422：账号不存在
		}
		return nil, err
	}
	if err := s.verifyAccountPassword(ctx, email, password, p.Sub, u.ID); err != nil {
		return nil, err
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

func (s *ssoService) Unbind(ctx context.Context, userID int, email, password string) error {
	if userID <= 0 || len(password) < 6 {
		return domain.ErrBadRequest
	}
	if strings.TrimSpace(email) == "" {
		u, err := s.users.FindByID(ctx, userID)
		if err != nil {
			return err
		}
		email = u.Email
	}
	b, err := s.bindings.FindByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.verifyAccountPassword(ctx, email, password, b.SsoSub, userID); err != nil {
		return err
	}
	return s.bindings.DeleteByUserID(ctx, userID)
}

func (s *ssoService) verifyAccountPassword(ctx context.Context, email, password, expectedSub string, userID int) error {
	if !config.SsoEnabled() || config.SsoPasswordVerifyURL() == "" {
		return ErrSsoUnavailable
	}
	u, _, err := s.verifyPassword(ctx, strings.ToLower(strings.TrimSpace(email)), password)
	if errors.Is(err, ErrSsoBadCredentials) || errors.Is(err, ErrSsoVerifyForbidden) {
		return domain.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if !bool(u.Internal) || u.Sub != expectedSub || (u.SseSub != "" && u.SseSub != strconv.Itoa(userID)) {
		return domain.ErrUnauthorized
	}
	return nil
}
