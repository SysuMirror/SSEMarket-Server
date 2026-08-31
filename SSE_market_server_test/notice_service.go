package service

import (
	"context"
	"errors"

	"loginTest/domain"
	"loginTest/repository"
)

type noticeService struct {
	notices repository.NoticeRepository
}

func NewNoticeService(notices repository.NoticeRepository) NoticeService {
	return &noticeService{notices: notices}
}

func (s *noticeService) List(ctx context.Context, receiverID, read, requireID, pageSize int, receiverName string) (*domain.NoticePage, error) {
	notices, err := s.notices.ListPage(ctx, receiverID, read, requireID, pageSize)
	if err != nil {
		return nil, err
	}
	page := &domain.NoticePage{}
	if requireID == 0 {
		total, err := s.notices.CountByReceiver(ctx, receiverID)
		if err != nil {
			return nil, err
		}
		filtered, err := s.notices.CountByReceiverAndRead(ctx, receiverID, read == 1)
		if err != nil {
			return nil, err
		}
		page.TotalNum = int(total)
		page.UnreadTotalNum = int(filtered)
	}
	if len(notices) == 0 {
		return page, nil
	}
	page.LastID = notices[len(notices)-1].NoticeID
	for _, notice := range notices {
		item, err := s.buildNoticeItem(ctx, notice, receiverName)
		if err != nil {
			return nil, err
		}
		page.NoticeList = append(page.NoticeList, item)
	}
	return page, nil
}

func (s *noticeService) GetCounts(ctx context.Context, receiverID int) (*domain.NoticeCounts, error) {
	unread, err := s.notices.CountByReceiverAndRead(ctx, receiverID, false)
	if err != nil {
		return nil, err
	}
	read, err := s.notices.CountByReceiverAndRead(ctx, receiverID, true)
	if err != nil {
		return nil, err
	}
	return &domain.NoticeCounts{
		TotalNum:       int(read + unread),
		ReadTotalNum:   int(read),
		UnreadTotalNum: int(unread),
	}, nil
}

func (s *noticeService) MarkRead(ctx context.Context, noticeID int) error {
	return s.notices.MarkRead(ctx, noticeID)
}

func (s *noticeService) buildNoticeItem(ctx context.Context, notice repository.NoticeRecord, receiverName string) (domain.NoticeItem, error) {
	sender, err := s.notices.FindUserByID(ctx, notice.Sender)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.NoticeItem{}, err
	}
	senderName := ""
	senderAvatar := ""
	if sender != nil {
		senderName = sender.Name
		senderAvatar = sender.AvatarURL
	}
	item := domain.NoticeItem{
		NoticeID:     notice.NoticeID,
		ReceiverName: receiverName,
		SenderName:   senderName,
		SenderAvatar: senderAvatar,
		SenderID:     notice.Sender,
		Type:         notice.Type,
		Content:      notice.Content,
		Read:         notice.Read,
		Target:       notice.Target,
		Time:         notice.Time,
	}
	switch notice.Type {
	case "ccomment":
		pcommentID, postID, commentTime, err := s.notices.FindCcommentChain(ctx, notice.Target)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.NoticeItem{}, err
		}
		item.PcommentID = pcommentID
		item.PostID = postID
		if !commentTime.IsZero() {
			item.Time = commentTime
		}
	case "pcomment":
		postID, commentTime, err := s.notices.FindPcommentTargetPost(ctx, notice.Target)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.NoticeItem{}, err
		}
		item.PostID = postID
		if !commentTime.IsZero() {
			item.Time = commentTime
		}
	case "post":
		item.PostID = notice.Target
		item.Time = notice.Time
	case "lottery_win":
		item.PostID = notice.Target
		item.Time = notice.Time
	default:
		item.Time = notice.Time
	}
	// 匿名帖作者在自己帖下的评论/回复/点赞等触发的通知，sender 脱敏为匿名用户，
	// 避免通知列表暴露匿名帖作者真实身份（与评论列表脱敏范围一致）。
	if item.PostID != 0 && sender != nil && sender.ID != 0 {
		if authorID, anon, err := s.notices.FindPostAnonMeta(ctx, item.PostID); err == nil && anon && authorID == sender.ID {
			item.SenderName = "匿名用户"
			item.SenderAvatar = ""
			item.SenderID = 0
		}
	}
	return item, nil
}
