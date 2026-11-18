package util

import (
	"crypto/rand"
	"encoding/hex"
	mathRand "math/rand"
	"time"
	"unicode/utf8"
)

const BrowseContentPreviewRunes = 100

// TruncateRunes 按 Unicode 字符数截断字符串（中文算 1 字）。
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

// 生成随机数字
func GenerateRandomDigits(length int) string {
	digits := "0123456789"
	result := make([]byte, length)
	mathRand.Seed(time.Now().Unix())
	for i := 0; i < length; i++ {
		result[i] = digits[mathRand.Intn(len(digits))]
	}
	return string(result)
}

// 用于OAuth2授权过程生成随机字符串
func GenerateRandomString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// 如果crypto/rand失败，回退到math/rand
		r := mathRand.New(mathRand.NewSource(time.Now().UnixNano()))
		for i := range bytes {
			bytes[i] = byte(r.Intn(256))
		}
	}
	return hex.EncodeToString(bytes)
}
