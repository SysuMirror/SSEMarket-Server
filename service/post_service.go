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
	// coins 币发放（每日首次发帖 +1）。允许为 nil（单测与未开通的场景），调用处判空。
	coins CoinService
}

func NewPostService(
	posts repository.PostRepository,
	users repository.UserRepository,
	sues repository.SueRepository,
	ranks RankService,
	coins CoinService,
) PostService {
	return &postService{posts: posts, users: users, sues: sues, ranks: ranks, coins: coins}
}

func (s *postService) CountBrowse(ctx context.Context, filter domain.BrowseFilter) (int64, error) {
	return s.posts.Count(ctx, filter)
}

// Browse 帖子浏览列表（首页/分区/搜索/收藏/历史等）。
//
// 主页(home)场景下:
//   - 先 ListPinned 取置顶队列(最多 MaxPinnedQueueSize,按 pinned_at 先进先出)
//   - 将这些 ID 放入 filter.ExcludePostIDs,普通列表查询会自动排除
//   - 返回 BrowseResult{Pinned, Posts, Total}
//
// 其他场景(searchsort != "home")不走置顶逻辑,Pinned 字段为空,
// 所有帖都在 Posts 中(与旧行为兼容)。
func (s *postService) Browse(ctx context.Context, filter domain.BrowseFilter) (domain.BrowseResult, error) {
	result := domain.BrowseResult{FeedMode: domain.RecEffectiveTime}

	// 仅主页列表展示置顶帖。其他场景(收藏/历史/分区/搜索)无置顶逻辑。
	if filter.Searchsort == "home" || filter.Searchsort == "" {
		pinnedPosts, err := s.posts.ListPinned(ctx, filter.ViewerUserID)
		if err != nil {
			return result, err
		}
		if len(pinnedPosts) > 0 {
			pinnedItems, err := s.attachPersonalBrowseState(ctx, filter, pinnedPosts)
			if err != nil {
				return result, err
			}
			result.Pinned = pinnedItems

			// 排除置顶帖 ID,避免分页列表中重复出现。
			excludeIDs := make([]int, 0, len(pinnedPosts))
			for _, p := range pinnedPosts {
				excludeIDs = append(excludeIDs, p.ID)
			}
			filter.ExcludePostIDs = excludeIDs
		}
	}

	// 默认主页（无搜索/标签/时间筛选、未改排序）在用户开启或命中灰度时走推荐流。
	// 推荐排序失败或索引尚未就绪时静默退回时间流，绝不影响首页可用性。
	if recEligible(filter) && Recs != nil {
		if mode, _, _ := Recs.EffectiveMode(ctx, filter.ViewerUserID); mode == domain.RecEffectiveRecommend {
			limit, offset := filter.Limit, filter.Offset
			if limit <= 0 {
				limit = 15
			}
			exclude := make(map[int]struct{}, len(filter.ExcludePostIDs))
			for _, id := range filter.ExcludePostIDs {
				exclude[id] = struct{}{}
			}
			ids, total, err := Recs.RankPostIDs(ctx, filter.ViewerUserID, exclude, limit, offset)
			if err == nil {
				var recPosts []*domain.Post
				if len(ids) > 0 {
					recPosts, err = s.posts.ListByIDs(ctx, ids, filter.ViewerUserID)
				}
				if err == nil {
					items, attachErr := s.attachPersonalBrowseState(ctx, filter, recPosts)
					if attachErr == nil {
						result.Posts = items
						result.Total = int64(total)
						result.FeedMode = domain.RecEffectiveRecommend
						// 记录本页曝光，供下次刷新降权轮换。只认真实 feed 页大小，
						// 后台新帖轮询（limit=20）和置顶探测（limit=1）不算用户看过。
						if limit == recFeedPageSize && len(items) > 0 {
							seen := make([]int, 0, len(items))
							for _, item := range items {
								if item.Post != nil && item.Post.ID > 0 {
									seen = append(seen, item.Post.ID)
								}
							}
							Recs.RecordFeedImpressions(ctx, filter.ViewerUserID, seen)
						}
						return result, nil
					}
				}
			}
		}
	}

	posts, err := s.fetchPublicFeed(ctx, filter)
	if err != nil {
		return result, err
	}
	items, err := s.attachPersonalBrowseState(ctx, filter, posts)
	if err != nil {
		return result, err
	}
	result.Posts = items
	return result, nil
}

