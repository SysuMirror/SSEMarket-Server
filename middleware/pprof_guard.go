package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// PprofGuard 仅 GIN_MODE=debug 或内网 IP 白名单可访问 pprof。
func PprofGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("GIN_MODE") == "debug" {
			c.Next()
			return
		}
		if isInternalIP(c.ClientIP()) {
			c.Next()
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
	if extra := os.Getenv("PPROF_IP_WHITELIST"); extra != "" {
		for _, item := range strings.Split(extra, ",") {
			item = strings.TrimSpace(item)
			if item == ipStr {
				return true
			}
			if _, cidr, err := net.ParseCIDR(item); err == nil && cidr.Contains(ip) {
				return true
			}
		}
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
