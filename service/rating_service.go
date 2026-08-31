package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/repository"
)

type ratingService struct {
	ratings repository.RatingRepository
}

func NewRatingService(ratings repository.RatingRepository) RatingService {
	return &ratingService{ratings: ratings}
}

func (s *ratingService) ListCategories(ctx context.Context) ([]*domain.RatingCategory, error) {
	return s.ratings.ListCategories(ctx)
}

func (s *ratingService) CreateCategory(ctx context.Context, in domain.RatingCategoryCreateInput) (*domain.RatingCategory, error) {
	name := strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 30 {
		return nil, domain.ErrBadRequest
	}
	desc := strings.TrimSpace(in.Description)
	if utf8.RuneCountInString(desc) > 200 {
		return nil, domain.ErrBadRequest
	}
	if _, err := s.ratings.FindCategoryByName(ctx, name); err == nil {
		return nil, domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	maxOrder, err := s.ratings.MaxCategorySortOrder(ctx)
	if err != nil {
		return nil, err
	}
	cat := &domain.RatingCategory{
		Name:        name,
		Description: desc,
		CreatorID:   in.CreatorID,
		IsOfficial:  false,
		SortOrder:   maxOrder + 1,
		CreatedAt:   time.Now(),
	}
	if in.SyncFeedPost {
		user, err := s.ratings.FindUserByID(ctx, in.CreatorID)
		if err != nil {
			return nil, err
		}
		feedTitle := common.FeedTitleWithPrefix("新增分区-", cat.Name)
		if err := s.ratings.CreateCategoryWithFeedPost(ctx, cat, user, feedTitle); err != nil {
			return nil, err
		}
	} else if err := s.ratings.CreateCategory(ctx, cat); err != nil {
		return nil, err
	}
	return cat, nil
}

func (s *ratingService) ListPosts(ctx context.Context, filter domain.RatingBrowseFilter) ([]*domain.RatingPost, error) {
	return s.ratings.ListPosts(ctx, filter)
}

func (s *ratingService) CountPosts(ctx context.Context, filter domain.RatingBrowseFilter) (int64, error) {
	return s.ratings.CountPosts(ctx, filter)
}

func (s *ratingService) CreatePost(ctx context.Context, in domain.RatingPostCreateInput) (int, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" || utf8.RuneCountInString(title) > 50 {
		return 0, domain.ErrBadRequest
	}
	content := strings.TrimSpace(in.Content)
	if utf8.RuneCountInString(content) > 5000 {
		return 0, domain.ErrBadRequest
	}
	if in.CategoryID <= 0 {
		return 0, domain.ErrBadRequest
	}
	if _, err := s.ratings.FindCategoryByID(ctx, in.CategoryID); err != nil {
		return 0, err
	}
	post := &domain.RatingPost{
		CategoryID: in.CategoryID,
		UserID:     in.UserID,
		Title:      title,
		Content:    content,
		Photos:     in.Photos,
		CreatedAt:  time.Now(),
	}
	if err := s.ratings.CreatePost(ctx, post); err != nil {
		return 0, err
	}
	_ = s.ratings.IncrCategoryPostCount(ctx, in.CategoryID, 1)
	return post.ID, nil
}

func (s *ratingService) GetPostDetail(ctx context.Context, postID, viewerID int) (*domain.RatingPost, int, error) {
	post, err := s.ratings.FindPostByID(ctx, postID)
	if err != nil {
		return nil, 0, err
	}
	_ = s.ratings.IncrPostBrowse(ctx, postID)
	post.BrowseNum++
	userStar, _ := s.ratings.FindUserStar(ctx, postID, viewerID)
	return post, userStar, nil
}

func (s *ratingService) DeletePost(ctx context.Context, postID, userID int) error {
	post, err := s.ratings.FindPostByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return domain.ErrForbidden
	}
	return s.ratings.DeletePostCascade(ctx, postID, post.CategoryID)
}

func (s *ratingService) TogglePostLike(ctx context.Context, postID, userID int) error {
	if _, err := s.ratings.FindPostByID(ctx, postID); err != nil {
		return err
	}
	return s.ratings.TogglePostLike(ctx, postID, userID)
}

func (s *ratingService) SubmitStar(ctx context.Context, postID, userID, starNum int) error {
	if postID <= 0 || starNum < 1 || starNum > 5 {
		return domain.ErrBadRequest
	}
	if _, err := s.ratings.FindPostByID(ctx, postID); err != nil {
		return err
	}
	return s.ratings.UpsertStar(ctx, postID, userID, starNum)
}

