package service

import (
	"context"
	"fmt"

	"loginTest/domain"
	"loginTest/repository"
)

const (
	sueStatusNoViolation = "nosin"
	sueStatusViolation   = " ok"
)

type sueService struct {
	sues repository.SueRepository
}

func NewSueService(sues repository.SueRepository) SueService {
	return &sueService{sues: sues}
}

func (s *sueService) ListPending(ctx context.Context) ([]domain.SueListItem, error) {
	sues, err := s.sues.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SueListItem, 0, len(sues))
	for _, sue := range sues {
		title, detail, ok := s.sues.LoadTargetSummary(ctx, sue.TargetType, sue.TargetID)
		if !ok {
			continue
		}
		out = append(out, domain.SueListItem{
			ID:           sue.ID,
			TargetType:   sue.TargetType,
			TargetTitle:  title,
			TargetDetail: detail,
			Reason:       sue.Reason,
			Time:         sue.Time,
		})
	}
	return out, nil
}

func (s *sueService) MarkNoViolation(ctx context.Context, sueID int) error {
	sue, err := s.sues.FindByID(ctx, sueID)
	if err != nil {
		return err
	}
	if err := s.sues.MarkFinishedByID(ctx, sueID, sueStatusNoViolation); err != nil {
		return err
	}
	return s.sues.MarkFinishedByTarget(ctx, sue.TargetType, sue.TargetID, sueStatusNoViolation)
}

func (s *sueService) MarkViolation(ctx context.Context, sueID int) error {
	sue, err := s.sues.FindByID(ctx, sueID)
	if err != nil {
		return err
	}
	if err := s.sues.MarkFinishedByID(ctx, sueID, sueStatusViolation); err != nil {
		return err
	}
	if err := s.sues.MarkFinishedByTarget(ctx, sue.TargetType, sue.TargetID, sueStatusViolation); err != nil {
		return err
	}
	result, err := s.sues.ProcessViolation(ctx, sue)
	if err != nil {
		return err
	}
	user, err := s.sues.PunishUser(ctx, result.TargetUserID)
	if err != nil {
		return err
	}
	result.BanEnd = user.BanEnd
	punishText := fmt.Sprintf("你的%s[%s]内容涉嫌违规被举报，相关内容已被删除，你的账号被封禁，封禁时间到%s。",
		result.SueType, result.Content, user.BanEnd.Format("2006-01-02 15:04:05"))
	if err := s.sues.CreateNotice(ctx, result.TargetUserID, "punish", punishText, result.SueID); err != nil {
		return err
	}
	return s.sues.CreateNotice(ctx, result.ReporterID, "sue",
		"我们已经收到你的举报，违规内容已被删除，感谢你的支持", result.SueID)
}
