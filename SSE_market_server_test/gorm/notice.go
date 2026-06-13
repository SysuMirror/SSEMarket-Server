package gormrepo

import (
	"context"
	"errors"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type noticeRepository struct {
	db *gorm.DB
}

func NewNoticeRepository(db *gorm.DB) repository.NoticeRepository {
	return &noticeRepository{db: db}
}

func (r *noticeRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *noticeRepository) CountByReceiver(ctx context.Context, receiverID int) (int64, error) {
	var n int64
	err := r.session(ctx).Model(&model.Notice{}).
		Where("receiver = ?", receiverID).
		Count(&n).Error
	return n, err
}

func (r *noticeRepository) CountByReceiverAndRead(ctx context.Context, receiverID int, read bool) (int64, error) {
	var n int64
	err := r.session(ctx).Model(&model.Notice{}).
		Where("receiver = ? AND `read` = ?", receiverID, read).
		Count(&n).Error
	return n, err
}

func (r *noticeRepository) ListPage(ctx context.Context, receiverID, read, requireID, pageSize int) ([]repository.NoticeRecord, error) {
	var notices []model.Notice
	q := r.session(ctx).Where("receiver = ? AND `read` = ?", receiverID, read).Order("noticeID DESC")
	if requireID > 0 {
		q = q.Where("noticeID < ?", requireID)
	}
	if pageSize > 0 {
		q = q.Limit(pageSize)
	}
	if err := q.Find(&notices).Error; err != nil {
		return nil, err
	}
	out := make([]repository.NoticeRecord, 0, len(notices))
	for _, n := range notices {
		out = append(out, noticeToRecord(&n))
	}
	return out, nil
}

func (r *noticeRepository) MarkRead(ctx context.Context, noticeID int) error {
	return r.session(ctx).Model(&model.Notice{}).
		Where("noticeID = ?", noticeID).
		Update("read", true).Error
}

func (r *noticeRepository) FindUserByID(ctx context.Context, userID int) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("userID = ?", userID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *noticeRepository) FindPcommentTargetPost(ctx context.Context, pcommentID int) (int, time.Time, error) {
	var pc model.Pcomment
	if err := r.session(ctx).Where("pcommentID = ?", pcommentID).First(&pc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, time.Time{}, domain.ErrNotFound
		}
		return 0, time.Time{}, err
	}
	return pc.PtargetID, pc.Time, nil
}

func (r *noticeRepository) FindCcommentChain(ctx context.Context, ccommentID int) (int, int, time.Time, error) {
	var cc model.Ccomment
	if err := r.session(ctx).Where("ccommentID = ?", ccommentID).First(&cc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, time.Time{}, domain.ErrNotFound
		}
		return 0, 0, time.Time{}, err
	}
	postID, _, err := r.FindPcommentTargetPost(ctx, cc.CtargetID)
	if err != nil {
		return 0, 0, time.Time{}, err
	}
	return cc.CtargetID, postID, cc.Time, nil
}

func noticeToRecord(n *model.Notice) repository.NoticeRecord {
	if n == nil {
		return repository.NoticeRecord{}
	}
	return repository.NoticeRecord{
		NoticeID: n.NoticeID,
		Receiver: n.Receiver,
		Sender:   n.Sender,
		Type:     n.Type,
		Content:  n.Ntext,
		Read:     n.Read,
		Target:   n.Target,
		Time:     n.Time,
	}
}