func (s *ratingService) ListComments(ctx context.Context, postID, viewerID int) ([]*domain.RatingComment, map[int]*domain.User, map[int]bool, error) {
	if postID <= 0 {
		return nil, nil, nil, nil
	}
	comments, err := s.ratings.ListCommentsByPost(ctx, postID)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(comments) == 0 {
		return comments, nil, nil, nil
	}
	userIDs := make([]int, 0, len(comments))
	commentIDs := make([]int, 0, len(comments))
	for _, c := range comments {
		userIDs = append(userIDs, c.UserID)
		commentIDs = append(commentIDs, c.ID)
	}
	userMap, err := s.ratings.UsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	likeMap, err := s.ratings.CommentLikedSet(ctx, viewerID, commentIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	return comments, userMap, likeMap, nil
}

func (s *ratingService) CreateComment(ctx context.Context, in domain.RatingCommentCreateInput) (*domain.RatingComment, *domain.User, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > 1000 {
		return nil, nil, domain.ErrBadRequest
	}
	if in.StarNum < 0 || in.StarNum > 5 {
		return nil, nil, domain.ErrBadRequest
	}
	post, err := s.ratings.FindPostByID(ctx, in.RatingPostID)
	if err != nil {
		return nil, nil, err
	}
	author, err := s.ratings.FindUserByID(ctx, in.UserID)
	if err != nil {
		return nil, nil, err
	}
	if author.BanEnd.After(time.Now()) {
		return nil, nil, domain.ErrBadRequest
	}
	cm := &domain.RatingComment{
		RatingPostID: in.RatingPostID,
		UserID:       in.UserID,
		Content:      content,
		StarNum:      in.StarNum,
		CreatedAt:    time.Now(),
	}
	if err := s.ratings.CreateComment(ctx, cm); err != nil {
		return nil, nil, err
	}
	_ = s.ratings.IncrPostCommentNum(ctx, in.RatingPostID)
	if in.StarNum >= 1 && in.StarNum <= 5 {
		_ = s.ratings.UpsertStar(ctx, in.RatingPostID, in.UserID, in.StarNum)
	}
	var notifyReceiver *domain.User
	if post.UserID != in.UserID {
		_ = s.ratings.CreateNotice(ctx, post.UserID, in.UserID, "rating_comment", content, cm.ID)
		notifyReceiver, _ = s.ratings.FindUserByID(ctx, post.UserID)
	}
	return cm, notifyReceiver, nil
}

func (s *ratingService) ToggleCommentLike(ctx context.Context, commentID, userID int) error {
	if _, err := s.ratings.FindCommentByID(ctx, commentID); err != nil {
		return err
	}
	return s.ratings.ToggleCommentLike(ctx, commentID, userID)
}

func (s *ratingService) EnrichPosts(ctx context.Context, posts []*domain.RatingPost, viewerID int) (
	map[int]*domain.RatingCategory,
	map[int]*domain.User,
	map[int]struct{},
	map[int]domain.RatingStarSummary,
	error,
) {
	if len(posts) == 0 {
		return nil, nil, nil, nil, nil
	}
	postIDs := make([]int, 0, len(posts))
	catIDs := make([]int, 0, len(posts))
	userIDs := make([]int, 0, len(posts))
	for _, p := range posts {
		postIDs = append(postIDs, p.ID)
		catIDs = append(catIDs, p.CategoryID)
		if p.UserID != 0 {
			userIDs = append(userIDs, p.UserID)
		}
	}
	catMap, err := s.ratings.CategoriesByIDs(ctx, catIDs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	userMap, err := s.ratings.UsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	likedSet, err := s.ratings.PostLikedSet(ctx, viewerID, postIDs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	starsRaw, err := s.ratings.StarsByPostIDs(ctx, postIDs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	summary := make(map[int]domain.RatingStarSummary)
	for postID, stars := range starsRaw {
		summary[postID] = aggregateStars(stars)
	}
	return catMap, userMap, likedSet, summary, nil
}

func (s *ratingService) FindUser(ctx context.Context, id int) (*domain.User, error) {
	return s.ratings.FindUserByID(ctx, id)
}

func aggregateStars(stars []int) domain.RatingStarSummary {
	var cnt [5]int
	var total float64
	var n int
	for _, star := range stars {
		if star < 1 || star > 5 {
			continue
		}
		cnt[star-1]++
		total += float64(star)
		n++
	}
	avg := 0.0
	if n > 0 {
		avg = total / float64(n)
	}
	return domain.RatingStarSummary{Stars: cnt, AverageRating: avg, RaterCount: n}
}

