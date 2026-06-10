// 返回token的文件，不用管，要看也行。模板写法来着，至于为什么，我也不是很懂
package common

import (
	"loginTest/model"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

// 采用jwt方式生成token
var jwtKey []byte

type Claims struct {
	UserID int
	jwt.RegisteredClaims
}

type Claims_admin struct {
	AdminID int
	jwt.RegisteredClaims
}

// RefreshClaims 用于刷新 token 的 JWT 声明
type RefreshClaims struct {
	UserID int
	jwt.RegisteredClaims
}

// OAuth2Claims OAuth2访问令牌的Claims
type OAuth2Claims struct {
	UserID int
	AppID  string
	Scope  string
	jwt.RegisteredClaims
}

func InitJWTkey() {
	jwtKey = []byte(viper.GetString("crypto.jwtKey"))
}

func registeredClaims(exp time.Time, subject string) jwt.RegisteredClaims {
	now := time.Now()
	return jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(now),
		Issuer:    "loginTest",
		Subject:   subject,
	}
}

func ReleaseToken(user model.User) (string, error) {
	claims := &Claims{
		UserID:           user.UserID,
		RegisteredClaims: registeredClaims(time.Now().Add(7*24*time.Hour), "user token"),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// ReleaseRefreshToken 生成用于刷新 access token 的 refresh token
// 有效期：30 天
func ReleaseRefreshToken(user model.User) (string, error) {
	claims := &RefreshClaims{
		UserID:           user.UserID,
		RegisteredClaims: registeredClaims(time.Now().Add(30*24*time.Hour), "refresh token"),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

func ReleaseToken_admin(admin model.Admin) (string, error) {
	claims := &Claims_admin{
		AdminID:          admin.AdminID,
		RegisteredClaims: registeredClaims(time.Now().Add(24*time.Hour), "admin token"),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

func ParseToken(tokenString string) (*jwt.Token, *Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return token, claims, err
}

func ParseToken_admin(tokenString string) (*jwt.Token, *Claims_admin, error) {
	claimsAdmin := &Claims_admin{}
	token, err := jwt.ParseWithClaims(tokenString, claimsAdmin, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return token, claimsAdmin, err
}

// ParseRefreshToken 解析 refresh token
func ParseRefreshToken(tokenString string) (*jwt.Token, *RefreshClaims, error) {
	claims := &RefreshClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return token, claims, err
}

// 生成OAuth2访问令牌
func ReleaseOAuth2Token(userID int, appID, scope string) (string, error) {
	claims := &OAuth2Claims{
		UserID:           userID,
		AppID:            appID,
		Scope:            scope,
		RegisteredClaims: registeredClaims(time.Now().Add(5*time.Minute), "oauth2 token"),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// 解析OAuth2访问令牌
func ParseOAuth2Token(tokenString string) (*jwt.Token, *OAuth2Claims, error) {
	claims := &OAuth2Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	return token, claims, err
}
