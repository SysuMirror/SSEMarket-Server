package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"loginTest/domain"
	"loginTest/service"

	"github.com/spf13/viper"
)

// fakeUnisso 自定义 userinfo 响应的假 UniSSO（token 端点直接发 fake-at）。
func fakeUnisso(t *testing.T, userinfoBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fake-at","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userinfoBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	viper.Set("sso.token_url", srv.URL+"/token")
	viper.Set("sso.userinfo_url", srv.URL+"/userinfo")
	return srv
}

// stateOf 提取 authorize URL 的 state。
func stateOf(t *testing.T, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("parse authorize url: %v", err)
	}
	return u.Query().Get("state")
}

// TestSsoConsumeCallback_NonInternalRejected 非院内账号（internal=false）必须被拒。
func TestSsoConsumeCallback_NonInternalRejected(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnisso(t, `{"sub":"sub-x","preferred_username":"outsider","email":"o@x.cn","internal":false}`)

	bindings := newFakeBindings()
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ConsumeCallback(context.Background(), "code", stateOf(t, authorizeURL))
	if !errors.Is(err, service.ErrSsoForbidden) {
		t.Fatalf("err = %v, want ErrSsoForbidden", err)
	}
	if len(bindings.bySub) != 0 {
		t.Errorf("binding must not be created for non-internal user")
	}
}

// TestSsoConsumeCallback_InternalMissingRejected internal claim 缺失视为非院内。
func TestSsoConsumeCallback_InternalMissingRejected(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnisso(t, `{"sub":"sub-y","preferred_username":"alice","email":"a@x.cn"}`)

	bindings := newFakeBindings()
	kv := newMemKV()
	svc := service.NewSsoServiceForTest(bindings, newFakeUsers(), fakeTokens{}, kv, &http.Client{})

	authorizeURL, err := svc.BuildAuthorizeURL(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ConsumeCallback(context.Background(), "code", stateOf(t, authorizeURL))
	if !errors.Is(err, service.ErrSsoForbidden) {
		t.Fatalf("err = %v, want ErrSsoForbidden", err)
	}
}

// TestSsoConsumeCallback_InternalTrueAccepted internal=true 且已绑定的账号可登录。
func TestSsoConsumeCallback_InternalTrueAccepted(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnisso(t, `{"sub":"sub-1","preferred_username":"alice","email":"a@x.cn","internal":true}`)

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
		t.Fatal(err)
	}

	ticket, err := svc.ConsumeCallback(context.Background(), "code", stateOf(t, authorizeURL))
	if err != nil {
		t.Fatalf("ConsumeCallback: %v", err)
	}
	res, err := svc.ExchangeTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("ExchangeTicket: %v", err)
	}
	if res.Type != "login" || res.Token != "at-7" {
		t.Errorf("type=%s token=%s, want login/at-7", res.Type, res.Token)
	}
}

// ---------- LoginViaUnisso（M2M 密码校验主路径） ----------

func fakeUnissoPasswordVerify(t *testing.T, status int, body string, seen *map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/password/verify", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if seen != nil {
			(*seen)["client_id"] = r.Form.Get("client_id")
			(*seen)["email"] = r.Form.Get("email")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	viper.Set("sso.password_verify_url", srv.URL+"/password/verify")
	return srv
}

func TestLoginViaUnisso_SuccessWithSseSub(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	seen := map[string]string{}
	fakeUnissoPasswordVerify(t, 200, `{"ok":true,"user":{"sub":"sub-9","sse_sub":"1314","internal":true,"email":"m@x.cn","preferred_username":"music"}}`, &seen)

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 1314, Email: "m@x.cn"}, "pw")
	tokens := fakeTokens{users: map[int]struct{}{1314: {}}}
	svc := service.NewSsoServiceForTest(bindings, users, tokens, newMemKV(), &http.Client{})

	res, _, err := svc.LoginViaUnisso(context.Background(), "M@X.cn", "secret123")
	if err != nil {
		t.Fatalf("LoginViaUnisso: %v", err)
	}
	if res.AccessToken != "at-1314" {
		t.Errorf("token = %s, want at-1314", res.AccessToken)
	}
	if seen["client_id"] != "client_test" {
		t.Errorf("client_id = %q", seen["client_id"])
	}
	b := bindings.bySub["sub-9"]
	if b == nil || b.LocalUserID != 1314 {
		t.Errorf("binding = %+v, want sub-9→1314", bindings.bySub["sub-9"])
	}
}

func TestLoginViaUnisso_NonInternalRejected(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 200, `{"ok":true,"user":{"sub":"sub-o","internal":false,"email":"o@x.cn"}}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "o@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoForbidden) {
		t.Fatalf("err = %v, want ErrSsoForbidden", err)
	}
}

func TestLoginViaUnisso_EmailFallbackWhenNoSseSub(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 200, `{"ok":true,"user":{"sub":"sub-e","internal":true,"email":"e@x.cn"}}`, nil)

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 42, Email: "e@x.cn"}, "pw")
	tokens := fakeTokens{users: map[int]struct{}{42: {}}}
	svc := service.NewSsoServiceForTest(bindings, users, tokens, newMemKV(), &http.Client{})

	res, _, err := svc.LoginViaUnisso(context.Background(), "e@x.cn", "secret123")
	if err != nil {
		t.Fatalf("LoginViaUnisso: %v", err)
	}
	if res.AccessToken != "at-42" {
		t.Errorf("token = %s, want at-42", res.AccessToken)
	}
}

func TestLoginViaUnisso_NoLocalAccount(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 200, `{"ok":true,"user":{"sub":"sub-z","internal":true,"email":"z@x.cn"}}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "z@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoNoLocalAccount) {
		t.Fatalf("err = %v, want ErrSsoNoLocalAccount", err)
	}
}

