package controller

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var bilibiliBVRegexp = regexp.MustCompile(`^/video/(BV[0-9A-Za-z]{8,12})(?:[/?#]|$)`)

func allowedVideoHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "ssemarket.cn" {
		return true
	}
	hosts := os.Getenv("VIDEO_ALLOWED_SELF_HOSTS")
	if hosts == "" {
		hosts = ".myqcloud.com"
	}
	for _, item := range strings.Split(hosts, ",") {
		suffix := strings.ToLower(strings.TrimSpace(item))
		if suffix == "" {
			continue
		}
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func ParseVideoSource(sourceType, rawURL string) (string, string, string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", "", "", errors.New("视频地址必须是 HTTPS URL")
	}
	host := strings.ToLower(u.Hostname())
	path := u.EscapedPath()

	switch sourceType {
	case "bilibili":
		if host != "www.bilibili.com" && host != "m.bilibili.com" {
			return "", "", "", "", errors.New("Bilibili 地址域名不正确")
		}
		m := bilibiliBVRegexp.FindStringSubmatch(path)
		if m == nil {
			return "", "", "", "", errors.New("Bilibili 地址格式不正确")
		}
		return "bilibili", rawURL, m[1], rawURL, nil
	case "mp4":
		if !allowedVideoHost(host) || !strings.HasSuffix(strings.ToLower(path), ".mp4") {
			return "", "", "", "", errors.New("MP4 地址域名不受支持或扩展名不正确")
		}
		return "mp4", rawURL, "", "", nil
	case "hls":
		if !allowedVideoHost(host) || !strings.HasSuffix(strings.ToLower(path), ".m3u8") {
			return "", "", "", "", errors.New("HLS 地址域名不受支持或扩展名不正确")
		}
		return "hls", rawURL, "", "", nil
	case "auto":
		if (host == "www.bilibili.com" || host == "m.bilibili.com") && bilibiliBVRegexp.MatchString(path) {
			return ParseVideoSource("bilibili", rawURL)
		}
		if strings.HasSuffix(strings.ToLower(path), ".m3u8") {
			return ParseVideoSource("hls", rawURL)
		}
		if strings.HasSuffix(strings.ToLower(path), ".mp4") {
			return ParseVideoSource("mp4", rawURL)
		}
		return "", "", "", "", errors.New("无法识别视频来源")
	default:
		return "", "", "", "", fmt.Errorf("不支持的视频来源: %s", sourceType)
	}
}
