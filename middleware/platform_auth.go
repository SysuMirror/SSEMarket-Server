package middleware

import (
	"crypto/subtle"
	"encoding/base64"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"loginTest/config"

	"github.com/gin-gonic/gin"
	sm2 "github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/sm3"
)

// defaultSM2UID SM2 默认用户标识（与 sm-crypto 生成 ZA 时的默认值一致）。
const defaultSM2UID = "1234567812345678"

// 平台中转调用鉴权（x-model-authorization）：
// 集市开发者平台经 /api/open/invoke/:apiId 转发请求时，用平台 SM2 私钥对
// "{slug}\n{ts}\n{nonce}" 签名（slug 固定 __platform__，hash:true，非 DER），
// 头格式为 base64url(slug).base64url(ts).base64url(nonce).base64url(sign hex)。
// 验签通过即证明请求必然来自平台代理；真实调用方由 x-invoker-module 头携带。
const (
	PlatformSignerSlug       = "__platform__"
	PlatformInvokerHeader    = "x-invoker-module"
	PlatformAuthHeaderName   = "x-model-authorization"
	platformAuthMaxClockSkew = 5 * time.Minute
)

var platformAuthOnce sync.Once
var platformAuthPubKey *sm2.PublicKey

// platformPublicKey 平台 SM2 公钥。优先取 config PLATFORM_SM2_PUBLIC_KEY（hex，04 开头非压缩格式），
// 未配置时回退到文档内置值，与开发者平台 server/.env 的 PLATFORM_SM2_PUBLIC_KEY 保持一致。
func platformPublicKey() (*sm2.PublicKey, bool) {
	platformAuthOnce.Do(func() {
		hexKey := strings.TrimSpace(config.GetPlatformSM2PublicKey())
		if hexKey == "" {
			hexKey = "04f01e80d1f5218eb28dc223577f74725552ea30f38dc4d2e7bb03ecbabea509d33f9815ac63a5c930fc5cbd575684ea44067c249b85d51e4afe3df2af5fc09cf0"
		}
		data, err := hexDecodeSM2(hexKey)
		if err != nil {
			return
		}
		x := new(big.Int).SetBytes(data[1 : 1+32])
		y := new(big.Int).SetBytes(data[1+32 : 1+64])
		platformAuthPubKey = &sm2.PublicKey{
			Curve: sm2.P256Sm2(),
			X:     x,
			Y:     y,
		}
	})
	return platformAuthPubKey, platformAuthPubKey != nil
}

func hexDecodeSM2(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	return hexDecodeString(s)
}

func hexDecodeString(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, base64.CorruptInputError(0)
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		hi, ok := hexVal(s[2*i])
		if !ok {
			return nil, base64.CorruptInputError(2 * i)
		}
		lo, ok := hexVal(s[2*i+1])
		if !ok {
			return nil, base64.CorruptInputError(2*i + 1)
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexVal(b byte) (byte, bool) {
	switch {
	case '0' <= b && b <= '9':
		return b - '0', true
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10, true
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

func unb64u(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// VerifyPlatformAuth 解析并校验平台签发的认证 token。
// 返回 (是否通过, 签名 slug)。任一环节失败即拒绝。
func VerifyPlatformAuth(header string) bool {
	pub, ok := platformPublicKey()
	if !ok {
		return false
	}
	parts := strings.Split(strings.TrimSpace(header), ".")
	if len(parts) != 4 {
		return false
	}
	slugBytes, err := unb64u(parts[0])
	if err != nil {
		return false
	}
	tsBytes, err := unb64u(parts[1])
	if err != nil {
		return false
	}
	nonceBytes, err := unb64u(parts[2])
	if err != nil {
		return false
	}
	signBytes, err := unb64u(parts[3])
	if err != nil {
		return false
	}
	if subtle.ConstantTimeCompare(slugBytes, []byte(PlatformSignerSlug)) != 1 {
		return false
	}
	ts, err := strconv.ParseInt(string(tsBytes), 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.UnixMilli(ts)) > platformAuthMaxClockSkew ||
		time.UnixMilli(ts).Sub(time.Now()) > platformAuthMaxClockSkew {
		return false
	}
	if len(nonceBytes) == 0 {
		return false
	}
	payload := string(slugBytes) + "\n" + string(tsBytes) + "\n" + string(nonceBytes)
	r, s, err := sm2DecompressSig(signBytes)
	if err != nil {
		return false
	}
	// 平台 sm-crypto doSignature({hash:true, der:false})：
	// 签名时不传 publicKey → 对 SM3(payload) 纯摘要签名（无 ZA 预处理，等价 tjfoc sm2.Verify）
	// 传了 publicKey → 标准 SM2 签名（含 ZA(uid) 预处理，等价 tjfoc Sm2Verify）。两种路径都尝试。
	if sm2.Verify(pub, sm3.Sm3Sum([]byte(payload)), r, s) {
		return true
	}
	return sm2.Sm2Verify(pub, []byte(payload), []byte(defaultSM2UID), r, s)
}

// sm2DecompressSig 将 64 字节 r||s 裸签名拆成两个大数（tjfoc Verify 需要 r/s 形式）。
func sm2DecompressSig(sig []byte) (*big.Int, *big.Int, error) {
	if len(sig) != 64 {
		return nil, nil, base64.CorruptInputError(0)
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	return r, s, nil
}

// PlatformAuthMiddleware gin 中间件：仅放行经平台中转签发的请求。
// 通过后 ctx.Set("platformInvoker", 真实调用方模块 slug)。
func PlatformAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader(PlatformAuthHeaderName)
		if header == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少 x-model-authorization 请求头"})
			c.Abort()
			return
		}
		if !VerifyPlatformAuth(header) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "平台验签失败（格式/时间窗/签名）"})
			c.Abort()
			return
		}
		invoker := strings.TrimSpace(c.GetHeader(PlatformInvokerHeader))
		if invoker == "" {
			invoker = "unknown"
		}
		if len(invoker) > 100 {
			invoker = invoker[:100]
		}
		c.Set("platformInvoker", invoker)
		c.Next()
	}
}
