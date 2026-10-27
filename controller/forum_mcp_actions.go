package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type forumActionInput struct {
	IdempotencyKey string `json:"idempotency_key" jsonschema:"Client-generated key, 16 to 64 ASCII letters/digits/underscore/hyphen; reuse exactly on lookup"`
}

var forumActionKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{16,64}$`)

type forumActionStore interface {
	Reserve(context.Context, int, string, string) (*model.ForumAgentAction, bool, error)
	Get(context.Context, int, string) (*model.ForumAgentAction, error)
	Finish(context.Context, int, string, string) error
}
type forumDBActions struct{}

var forumActions forumActionStore = forumDBActions{}

func (forumDBActions) Get(ctx context.Context, uid int, key string) (*model.ForumAgentAction, error) {
	if common.GetDB() == nil {
		return nil, domain.ErrInternal
	}
	var row model.ForumAgentAction
	err := common.GetDB().WithContext(ctx).Where("user_id = ? AND action_key = ?", uid, key).First(&row).Error
	return &row, err
}

func (s forumDBActions) Reserve(ctx context.Context, uid int, key, hash string) (*model.ForumAgentAction, bool, error) {
	if common.GetDB() == nil {
		return nil, false, domain.ErrInternal
	}
	row := &model.ForumAgentAction{UserID: uid, ActionKey: key, PayloadHash: hash}
	err := common.GetDB().WithContext(ctx).Create(row).Error
	if err == nil {
		return row, true, nil
	}
	var duplicate *mysql.MySQLError
	if !errors.Is(err, gorm.ErrDuplicatedKey) && !(errors.As(err, &duplicate) && duplicate.Number == 1062) {
		return nil, false, err
	}
	row, err = s.Get(ctx, uid, key)
	return row, false, err
}

func (forumDBActions) Finish(ctx context.Context, uid int, key, result string) error {
	return common.GetDB().WithContext(ctx).Model(&model.ForumAgentAction{}).
		Where("user_id = ? AND action_key = ?", uid, key).Update("result", result).Error
}

func forumActionStatus(ctx context.Context, in forumActionInput) (map[string]any, error) {
	if !forumActionKeyPattern.MatchString(in.IdempotencyKey) {
		return nil, domain.ErrBadRequest
	}
	row, err := forumActions.Get(ctx, forumActor(ctx).User.ID, in.IdempotencyKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return map[string]any{"status": "not_found", "idempotency_key": in.IdempotencyKey}, nil
	}
	if err != nil {
		return nil, err
	}
	return forumStoredAction(row), nil
}

func forumStoredAction(row *model.ForumAgentAction) map[string]any {
	var result map[string]any
	if row.Result != "" && json.Unmarshal([]byte(row.Result), &result) == nil && result != nil {
		return result
	}
	return map[string]any{"success": false, "status": "pending", "idempotency_key": row.ActionKey,
		"note": "May already have executed. Do not retry with a different key; operator reconciliation is required."}
}

// A durable at-most-once attempt, not an exactly-once transaction with the
// forum service. A crash after commit leaves pending; it must not re-execute.
func forumRunAction(ctx context.Context, key string, payload any, execute func() (map[string]any, error)) (map[string]any, error) {
	if !forumActionKeyPattern.MatchString(key) {
		return nil, domain.ErrBadRequest
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(b)
	hash := hex.EncodeToString(h[:])
	uid := forumActor(ctx).User.ID
	row, fresh, err := forumActions.Reserve(ctx, uid, key, hash)
	if err != nil {
		return nil, err
	} // never execute without a durable reservation
	if row.PayloadHash != hash {
		return map[string]any{"success": false, "status": "conflict", "idempotency_key": key}, nil
	}
	if !fresh {
		return forumStoredAction(row), nil
	}
	result, err := execute()
	if err != nil || result == nil {
		result = map[string]any{"success": false, "status": "uncertain",
			"note": "Do not repeat this operation or create a new key before reconciliation."}
	}
	result["idempotency_key"] = key
	b, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	// HTTP cancellation may happen after the write succeeds. Give receipt
	// persistence a short independent deadline without replaying the effect.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := forumActions.Finish(finishCtx, uid, key, string(b)); err != nil {
		return map[string]any{"success": false, "status": "uncertain", "idempotency_key": key,
			"note": "Receipt persistence failed. Do not repeat; inspect the original target."}, nil
	}
	return result, nil
}
