package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mcpUsers struct {
	service.UserService
	user *domain.User
}

func (s mcpUsers) GetByID(context.Context, int) (*domain.User, error) { return s.user, nil }

type mcpPosts struct {
	service.PostService
	post *domain.Post
}

func (s mcpPosts) FindByID(_ context.Context, id, viewer int) (*domain.Post, error) {
	if s.post == nil || s.post.ID != id || (s.post.IsPrivate && s.post.UserID != viewer) {
		return nil, domain.ErrNotFound
	}
	return s.post, nil
}

type mcpComments struct {
	service.CommentService
	tree []*domain.PcommentWithMeta
}

func (s mcpComments) ListForPost(context.Context, int, string, int) ([]*domain.PcommentWithMeta, error) {
	return s.tree, nil
}

func forumTestRouter(uid int, scopes ...string) *gin.Engine {
	r := gin.New()
	h := NewForumMCPHandler()
	r.Any("/api/auth/mcp", func(c *gin.Context) {
		c.Set("user", &domain.User{ID: uid, Name: "bot"})
		c.Set("forum_pat_scopes", scopes)
		h(c)
	})
	return r
}
func forumTestRPC(t *testing.T, h http.Handler, method string, params any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	r := httptest.NewRequest(http.MethodPost, "/api/auth/mcp", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-06-18")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func forumTestCall(t *testing.T, h http.Handler, name string, args any) map[string]any {
	t.Helper()
	out := forumTestRPC(t, h, "tools/call", map[string]any{"name": name, "arguments": args})
	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %#v", out)
	}
	return result
}

func TestForumMCPScopesAndSchemas(t *testing.T) {
	for _, tc := range []struct {
		scope       string
		write, read bool
	}{{"agent:chat", false, false}, {"forum:read", false, true}, {"forum:write", true, false}} {
		t.Run(tc.scope, func(t *testing.T) {
			r := forumTestRouter(7, tc.scope)
			out := forumTestRPC(t, r, "tools/list", map[string]any{})
			tools := out["result"].(map[string]any)["tools"].([]any)
			seen := map[string]bool{}
			for _, item := range tools {
				tool := item.(map[string]any)
				seen[tool["name"].(string)] = true
				if tool["outputSchema"] == nil {
					t.Fatal("missing output schema")
				}
			}
			if !seen["forum_me"] || seen["forum_publish_v2"] != tc.write || seen["forum_get_post"] != tc.read {
				t.Fatalf("unexpected tools: %v", seen)
			}
			if !tc.write {
				out := forumTestRPC(t, r, "tools/call", map[string]any{"name": "forum_publish_v2", "arguments": map[string]any{}})
				if out["error"] == nil {
					t.Fatal("write tool callable without scope")
				}
			}
		})
	}
}

func TestForumMCPRequestIdentityAndNoPrivateProfile(t *testing.T) {
	h := NewForumMCPHandler()
	r := gin.New()
	r.POST("/api/auth/mcp", func(c *gin.Context) {
		uid := 7
		if c.GetHeader("X-Test-Actor") == "other" {
			uid = 8
		}
		c.Set("user", &domain.User{ID: uid, Name: "bot", Phone: "private-phone", Email: "private-email"})
		c.Set("forum_pat_scopes", []string{"forum:read"})
		h(c)
	})
	for _, uid := range []int{7, 8} {
		b := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"forum_me","arguments":{}}}`)
		req := httptest.NewRequest("POST", "/api/auth/mcp", b)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		if uid == 8 {
			req.Header.Set("X-Test-Actor", "other")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var data struct {
			Result struct {
				StructuredContent struct {
					UserID int `json:"user_id"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if data.Result.StructuredContent.UserID != uid || strings.Contains(w.Body.String(), "private-") {
			t.Fatalf("identity/private data: %s", w.Body.String())
		}
	}
}

