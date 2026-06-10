package service

import (
	"context"
	"errors"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/model"
	"loginTest/repository"
	"loginTest/util"
)

type postService struct {
	posts repository.PostRepository
}

func NewPostService(posts repository.PostRepository) PostService {
	return &postService{posts: posts}
}

func (s *postService) CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error) {
	return s.posts.Count(ctx, filter)
}

func (s *postService) Browse(ctx context.Context, filter domain.BrowseFilter) ([]domain.BrowseItem, error) {
	posts, err := s.posts.ListBrowse(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, nil
	}

	viewerID := filter.ViewerUserID
	postIDs := make([]int, 0, len(posts))
	userIDSet := make(map[int]struct{})
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

	savedSet, _ := s.posts.SavedSet(ctx, viewerID, postIDs)
	likedSet, _ := s.posts.LikedSet(ctx, viewerID, postIDs)
	userIDs := make([]int, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}
	authorMap, _ := s.posts.AuthorsByIDs(ctx, userIDs)

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

func (s *postService) GetDetail(ctx context.Context, postID, viewerUserID int) (*domain.PostDetail, error) {
	if postID <= 0 {
		return nil, domain.ErrBadRequest
	}
	cacheKey := util.GetPostDetailKey(postID)
	baseExpire := 30 * time.Minute

	dtoObj, err := MultiLevelGet(ctx, cacheKey, func() (*dto.PostDetailCacheDTO, error) {
		return s.posts.LoadDetailDTO(ctx, postID)
	}, baseExpire)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	postModel, userModel := dtoObj.ConvertToModel()
	post := postToDomainFromModel(postModel)
	if !canViewPost(post, viewerUserID) {
		return nil, domain.ErrNotFound
	}

	// 缓存 miss 加载后 seed Redis 浏览计数
	if core.MyRedis != nil {
		key := util.GetPostBrowseCountKey(postID)
		_ = core.MyRedis.SetNX(ctx, key, postModel.BrowseNum, 0).Err()
	}

	browseNum := int(s.IncrBrowse(ctx, postID))
	isLiked, _ := s.posts.IsLiked(ctx, viewerUserID, postID)
	isSaved, _ := s.posts.IsSaved(ctx, viewerUserID, postID)

	return &domain.PostDetail{
		Post:      post,
		Author:    userToDomainFromModel(userModel),
		IsSaved:   isSaved,
		IsLiked:   isLiked,
		BrowseNum: browseNum,
	}, nil
}

func (s *postService) FindByID(ctx context.Context, postID, viewerUserID int) (*domain.Post, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if !canViewPost(post, viewerUserID) {
		return nil, domain.ErrNotFound
	}
	return post, nil
}

func (s *postService) IncrBrowse(ctx context.Context, postID int) int64 {
	if HotData == nil {
		return 0
	}
	return HotData.IncrBrowseCount(postID)
}

func canViewPost(post *domain.Post, viewerUserID int) bool {
	if post == nil {
		return false
	}
	return !post.IsPrivate || post.UserID == viewerUserID
}

func authorForPost(m map[int]*domain.User, userID int) *domain.User {
	if userID == 0 {
		return &domain.User{Name: "管理员", Phone: "11111111111"}
	}
	if u, ok := m[userID]; ok {
		return u
	}
	return &domain.User{}
}

func postToDomainFromModel(p model.Post) *domain.Post {
	return &domain.Post{
		ID:         p.PostID,
		UserID:     p.UserID,
		Partition:  p.Partition,
		Title:      p.Title,
		Content:    p.Ptext,
		CommentNum: p.CommentNum,
		LikeNum:    p.LikeNum,
		VoteNum:    p.VoteNum,
		BrowseNum:  p.BrowseNum,
		PostTime:   p.PostTime,
		Heat:       p.Heat,
		Photos:     p.Photos,
		Tag:        p.Tag,
		IsPrivate:  p.IsPrivate,
	}
}

func userToDomainFromModel(u model.User) *domain.User {
	return &domain.User{
		ID:        u.UserID,
		Phone:     u.Phone,
		Name:      u.Name,
		Score:     u.Score,
		AvatarURL: u.AvatarURL,
		Identity:  u.Identity,
	}
}
