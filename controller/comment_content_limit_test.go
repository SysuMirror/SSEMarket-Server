package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"loginTest/config"
	"loginTest/domain"

	"github.com/gin-gonic/gin"
)

// 4000 字长评论能通过 controller 参数校验（后续因缺手机号返回“评论人不能为空”，
// 说明不是被“评论内容过长”拦下）；4001 字才被长度校验拒绝。
func TestCommentControllerAccepts4000Runes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name    string
		content string
		wantToo bool
	}{
		{"一级评论 4000 字", strings.Repeat("字", config.CommentContentMaxRunes), false},
		{"一级评论 4001 字", strings.Repeat("字", config.CommentContentMaxRunes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body, _ := json.Marshal(PcommentMsg{Content: tc.content})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/postPcomment", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			PostPcomment(c)

			got := w.Body.String()
			if strings.Contains(got, "评论内容过长") != tc.wantToo {
				t.Fatalf("body=%s wantToo=%v", got, tc.wantToo)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		content string
		wantToo bool
	}{
		{"楼中楼 4000 字", strings.Repeat("字", config.CommentContentMaxRunes), false},
		{"楼中楼 4001 字", strings.Repeat("字", config.CommentContentMaxRunes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body, _ := json.Marshal(CcommentMsg{Content: tc.content})
			c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/postCcomment", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			PostCcomment(c)

			got := w.Body.String()
			if strings.Contains(got, "评论内容过长") != tc.wantToo {
				t.Fatalf("body=%s wantToo=%v", got, tc.wantToo)
			}
		})
	}
}

func TestForumMCPCommentAccepts4000Runes(t *testing.T) {
	key := strings.Repeat("k", 16)
	for _, action := range []string{"comment", "reply"} {
		in := forumPublishInput{
			IdempotencyKey: key,
			Action:         action,
			PostID:         1,
			Content:        strings.Repeat("字", config.CommentContentMaxRunes),
		}
		if action == "reply" {
			in.ParentCommentID = 1
		}
		if err := validateForumPublication(in); err != nil {
			t.Fatalf("%s 4000 字被拒: %v", action, err)
		}
		in.Content = strings.Repeat("字", config.CommentContentMaxRunes+1)
		if err := validateForumPublication(in); !errors.Is(err, domain.ErrBadRequest) {
			t.Fatalf("%s 4001 字 err=%v, want ErrBadRequest", action, err)
		}
	}
}
