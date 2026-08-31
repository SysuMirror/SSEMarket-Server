package domain

import "time"

// WorkbenchApp 工作台应用实体。
type WorkbenchApp struct {
	AppID       int
	Name        string
	Icon        string
	TargetType  string // route | link
	Target      string
	GroupName   string
	ColorBg     string
	ColorFg     string
	Source      string // local | platform
	InvokerSlug string
	SortOrder   int
	IsVisible   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkbenchAppCreateInput 创建工作台应用入参。
// InvokerSlug 非空表示来自平台中转调用（真实调用方模块 slug）。
type WorkbenchAppCreateInput struct {
	Name        string
	Icon        string
	TargetType  string
	Target      string
	GroupName   string
	ColorBg     string
	ColorFg     string
	SortOrder   int
	IsVisible   bool
	InvokerSlug string
}

// WorkbenchApp 相关业务错误。
var (
	ErrWorkbenchAppNameRequired    = ErrBadRequest
	ErrWorkbenchAppIconRequired    = ErrBadRequest
	ErrWorkbenchAppTargetInvalid   = ErrBadRequest
	ErrWorkbenchAppTargetTypeError = ErrBadRequest
)
