package cache

import (
	"encoding/json"
	"reflect"
	"time"
)

// CacheValue 带逻辑过期时间的缓存包装，支持「返回旧值 + 异步刷新」防雪崩。
type CacheValue[T any] struct {
	Data     T         `json:"data"`
	ExpireAt time.Time `json:"expire_at"`
	Version  int64     `json:"version"`
}

func wrapValue[T any](data T, logicalTTL time.Duration) CacheValue[T] {
	return CacheValue[T]{
		Data:     data,
		ExpireAt: time.Now().Add(logicalTTL),
		Version:  time.Now().UnixNano(),
	}
}

func encodeValue[T any](cv CacheValue[T]) ([]byte, error) {
	return json.Marshal(cv)
}

func decodeValue[T any](raw []byte) (CacheValue[T], bool) {
	var cv CacheValue[T]
	if err := json.Unmarshal(raw, &cv); err != nil {
		return cv, false
	}
	return cv, true
}

func isLogicValid[T any](cv CacheValue[T]) bool {
	return time.Now().Before(cv.ExpireAt)
}

// isEmptySlice 判断值是否为 nil 或 len==0 的 slice（用于跳过空列表缓存）。
func isEmptySlice(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		return rv.Len() == 0
	default:
		return false
	}
}
