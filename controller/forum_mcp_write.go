package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

type forumPublishInput struct {
	IdempotencyKey  string   `json:"idempotency_key"`
	Action          string   `json:"action" jsonschema:"post, comment, or reply"`
	PostID          int      `json:"post_id,omitempty"`
	ParentCommentID int      `json:"parent_comment_id,omitempty"`
	ReplyCommentID  int      `json:"reply_comment_id,omitempty"`
	Title           string   `json:"title,omitempty"`
	Partition       string   `json:"partition,omitempty"`
	Content         string   `json:"content"`
	MediaRefs       []string `json:"media_refs,omitempty" jsonschema:"At most 4 image references uploaded by this PAT account"`
}

func forumPostURL(id int) string { return fmt.Sprintf("https://ssemarket.cn/new/postdetail/%d", id) }

func forumPublish(ctx context.Context, in forumPublishInput) (map[string]any, error) {
	p := forumActor(ctx)
	if p.User.BanEnd.After(time.Now()) {
		return nil, domain.ErrForbidden
	}
	if err := validateForumPublication(in); err != nil {
		return nil, err
	}
	// Check receipts before revalidating mutable targets/media. An already
	// confirmed reply must still be reported if its parent was later deleted.
	return forumRunAction(ctx, in.IdempotencyKey, in, func() (map[string]any, error) {
		content := strings.TrimSpace(in.Content)
		urls, err := forumMediaURLs(ctx, p.User.ID, in.MediaRefs)
		if err != nil {
			return nil, err
		}
		if in.Action != "post" {
			if _, err := service.Posts.FindByID(ctx, in.PostID, p.User.ID); err != nil {
				return nil, err
			}
			for _, url := range urls {
				content += "\n\n![图片](" + url + ")"
			}
			if utf8.RuneCountInString(content) > 1000 {
				return nil, domain.ErrBadRequest
			}
		}
		body := map[string]any{"userTelephone": "PAT", "content": content, "postID": in.PostID}
		handler := PostPcomment
		if in.Action == "post" {
			handler = Post
			body = map[string]any{"title": in.Title, "content": content, "partition": in.Partition,
				"photos": strings.Join(urls, ","), "isPrivate": false, "isAnonymous": false, "pinned": false}
		} else if in.Action == "reply" {
			pc, err := service.Comments.FindPcomment(ctx, in.ParentCommentID)
			if err != nil || pc == nil || pc.PostID != in.PostID {
				return nil, domain.ErrBadRequest
			}
			handler = PostCcomment
			body["pcommentID"] = in.ParentCommentID
			if in.ReplyCommentID > 0 {
				cc, err := service.Comments.FindCcomment(ctx, in.ReplyCommentID)
				if err != nil || cc == nil || cc.PcommentID != in.ParentCommentID {
					return nil, domain.ErrBadRequest
				}
				// Never accept target identity from model-supplied author fields.
				body["ccommentID"] = cc.ID
				name, err := forumReplyTargetName(ctx, in.PostID, in.ParentCommentID, cc.ID, p.User.ID)
				if err != nil {
					return nil, err
				}
				body["userTargetName"] = name
			}
		}
		return forumInvokeWrite(ctx, p.User, handler, body, in)
	})
}

// Use the same public identity as the comment reader; raw user IDs can reveal
// the author of an anonymous post. The reply ID still drives notifications.
func forumReplyTargetName(ctx context.Context, postID, parentID, replyID, viewerID int) (string, error) {
	tree, err := service.Comments.ListForPost(ctx, postID, "post", viewerID)
	if err != nil {
		return "", err
	}
	for _, item := range tree {
		if item == nil || item.Pcomment == nil || item.Pcomment.ID != parentID {
			continue
		}
		for _, sub := range item.Subcomments {
			if sub == nil || sub.Ccomment == nil || sub.Ccomment.ID != replyID {
				continue
			}
			return publicForumAuthor(sub.Author, sub.Author == nil || sub.Author.ID <= 0, false).Author, nil
		}
	}
	return "", domain.ErrBadRequest
}

func validateForumPublication(in forumPublishInput) error {
	if !forumActionKeyPattern.MatchString(in.IdempotencyKey) || strings.TrimSpace(in.Content) == "" || len(in.MediaRefs) > 4 {
		return domain.ErrBadRequest
	}
	if in.PostID < 0 || in.ParentCommentID < 0 || in.ReplyCommentID < 0 {
		return domain.ErrBadRequest
	}
	switch in.Action {
	case "post":
		if strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 30 || strings.TrimSpace(in.Partition) == "" || in.Partition == CoursePartition || utf8.RuneCountInString(in.Content) > config.PostContentMaxRunes || in.PostID != 0 || in.ParentCommentID != 0 || in.ReplyCommentID != 0 {
			return domain.ErrBadRequest
		}
	case "comment":
		if in.PostID <= 0 || in.ParentCommentID != 0 || in.ReplyCommentID != 0 || utf8.RuneCountInString(in.Content) > 1000 {
			return domain.ErrBadRequest
		}
	case "reply":
		if in.PostID <= 0 || in.ParentCommentID <= 0 || utf8.RuneCountInString(in.Content) > 1000 {
			return domain.ErrBadRequest
		}
	default:
		return domain.ErrBadRequest
	}
	return nil
}

// Invoke existing controllers in-process to retain lottery participation,
// notifications, email, cache invalidation and RAG hooks. There is no loopback
// HTTP request, JWT minting or direct forum database INSERT here.
func forumInvokeWrite(ctx context.Context, user *domain.User, handler gin.HandlerFunc, body any, in forumPublishInput) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/internal-forum-operation", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", user)
	handler(c)
	var ack struct {
		Code       int `json:"code"`
		PcommentID int `json:"PcommentID"`
		Data       struct {
			PostID     int `json:"postID"`
			CcommentID int `json:"ccommentID"`
		} `json:"data"`
	}
	if recorder.Code < 200 || recorder.Code >= 300 || json.Unmarshal(recorder.Body.Bytes(), &ack) != nil {
		return nil, domain.ErrInternal
	}
	if ack.Code != 0 && ack.Code != 200 {
		return nil, domain.ErrInternal
	}
	postID, commentID, replyID := in.PostID, ack.PcommentID, ack.Data.CcommentID
	if in.Action == "post" {
		postID = ack.Data.PostID
	}
	if (in.Action == "post" && postID <= 0) || (in.Action == "comment" && commentID <= 0) || (in.Action == "reply" && replyID <= 0) {
		return nil, domain.ErrInternal
	}
	return map[string]any{"success": true, "status": "confirmed", "action": in.Action,
		"post_id": postID, "comment_id": commentID, "reply_id": replyID, "parent_comment_id": in.ParentCommentID, "url": forumPostURL(postID)}, nil
}

func forumMediaURLs(ctx context.Context, uid int, refs []string) ([]string, error) {
	urls := make([]string, 0, len(refs))
	for _, ref := range refs {
		if !forumMediaRefPattern.MatchString(ref) {
			return nil, domain.ErrBadRequest
		}
		if common.GetDB() == nil {
			return nil, domain.ErrInternal
		}
		var media model.ForumAgentMedia
		if err := common.GetDB().WithContext(ctx).Where("media_ref = ? AND user_id = ?", ref, uid).First(&media).Error; err != nil {
			return nil, domain.ErrForbidden
		}
		if !validForumImageURL(media.URL) {
			return nil, domain.ErrBadRequest
		}
		urls = append(urls, media.URL)
	}
	return urls, nil
}
