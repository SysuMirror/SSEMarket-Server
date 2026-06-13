package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"loginTest/cache"
	"loginTest/common"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/model"
	"loginTest/repository"
	"loginTest/util"
)

const (
	coursePartition = "课程交流"
	hotPostsZSetKey = "hot_posts_zset"
	weightLike      = float64(3)
	weightBrowse    = float64(1)
)

type postService struct {
	posts repository.PostRepository
	users repository.UserRepository
	sues  repository.SueRepository
	ranks RankService
}

func NewPostService(
	posts repository.PostRepository,
	users repository.UserRepository,
	sues repository.SueRepository,
	ranks RankService,
) PostService {
	return &postService{posts: posts, users: users, sues: sues, ranks: ranks}
}

func (s *postService) CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error) {
	return s.posts.Count(ctx, filter)
}

func (s *postService) Browse(ctx context.Context, filter domain.BrowseFilter) ([]domain.BrowseItem, error) {
	posts, err := s.fetchPublicFeed(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.attachPersonalBrowseState(ctx, filter, posts)
}

func (s *postService) GetDetail(ctx context.Context, postID, viewerUserID int) (*domain.PostDetail, error) {
	if postID <= 0 {
		return nil, domain.ErrBadRequest
	}
	cacheKey := util.GetPostDetailKey(postID)

	dtoObj, err := MultiLevelGet(ctx, cacheKey, func() (*dto.PostDetailCacheDTO, error) {
		return s.posts.LoadDetailDTO(ctx, postID)
	}, cache.ProfilePostDetail)
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

func (s *postService) Create(ctx context.Context, in domain.PostCreateInput) error {
	user, err := s.users.FindByID(ctx, in.UserID)
	if err != nil {
		return err
	}
	if user.BanEnd.After(time.Now()) {
		return domain.ErrForbidden
	}
	if in.Partition == coursePartition {
		return domain.ErrForbidden
	}
	tagString := strings.Join(in.Tags, ",")
	post := &domain.Post{
		UserID:     in.UserID,
		Partition:  in.Partition,
		Title:      in.Title,
		Content:    in.Content,
		LikeNum:    0,
		CommentNum: 0,
		BrowseNum:  0,
		Heat:       0,
		PostTime:   time.Now(),
		Photos:     in.Photos,
		Tag:        tagString,
		IsPrivate:  in.IsPrivate,
	}
	if err := s.posts.Create(ctx, post); err != nil {
		return err
	}
	common.AddPostID(post.ID)
	InvalidateHomeBrowse()

	if len(in.Tags) > 0 {
		tag, err := s.posts.FindTagByName(ctx, in.Tags[0])
		if err == nil && tag.Type == "course" {
			receiver, err := s.users.FindByEmail(ctx, tag.Value)
			if err == nil && receiver.ID != 0 && receiver.Phone != user.Phone {
				_ = s.posts.CreatePostNotice(ctx, receiver.ID, user.ID, "post", in.Title, post.ID)
			}
		}
	}
	return nil
}

func (s *postService) UserProfilePosts(ctx context.Context, filter domain.UserProfilePostsFilter) ([]domain.BrowseItem, error) {
	target, err := s.users.FindByID(ctx, filter.TargetUserID)
	if err != nil {
		return nil, nil
	}
	if target.HideProfilePosts && filter.ViewerUserID != target.ID {
		return nil, nil
	}
	posts, err := s.posts.ListUserProfile(ctx, filter.TargetUserID, filter.ViewerUserID, filter.Limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	if len(posts) == 0 {
		return nil, nil
	}
	return s.buildBrowseItems(ctx, filter.ViewerUserID, posts, false)
}

func (s *postService) UserProfilePostNum(ctx context.Context, targetUserID, viewerUserID int) (int64, error) {
	target, err := s.users.FindByID(ctx, targetUserID)
	if err != nil {
		return 0, nil
	}
	if target.HideProfilePosts && viewerUserID != target.ID {
		return 0, nil
	}
	return s.posts.CountUserProfile(ctx, targetUserID, viewerUserID)
}

func (s *postService) UpdatePrivacy(ctx context.Context, postID, userID int, isPrivate bool) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	if err := s.posts.UpdatePrivacy(ctx, postID, isPrivate); err != nil {
		return err
	}
	InvalidatePostDetail(postID)
	InvalidateHomeBrowse()
	return nil
}

func (s *postService) ToggleSave(ctx context.Context, userID, postID int) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if !canViewPost(post, userID) {
		return domain.ErrNotFound
	}
	isSaved, err := s.posts.IsSaved(ctx, userID, postID)
	if err != nil {
		return err
	}
	if isSaved {
		return s.posts.DeleteSave(ctx, userID, postID)
	}
	return s.posts.CreateSave(ctx, userID, postID)
}

func (s *postService) ToggleLike(ctx context.Context, userID, postID int) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if !canViewPost(post, userID) {
		return domain.ErrNotFound
	}
	isLiked, err := s.posts.IsLiked(ctx, userID, postID)
	if err != nil {
		return err
	}
	if isLiked {
		if err := s.posts.UpdateLikeNum(ctx, postID, -1); err != nil {
			return err
		}
		if err := s.posts.DeleteLike(ctx, userID, postID); err != nil {
			return err
		}
		s.ranks.AdjustUserReceivedLike(ctx, post.UserID, -1)
	} else {
		if err := s.posts.UpdateLikeNum(ctx, postID, 1); err != nil {
			return err
		}
		if err := s.posts.UpdateHeat(ctx, postID, weightLike); err != nil {
			return err
		}
		if err := s.posts.CreateLike(ctx, userID, postID); err != nil {
			return err
		}
		s.ranks.AdjustUserReceivedLike(ctx, post.UserID, 1)
	}
	InvalidatePostDetail(postID)
	return nil
}

