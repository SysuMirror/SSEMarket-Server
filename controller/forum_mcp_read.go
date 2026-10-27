package controller

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/service"
)

type forumEmpty struct{}
type forumPostInput struct {
	PostID int `json:"post_id" jsonschema:"Positive post ID"`
}
type forumFeedInput struct {
	Limit     int    `json:"limit,omitempty" jsonschema:"Page size 1 to 30; default 15"`
	Offset    int    `json:"offset,omitempty" jsonschema:"Nonnegative feed offset, at most 10000"`
	Query     string `json:"query,omitempty" jsonschema:"Optional text search, at most 200 characters"`
	Partition string `json:"partition,omitempty" jsonschema:"Optional forum partition"`
}
type forumCommentsInput struct {
	PostID  int `json:"post_id"`
	AfterID int `json:"after_id,omitempty" jsonschema:"Return first-level comments with IDs greater than this cursor"`
	Limit   int `json:"limit,omitempty" jsonschema:"1 to 50; default 20"`
}
type forumRepliesInput struct {
	PostID          int `json:"post_id"`
	ParentCommentID int `json:"parent_comment_id"`
	AfterID         int `json:"after_id,omitempty"`
	Limit           int `json:"limit,omitempty"`
}
type forumNoticesInput struct {
	BeforeID int  `json:"before_id,omitempty" jsonschema:"Older-than notification ID; zero starts at newest"`
	Limit    int  `json:"limit,omitempty" jsonschema:"1 to 50; default 20"`
	Read     bool `json:"read,omitempty" jsonschema:"false lists unread, true lists read; this call never marks notices read"`
}

// Whitelist DTOs: domain.User and legacy controller responses contain private
// data and must never be serialized directly by an agent tool.
type forumAuthor struct {
	AuthorID    *int   `json:"author_id"`
	Author      string `json:"author"`
	IsAnonymous bool   `json:"is_anonymous"`
	IsSelf      bool   `json:"is_self"`
}
type forumPost struct {
	forumAuthor
	PostID          int       `json:"post_id"`
	Title           string    `json:"title"`
	Content         string    `json:"content"`
	Partition       string    `json:"partition"`
	Photos          string    `json:"photos"`
	Time            time.Time `json:"time"`
	ContentComplete bool      `json:"content_complete"`
	URL             string    `json:"url"`
}
type forumComment struct {
	forumAuthor
	CommentID       int       `json:"comment_id"`
	PostID          int       `json:"post_id"`
	ParentCommentID int       `json:"parent_comment_id,omitempty"`
	Content         string    `json:"content"`
	Time            time.Time `json:"time"`
	ContentComplete bool      `json:"content_complete"`
	ReplyCount      int       `json:"reply_count"`
}

func publicForumAuthor(user *domain.User, anonymous, self bool) forumAuthor {
	a := forumAuthor{IsAnonymous: anonymous, IsSelf: self}
	if anonymous {
		a.Author = "匿名用户"
	} else if user != nil && user.ID > 0 {
		id := user.ID
		a.AuthorID, a.Author = &id, user.Name
	}
	return a
}

func publicForumPost(post *domain.Post, user *domain.User, viewerID int, complete bool) forumPost {
	content := post.Content
	if !complete {
		r := []rune(content)
		if len(r) > 240 {
			content = string(r[:240])
		}
	}
	return forumPost{forumAuthor: publicForumAuthor(user, post.IsAnonymous, post.UserID == viewerID),
		PostID: post.ID, Title: post.Title, Content: content, Partition: post.Partition,
		Photos: post.Photos, Time: post.PostTime, ContentComplete: complete, URL: forumPostURL(post.ID)}
}

func forumPageLimit(n, fallback, max int) (int, error) {
	if n == 0 {
		return fallback, nil
	}
	if n < 1 || n > max {
		return 0, domain.ErrBadRequest
	}
	return n, nil
}

func forumMe(ctx context.Context, _ forumEmpty) (map[string]any, error) {
	p := forumActor(ctx)
	return map[string]any{"user_id": p.User.ID, "name": p.User.Name, "scopes": p.Scopes,
		"identity_verified": true, "transport": "native-go-mcp", "max_requests_per_minute": 30,
		"max_image_bytes": forumMaxImageBytes, "supports_child_replies": true,
		"write_statuses": []string{"pending", "confirmed", "uncertain"},
		"image_policy":   "Generated images require client-side request/prompt/output moderation and per-requester seven-day quota; server uploads do not perform generation or moderation."}, nil
}

func forumFeed(ctx context.Context, in forumFeedInput) (map[string]any, error) {
	limit, err := forumPageLimit(in.Limit, 15, 30)
	if err != nil || in.Offset < 0 || in.Offset > 10000 || len([]rune(in.Query)) > 200 || len([]rune(in.Partition)) > 50 {
		return nil, domain.ErrBadRequest
	}
	p := forumActor(ctx)
	// search bypasses homepage pinned merging, preserving simple pagination.
	page, err := service.Posts.Browse(ctx, domain.BrowseFilter{ViewerUserID: p.User.ID,
		Searchsort: "search", OrderBy: "time_desc", Limit: limit + 1, Offset: in.Offset,
		Searchinfo: strings.TrimSpace(in.Query), Partition: in.Partition})
	if err != nil {
		return nil, err
	}
	hasMore := len(page.Posts) > limit
	if hasMore {
		page.Posts = page.Posts[:limit]
	}
	posts := make([]forumPost, 0, len(page.Posts))
	for _, item := range page.Posts {
		if item.Post != nil {
			posts = append(posts, publicForumPost(item.Post, item.Author, p.User.ID, false))
		}
	}
	return map[string]any{"posts": posts, "has_more": hasMore, "next_offset": in.Offset + len(page.Posts)}, nil
}

