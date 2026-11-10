package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"loginTest/config"
)

// UniSSO 可用性探测只反映服务状态，不改变密码校验来源。
const (
	SsoModeSso         = "sso"
	SsoModeUnavailable = "unavailable"
	SsoModeDisabled    = "disabled"

	ssoProbeTimeout  = 2 * time.Second
	ssoProbeInterval = 30 * time.Second
	ssoFailThreshold = 3
	ssoAuditLogPath  = "/app/log/sso-audit.log"
)

// SsoModeInfo 下发给前端的登录模式快照。
type SsoModeInfo struct {
	Mode                string    `json:"mode"`
	PasswordLogin       bool      `json:"password_login"`
	UnissoHealthy       bool      `json:"unisso_healthy"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CheckedAt           time.Time `json:"checked_at"`
	LastError           string    `json:"last_error,omitempty"`
}

type ssoHealth struct {
	mu        sync.RWMutex
	mode      string
	failures  int
	lastProbe time.Time
	lastErr   string
	probing   bool
	startOnce sync.Once
}

var ssoHealthState = &ssoHealth{mode: SsoModeSso}

// CurrentSsoMode 返回当前登录模式（sso / unavailable / disabled）。
func CurrentSsoMode() string {
	if !config.SsoEnabled() {
		return SsoModeDisabled
	}
	ssoHealthState.mu.RLock()
	defer ssoHealthState.mu.RUnlock()
	return ssoHealthState.mode
}

// CurrentSsoModeInfo 返回状态快照；status 接口与登录门禁共用。
func CurrentSsoModeInfo() SsoModeInfo {
	info := SsoModeInfo{Mode: SsoModeDisabled}
	if !config.SsoEnabled() {
		info.CheckedAt = time.Now()
		return info
	}
	ssoHealthState.mu.RLock()
	info.Mode = ssoHealthState.mode
	info.ConsecutiveFailures = ssoHealthState.failures
	info.CheckedAt = ssoHealthState.lastProbe
	info.LastError = ssoHealthState.lastErr
	ssoHealthState.mu.RUnlock()
	info.UnissoHealthy = info.Mode == SsoModeSso
	return info
}

// EnsureSsoModeFresh 超过缓存周期（30s）时触发一次异步探测，避免状态长期陈旧。
func EnsureSsoModeFresh() {
	if !config.SsoEnabled() {
		return
	}
	ssoHealthState.mu.RLock()
	stale := time.Since(ssoHealthState.lastProbe) >= ssoProbeInterval
	ssoHealthState.mu.RUnlock()
	if stale {
		go ssoHealthState.probe()
	}
}

// StartSsoHealthMonitor 启动后台探测（幂等）。
func StartSsoHealthMonitor() {
	ssoHealthState.startOnce.Do(func() {
		if !config.SsoEnabled() {
			SsoAudit("sso_disabled", "reason", "sso.enabled=false")
			return
		}
		if config.SsoClientID() == "" || config.SsoClientSecret() == "" || config.SsoDiscoveryURL() == "" {
			ssoHealthState.mu.Lock()
			ssoHealthState.mode = SsoModeUnavailable
			ssoHealthState.lastErr = "sso client/discovery not configured"
			ssoHealthState.lastProbe = time.Now()
			ssoHealthState.mu.Unlock()
			SsoAudit("mode_change", "from", SsoModeSso, "to", SsoModeUnavailable, "reason", "client_not_configured")
			return
		}
		go func() {
			ssoHealthState.probe()
			ticker := time.NewTicker(ssoProbeInterval)
			defer ticker.Stop()
			for range ticker.C {
				ssoHealthState.probe()
			}
		}()
	})
}

func (h *ssoHealth) probe() {
	h.mu.Lock()
	if h.probing {
		h.mu.Unlock()
		return
	}
	h.probing = true
	h.mu.Unlock()

	err := probeSsoDiscovery()

	h.mu.Lock()
	h.lastProbe = time.Now()
	was := h.mode
	var transition string
	if err != nil {
		h.failures++
		h.lastErr = err.Error()
		if h.failures >= ssoFailThreshold && h.mode != SsoModeUnavailable {
			h.mode = SsoModeUnavailable
			transition = SsoModeUnavailable
		}
	} else {
		h.failures = 0
		h.lastErr = ""
		if h.mode != SsoModeSso {
			h.mode = SsoModeSso
			transition = SsoModeSso
		}
	}
	failures := h.failures
	mode := h.mode
	h.probing = false
	h.mu.Unlock()

	if err != nil {
		SsoAudit("probe_failed", "failures", strconv.Itoa(failures), "error", err.Error())
	}
	if transition != "" {
		reason := "discovery_failed"
		if transition == SsoModeSso {
			reason = "discovery_recovered"
		}
		SsoAudit("mode_change", "from", was, "to", mode, "reason", reason, "failures", strconv.Itoa(failures))
	}
}

func probeSsoDiscovery() error {
	if config.SsoClientID() == "" || config.SsoClientSecret() == "" {
		return errors.New("sso client not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ssoProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.SsoDiscoveryURL(), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: ssoProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var doc struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return errors.New("discovery document invalid json")
	}
	if doc.Issuer == "" || doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" {
		return errors.New("discovery document missing endpoints")
	}
	return nil
}

// SsoAudit 兜底/模式切换审计：只记录时间、模式、原因与结果，绝不记录密码或 token。
func SsoAudit(event string, kv ...string) {
	parts := []string{"ts=" + time.Now().Format(time.RFC3339), "event=" + event}
	for i := 0; i+1 < len(kv); i += 2 {
		val := strings.NewReplacer("\n", " ", "\r", " ").Replace(kv[i+1])
		parts = append(parts, kv[i]+"="+val)
	}
	line := strings.Join(parts, " ")
	log.Print("[sso-audit] " + line)
	f, err := os.OpenFile(ssoAuditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\n")
	_ = f.Close()
}
