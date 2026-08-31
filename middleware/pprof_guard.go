package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func isWhitelisted(ipStr string) bool {
	if ipStr == "" {
		return false
	}
	extra := os.Getenv("PPROF_IP_WHITELIST")
	if extra == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, item := range strings.Split(extra, ",") {
		item = strings.TrimSpace(item)
		if item == ipStr {
			return true
		}
		if _, cidr, err := net.ParseCIDR(item); err == nil && cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// PprofGuard 仅 GIN_MODE=debug、PPROF_ENABLED 内网/白名单、或内网 IP 可访问 pprof。
func PprofGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("GIN_MODE") == "debug" {
			c.Next()
			return
		}
		clientIP := c.ClientIP()
		if isInternalIP(clientIP) || isWhitelisted(clientIP) {
			c.Next()
			return
		}
		if os.Getenv("PPROF_ENABLED") == "true" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"msg": "pprof forbidden: set PPROF_IP_WHITELIST or access from server localhost / private network",
			})
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "forbidden"})
	}
}

func isInternalIP(ipStr string) bool {
	if ipStr == "" {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	if isWhitelisted(ipStr) {
		return true
	}
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
	}
	for _, block := range privateBlocks {
		_, cidr, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
