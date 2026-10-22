package service

import (
	"context"
	"sort"
	"strings"

	"loginTest/domain"
	"loginTest/repository"
)

// userSearchService 搜索页用户卡片。
//
// 先由 repository 用 LIKE 粗筛候选（最多 ~60 人，站点用户量在几百级），
// 再用 domain.ScoreUserSearch 计算可解释的相关度并按阈值过滤。
// 这样既不会全表扫描，也不依赖外部向量服务。
type userSearchService struct {
	users repository.UserRepository
}

// NewUserSearchService 构造用户搜索服务。
func NewUserSearchService(users repository.UserRepository) UserSearchService {
	return &userSearchService{users: users}
}

func (s *userSearchService) Search(ctx context.Context, keyword string, limit, viewerUserID int) ([]domain.UserSearchResult, error) {
	keyword = strings.TrimSpace(keyword)
	results := make([]domain.UserSearchResult, 0)
	if keyword == "" {
		return results, nil
	}
	if limit <= 0 {
		limit = domain.UserSearchDefaultLimit
	}
	if limit > domain.UserSearchMaxLimit {
		limit = domain.UserSearchMaxLimit
	}

	terms := domain.UserSearchTerms(keyword)
	candidateLimit := limit * 8
	if candidateLimit < 24 {
		candidateLimit = 24
	}
	if candidateLimit > 60 {
		candidateLimit = 60
	}
	candidates, err := s.users.SearchCandidateUsers(ctx, keyword, terms, candidateLimit)
	if err != nil {
		return nil, err
	}

	scored := make([]domain.UserSearchResult, 0, len(candidates))
	for _, u := range candidates {
		if u == nil || u.ID <= 0 {
			continue
		}
		relevance, matchType := domain.ScoreUserSearch(keyword, u)
		if matchType == "" {
			continue
		}
		scored = append(scored, domain.UserSearchResult{
			UserID:          u.ID,
			Name:            u.Name,
			AvatarURL:       u.AvatarURL,
			Identity:        u.Identity,
			Intro:           u.Intro,
			Score:           u.Score,
			CustomTitle:     u.CustomTitle,
			ReceivedLikeNum: u.ReceivedLikeNum,
			Hidden:          u.HideProfilePosts && u.ID != viewerUserID,
			MatchType:       matchType,
			Relevance:       relevance,
			Posts:           []domain.UserSearchPostCard{},
		})
	}

	// 先排序截断，再只为最终要展示的几张卡片补帖子预览。
	// 候选集可能有几十人，若在截断前逐个查帖子/计数会变成 N+1 查询。
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Relevance != scored[j].Relevance {
			return scored[i].Relevance > scored[j].Relevance
		}
		if scored[i].ReceivedLikeNum != scored[j].ReceivedLikeNum {
			return scored[i].ReceivedLikeNum > scored[j].ReceivedLikeNum
		}
		return scored[i].UserID < scored[j].UserID
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}

	for i := range scored {
		if scored[i].Hidden {
			continue
		}
		posts, err := s.users.ListPublicPostCards(ctx, scored[i].UserID, 3)
		if err != nil {
			return nil, err
		}
		count, err := s.users.CountPublicPosts(ctx, scored[i].UserID)
		if err != nil {
			return nil, err
		}
		if posts != nil {
			scored[i].Posts = posts
		}
		scored[i].PostCount = int(count)
	}
	return scored, nil
}
