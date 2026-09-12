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
	"loginTest/observability"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	sm2 "github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/sm3"
)

// 平台中转调用鉴权（x-model-authorization）：
// 集市开发者平台经 /api/open/invoke/:apiId 转发请求时，用平台 SM2 私钥对
// "{slug}\n{ts}\n{nonce}\n{queryTime}" 签名（slug 固定 __platform__，hash:true，非 DER），
// 头格式为 base64url(slug).base64url(ts).base64url(nonce).base64url(sign raw 64B)。
// 调用时间经 x-model-query-time 头独立携带并参与签名。验签通过即证明请求必然来自平台代理；
// 真实调用方由 x-invoker-module 头携带。
const (
	PlatformSignerSlug         = "__platform__"
	PlatformInvokerHeader      = "x-invoker-module"
	PlatformAuthHeaderName     = "x-model-authorization"
	PlatformQueryTimeHeader    = "x-model-query-time"
	platformAuthMaxClockSkew   = 5 * time.Minute
	platformAuthMaxQueryCount  = int64(platformAuthMaxClockSkew / time.Millisecond)
)

// defaultSM2UID SM2 默认用户标识（与 sm-crypto 生成 ZA 时的默认值一致）。
const defaultSM2UID = "1234567812345678"

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
			log.Errorf("[platform-auth] 平台公钥解析失败: %v", err)
			return
		}
		if len(data) != 65 || data[0] != 0x04 {
			log.Errorf("[platform-auth] 平台公钥格式错误: 期望 04||X||Y 共 65 字节, 实际 %d 字节", len(data))
			return
		}
		x := new(big.Int).SetBytes(data[1 : 1+32])
		y := new(big.Int).SetBytes(data[1+32 : 1+64])
		platformAuthPubKey = &sm2.PublicKey{
			Curve: sm2.P256Sm2(),
			X:     x,
			Y:     y,
		}
		log.Infof("[platform-auth] 平台公钥加载成功 (key 前 16 位: %s...)", hexKey[:16])
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
// 签名串：{slug}\n{ts}\n{nonce}\n{queryTime}（四段，queryTime 取 x-model-query-time 头）。
// 兼容旧三段串（无 queryTime）。返回 (是否通过, 失败原因)。任一环节失败即拒绝。
func VerifyPlatformAuth(header, queryTimeHeader string) (bool, string) {
	pub, ok := platformPublicKey()
	if !ok {
		return false, "平台公钥未加载"
	}
	parts := strings.Split(strings.TrimSpace(header), ".")
	if len(parts) != 4 {
		return false, "格式无效（应为 slug.ts.nonce.sign 四段）"
	}
	slugBytes, err := unb64u(parts[0])
	if err != nil {
		return false, "slug 段 base64url 解码失败"
	}
	tsBytes, err := unb64u(parts[1])
	if err != nil {
		return false, "ts 段 base64url 解码失败"
	}
	nonceBytes, err := unb64u(parts[2])
	if err != nil {
		return false, "nonce 段 base64url 解码失败"
	}
	signBytes, err := unb64u(parts[3])
	if err != nil {
		return false, "sign 段 base64url 解码失败"
	}
	if subtle.ConstantTimeCompare(slugBytes, []byte(PlatformSignerSlug)) != 1 {
		return false, "slug 不是 __platform__（收到: " + truncateForLog(string(slugBytes)) + "）"
	}
	ts, err := strconv.ParseInt(string(tsBytes), 10, 64)
	if err != nil {
		return false, "ts 不是合法毫秒时间戳（收到: " + truncateForLog(string(tsBytes)) + "）"
	}
	skew := time.Since(time.UnixMilli(ts))
	if skew > platformAuthMaxClockSkew || -skew > platformAuthMaxClockSkew {
		return false, "时间戳超出 ±5 分钟窗口（偏差 " + skew.Truncate(time.Second).String() + "）"
	}
	if len(nonceBytes) == 0 {
		return false, "nonce 为空"
	}
	if len(signBytes) != 64 {
		return false, "签名长度错误（期望 64 字节 r||s，实际 " + strconv.Itoa(len(signBytes)) + " 字节，可能为 DER 编码）"
	}
	queryTime := strings.TrimSpace(queryTimeHeader)
	if queryTime != "" {
		qt, qerr := strconv.ParseInt(queryTime, 10, 64)
		if qerr != nil {
			return false, "x-model-query-time 不是合法毫秒时间戳（收到: " + truncateForLog(queryTime) + "）"
		}
		qtSkew := time.Since(time.UnixMilli(qt))
		if qtSkew > platformAuthMaxClockSkew || -qtSkew > platformAuthMaxClockSkew {
			return false, "调用时间 queryTime 超出 ±5 分钟窗口（偏差 " + qtSkew.Truncate(time.Second).String() + "）"
		}
		// queryTime 与 ts 偏差需 ≤5 分钟
		if d := ts - qt; d > int64(platformAuthMaxClockSkew/time.Millisecond) || -d > int64(platformAuthMaxClockSkew/time.Millisecond) {
			return false, "调用时间与签名时间戳不一致（疑似挪用 token）"
		}
	}
	// 对齐文档：签名串包含 queryTime；若调用方带 x-model-query-time 则拼四段，否则退回旧三段。
	payload := string(slugBytes) + "\n" + string(tsBytes) + "\n" + string(nonceBytes)
	if queryTime != "" {
		payload = payload + "\n" + queryTime
	}
	r := new(big.Int).SetBytes(signBytes[:32])
	s := new(big.Int).SetBytes(signBytes[32:])
	// 平台 sm-crypto doSignature({hash:true, der:false})：
	// 签名时不传 publicKey → 对 SM3(payload) 纯摘要签名（无 ZA 预处理，等价 tjfoc sm2.Verify）
	// 传了 publicKey → 标准 SM2 签名（含 ZA(uid) 预处理，等价 tjfoc Sm2Verify）。两种路径都尝试。
	if sm2.Verify(pub, sm3.Sm3Sum([]byte(payload)), r, s) {
		return true, ""
	}
	if sm2.Sm2Verify(pub, []byte(payload), []byte(defaultSM2UID), r, s) {
		return true, ""
	}
	return false, "SM2 验签失败（公钥与签发私钥不配对，或 payload 拼接不一致）"
}