func forumGetPost(ctx context.Context, in forumPostInput) (map[string]any, error) {
	if in.PostID <= 0 {
		return nil, domain.ErrBadRequest
	}
	p := forumActor(ctx)
	// FindByID enforces visibility without inflating browse/heat counters on
	// every autonomous wake-up.
	post, err := service.Posts.FindByID(ctx, in.PostID, p.User.ID)
	if err != nil {
		return nil, err
	}
	var author *domain.User
	if !post.IsAnonymous {
		author, err = service.Users.GetByID(ctx, post.UserID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}
	return map[string]any{"post": publicForumPost(post, author, p.User.ID, true)}, nil
}

func forumComments(ctx context.Context, in forumCommentsInput) (map[string]any, error) {
	limit, err := forumPageLimit(in.Limit, 20, 50)
	if err != nil || in.PostID <= 0 || in.AfterID < 0 {
		return nil, domain.ErrBadRequest
	}
	p := forumActor(ctx)
	tree, err := service.Comments.ListForPost(ctx, in.PostID, "post", p.User.ID)
	if err != nil {
		return nil, err
	}
	items := make([]forumComment, 0)
	for _, item := range tree {
		if item == nil || item.Pcomment == nil || item.Pcomment.ID <= in.AfterID {
			continue
		}
		pc := item.Pcomment
		// ListForPost masks an anonymous post author's Author; never fall back
		// to raw Pcomment.UserID when producing the public author ID.
		anon := item.Author == nil || item.Author.ID <= 0
		items = append(items, forumComment{forumAuthor: publicForumAuthor(item.Author, anon, pc.UserID == p.User.ID),
			CommentID: pc.ID, PostID: in.PostID, Content: pc.Content, Time: pc.CreatedAt,
			ContentComplete: true, ReplyCount: len(item.Subcomments)})
	}
	return forumCommentPage(items, limit, in.AfterID, "comments"), nil
}

func forumReplies(ctx context.Context, in forumRepliesInput) (map[string]any, error) {
	limit, err := forumPageLimit(in.Limit, 20, 50)
	if err != nil || in.PostID <= 0 || in.ParentCommentID <= 0 || in.AfterID < 0 {
		return nil, domain.ErrBadRequest
	}
	p := forumActor(ctx)
	tree, err := service.Comments.ListForPost(ctx, in.PostID, "post", p.User.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range tree {
		if item == nil || item.Pcomment == nil || item.Pcomment.ID != in.ParentCommentID {
			continue
		}
		items := make([]forumComment, 0)
		for _, sub := range item.Subcomments {
			if sub == nil || sub.Ccomment == nil || sub.Ccomment.ID <= in.AfterID {
				continue
			}
			cc := sub.Ccomment
			anon := sub.Author == nil || sub.Author.ID <= 0
			items = append(items, forumComment{forumAuthor: publicForumAuthor(sub.Author, anon, cc.UserID == p.User.ID),
				CommentID: cc.ID, PostID: in.PostID, ParentCommentID: in.ParentCommentID,
				Content: cc.Content, Time: cc.CreatedAt, ContentComplete: true})
		}
		return forumCommentPage(items, limit, in.AfterID, "replies"), nil
	}
	return nil, domain.ErrNotFound
}

func forumCommentPage(items []forumComment, limit, after int, key string) map[string]any {
	sort.Slice(items, func(i, j int) bool { return items[i].CommentID < items[j].CommentID })
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	if len(items) > 0 {
		after = items[len(items)-1].CommentID
	}
	return map[string]any{key: items, "has_more": more, "next_after_id": after}
}

func forumNotifications(ctx context.Context, in forumNoticesInput) (map[string]any, error) {
	limit, err := forumPageLimit(in.Limit, 20, 50)
	if err != nil || in.BeforeID < 0 {
		return nil, domain.ErrBadRequest
	}
	p := forumActor(ctx)
	read := 0
	if in.Read {
		read = 1
	}
	page, err := service.Notices.List(ctx, p.User.ID, read, in.BeforeID, limit+1, p.User.Name)
	if err != nil {
		return nil, err
	}
	more := len(page.NoticeList) > limit
	if more {
		page.NoticeList = page.NoticeList[:limit]
	}
	items := make([]map[string]any, 0)
	next := in.BeforeID
	for _, n := range page.NoticeList {
		next = n.NoticeID // advance even when a deleted/private target is skipped
		if n.PostID <= 0 {
			continue
		} // only forum-related notifications
		if _, err := service.Posts.FindByID(ctx, n.PostID, p.User.ID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return nil, err
		}
		var sender *int
		if n.SenderID > 0 {
			id := n.SenderID
			sender = &id
		}
		items = append(items, map[string]any{"notification_id": n.NoticeID, "sender_id": sender,
			"sender": n.SenderName, "type": n.Type, "content": n.Content, "read": n.Read,
			"post_id": n.PostID, "parent_comment_id": n.PcommentID, "target_id": n.Target, "time": n.Time})
	}
	return map[string]any{"notifications": items, "has_more": more, "next_before_id": next}, nil
}
