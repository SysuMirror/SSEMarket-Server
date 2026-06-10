package apperr

import (
	"errors"
	"net/http"

	"loginTest/domain"
	"loginTest/response"

	"github.com/gin-gonic/gin"
)

// StatusOf 将领域错误映射为 HTTP 状态码。
func StatusOf(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// CodeOf 将领域错误映射为业务 code（与 HTTP 状态码对齐）。
func CodeOf(err error) int {
	return StatusOf(err)
}

// Write 统一错误响应：HTTP 状态码与 body code 一致。
func Write(c *gin.Context, err error) {
	if err == nil {
		return
	}
	status := StatusOf(err)
	response.Response(c, status, status, nil, err.Error())
}

// WriteMsg 带自定义文案的错误响应。
func WriteMsg(c *gin.Context, err error, msg string) {
	if err == nil {
		return
	}
	status := StatusOf(err)
	if msg == "" {
		msg = err.Error()
	}
	response.Response(c, status, status, nil, msg)
}