// recEligible 判断当前列表请求是否可以走推荐流：
// 只有「主页默认信息流」可以；搜索、标签、时间筛选、分区、显式改排序都保持原行为。
func recEligible(filter domain.BrowseFilter) bool {
	if filter.ViewerUserID <= 0 {
		return false
	}
	if filter.Searchsort != "" && filter.Searchsort != "home" {
		return false
	}
	if filter.Partition != "" && filter.Partition != "主页" {
		return false
	}
	if filter.Searchinfo != "" || filter.Tag != "" || filter.TimeFrom != "" || filter.TimeTo != "" {
		return false
	}
	return filter.OrderBy == "" || filter.OrderBy == "time_desc"
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
	isWatched, _ := s.posts.IsWatched(ctx, viewerUserID, postID)
	// 我在这帖上累计投了几个币（含历史投票折算），投币按钮态用。
	myCoins := 0
	if s.coins != nil {
		thrown, _ := s.coins.MyThrows(ctx, viewerUserID, domain.CoinTargetPost, []int{postID})
		myCoins = thrown[postID]
	}

	return &domain.PostDetail{
		Post:      post,
		Author:    userToDomainFromModel(userModel),
		IsSaved:   isSaved,
		IsLiked:   isLiked,
		IsWatched: isWatched,
		BrowseNum: browseNum,
		MyCoins:   myCoins,
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

// Create 创建帖子并返回带 ID 的实体，便于上层（如归档 MQ）取用 post.ID。
func (s *postService) Create(ctx context.Context, in domain.PostCreateInput) (*domain.Post, error) {
	user, err := s.users.FindByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if user.BanEnd.After(time.Now()) {
		return nil, domain.ErrForbidden
	}
	if in.Partition == coursePartition {
		return nil, domain.ErrForbidden
	}
	tagString := strings.Join(in.Tags, ",")
	post := &domain.Post{
		UserID:      in.UserID,
		Partition:   in.Partition,
		Title:       in.Title,
		Content:     in.Content,
		LikeNum:     0,
		CommentNum:  0,
		BrowseNum:   0,
		Heat:        0,
		PostTime:    time.Now(),
		Photos:      in.Photos,
		Tag:         tagString,
		IsPrivate:   in.IsPrivate,
		IsAnonymous: in.IsAnonymous,
	}
	if err := s.posts.Create(ctx, post); err != nil {
		return nil, err
	}
	common.AddPostID(post.ID)
	InvalidateHomeBrowse()

	// 币：每日首次发帖 +1（幂等键含日期，当天第二帖不再给）。
	// 放在置顶逻辑之前：置顶失败不影响已经拿到的币。
	if s.coins != nil {
		_, _ = s.coins.GrantFirstPost(ctx, in.UserID)
	}

	if len(in.Tags) > 0 {
		tag, err := s.posts.FindTagByName(ctx, in.Tags[0])
		if err == nil && tag.Type == "course" {
			receiver, err := s.users.FindByEmail(ctx, tag.Value)
			if err == nil && receiver.ID != 0 && receiver.Phone != user.Phone {
				_ = s.posts.CreatePostNotice(ctx, receiver.ID, user.ID, "post", in.Title, post.ID)
			}
		}
	}
	// 若发帖入参指定置顶,尝试置顶。
	// 旧逻辑静默忽略失败,前端勾选「同时置顶」时无提示:
	// if in.Pinned {
	// 	_ = s.PinPost(ctx, post.ID, in.UserID)
	// }
	// return post, nil
	if in.Pinned {
		if err := s.PinPost(ctx, post.ID, in.UserID); err != nil {
			// 发帖已成功;把置顶失败包装返回,由 controller 返回「发帖成功,但置顶失败」文案。
			return post, &domain.PinAfterCreateError{Post: post, PinErr: err}
		}
	}
	return post, nil
}

// canPin 当前对所有登录用户开放置顶权限。
// 单用户最多 1 个未过期置顶帖;全站置顶队列最多 MaxPinnedQueueSize 条。
func canPin(userID int) bool {
	return userID > 0
}

// CountActivePinned 当前全站置顶队列长度。
func (s *postService) CountActivePinned(ctx context.Context) (int64, error) {
	return s.posts.CountActivePinned(ctx)
}

// PinPost 给帖子置顶 1 小时并入队。
//
// 校验链:
//  1. 权限:canPin(userID)
//  2. 帖子归属:FindByID 校验是本人帖子
//  3. 分区须为「主页」(ListPinned 仅收录主页帖)
//  4. 同帖未置顶:is_pinned && pinned_expires_at > NOW() → ErrAlreadyPinned
//  5. 每人 1 个:CountPinnedByUser → ErrPinnedLimitReached
//  6. 全站队列上限:CountActivePinned >= MaxPinnedQueueSize → ErrGlobalPinnedLimitReached
//
// 入队时写入 pinned_at=NOW(),ListPinned 按此字段先进先出排序。
func (s *postService) PinPost(ctx context.Context, postID, userID int) error {
	if !canPin(userID) {
		return domain.ErrForbidden
	}
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	if post.Partition != "主页" {
		return domain.ErrPinnedWrongPartition
	}
	if post.IsPinned && post.PinnedExpiresAt != nil && post.PinnedExpiresAt.After(time.Now()) {
		return domain.ErrAlreadyPinned
	}
	count, err := s.posts.CountPinnedByUser(ctx, userID)
	if err != nil {
		return err
	}
	if count >= 1 {
		return domain.ErrPinnedLimitReached
	}
	globalCount, err := s.posts.CountActivePinned(ctx)
	if err != nil {
		return err
	}
	if globalCount >= int64(domain.MaxPinnedQueueSize) {
		return domain.ErrGlobalPinnedLimitReached
	}
	expiresAt := time.Now().Add(1 * time.Hour)
	if err := s.posts.PinPost(ctx, postID, userID, expiresAt); err != nil {
		return err
	}
	InvalidateHomeBrowse()
	return nil
}

// UnpinPost 取消帖子置顶。校验是本人帖子。
func (s *postService) UnpinPost(ctx context.Context, postID, userID int) error {
	if !canPin(userID) {
		return domain.ErrForbidden
	}
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	if !post.IsPinned {
		return domain.ErrNotPinned
	}
	if err := s.posts.UnpinPost(ctx, postID, userID); err != nil {
		return err
	}
	InvalidateHomeBrowse()
	return nil
}

// ClearExpiredPins 清理过期/无效置顶标记;成功清理后失效首页缓存。
func (s *postService) ClearExpiredPins(ctx context.Context) (int64, error) {
	n, err := s.posts.ClearExpiredPins(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		InvalidateHomeBrowse()
	}
	return n, nil
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

func (s *postService) ToggleSave(ctx context.Context, userID, postID int) (bool, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return false, err
	}
	if !canViewPost(post, userID) {
		return false, domain.ErrNotFound
	}
	isSaved, err := s.posts.IsSaved(ctx, userID, postID)
	if err != nil {
		return false, err
	}
	if isSaved {
		if err := s.posts.DeleteSave(ctx, userID, postID); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := s.posts.CreateSave(ctx, userID, postID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *postService) ToggleWatch(ctx context.Context, userID, postID int) (bool, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return false, err
	}
	if !canViewPost(post, userID) {
		return false, domain.ErrNotFound
	}
	isWatched, err := s.posts.IsWatched(ctx, userID, postID)
	if err != nil {
		return false, err
	}
	if isWatched {
		return false, s.posts.DeleteWatch(ctx, userID, postID)
	}
	return true, s.posts.CreateWatch(ctx, userID, postID)
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
		deleted, err := s.posts.DeleteLikeOnce(ctx, userID, postID)
		if err != nil {
			return err
		}
		if deleted {
			s.ranks.AdjustUserReceivedLike(ctx, post.UserID, -1)
		}
	} else {
		inserted, err := s.posts.CreateLikeOnce(ctx, userID, postID, weightLike)
		if err != nil {
			return err
		}
		if inserted {
			s.ranks.AdjustUserReceivedLike(ctx, post.UserID, 1)
		}
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
	return s.posts.CreateBrowseOnce(ctx, userID, postID, weightBrowse)
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

// fetchBrowseInteractionSets 并行批量查询 browse 所需的个人态数据。
//
// 使用 errgroup 同时发起 4 路 SQL，将原本串行的 SavedSet + LikedSet + AuthorsByIDs + 投币数
// 合并为一次等待，缩短 attachPersonalBrowseState 的尾延迟。
func (s *postService) fetchBrowseInteractionSets(ctx context.Context, viewerID int, postIDs, userIDs []int) (
	savedSet map[int]struct{}, likedSet map[int]struct{}, authorMap map[int]*domain.User, thrownSet map[int]int,
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
	g.Go(func() error {
		if s.coins != nil {
			thrownSet, _ = s.coins.MyThrows(ctx, viewerID, domain.CoinTargetPost, postIDs)
		}
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
	if thrownSet == nil {
		thrownSet = map[int]int{}
	}
	return savedSet, likedSet, authorMap, thrownSet
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
	savedSet, likedSet, authorMap, thrownSet := s.fetchBrowseInteractionSets(ctx, viewerID, postIDs, userIDs)

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
			MyCoins: thrownSet[p.ID],
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
		ID:          p.PostID,
		UserID:      p.UserID,
		Partition:   p.Partition,
		Title:       p.Title,
		Content:     p.Ptext,
		CommentNum:  p.CommentNum,
		LikeNum:     p.LikeNum,
		CoinNum:     p.CoinNum + p.VoteNum, // 历史票数并入投币数，见 domain.Post.CoinNum
		BrowseNum:   p.BrowseNum,
		PostTime:    p.PostTime,
		Heat:        p.Heat,
		Photos:      p.Photos,
		Tag:         p.Tag,
		IsPrivate:   p.IsPrivate,
		IsAnonymous: p.IsAnonymous,
	}
}

func userToDomainFromModel(u model.User) *domain.User {
	return &domain.User{
		ID:          u.UserID,
		Phone:       u.Phone,
		Name:        u.Name,
		Score:       u.Score,
		AvatarURL:   u.AvatarURL,
		Identity:    u.Identity,
		CustomTitle: u.CustomTitle,
	}
}
