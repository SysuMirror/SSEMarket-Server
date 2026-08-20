package domain

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrBadRequest       = errors.New("bad request")
	ErrConflict         = errors.New("conflict")
	ErrInternal         = errors.New("internal error")
	ErrTokenExpired     = errors.New("token expired")
	ErrTooManyRequests  = errors.New("too many requests")
)

// 置顶相关业务错误。
var (
	ErrPinnedLimitReached = errors.New("每人仅可同时置顶 1 个帖子,请先取消现有置顶")
	ErrAlreadyPinned      = errors.New("该帖子已置顶")
	ErrNotPinned          = errors.New("该帖子未置顶")
)