func TestForumMCPCompletePostAndAnonymousIdentity(t *testing.T) {
	oldP, oldU := service.Posts, service.Users
	t.Cleanup(func() { service.Posts, service.Users = oldP, oldU })
	text := strings.Repeat("完整正文\n", 300)
	post := &domain.Post{ID: 22, UserID: 99, Content: text, IsAnonymous: true}
	service.Posts = mcpPosts{post: post}
	service.Users = mcpUsers{user: &domain.User{ID: 99, Name: "hidden", Phone: "secret"}}
	r := forumTestRouter(7, "forum:read")
	out := forumTestCall(t, r, "forum_get_post", map[string]any{"post_id": 22})
	p := out["structuredContent"].(map[string]any)["post"].(map[string]any)
	if p["content"] != text || p["content_complete"] != true || p["author_id"] != nil || p["author"] != "匿名用户" {
		t.Fatalf("bad post DTO: %#v", p)
	}
	post.IsPrivate = true
	out = forumTestCall(t, r, "forum_get_post", map[string]any{"post_id": 22})
	if out["isError"] != true {
		t.Fatal("private post exposed")
	}
}

func TestForumMCPRejectsSpoofedInput(t *testing.T) {
	r := forumTestRouter(7, "forum:read")
	out := forumTestCall(t, r, "forum_get_post", map[string]any{"post_id": 1, "user_id": 99})
	if out["isError"] != true {
		t.Fatal("unknown identity field accepted")
	}
}

func TestForumMCPCommentCursorAndAnonReplies(t *testing.T) {
	old := service.Comments
	t.Cleanup(func() { service.Comments = old })
	long := strings.Repeat("评论", 150)
	service.Comments = mcpComments{tree: []*domain.PcommentWithMeta{
		{Pcomment: &domain.Pcomment{ID: 30, UserID: 99, Content: long}, Author: &domain.User{Name: "匿名用户"}, Subcomments: []*domain.CcommentWithMeta{
			{Ccomment: &domain.Ccomment{ID: 40, UserID: 99, Content: long}, Author: &domain.User{Name: "匿名用户"}},
			{Ccomment: &domain.Ccomment{ID: 41, UserID: 8, Content: "second"}, Author: &domain.User{ID: 8, Name: "public"}},
		}},
		{Pcomment: &domain.Pcomment{ID: 10, UserID: 8, Content: "first"}, Author: &domain.User{ID: 8, Name: "public"}},
	}}
	r := forumTestRouter(7, "forum:read")
	out := forumTestCall(t, r, "forum_comments_v2", map[string]any{"post_id": 22, "limit": 1})["structuredContent"].(map[string]any)
	if out["has_more"] != true || out["next_after_id"] != float64(10) {
		t.Fatal(out)
	}
	out = forumTestCall(t, r, "forum_replies_v2", map[string]any{"post_id": 22, "parent_comment_id": 30, "after_id": 0, "limit": 1})["structuredContent"].(map[string]any)
	item := out["replies"].([]any)[0].(map[string]any)
	if item["author_id"] != nil || item["content"] != long || out["next_after_id"] != float64(40) || out["has_more"] != true {
		t.Fatal(out)
	}
}

type memoryForumActions struct {
	mu         sync.Mutex
	rows       map[string]*model.ForumAgentAction
	failFinish bool
}

