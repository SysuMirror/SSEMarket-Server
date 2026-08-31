package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"loginTest/domain"
	"loginTest/service"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
)

// ---------- fakes ----------

type memKV struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemKV() *memKV { return &memKV{m: map[string]string{}} }

func (k *memKV) Set(_ context.Context, key, val string, _ time.Duration) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[key] = val
	return nil
}

func (k *memKV) Get(_ context.Context, key string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (k *memKV) GetDel(_ context.Context, key string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[key]
	delete(k.m, key)
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

type fakeBindings struct {
	mu       sync.Mutex
	bySub    map[string]*domain.SsoBinding
	byUser   map[int]*domain.SsoBinding
	nextID   int
	createFn func(*domain.SsoBinding) error // 可注入冲突；nil 走默认
}

func newFakeBindings() *fakeBindings {
	return &fakeBindings{bySub: map[string]*domain.SsoBinding{}, byUser: map[int]*domain.SsoBinding{}}
}

func (f *fakeBindings) FindBySub(_ context.Context, sub string) (*domain.SsoBinding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.bySub[sub]; ok {
		return b, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeBindings) FindByUserID(_ context.Context, uid int) (*domain.SsoBinding, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.byUser[uid]; ok {
		return b, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeBindings) Create(_ context.Context, b *domain.SsoBinding) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createFn != nil {
		err := f.createFn(b)
		if err != nil {
			return err
		}
	}
	if _, ok := f.bySub[b.SsoSub]; ok {
		return domain.ErrConflict
	}
	if _, ok := f.byUser[b.LocalUserID]; ok {
		return domain.ErrConflict
	}
	f.nextID++
	b.ID = f.nextID
	c := *b
	f.bySub[b.SsoSub] = &c
	f.byUser[b.LocalUserID] = &c
	return nil
}

func (f *fakeBindings) DeleteByUserID(_ context.Context, uid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.byUser[uid]
	if !ok {
		return domain.ErrNotFound
	}
	delete(f.byUser, uid)
	delete(f.bySub, b.SsoSub)
	return nil
}

type fakeUsers struct {
	users map[string]struct {
		u    *domain.User
		hash string
	}
	byID map[int]*domain.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		users: map[string]struct {
			u    *domain.User
			hash string
		}{},
		byID: map[int]*domain.User{},
	}
}

func (f *fakeUsers) seed(u *domain.User, password string) {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	f.users[strings.ToLower(u.Email)] = struct {
		u    *domain.User
		hash string
	}{u, string(hash)}
	f.byID[u.ID] = u
}

func (f *fakeUsers) FindByID(_ context.Context, id int) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) FindAuthByEmail(_ context.Context, email string) (*domain.User, string, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	if v, ok := f.users[e]; ok {
		return v.u, v.hash, nil
	}
	return nil, "", domain.ErrNotFound
}

type fakeTokens struct{ users map[int]struct{} }

func (f fakeTokens) IssueTokensForUser(_ context.Context, id int) (*domain.AuthTokens, error) {
	if _, ok := f.users[id]; !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.AuthTokens{
		AccessToken:  "at-" + strconv.Itoa(id),
		RefreshToken: "rt-" + strconv.Itoa(id),
	}, nil
}

// ---------- helpers ----------

func ssoSetup(t *testing.T) {
	t.Helper()
	viper.Set("sso.enabled", true)
	viper.Set("sso.client_id", "client_test")
	viper.Set("sso.client_secret", "secret_test")
	viper.Set("sso.redirect_uri", "https://market.test/cb")
	viper.Set("sso.authorize_url", "https://sso.test/authorize")
}

func newSsoServer(t *testing.T, expectedVerifierRef *string, sub string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		v := r.Form.Get("code_verifier")
		if expectedVerifierRef != nil && *expectedVerifierRef != "" && v != *expectedVerifierRef {
			http.Error(w, `{"error":"invalid_verifier"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-at","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-at" {
			http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"` + sub + `","preferred_username":"alice","email":"a@x.cn"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	viper.Set("sso.token_url", srv.URL+"/token")
	viper.Set("sso.userinfo_url", srv.URL+"/userinfo")
	return srv
}

// fakeSsoServerWithFail 注入 token 端点 400（用于 ConsumeCallback_ExchangeFailure）。
func newFailingTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"s1","preferred_username":"a","email":"a@x.cn"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	viper.Set("sso.token_url", srv.URL+"/token")
	viper.Set("sso.userinfo_url", srv.URL+"/userinfo")
	return srv
}

// extractVerifier 从 KV 里拉出 state payload 的 verifier（验证 PKCE 串联）。
func stateVerifier(t *testing.T, kv *memKV, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatal("authorize url missing state")
	}
	raw, err := kv.Get(context.Background(), "sso:state:"+state)
	if err != nil {
		t.Fatalf("kv Get state: %v", err)
	}
	var p domain.SsoStatePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return p.Verifier
}

// ---------- tests ----------

// TestSsoPKCEChallenge_RFC7636Vector RFC 7636 附录 B 向量。
func TestSsoPKCEChallenge_RFC7636Vector(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	got := service.SsoPKCEChallenge(verifier)
	if got != want {
		t.Fatalf("challenge = %q, want %q", got, want)
	}
}

func TestSsoBuildAuthorizeURL_Disabled(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", false)
	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("err = %v, want ErrBadRequest", err)
	}
}

func TestSsoBuildAuthorizeURL_Params(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	u, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL: %v", err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Host != "sso.test" {
		t.Fatalf("host = %s, want sso.test", parsed.Host)
	}
	q := parsed.Query()
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q", q.Get("response_type"))
	}
	if q.Get("client_id") != "client_test" {
		t.Errorf("client_id = %q", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://market.test/cb" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("scope") != "openid profile email" {
		t.Errorf("scope = %q", q.Get("scope"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	state := q.Get("state")
	if len(state) != 32 {
		t.Errorf("state len = %d, want 32 (hex of 16 bytes)", len(state))
	}
	challenge := q.Get("code_challenge")
	if challenge == "" {
		t.Fatal("code_challenge empty")
	}
	// 验证 challenge == S256(verifier)，且 verifier 存于 Redis（串联正确）
	verifier := stateVerifier(t, kv, u)
	if service.SsoPKCEChallenge(verifier) != challenge {
		t.Error("code_challenge != S256(state.verifier): PKCE 串联错误")
	}
}

func TestSsoConsumeCallback_LoginPath(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var verifierRef string
	newSsoServer(t, &verifierRef, "sub-1")

	bindings := newFakeBindings()
	bindings.bySub["sub-1"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-1", SsoUsername: "alice"}
	bindings.byUser[7] = bindings.bySub["sub-1"]

	users := newFakeUsers()
	users.seed(&domain.User{ID: 7, Email: "u7@x.cn"}, "secret123")
	tokens := fakeTokens{users: map[int]struct{}{7: {}}}

	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL: %v", err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	verifierRef = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code123", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	res, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ExchangeTicket: %v", err)
	}
	if res.Type != "login" {
		t.Errorf("type = %q, want login", res.Type)
	}
	if res.Token != "at-7" || res.RefreshToken != "rt-7" {
		t.Errorf("token=%s refresh=%s", res.Token, res.RefreshToken)
	}
}

func TestSsoConsumeCallback_BindNeededPath(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-2")

	bindings := newFakeBindings()
	users := newFakeUsers()
	tokens := fakeTokens{}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	res1, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("first ExchangeTicket: %v", err)
	}
	if res1.Type != "bind_needed" {
		t.Fatalf("type = %q, want bind_needed", res1.Type)
	}
	if res1.SsoUsername != "alice" || res1.SsoEmail != "a@x.cn" {
		t.Errorf("username=%s email=%s", res1.SsoUsername, res1.SsoEmail)
	}
	// bind_needed 可重复读
	res2, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("second ExchangeTicket (bind_needed should be re-readable): %v", err)
	}
	if res2.Type != "bind_needed" {
		t.Errorf("second exchange type = %q", res2.Type)
	}
}

func TestSsoConsumeCallback_BindUIDPath_Bound(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-3")

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 5, Email: "u5@x.cn"}, "pw123456")
	tokens := fakeTokens{users: map[int]struct{}{5: {}}}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 5) // 已登录 uid=5
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	if len(bindings.bySub) != 1 || bindings.bySub["sub-3"] == nil {
		t.Fatalf("binding not created: %+v", bindings.bySub)
	}
	if bindings.bySub["sub-3"].LocalUserID != 5 {
		t.Errorf("binding LocalUserID = %d, want 5", bindings.bySub["sub-3"].LocalUserID)
	}
	res, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ExchangeTicket: %v", err)
	}
	if res.Type != "bound" {
		t.Errorf("type = %q, want bound", res.Type)
	}
	if res.Token != "at-5" {
		t.Errorf("token = %s", res.Token)
	}
}

func TestSsoConsumeCallback_StateReplay(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-1")

	bindings := newFakeBindings()
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	_, _ = svc.ConsumeCallback(context.Background(), "code", state) // 第一次消费
	_, err = svc.ConsumeCallback(context.Background(), "code", state)
	if !errors.Is(err, service.ErrSsoStateInvalid) {
		t.Fatalf("second call err = %v, want ErrSsoStateInvalid", err)
	}
}

func TestSsoConsumeCallback_BindUIDConflict(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-1")

	// sub-1 已绑 uid=7；尝试用 bind_uid=9 绑定 → 冲突
	bindings := newFakeBindings()
	bindings.bySub["sub-1"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-1"}
	bindings.byUser[7] = bindings.bySub["sub-1"]
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 9)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	_, err = svc.ConsumeCallback(context.Background(), "code", state)
	if !errors.Is(err, service.ErrSsoConflict) {
		t.Fatalf("err = %v, want ErrSsoConflict", err)
	}

	// 同一 uid 幂等：sub-1 已绑 uid=7，bind_uid=7 → login
	authorizeURL2, err := svc.BuildAuthorizeURL(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	parsed2, _ := url.Parse(authorizeURL2)
	state2 := parsed2.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL2)
	_, err = svc.ConsumeCallback(context.Background(), "code", state2)
	if err != nil {
		t.Fatalf("idempotent bind_uid match: err = %v", err)
	}
}

func TestSsoConsumeCallback_BindUIDSelfConflict(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-9")

	// uid=5 已绑 sub-other；用 bind_uid=5 绑 sub-9 → 本账号已绑 → 冲突
	bindings := newFakeBindings()
	bindings.byUser[5] = &domain.SsoBinding{ID: 1, LocalUserID: 5, SsoSub: "sub-other"}
	bindings.bySub["sub-other"] = bindings.byUser[5]
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)
	_, err = svc.ConsumeCallback(context.Background(), "code", state)
	if !errors.Is(err, service.ErrSsoConflict) {
		t.Fatalf("err = %v, want ErrSsoConflict", err)
	}
}

func TestSsoConsumeCallback_ExchangeFailure(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	newFailingTokenServer(t)

	kv := newMemKV()
	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	_, err = svc.ConsumeCallback(context.Background(), "code", state)
	if !errors.Is(err, service.ErrSsoExchange) {
		t.Fatalf("err = %v, want ErrSsoExchange", err)
	}
}

func TestSsoExchangeTicket_LoginOneTime(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-1")

	bindings := newFakeBindings()
	bindings.bySub["sub-1"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-1"}
	bindings.byUser[7] = bindings.bySub["sub-1"]
	users := newFakeUsers()
	users.seed(&domain.User{ID: 7, Email: "u7@x.cn"}, "x")
	tokens := fakeTokens{users: map[int]struct{}{7: {}}}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	if _, err := svc.ExchangeTicket(context.Background(), ticket); err != nil {
		t.Fatalf("first ExchangeTicket: %v", err)
	}
	// 第二次：login ticket 已被 GETDEL，应失败
	_, err = svc.ExchangeTicket(context.Background(), ticket)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("second ExchangeTicket err = %v, want ErrUnauthorized", err)
	}
}

func TestSsoBindWithCredentials_HappyPath(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-7")

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 5, Email: "U5@x.cn"}, "secret123")
	tokens := fakeTokens{users: map[int]struct{}{5: {}}}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	res, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil || res.Type != "bind_needed" {
		t.Fatalf("exchange: err=%v res=%+v", err, res)
	}

	tokens2, err := svc.BindWithCredentials(context.Background(), ticket, "U5@x.cn", "secret123")
	if err != nil {
		t.Fatalf("BindWithCredentials: %v", err)
	}
	if tokens2.AccessToken != "at-5" {
		t.Errorf("token = %s", tokens2.AccessToken)
	}
	if len(bindings.bySub) != 1 || bindings.bySub["sub-7"] == nil {
		t.Fatalf("binding not created: %+v", bindings.bySub)
	}
	if bindings.bySub["sub-7"].LocalUserID != 5 {
		t.Errorf("binding LocalUserID = %d", bindings.bySub["sub-7"].LocalUserID)
	}
}

func TestSsoBindWithCredentials_WrongPassword(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-7")

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 5, Email: "u5@x.cn"}, "secret123")
	tokens := fakeTokens{users: map[int]struct{}{5: {}}}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}

	_, err = svc.BindWithCredentials(context.Background(), ticket, "u5@x.cn", "wrong-pwd")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	// ticket 已被消费
	_, err = svc.BindWithCredentials(context.Background(), ticket, "u5@x.cn", "secret123")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("second bind err = %v, want ErrUnauthorized (ticket consumed)", err)
	}
	// 绑定未写入
	if len(bindings.bySub) != 0 {
		t.Errorf("bindings should be empty, got %+v", bindings.bySub)
	}
}

func TestSsoBindWithCredentials_UnknownEmail(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-7")

	bindings := newFakeBindings()
	users := newFakeUsers()
	tokens := fakeTokens{}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	_, err = svc.BindWithCredentials(context.Background(), ticket, "nobody@x.cn", "secret123")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSsoBindWithCredentials_UserAlreadyBound(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	var vref string
	newSsoServer(t, &vref, "sub-7")

	// uid=5 已绑 sub-other
	bindings := newFakeBindings()
	bindings.byUser[5] = &domain.SsoBinding{ID: 1, LocalUserID: 5, SsoSub: "sub-other"}
	bindings.bySub["sub-other"] = bindings.byUser[5]
	users := newFakeUsers()
	users.seed(&domain.User{ID: 5, Email: "u5@x.cn"}, "secret123")
	tokens := fakeTokens{users: map[int]struct{}{5: {}}}
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, users, tokens, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	state := parsed.Query().Get("state")
	vref = stateVerifier(t, kv, authorizeURL)

	ticket, err := svc.ConsumeCallback(context.Background(), "code", state)
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	_, err = svc.BindWithCredentials(context.Background(), ticket, "u5@x.cn", "secret123")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestSsoUnbind_PasswordOK(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)

	bindings := newFakeBindings()
	bindings.bySub["sub-1"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-1"}
	bindings.byUser[7] = bindings.bySub["sub-1"]
	users := newFakeUsers()
	users.seed(&domain.User{ID: 7, Email: "u7@x.cn"}, "secret123")
	tokens := fakeTokens{}
	svc := service.NewSsoServiceForTest(bindings, users, tokens, newMemKV(), &http.Client{})

	err := svc.Unbind(context.Background(), 7, "secret123")
	if err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if len(bindings.bySub) != 0 {
		t.Errorf("binding should be removed, got %+v", bindings.bySub)
	}
}

func TestSsoUnbind_PasswordWrong(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)

	bindings := newFakeBindings()
	bindings.bySub["sub-1"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-1"}
	bindings.byUser[7] = bindings.bySub["sub-1"]
	users := newFakeUsers()
	users.seed(&domain.User{ID: 7, Email: "u7@x.cn"}, "secret123")
	svc := service.NewSsoServiceForTest(bindings, users, fakeTokens{}, newMemKV(), &http.Client{})

	err := svc.Unbind(context.Background(), 7, "wrong-pwd")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	// 绑定应仍在
	if len(bindings.bySub) != 1 {
		t.Errorf("binding should remain, got %+v", bindings.bySub)
	}
}

func TestSsoGetStatus(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)

	bindings := newFakeBindings()
	bindings.byUser[7] = &domain.SsoBinding{
		ID: 1, LocalUserID: 7, SsoSub: "sub-1", SsoUsername: "alice",
		CreatedAt: time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC),
	}
	bindings.bySub["sub-1"] = bindings.byUser[7]
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})

	st, err := svc.GetStatus(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !st.Bound || st.SsoUsername != "alice" {
		t.Errorf("status = %+v", st)
	}

	// 未绑定
	st2, err := svc.GetStatus(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetStatus unbound: %v", err)
	}
	if st2.Bound {
		t.Errorf("status should be unbound, got %+v", st2)
	}
}

// 防 unused 警告（io 在测试中未直接使用，保留以备扩展）。
var _ = io.Discard
