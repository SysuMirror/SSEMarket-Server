package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"loginTest/config"
	"loginTest/response"

	"github.com/gin-gonic/gin"
)

type resetInput struct {
	Email       string `json:"email" binding:"required,email,max=254"`
	Code        string `json:"code,omitempty"`
	NewPassword string `json:"new_password,omitempty"`
}

func StartUnissoPasswordReset(c *gin.Context)   { unissoPasswordReset(c, false) }
func ConfirmUnissoPasswordReset(c *gin.Context) { unissoPasswordReset(c, true) }

func unissoPasswordReset(c *gin.Context, confirm bool) {
	c.Header("Cache-Control", "no-store")
	var input resetInput
	contentType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if contentType != "application/json" || c.ShouldBindJSON(&input) != nil ||
		(confirm && (len(input.Code) != 6 || strings.Trim(input.Code, "0123456789") != "" || input.NewPassword == "")) {
		response.Response(c, 400, 400, nil, "请检查邮箱、验证码和新密码")
		return
	}
	status, result := callUnissoPasswordReset(c.Request.Context(), input, confirm, nil)
	if status == http.StatusOK {
		response.Success(c, gin.H{"cooldown": result.Cooldown}, result.Message)
		return
	}
	response.Response(c, status, status, nil, result.Message)
}

type resetResult struct {
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Cooldown int    `json:"cooldown"`
}

func callUnissoPasswordReset(ctx context.Context, input resetInput, confirm bool, transport http.RoundTripper) (int, resetResult) {
	unavailable := resetResult{Message: "统一身份服务暂时不可用，请稍后再试"}
	base, err := url.Parse(config.SsoPasswordVerifyURL())
	if err != nil || !config.SsoEnabled() || base.Scheme != "https" || base.Host == "" {
		return http.StatusServiceUnavailable, unavailable
	}
	origin := &url.URL{Scheme: base.Scheme, Host: base.Host}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:           jar,
		Transport:     transport,
		Timeout:       6 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	read := func(req *http.Request, result any) (int, error) {
		res, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer res.Body.Close()
		return res.StatusCode, json.NewDecoder(io.LimitReader(res.Body, 16<<10)).Decode(result)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin.JoinPath("/api/csrf-token").String(), nil)
	if err != nil {
		return http.StatusServiceUnavailable, unavailable
	}
	var csrf struct {
		Token string `json:"csrf_token"`
	}
	if status, err := read(req, &csrf); err != nil || status != http.StatusOK || csrf.Token == "" {
		return http.StatusServiceUnavailable, unavailable
	}

	path := "/api/auth/password-reset/start"
	if confirm {
		path = "/api/auth/password-reset/confirm"
	} else {
		input.Code, input.NewPassword = "", ""
	}
	body, _ := json.Marshal(input)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, origin.JoinPath(path).String(), bytes.NewReader(body))
	if err != nil {
		return http.StatusServiceUnavailable, unavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf.Token)
	req.Header.Set("Origin", origin.String())
	var result resetResult
	status, err := read(req, &result)
	if err == nil && status == http.StatusOK && result.OK {
		return http.StatusOK, result
	}
	if err == nil && (status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusUnprocessableEntity || status == http.StatusTooManyRequests) {
		if result.Message == "" {
			result.Message = "请求未完成，请检查输入或稍后重试"
		}
		return status, result
	}
	return http.StatusServiceUnavailable, unavailable
}
