package domain

// LoginInput 登录入参（密码为明文）。
type LoginInput struct {
	Email    string
	Password string
}
