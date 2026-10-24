package domain

import (
	"errors"
	"time"
)

// 用户自定义皮肤业务错误。
var (
	ErrUserSkinNotFound     = errors.New("皮肤不存在")
	ErrUserSkinNotOwner     = errors.New("只能管理自己上传的皮肤")
	ErrUserSkinNotPublic    = errors.New("作者尚未公开这套皮肤")
	ErrUserSkinNotInLibrary = errors.New("这套皮肤不在你的皮肤库中")
	ErrUserSkinAlreadyOwned = errors.New("自己的皮肤无需重复添加")
	ErrUserSkinNameInvalid  = errors.New("皮肤名称需为 1-30 个字符")
	ErrUserSkinImageInvalid = errors.New("皮肤图片只接受图床返回的 http(s) 绝对地址")
)

// UserSkin 用户自定义皮肤（桌面/手机双图）。
type UserSkin struct {
	ID          int
	OwnerID     int
	OwnerName   string
	Name        string
	Description string
	DesktopURL  string
	MobileURL   string
	IsPublic    bool
	ShareToken  string
	CreatedAt   time.Time
	UpdatedAt   time.Time

	// 列表上下文：当前查看者是否拥有/皮肤库里是否已有。
	IsOwner   bool
	InLibrary bool
}

// UserSkinInput 上传/创建自定义皮肤入参（图片已先上传到图床）。
type UserSkinInput struct {
	OwnerID     int
	Name        string
	Description string
	DesktopURL  string
	MobileURL   string
}
