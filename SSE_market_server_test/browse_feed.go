package service

import (
	"context"
	"sort"

	"loginTest/cache"
	"loginTest/domain"
	"loginTest/util"
)

const publicFeedMaxOffset = 150 // 最多缓存 offset 0~150（约 10 页 ×15）

// isPublicFeedCacheable 是否可走公共 Feed 缓存（与用户无关的列表段）。
func isPublicFeedCacheable(filter domain.BrowseFilter) bool {
	switch filter.Searchsort {
	case "save", "history":
		return false
	}
	if filter.Searchinfo != "" || filter.Tag != "" || filter.TimeFrom != "" || filter.TimeTo != "" {
		return false
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 15
	}
	if limit > 15 || filter.Offset < 0 || filter.Offset > publicFeedMaxOffset {
		return false
	}
	return true
}

func feedKeyInput(filter domain.BrowseFilter) util.BrowseFeedKeyInput {
	limit := filter.Limit
	if limit <= 0 {
		limit = 15
	}
	return util.BrowseFeedKeyInput{
		Searchsort: filter.Searchsort,
		Partition:  filter.Partition,
		OrderBy:    filter.OrderBy,
		Offset:     filter.Offset,
		Limit:      limit,
	}
}

// fetchPublicFeed 公共帖子列表：命中 L1/L2 则不打 MySQL；仅含公开帖（is_private=0）。
func (s *postService) fetchPublicFeed(ctx context.Context, filter domain.BrowseFilter) ([]*domain.Post, error) {
	if filter.Searchsort == "save" || filter.Searchsort == "history" {
		return s.posts.ListBrowse(ctx, filter)
	}
	if !isPublicFeedCacheable(filter) {
		return s.posts.ListBrowse(ctx, filter)
	}

	pubFilter := filter
	pubFilter.ViewerUserID = 0
	gen := cache.PublicFeedGeneration(ctx)
	key := util.GetPublicFeedCacheKey(feedKeyInput(filter), gen)

	posts, err := MultiLevelGet(ctx, key, func() ([]*domain.Post, error) {
		return s.posts.ListBrowse(ctx, pubFilter)
	}, cache.ProfilePublicFeed)
	if err != nil {
		return nil, err
	}
	return s.mergeViewerPrivatePosts(ctx, filter, posts)
}

// mergeViewerPrivatePosts 首页第一屏合并当前用户私密帖（公共缓存不含 viewer 私密帖）。
func (s *postService) mergeViewerPrivatePosts(ctx context.Context, filter domain.BrowseFilter, public []*domain.Post) ([]*domain.Post, error) {
	if filter.ViewerUserID <= 0 || filter.Offset != 0 {
		return public, nil
	}
	sortMode := filter.Searchsort
	if sortMode == "" {
		sortMode = "home"
	}
	if sortMode != "home" {
		return public, nil
	}
	if filter.Searchinfo != "" {
		return public, nil
	}
	if filter.Partition != "" && filter.Partition != "主页" {
		return public, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 15
	}
	private, err := s.posts.ListViewerPrivatePostsRecent(ctx, filter.ViewerUserID, limit)
	if err != nil || len(private) == 0 {
		return public, err
	}
	return mergePostsByRecency(public, private, limit), nil
}

func mergePostsByRecency(public, extra []*domain.Post, limit int) []*domain.Post {
	merged := make([]*domain.Post, 0, len(public)+len(extra))
	merged = append(merged, public...)
	merged = append(merged, extra...)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].PostTime.After(merged[j].PostTime)
	})
	seen := make(map[int]struct{}, len(merged))
	out := make([]*domain.Post, 0, limit)
	for _, p := range merged {
		if p == nil {
			continue
		}
		if _, dup := seen[p.ID]; dup {
			continue
		}
		seen[p.ID] = struct{}{}
		out = append(out, p)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// attachPersonalBrowseState 个人段：点赞/收藏/作者（不可公共缓存），组装 BrowseItem。
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
