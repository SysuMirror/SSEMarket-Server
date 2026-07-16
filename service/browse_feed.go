// browse_feed.go Browse 列表查询。
//
// 当前策略（线上稳定优先）：
//   - 列表段 fetchPublicFeed：直查 MySQL ListBrowse，不走公共 Feed L1/L2 缓存。
//     SQL 含 applyVisibility(viewerID)，公开帖 + 本人私密帖一次查齐，分页正确。
//   - 个人段 attachPersonalBrowseState：实时查 IsLiked / IsSaved / 作者并组装 BrowseItem。
//
// 公共 Feed 缓存相关代码已暂时停用，待失效策略与分页验证完善后再启用。
package service

import (
	"context"

	"loginTest/domain"
)

// fetchPublicFeed 帖子列表段（Browse 第一阶段）：直查 DB，不使用缓存。
func (s *postService) fetchPublicFeed(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, error) {
	return s.posts.ListBrowse(ctx, filter)
}

// attachPersonalBrowseState 个人态段（Browse 第二阶段）。
//
// 对列表结果：
//   - canViewPost 二次过滤（私密帖仅作者可见）；
//   - fetchBrowseInteractionSets 并行查 SavedSet / LikedSet / AuthorsByIDs；
//   - 组装 BrowseItem{Post, Author, IsSaved, IsLiked} 供 controller 序列化。
func (s *postService) attachPersonalBrowseState(ctx context.Context, filter domain.BrowseFilter, posts []*domain.Post) ([]domain.BrowseItem, error) {
	if len(posts) == 0 {
		return nil, nil
	}
	viewerID := filter.ViewerUserID
	postIDs := make([]int, 0, len(posts))
	userIDSet := make(map[int]struct{}, len(posts))
	visible := make([]*domain.Post, 0, len(posts))
	for _, p := range posts {
		if !canViewPost(p, viewerID) {
			continue
		}
		visible = append(visible, p)
		postIDs = append(postIDs, p.ID)
		if p.UserID != 0 {
			userIDSet[p.UserID] = struct{}{}
		}
	}
	if len(visible) == 0 {
		return nil, nil
	}
	userIDs := make([]int, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}
	savedSet, likedSet, authorMap := s.fetchBrowseInteractionSets(ctx, viewerID, postIDs, userIDs)

	isSaveMode := filter.Searchsort == "save"
	items := make([]domain.BrowseItem, 0, len(visible))
	for _, p := range visible {
		author := authorForPost(authorMap, p.UserID)
		_, liked := likedSet[p.ID]
		_, saved := savedSet[p.ID]
		if isSaveMode {
			saved = true
		}
		items = append(items, domain.BrowseItem{
			Post:    p,
			Author:  author,
			IsSaved: saved,
			IsLiked: liked,
		})
	}
	return items, nil
}
