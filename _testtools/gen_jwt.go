package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID int
	jwt.RegisteredClaims
}

func main() {
	key := []byte("qeYc&*s18lP+3Ga%w^I59Paq3jx3&o!X")
	claims := &Claims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "loginTest",
			Subject:   "user token",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
	if err != nil {
		panic(err)
	}
	fmt.Print(token)
}