func (s *postService) RecordBrowse(ctx context.Context, userID, postID int) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if !canViewPost(post, userID) {
		return nil
	}
	browsed, err := s.posts.HasBrowsed(ctx, userID, postID)
	if err != nil {
		return err
	}
	if browsed {
		return nil
	}
	if err := s.posts.UpdateHeat(ctx, postID, weightBrowse); err != nil {
		return err
	}
	return s.posts.CreateBrowse(ctx, userID, postID)
}

func (s *postService) Delete(ctx context.Context, postID, userID int) error {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	s.ranks.OnPostDeleted(ctx, post)
	if err := s.posts.Delete(ctx, postID); err != nil {
		return err
	}
	InvalidatePostDetail(postID)
	InvalidateHomeBrowse()
	return nil
}

func (s *postService) SubmitReport(ctx context.Context, in domain.SubmitReportInput) error {
	if strings.TrimSpace(in.Reason) == "" {
		return domain.ErrBadRequest
	}
	if in.TargetType == "post" {
		if _, err := s.FindByID(ctx, in.TargetID, in.UserID); err != nil {
			return err
		}
	}
	sue := &domain.Sue{
		TargetType: in.TargetType,
		TargetID:   in.TargetID,
		UserID:     in.UserID,
		Reason:     in.Reason,
		Time:       time.Now(),
		Status:     "wait",
		Finish:     false,
	}
	return s.sues.Create(ctx, sue)
}

func (s *postService) SubmitFeedback(ctx context.Context, text, attachment string) (*domain.Feedback, error) {
	return s.posts.CreateFeedback(ctx, text, attachment)
}

func (s *postService) ListAllFeedback(ctx context.Context) ([]*domain.Feedback, error) {
	list, err := s.posts.ListAllFeedback(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, domain.ErrNotFound
	}
	sort.Slice(list, func(i, j int) bool {
		return list[j].Time.Before(list[i].Time)
	})
	return list, nil
}

func (s *postService) CalculateHeat(ctx context.Context) ([]domain.HeatItem, error) {
	if core.MyRedis == nil {
		return nil, domain.ErrInternal
	}
	topPosts, err := core.MyRedis.ZRevRangeWithScores(ctx, hotPostsZSetKey, 0, 19).Result()
	if err != nil {
		return nil, err
	}
	out := make([]domain.HeatItem, 0, 10)
	for _, z := range topPosts {
		if len(out) >= 10 {
			break
		}
		var info domain.HotPostZSetMember
		member, ok := z.Member.(string)
		if !ok {
			continue
		}
		if err := json.Unmarshal([]byte(member), &info); err != nil {
			return nil, err
		}
		post, err := s.posts.FindPublicByID(ctx, int(info.PostID))
		if err != nil {
			continue
		}
		out = append(out, domain.HeatItem{
			PostID: info.PostID,
			Title:  post.Title,
			Heat:   info.BrowseNum,
		})
	}
	return out, nil
}

func (s *postService) ListTags(ctx context.Context) ([]*domain.Tag, error) {
	return s.posts.ListAllTags(ctx)
}

// fetchBrowseInteractionSets 并行查询收藏/点赞/作者，缩短 browse 串行 DB 等待。
func (s *postService) fetchBrowseInteractionSets(ctx context.Context, viewerID int, postIDs, userIDs []int) (
	savedSet map[int]struct{}, likedSet map[int]struct{}, authorMap map[int]*domain.User,
) {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		savedSet, _ = s.posts.SavedSet(ctx, viewerID, postIDs)
		return nil
	})
	g.Go(func() error {
		likedSet, _ = s.posts.LikedSet(ctx, viewerID, postIDs)
		return nil
	})
	g.Go(func() error {
		authorMap, _ = s.posts.AuthorsByIDs(ctx, userIDs)
		return nil
	})
	_ = g.Wait()
	if savedSet == nil {
		savedSet = map[int]struct{}{}
	}
	if likedSet == nil {
		likedSet = map[int]struct{}{}
	}
	if authorMap == nil {
		authorMap = map[int]*domain.User{}
	}
	return savedSet, likedSet, authorMap
}

func (s *postService) buildBrowseItems(ctx context.Context, viewerID int, posts []*domain.Post, isSaveMode bool) ([]domain.BrowseItem, error) {
	postIDs := make([]int, 0, len(posts))
	userIDSet := make(map[int]struct{})
	for _, p := range posts {
		postIDs = append(postIDs, p.ID)
		if p.UserID != 0 {
			userIDSet[p.UserID] = struct{}{}
		}
	}
	userIDs := make([]int, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}
	savedSet, likedSet, authorMap := s.fetchBrowseInteractionSets(ctx, viewerID, postIDs, userIDs)

	items := make([]domain.BrowseItem, 0, len(posts))
	for _, p := range posts {
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