func TestLoginViaUnisso_BadCredentials(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 401, `{"error":"invalid_credentials","error_description":"邮箱或密码错误"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "m@x.cn", "wrong-pass")
	if !errors.Is(err, service.ErrSsoBadCredentials) {
		t.Fatalf("err = %v, want ErrSsoBadCredentials", err)
	}
}

func TestLoginViaUnisso_Unavailable(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 503, `{"error":"server_error"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "m@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoUnavailable) {
		t.Fatalf("err = %v, want ErrSsoUnavailable", err)
	}
}

func TestLoginViaUnisso_BindingConflict(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 200, `{"ok":true,"user":{"sub":"sub-c","sse_sub":"100","internal":true,"email":"c@x.cn"}}`, nil)

	bindings := newFakeBindings()
	bindings.bySub["sub-c"] = &domain.SsoBinding{ID: 1, LocalUserID: 7, SsoSub: "sub-c"}
	users := newFakeUsers()
	users.seed(&domain.User{ID: 100, Email: "c@x.cn"}, "pw")
	svc := service.NewSsoServiceForTest(bindings, users, fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "c@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoConflict) {
		t.Fatalf("err = %v, want ErrSsoConflict", err)
	}
}

func TestLoginViaUnisso_VerifyForbidden(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 403, `{"error":"password_verify_forbidden"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "m@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoVerifyForbidden) {
		t.Fatalf("err = %v, want ErrSsoVerifyForbidden", err)
	}
}

func TestLoginViaUnisso_TooManyAttempts(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoPasswordVerify(t, 429, `{"error":"too_many_attempts"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, _, err := svc.LoginViaUnisso(context.Background(), "m@x.cn", "secret123")
	if !errors.Is(err, service.ErrSsoTooManyAttempts) {
		t.Fatalf("err = %v, want ErrSsoTooManyAttempts", err)
	}
}

// ---------- LoginViaSession（共享 Cookie 静默 SSO） ----------

func fakeUnissoSessionVerify(t *testing.T, status int, body string, seen *map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/session/verify", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if seen != nil {
			(*seen)["session"] = r.Form.Get("session")
			(*seen)["client_id"] = r.Form.Get("client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	viper.Set("sso.session_verify_url", srv.URL+"/session/verify")
	return srv
}

func TestLoginViaSession_Success(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	seen := map[string]string{}
	fakeUnissoSessionVerify(t, 200, `{"ok":true,"user":{"sub":"sub-s","sse_sub":"77","internal":true,"email":"s@x.cn"}}`, &seen)

	bindings := newFakeBindings()
	users := newFakeUsers()
	users.seed(&domain.User{ID: 77, Email: "s@x.cn"}, "pw")
	svc := service.NewSsoServiceForTest(bindings, users, fakeTokens{users: map[int]struct{}{77: {}}}, newMemKV(), &http.Client{})

	res, err := svc.LoginViaSession(context.Background(), "sess_abc")
	if err != nil {
		t.Fatalf("LoginViaSession: %v", err)
	}
	if res.AccessToken != "at-77" {
		t.Errorf("token = %s, want at-77", res.AccessToken)
	}
	if seen["session"] != "sess_abc" || seen["client_id"] != "client_test" {
		t.Errorf("form = %+v, want session=sess_abc client=client_test", seen)
	}
	if b := bindings.bySub["sub-s"]; b == nil || b.LocalUserID != 77 {
		t.Errorf("binding = %+v, want sub-s→77", bindings.bySub["sub-s"])
	}
}

func TestLoginViaSession_InvalidSession(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoSessionVerify(t, 401, `{"error":"invalid_session"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, err := svc.LoginViaSession(context.Background(), "sess_gone")
	if !errors.Is(err, service.ErrSsoInvalidSession) {
		t.Fatalf("err = %v, want ErrSsoInvalidSession", err)
	}
}

func TestLoginViaSession_Unavailable(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoSessionVerify(t, 500, `{"error":"server_error"}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, err := svc.LoginViaSession(context.Background(), "sess_abc")
	if !errors.Is(err, service.ErrSsoUnavailable) {
		t.Fatalf("err = %v, want ErrSsoUnavailable", err)
	}
}

func TestLoginViaSession_NonInternalRejected(t *testing.T) {
	t.Cleanup(viper.Reset)
	ssoSetup(t)
	fakeUnissoSessionVerify(t, 200, `{"ok":true,"user":{"sub":"sub-out","internal":false,"email":"o@x.cn"}}`, nil)

	svc := service.NewSsoServiceForTest(newFakeBindings(), newFakeUsers(), fakeTokens{}, newMemKV(), &http.Client{})
	_, err := svc.LoginViaSession(context.Background(), "sess_abc")
	if !errors.Is(err, service.ErrSsoForbidden) {
		t.Fatalf("err = %v, want ErrSsoForbidden", err)
	}
}
