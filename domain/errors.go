package domain

import "errors"

var (
	ErrNotFound        = errors.New("not found")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrBadRequest      = errors.New("bad request")
	ErrConflict        = errors.New("conflict")
	ErrInternal        = errors.New("internal error")
	ErrStale           = errors.New("stale data")
	ErrEmailUsed       = errors.New("email previously used")
	ErrTokenExpired    = errors.New("token expired")
	ErrTooManyRequests = errors.New("too many requests")
)

// 置顶相关业务错误。
var (
	ErrPinnedLimitReached       = errors.New("每人仅可同时置顶 1 个帖子,请先取消现有置顶")
	ErrGlobalPinnedLimitReached = errors.New("全站置顶名额已满(最多 5 个),请稍后再试")
	ErrAlreadyPinned            = errors.New("该帖子已置顶")
	ErrNotPinned                = errors.New("该帖子未置顶")
	// ErrPinnedWrongPartition 仅「主页」分区帖可进入置顶区(ListPinned 限定 partition=主页)。
	ErrPinnedWrongPartition = errors.New("仅主页分区的帖子可以置顶")
)

// PinAfterCreateError 发帖已成功但后续置顶失败时返回,便于 controller 区分文案。
type PinAfterCreateError struct {
	Post   *Post
	PinErr error
}

func (e *PinAfterCreateError) Error() string {
	if e == nil || e.PinErr == nil {
		return "发帖成功,但置顶失败"
	}
	return e.PinErr.Error()
}

func (e *PinAfterCreateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.PinErr
}
