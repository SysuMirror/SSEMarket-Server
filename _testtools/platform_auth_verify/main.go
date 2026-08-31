package main

// 平台验签端到端自测（不连数据库）：
// 1. 生成一对 SM2 密钥，模拟平台私钥签发 x-model-authorization
// 2. 用服务端同款验签逻辑（先 SM3 纯摘要、再标准 ZA 路径）验证
// 3. 覆盖 sm-crypto 两种 hash:true 形态：无 publicKey(纯SM3摘要) / 带 publicKey(标准 ZA)

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	sm2 "github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/sm3"
)

const uid = "1234567812345678"

func b64u(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func bigTo32Bytes(x *big.Int) []byte {
	out := make([]byte, 32)
	b := x.Bytes()
	copy(out[32-len(b):], b)
	return out
}

// signRawHash 模拟 sm-crypto doSignature({hash:true,der:false}) 不传 publicKey：
// 对 SM3(payload) 纯摘要做签名（无 ZA 预处理），输出 64 字节 r||s。
func signRawHash(priv *sm2.PrivateKey, payload string) []byte {
	digest := sm3.Sm3Sum([]byte(payload))
	return sm2SignDigest(priv, digest)
}

// signStandard 标准 SM2 签名（Sm2Sign 内部含 ZA(uid) 预处理），输出 64 字节 r||s。
func signStandard(priv *sm2.PrivateKey, payload string) []byte {
	r, s, err := sm2.Sm2Sign(priv, []byte(payload), []byte(uid), rand.Reader)
	if err != nil {
		panic(err)
	}
	return append(bigTo32Bytes(r), bigTo32Bytes(s)...)
}

// sm2SignDigest 对给定摘要 e 做 SM2 数字签名运算（SM2 椭圆曲线部分），输出 r||s。
// 算法与 GB/T 32918 一致：r=(e + x1) mod n，s=((k - d*r)/(1+d)) mod n。
func sm2SignDigest(priv *sm2.PrivateKey, eBytes []byte) []byte {
	c := priv.Curve
	N := c.Params().N
	e := new(big.Int).SetBytes(eBytes)
	if e.Cmp(N) >= 0 {
		e.Mod(e, N)
	}
	for {
		k, err := rand.Int(rand.Reader, N)
		if err != nil {
			panic(err)
		}
		if k.Sign() == 0 {
			continue
		}
		x1, _ := c.ScalarBaseMult(k.Bytes())
		r := new(big.Int).Add(e, x1)
		r.Mod(r, N)
		if r.Sign() == 0 {
			continue
		}
		if new(big.Int).Add(r, k).Cmp(N) == 0 {
			continue
		}
		d := priv.D
		num := new(big.Int).Sub(k, new(big.Int).Mul(d, r))
		num.Mod(num, N)
		den := new(big.Int).Add(d, big.NewInt(1))
		denInv := new(big.Int).ModInverse(den, N)
		s := new(big.Int).Mul(num, denInv)
		s.Mod(s, N)
		if s.Sign() == 0 {
			continue
		}
		return append(bigTo32Bytes(r), bigTo32Bytes(s)...)
	}
}

// verifyServerSide 与 middleware/platform_auth.go 保持一致的验签逻辑
func verifyServerSide(pub *sm2.PublicKey, header string) bool {
	parts := strings.Split(strings.TrimSpace(header), ".")
	if len(parts) != 4 {
		return false
	}
	dec := make([][]byte, 4)
	for i, p := range parts {
		b, err := base64.RawURLEncoding.DecodeString(p)
		if err != nil {
			return false
		}
		dec[i] = b
	}
	slug, tsB, nonceB, signB := dec[0], dec[1], dec[2], dec[3]
	if string(slug) != "__platform__" {
		return false
	}
	ts, err := strconv.ParseInt(string(tsB), 10, 64)
	if err != nil {
		return false
	}
	if time.Since(time.UnixMilli(ts)) > 5*time.Minute ||
		time.UnixMilli(ts).Sub(time.Now()) > 5*time.Minute {
		return false
	}
	if len(nonceB) == 0 || len(signB) != 64 {
		return false
	}
	payload := string(slug) + "\n" + string(tsB) + "\n" + string(nonceB)
	r := new(big.Int).SetBytes(signB[:32])
	s := new(big.Int).SetBytes(signB[32:])
	if sm2.Verify(pub, sm3.Sm3Sum([]byte(payload)), r, s) {
		return true
	}
	return sm2.Sm2Verify(pub, []byte(payload), []byte(uid), r, s)
}

func main() {
	priv, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	pub := &priv.PublicKey
	fmt.Println("公钥(04||X||Y):", "04"+fmt.Sprintf("%064x%064x", pub.X, pub.Y))

	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := "testnonce123"
	payload := "__platform__\n" + ts + "\n" + nonce

	cases := []struct {
		name  string
		sigFn func(*sm2.PrivateKey, string) []byte
	}{
		{"sm-crypto hash:true 无 publicKey(纯SM3摘要)", signRawHash},
		{"标准 SM2 含 ZA(uid)", signStandard},
	}

	failed := false
	for _, tc := range cases {
		sig := tc.sigFn(priv, payload)
		header := b64u("__platform__") + "." + b64u(ts) + "." + b64u(nonce) + "." + base64.RawURLEncoding.EncodeToString(sig)
		ok := verifyServerSide(pub, header)
		fmt.Printf("[%s] 验签: %v\n", tc.name, pass(ok))
		if !ok {
			failed = true
		}
	}

	// 伪造 slug（用真私钥签、但 slug 是攻击者）必须失败
	badSlugPayload := "attacker\n" + ts + "\n" + nonce
	badSlugHeader := b64u("attacker") + "." + b64u(ts) + "." + b64u(nonce) + "." + base64.RawURLEncoding.EncodeToString(signRawHash(priv, badSlugPayload))
	ok := !verifyServerSide(pub, badSlugHeader)
	fmt.Printf("[%s] 验签(应为 false): %v\n", "伪造 slug", pass(ok))
	if !ok {
		failed = true
	}

	// 篡改时间戳必须失败
	futureTS := strconv.FormatInt(time.Now().Add(30*time.Minute).UnixMilli(), 10)
	tamperPayload := "__platform__\n" + futureTS + "\n" + nonce
	tamperHeader := b64u("__platform__") + "." + b64u(futureTS) + "." + b64u(nonce) + "." + base64.RawURLEncoding.EncodeToString(signRawHash(priv, tamperPayload))
	ok = !verifyServerSide(pub, tamperHeader)
	fmt.Printf("[%s] 验签(应为 false): %v\n", "超时时间戳", pass(ok))
	if !ok {
		failed = true
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("ALL_PASS")
}

func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
