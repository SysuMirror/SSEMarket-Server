// Package mq 提供帖子归档延迟队列能力。
//
// 设计要点：
//   - 双 key 结构：ZSet 存时间索引（score=到期时间戳），Hash 存消息 payload
//   - msg_id 格式：<type>:<id>:<unix_ms>（带时间戳避免同资源多次入队时 member 冲突）
//   - 1 小时延迟：发帖/评论后 1h 才被消费，给「1h 内删除」留缓冲
//   - 取消机制：删除帖子/评论时扫描 Hash 找匹配 msg_id 并 ZREM+HDEL
//
// 消费端（server-task Python）：
//   每 30s 用 ZRANGEBYSCORE 取到期消息，HGET 取 payload，处理成功后 ZREM+HDEL。
//   失败时 score=now+60 重试，3 次后进死信 ZSet。
package mq

import (
	"context"
	// "encoding/json" // 随 enqueue 暂时停用；恢复入队时取消注释
	"fmt"
	"log"
	"time"

	"loginTest/core"

	// "github.com/redis/go-redis/v9" // 随 enqueue 暂时停用；恢复入队时取消注释
)

// 队列默认 key 名称（与 Python 端约定一致）
const (
	defaultQueueKey     = "archive_delay_queue"
	defaultPayloadKey   = "archive_msg_payloads"
	defaultDeadQueueKey = "archive_delay_queue_dead"

	// 默认延迟时间：1 小时（评论可能在 1h 内被删除）
	defaultDelay = time.Hour

	// 取消消息时一次最多扫描的 Hash 字段数
	scanCount = 200
)

// MessageType 消息类型
type MessageType string

const (
	TypePostCreate    MessageType = "post"       // 帖子创建
	TypePcommentCreate MessageType = "pcomment"  // 一级评论创建
)

// Message 入队/出队用的消息体
type Message struct {
	MsgID      string      `json:"msg_id"`       // 形如 post:123:1698765432000
	Type       MessageType `json:"type"`         // post / pcomment
	PostID     int         `json:"post_id"`      // 帖子 ID（两种类型都有）
	PcommentID int         `json:"pcomment_id"`  // 评论 ID（仅 pcomment 类型）
	Title      string      `json:"title"`        // 帖子标题（便于排查）
	Content    string      `json:"content"`      // 评论内容预览（便于排查，仅 pcomment 类型有）
	EnqueueAt  int64       `json:"enqueue_at"`   // 入队时间戳（ms）
	ExpireAt   int64       `json:"expire_at"`    // 到期时间戳（ms）
}

// EnqueuePost 帖子创建后入队。delay=1h 后由 server-task 消费。
func EnqueuePost(ctx context.Context, postID int, title string) error {
	return enqueue(ctx, TypePostCreate, postID, 0, title, "")
}

// EnqueuePcomment 一级评论创建后入队。携带 post_id（worker 会读完整帖子+所有评论重新归档）。
func EnqueuePcomment(ctx context.Context, postID, pcommentID int, content string) error {
	return enqueue(ctx, TypePcommentCreate, postID, pcommentID, "", content)
}

// CancelPost 帖子删除时调用：扫描并移除该帖子所有未消费的入队消息。
// 1h 内删除的帖子，对应消息会被移除，worker 不会再处理。
func CancelPost(ctx context.Context, postID int) error {
	return cancel(ctx, fmt.Sprintf("%s:%d:", TypePostCreate, postID))
}

// CancelPcomment 一级评论删除时调用：扫描并移除该评论所有未消费的入队消息。
func CancelPcomment(ctx context.Context, pcommentID int) error {
	return cancel(ctx, fmt.Sprintf("%s:%d:", TypePcommentCreate, pcommentID))
}

// enqueue 通用入队逻辑
func enqueue(ctx context.Context, msgType MessageType, postID, pcommentID int, title, content string) error {
	// 暂时停止向 iwiki 提取服务消费的 Redis key（archive_delay_queue / archive_msg_payloads）塞入数据，
	// 以节省服务器成本。恢复时删除本 early return，并取消下方及 import 中的注释即可。
	return nil
	/*
		if core.MyRedis == nil {
			return fmt.Errorf("redis not initialized")
		}
		now := time.Now()
		nowMs := now.UnixMilli()
		expireMs := now.Add(defaultDelay).UnixMilli()

		msgID := fmt.Sprintf("%s:%d:%d", msgType, postID, nowMs)
		// pcomment 消息额外携带 pcomment_id 用于取消时匹配
		if msgType == TypePcommentCreate {
			msgID = fmt.Sprintf("%s:%d:%d", msgType, pcommentID, nowMs)
		}

		msg := Message{
			MsgID:      msgID,
			Type:       msgType,
			PostID:     postID,
			PcommentID: pcommentID,
			Title:      title,
			Content:    content,
			EnqueueAt:  nowMs,
			ExpireAt:   expireMs,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("marshal message: %w", err)
		}

		// ZADD + HSET 用 pipeline 一次性提交，保证原子性
		pipe := core.MyRedis.Pipeline()
		pipe.ZAdd(ctx, defaultQueueKey, redis.Z{
			Score:  float64(expireMs),
			Member: msgID,
		})
		pipe.HSet(ctx, defaultPayloadKey, msgID, payload)
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("enqueue to redis: %w", err)
		}
		log.Printf("[archive-mq] enqueue type=%s postID=%d pcommentID=%d expire_at=%s",
			msgType, postID, pcommentID, time.UnixMilli(expireMs).Format(time.RFC3339))
		return nil
	*/
}

// cancel 扫描 Hash 找匹配 prefix 的 msg_id 并移除（ZREM + HDEL）
func cancel(ctx context.Context, prefix string) error {
	if core.MyRedis == nil {
		return fmt.Errorf("redis not initialized")
	}
	var removed int
	var cursor uint64
	for {
		keys, nextCursor, err := core.MyRedis.HScan(ctx, defaultPayloadKey, cursor, prefix+"*", scanCount).Result()
		if err != nil {
			return fmt.Errorf("hscan %s: %w", prefix, err)
		}
		// HScan 返回 [field, value, field, value, ...]，取偶数下标为 field
		var msgIDs []string
		for i := 0; i < len(keys); i += 2 {
			msgIDs = append(msgIDs, keys[i])
		}
		if len(msgIDs) > 0 {
			pipe := core.MyRedis.Pipeline()
			pipe.ZRem(ctx, defaultQueueKey, toInterfaceSlice(msgIDs)...)
			pipe.HDel(ctx, defaultPayloadKey, msgIDs...)
			_, err = pipe.Exec(ctx)
			if err != nil {
				return fmt.Errorf("remove cancelled msgs: %w", err)
			}
			removed += len(msgIDs)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	if removed > 0 {
		log.Printf("[archive-mq] cancel prefix=%s removed=%d", prefix, removed)
	}
	return nil
}

// toInterfaceSlice 把 []string 转成 []interface{}，便于 redis pipeline 接收变长参数
func toInterfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
