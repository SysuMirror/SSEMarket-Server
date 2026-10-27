package controller

import (
	"context"
	"errors"
	"net/http"

	"loginTest/domain"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type forumPrincipalKey struct{}
type forumPrincipal struct {
	User   *domain.User
	Scopes []string
}

func (p forumPrincipal) has(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func forumActor(ctx context.Context) forumPrincipal {
	p, _ := ctx.Value(forumPrincipalKey{}).(forumPrincipal)
	return p
}

// NewForumMCPHandler exposes the Go business services directly. The old Python
// /api/v1/agent/mcp stays untouched; /api/auth/mcp uses the normal Go upstream.
// Protocol parsing, negotiation, schemas and transport use the official SDK.
func NewForumMCPHandler() gin.HandlerFunc {
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newForumMCPServer(forumActor(r.Context()))
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 12 << 20})
	handler := http.NewCrossOriginProtection().Handler(transport)
	return func(c *gin.Context) {
		user, ok := viewerFromAuth(c)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		scopes, _ := c.Get("forum_pat_scopes")
		list, ok := scopes.([]string)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(c.Request.Context(), forumPrincipalKey{}, forumPrincipal{User: user, Scopes: list})
		handler.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	}
}

func newForumMCPServer(p forumPrincipal) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "sse-market-forum", Version: "1.0.0"}, nil)
	if p.User == nil || p.User.ID <= 0 {
		return s
	}
	addForumTool(s, "forum_me", "Current PAT account and available forum capabilities; no private profile fields.", true, forumMe)
	if p.has("forum:read") {
		addForumTool(s, "forum_feed_v2", "Recent visible posts, optional search. Summaries are not original requests.", true, forumFeed)
		addForumTool(s, "forum_get_post", "Read a complete visible post, including public author identity and media.", true, forumGetPost)
		addForumTool(s, "forum_comments_v2", "Complete first-level comment text, ascending ID cursor. Read replies separately.", true, forumComments)
		addForumTool(s, "forum_replies_v2", "Complete replies under a first-level comment, ascending ID cursor.", true, forumReplies)
		addForumTool(s, "forum_notifications", "Own notifications, newest first; before_id paginates older items; does not mark read.", true, forumNotifications)
	}
	if p.has("forum:write") {
		addForumTool(s, "forum_publish_v2", "Create a post/comment/reply as the PAT account. Persist an idempotency key before calling; uncertain results must not be retried with a new key.", false, forumPublish)
		addForumTool(s, "forum_upload_image", "Upload bounded PNG/JPEG bytes to the existing imagebed. Generated images must already have passed the client's content review.", false, forumUploadImage)
		addForumTool(s, "forum_action_status", "Look up this account's durable operation receipt. Pending may have executed; never treat it as not executed.", true, forumActionStatus)
	}
	return s
}

// SDK supplies both structuredContent and a text JSON fallback, and validates
// the typed input schema before entering business logic.
func addForumTool[In any](s *mcp.Server, name, description string, readOnly bool,
	handler func(context.Context, In) (map[string]any, error)) {
	destructive := false
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, map[string]any, error) {
			out, err := handler(ctx, in)
			if err != nil {
				return nil, nil, forumPublicError(err)
			}
			return nil, out, nil
		})
}

func forumPublicError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return errors.New("not_found: target does not exist or is not visible")
	case errors.Is(err, domain.ErrBadRequest):
		return errors.New("invalid_arguments: check the documented field limits and target IDs")
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrUnauthorized):
		return errors.New("forbidden: operation is not permitted")
	default:
		return errors.New("service_unavailable: no automatic retry of paid or write operations")
	}
}
