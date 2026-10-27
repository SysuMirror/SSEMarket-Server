package service

import (
	"context"
	"errors"
	"testing"

	"loginTest/domain"
	"loginTest/repository"
)

type replyTargetRepository struct {
	repository.CommentRepository
	parentPostID  int
	replyParentID int
	inserts       int
}

func (r *replyTargetRepository) FindUserByID(context.Context, int) (*domain.User, error) {
	return &domain.User{ID: 7}, nil
}
func (r *replyTargetRepository) FindPostByID(context.Context, int) (*domain.Post, error) {
	return &domain.Post{ID: 22, UserID: 7}, nil
}
func (r *replyTargetRepository) FindPcommentByID(context.Context, int) (*domain.Pcomment, error) {
	return &domain.Pcomment{ID: 30, PostID: r.parentPostID}, nil
}
func (r *replyTargetRepository) FindCcommentByID(context.Context, int) (*domain.Ccomment, error) {
	return &domain.Ccomment{ID: 40, PcommentID: r.replyParentID}, nil
}
func (r *replyTargetRepository) CreateCcomment(context.Context, *domain.Ccomment) error {
	r.inserts++
	return nil
}

func TestReplyTargetChainValidatedBeforeInsert(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		parentPost, replyParent int
	}{{"different post", 99, 30}, {"different reply parent", 22, 99}} {
		t.Run(tc.name, func(t *testing.T) {
			r := &replyTargetRepository{parentPostID: tc.parentPost, replyParentID: tc.replyParent}
			s := NewCommentService(r, nil, nil)
			_, err := s.CreateCcomment(context.Background(), domain.CcommentCreateInput{UserID: 7, PostID: 22, PcommentID: 30, ReplyCcommentID: 40, Content: "reply"})
			if !errors.Is(err, domain.ErrBadRequest) || r.inserts != 0 {
				t.Fatalf("err=%v inserts=%d", err, r.inserts)
			}
		})
	}
}
