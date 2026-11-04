package controller

import (
	"context"
	"errors"
	"testing"

	"loginTest/domain"
	"loginTest/service"
)

type replyTargetComments struct {
	mcpComments
	inputs []domain.CcommentCreateInput
}

func (s *replyTargetComments) FindPcomment(context.Context, int) (*domain.Pcomment, error) {
	return &domain.Pcomment{ID: 30, PostID: 22, UserID: 8}, nil
}

func (s *replyTargetComments) FindCcomment(context.Context, int) (*domain.Ccomment, error) {
	return &domain.Ccomment{ID: 40, PcommentID: 30, UserID: 99}, nil
}

func (s *replyTargetComments) CreateCcomment(_ context.Context, in domain.CcommentCreateInput) (*domain.CcommentCreateResult, error) {
	s.inputs = append(s.inputs, in)
	return &domain.CcommentCreateResult{Ccomment: &domain.Ccomment{ID: 45, UserTargetName: in.UserTargetName}}, nil
}

func TestForumMCPReplyRecipientSurvivesLegacyWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		author  *domain.User
		replyID int
		want    string
	}{
		{"questioner", &domain.User{ID: 99, Name: "提问的人"}, 40, "提问的人"},
		{"anonymous", &domain.User{Name: "匿名用户"}, 40, "匿名用户"},
		{"missing public author", nil, 40, "匿名用户"},
		{"root comment", &domain.User{ID: 99, Name: "提问的人"}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := forumTestActions(t)
			oldC, oldP := service.Comments, service.Posts
			t.Cleanup(func() { service.Comments, service.Posts = oldC, oldP })
			comments := &replyTargetComments{mcpComments: mcpComments{tree: []*domain.PcommentWithMeta{
				{Pcomment: &domain.Pcomment{ID: 30, PostID: 22}, Author: &domain.User{ID: 8, Name: "层主"},
					Subcomments: []*domain.CcommentWithMeta{{Ccomment: &domain.Ccomment{ID: 40, PcommentID: 30, UserID: 99}, Author: tc.author}}},
			}}}
			service.Comments, service.Posts = comments, mcpPosts{post: &domain.Post{ID: 22}}
			in := forumPublishInput{IdempotencyKey: "reply-target-12345678", Action: "reply", PostID: 22, ParentCommentID: 30, ReplyCommentID: tc.replyID, Content: "回应提问"}
			out, err := forumPublish(ctx, in)
			if err != nil || out["status"] != "confirmed" || len(comments.inputs) != 1 {
				t.Fatal(out, err, comments.inputs)
			}
			got := comments.inputs[0]
			if got.UserTargetName != tc.want || got.ReplyCcommentID != tc.replyID || got.TargetUserID != 0 {
				t.Fatalf("wrong recipient or leaked identity: %+v", got)
			}
			// Retrying a confirmed action must not insert another reply.
			if _, err := forumPublish(ctx, in); err != nil || len(comments.inputs) != 1 {
				t.Fatal("duplicate reply", err)
			}
		})
	}
}

func TestForumMCPReplyTargetMustExistInPublicParent(t *testing.T) {
	old := service.Comments
	t.Cleanup(func() { service.Comments = old })
	service.Comments = mcpComments{tree: []*domain.PcommentWithMeta{
		{Pcomment: &domain.Pcomment{ID: 31}, Subcomments: []*domain.CcommentWithMeta{{Ccomment: &domain.Ccomment{ID: 40}, Author: &domain.User{ID: 99, Name: "wrong thread"}}}},
	}}
	if _, err := forumReplyTargetName(context.Background(), 22, 30, 40, 7); !errors.Is(err, domain.ErrBadRequest) {
		t.Fatal("cross-parent target accepted", err)
	}
}