func (s *memoryForumActions) Reserve(_ context.Context, uid int, key, hash string) (*model.ForumAgentAction, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := forumPostURL(uid) + key
	if row := s.rows[k]; row != nil {
		copy := *row
		return &copy, false, nil
	}
	row := &model.ForumAgentAction{UserID: uid, ActionKey: key, PayloadHash: hash}
	s.rows[k] = row
	copy := *row
	return &copy, true, nil
}
func (s *memoryForumActions) Get(_ context.Context, uid int, key string) (*model.ForumAgentAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.rows[forumPostURL(uid)+key]
	if row == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *row
	return &copy, nil
}
func (s *memoryForumActions) Finish(_ context.Context, uid int, key, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failFinish {
		return errors.New("storage unavailable")
	}
	s.rows[forumPostURL(uid)+key].Result = result
	return nil
}
func forumTestActions(t *testing.T) (context.Context, *memoryForumActions) {
	t.Helper()
	old := forumActions
	store := &memoryForumActions{rows: map[string]*model.ForumAgentAction{}}
	forumActions = store
	t.Cleanup(func() { forumActions = old })
	return context.WithValue(context.Background(), forumPrincipalKey{}, forumPrincipal{User: &domain.User{ID: 7}}), store
}
func TestForumMCPDurableAttemptAndConflict(t *testing.T) {
	ctx, _ := forumTestActions(t)
	key := "request-1234567890"
	var calls atomic.Int32
	run := func() (map[string]any, error) {
		calls.Add(1)
		return map[string]any{"success": true, "status": "confirmed", "comment_id": 45}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := forumRunAction(ctx, key, "same", run); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("executed %d times", calls.Load())
	}
	out, err := forumRunAction(ctx, key, "different", run)
	if err != nil || out["status"] != "conflict" {
		t.Fatal(out, err)
	}
	other := context.WithValue(ctx, forumPrincipalKey{}, forumPrincipal{User: &domain.User{ID: 8}})
	out, err = forumActionStatus(other, forumActionInput{IdempotencyKey: key})
	if err != nil || out["status"] != "not_found" {
		t.Fatal("cross-account receipt exposed", out, err)
	}
}
func TestForumMCPUncertainNeverReexecutes(t *testing.T) {
	ctx, store := forumTestActions(t)
	store.failFinish = true
	calls := 0
	run := func() (map[string]any, error) {
		calls++
		return map[string]any{"success": true, "status": "confirmed", "comment_id": 45}, nil
	}
	out, err := forumRunAction(ctx, "uncertain-12345678", "same", run)
	if err != nil || out["status"] != "uncertain" {
		t.Fatal(out, err)
	}
	store.failFinish = false
	out, err = forumRunAction(ctx, "uncertain-12345678", "same", run)
	if err != nil || out["status"] != "pending" || calls != 1 {
		t.Fatal("unsafe replay", out, calls, err)
	}
}

func TestForumMCPWriteAcknowledgesOnlyReturnedIDs(t *testing.T) {
	in := forumPublishInput{Action: "comment", PostID: 22}
	for _, tc := range []struct {
		body  string
		valid bool
	}{{`{"PcommentID":45}`, true}, {`{"code":200,"msg":"ok"}`, false}, {`{"code":500,"msg":"error"}`, false}, {`{"code":500,"PcommentID":45}`, false}} {
		out, err := forumInvokeWrite(context.Background(), &domain.User{ID: 7}, func(c *gin.Context) { c.Data(200, "application/json", []byte(tc.body)) }, nil, in)
		if (err == nil) != tc.valid {
			t.Fatal(out, err)
		}
	}
}

func TestForumMCPImageValidation(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	raw := b.Bytes()
	hash := sha256.Sum256(raw)
	in := forumImageInput{MIME: "image/png", SHA256: hex.EncodeToString(hash[:]), DataBase64: base64.StdEncoding.EncodeToString(raw)}
	if _, err := decodeForumImage(in); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*forumImageInput){func(i *forumImageInput) { i.MIME = "image/svg+xml" }, func(i *forumImageInput) { i.MIME = "image/jpeg" }, func(i *forumImageInput) { i.SHA256 = strings.Repeat("0", 64) }, func(i *forumImageInput) { i.DataBase64 = "not-base64" }} {
		bad := in
		change(&bad)
		if _, err := decodeForumImage(bad); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	for _, raw := range []string{"https://evil.invalid/a.png", "https://image.ssemarket.cn.evil.invalid/a.png", "https://user:pass@image.ssemarket.cn/a.png", "https://image.ssemarket.cn/a.png)"} {
		if validForumImageURL(raw) {
			t.Fatalf("unsafe URL %q", raw)
		}
	}
	if !validForumImageURL("https://image.ssemarket.cn/a.png") {
		t.Fatal("expected public image URL")
	}
}
