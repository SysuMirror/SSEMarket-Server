package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/spf13/viper"
)

func healthSetup(t *testing.T, discoveryURL, clientID string) {
	t.Helper()
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", true)
	viper.Set("sso.client_id", clientID)
	viper.Set("sso.client_secret", "sec")
	viper.Set("sso.discovery_url", discoveryURL)
}

func discoveryHandler(healthy *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		if healthy != nil && !healthy.Load() {
			http.Error(w, `{"error":"down"}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"https://sso.test","authorization_endpoint":"https://sso.test/authorize","token_endpoint":"https://sso.test/api/oauth/token"}`))
	})
	return mux
}

func TestSsoProbe_UnavailableAfterThreeFailuresThenRecover(t *testing.T) {
	var healthy atomic.Bool
	healthy.Store(false)
	srv := httptest.NewServer(discoveryHandler(&healthy))
	defer srv.Close()
	healthSetup(t, srv.URL+"/.well-known/openid-configuration", "cid")

	h := &ssoHealth{mode: SsoModeSso}
	h.probe()
	h.probe()
	if h.mode != SsoModeSso {
		t.Fatalf("mode = %s after 2 failures, want sso", h.mode)
	}
	if h.failures != 2 {
		t.Fatalf("failures = %d, want 2", h.failures)
	}
	h.probe()
	if h.mode != SsoModeUnavailable {
		t.Fatalf("mode = %s after 3 failures, want unavailable", h.mode)
	}
	healthy.Store(true)
	h.probe()
	if h.mode != SsoModeSso {
		t.Fatalf("mode = %s after recovery, want sso", h.mode)
	}
	if h.failures != 0 || h.lastErr != "" {
		t.Errorf("failures = %d, lastErr = %q", h.failures, h.lastErr)
	}
}

func TestSsoProbe_RequiresClientConfig(t *testing.T) {
	srv := httptest.NewServer(discoveryHandler(nil))
	defer srv.Close()
	healthSetup(t, srv.URL+"/.well-known/openid-configuration", "")
	if err := probeSsoDiscovery(); err == nil {
		t.Fatal("probe must fail when sso client not configured")
	}
}

func TestCurrentSsoMode_Disabled(t *testing.T) {
	t.Cleanup(viper.Reset)
	viper.Set("sso.enabled", false)
	if CurrentSsoMode() != SsoModeDisabled {
		t.Fatalf("mode = %s, want disabled", CurrentSsoMode())
	}
	if CurrentSsoModeInfo().PasswordLogin {
		t.Fatal("local password login must never be allowed")
	}
}