func truncateForLog(s string) string {
	if len(s) > 32 {
		return s[:32] + "..."
	}
	return s
}

// PlatformAuthMiddleware gin 中间件：仅放行经平台中转签发的请求。
// 通过后 ctx.Set("platformInvoker", 真实调用方模块 slug)。
func PlatformAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader(PlatformAuthHeaderName)
		if header == "" {
			logReject(c, "缺少 x-model-authorization 请求头")
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少 x-model-authorization 请求头"})
			c.Abort()
			return
		}
		ok, reason := VerifyPlatformAuth(header, c.GetHeader(PlatformQueryTimeHeader))
		if !ok {
			logReject(c, reason)
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "平台验签失败: " + reason})
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
		log.WithFields(log.Fields{
			"client_ip":      c.ClientIP(),
			"request_id":     observability.RequestIDFromGin(c),
			"invoker_module": invoker,
			"req_uri":        c.Request.RequestURI,
		}).Info("[platform-auth] 平台验签通过")
		c.Next()
	}
}

// logReject 输出结构化拒绝日志，便于排查 401 原因。
func logReject(c *gin.Context, reason string) {
	fields := log.Fields{
		"client_ip": c.ClientIP(),
		"req_uri":   c.Request.RequestURI,
		"reason":    reason,
	}
	if reqID := observability.RequestIDFromGin(c); reqID != "" {
		fields["request_id"] = reqID
	}
	if invoker := c.GetHeader(PlatformInvokerHeader); invoker != "" {
		fields["invoker_module"] = invoker
	}
	// 打码记录 token 前两段（slug/ts），sign 段绝不落日志
	parts := strings.SplitN(c.GetHeader(PlatformAuthHeaderName), ".", 3)
	if len(parts) >= 2 {
		fields["token_prefix"] = truncateForLog(parts[0] + "." + parts[1])
	}
	log.WithFields(fields).Warn("[platform-auth] 拒绝请求")
}
