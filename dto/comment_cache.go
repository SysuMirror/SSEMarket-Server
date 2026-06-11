package dto

import "loginTest/domain"

// CommentTreeCacheDTO 评论树缓存（不含浏览者点赞/点踩状态）。
type CommentTreeCacheDTO struct {
	Pcomments []*domain.Pcomment           `json:"pcomments"`
	Subs      map[int][]*domain.Ccomment   `json:"subs"`
}
