package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/cache"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/repository"
	"loginTest/util"
)

const commentWeight = float64(6)

type commentService struct {
	comments repository.CommentRepository
}

func NewCommentService(comments repository.CommentRepository) CommentService {
	return &commentService{comments: comments}
}

func (s *commentService) ListForPost(ctx context.Context, postID int, postType string, viewerUserID int) ([]*domain.PcommentWithMeta, error) {
	if postType != "" && postType != "post" {
		return nil, nil
	}
	post, err := s.comments.FindPostByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if !canViewPost(post, viewerUserID) {
		return nil, domain.ErrNotFound
	}

	cacheKey := util.GetCommentListKey(postID, postType)
	tree, err := MultiLevelGet(ctx, cacheKey, func() (*dto.CommentTreeCacheDTO, error) {
		return s.loadCommentTree(ctx, postID)
	}, cache.ProfileCommentList)
	if err != nil {
		return nil, err
	}
	if tree == nil || len(tree.Pcomments) == 0 {
		return nil, nil
	}
	return s.buildCommentResponse(ctx, tree, viewerUserID)
}

func (s *commentService) loadCommentTree(ctx context.Context, postID int) (*dto.CommentTreeCacheDTO, error) {
	pcomments, err := s.comments.ListPcommentsByPostID(ctx, postID)
	if err != nil {
		return nil, err
	}
	subs := make(map[int][]*domain.Ccomment, len(pcomments))
	for _, pc := range pcomments {
		ccomments, err := s.comments.ListCcommentsByPcommentID(ctx, pc.ID)
		if err != nil {
			return nil, err
		}
		subs[pc.ID] = ccomments
	}
	return &dto.CommentTreeCacheDTO{Pcomments: pcomments, Subs: subs}, nil
}

func (s *commentService) buildCommentResponse(ctx context.Context, tree *dto.CommentTreeCacheDTO, viewerUserID int) ([]*domain.PcommentWithMeta, error) {
	commentIDs := make([]int, 0, len(tree.Pcomments))
	userIDSet := make(map[int]struct{}, len(tree.Pcomments))
	for _, pc := range tree.Pcomments {
		commentIDs = append(commentIDs, pc.ID)
		userIDSet[pc.UserID] = struct{}{}
		for _, cc := range tree.Subs[pc.ID] {
			userIDSet[cc.UserID] = struct{}{}
		}
	}

	userMap, err := s.comments.UsersByIDs(ctx, intSetToSlice(userIDSet))
	if err != nil {
		return nil, err
	}
	likeMap, err := s.comments.PcommentLikedSet(ctx, viewerUserID, commentIDs)
	if err != nil {
		return nil, err
	}
	denyMap, err := s.comments.PcommentDeniedSet(ctx, viewerUserID, commentIDs)
	if err != nil {
		return nil, err
	}

	ccommentIDs := make([]int, 0)
	for _, pc := range tree.Pcomments {
		for _, cc := range tree.Subs[pc.ID] {
			ccommentIDs = append(ccommentIDs, cc.ID)
		}
	}
	ccLikeMap, err := s.comments.CcommentLikedSet(ctx, viewerUserID, ccommentIDs)
	if err != nil {
		return nil, err
	}
	ccDenyMap, err := s.comments.CcommentDeniedSet(ctx, viewerUserID, ccommentIDs)
	if err != nil {
		return nil, err
	}

	out := make([]*domain.PcommentWithMeta, 0, len(tree.Pcomments))
	for _, pc := range tree.Pcomments {
		subs, err := s.buildSubcommentsFromCache(tree.Subs[pc.ID], userMap, ccLikeMap, ccDenyMap)
		if err != nil {
			return nil, err
		}
		out = append(out, &domain.PcommentWithMeta{
			Pcomment:    pc,
			Author:      authorForComment(userMap, pc.UserID),
			Subcomments: subs,
			IsLiked:     likeMap[pc.ID],
			IsDenied:    denyMap[pc.ID],
		})
	}
	return out, nil
}

func (s *commentService) buildSubcommentsFromCache(ccomments []*domain.Ccomment, userMap map[int]*domain.User, ccLikeMap, ccDenyMap map[int]bool) ([]*domain.CcommentWithMeta, error) {
	if len(ccomments) == 0 {
		return []*domain.CcommentWithMeta{}, nil
	}
	out := make([]*domain.CcommentWithMeta, 0, len(ccomments))
	for _, cc := range ccomments {
		out = append(out, &domain.CcommentWithMeta{
			Ccomment: cc,
			Author:   authorForComment(userMap, cc.UserID),
			IsLiked:  ccLikeMap[cc.ID],
			IsDenied: ccDenyMap[cc.ID],
		})
	}
	return out, nil
}

func (s *commentService) buildSubcomments(ctx context.Context, pcommentID, viewerUserID int, userMap map[int]*domain.User) ([]*domain.CcommentWithMeta, error) {
	ccomments, err := s.comments.ListCcommentsByPcommentID(ctx, pcommentID)
	if err != nil {
		return nil, err
	}
	if len(ccomments) == 0 {
		return []*domain.CcommentWithMeta{}, nil
	}
	for _, cc := range ccomments {
		if _, ok := userMap[cc.UserID]; !ok && cc.UserID != 0 {
			userMap[cc.UserID] = nil
		}
	}
	missing := make([]int, 0)
	for id, u := range userMap {
		if u == nil && id != 0 {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		fetched, err := s.comments.UsersByIDs(ctx, missing)
		if err != nil {
			return nil, err
		}
		for id, u := range fetched {
			userMap[id] = u
		}
	}

	ccommentIDs := make([]int, 0, len(ccomments))
	for _, cc := range ccomments {
		ccommentIDs = append(ccommentIDs, cc.ID)
	}
	ccLikeMap, err := s.comments.CcommentLikedSet(ctx, viewerUserID, ccommentIDs)
	if err != nil {
		return nil, err
	}
	ccDenyMap, err := s.comments.CcommentDeniedSet(ctx, viewerUserID, ccommentIDs)
	if err != nil {
		return nil, err
	}

	out := make([]*domain.CcommentWithMeta, 0, len(ccomments))
	for _, cc := range ccomments {
		out = append(out, &domain.CcommentWithMeta{
			Ccomment: cc,
			Author:   authorForComment(userMap, cc.UserID),
			IsLiked:  ccLikeMap[cc.ID],
			IsDenied: ccDenyMap[cc.ID],
		})
	}
	return out, nil
}

func (s *commentService) DeletePcomment(ctx context.Context, pcommentID int) error {
	pc, err := s.comments.FindPcommentByID(ctx, pcommentID)
	if err != nil {
		return err
	}
	count, err := s.comments.CountCcommentsByPcommentID(ctx, pcommentID)
	if err != nil {
		return err
	}
	post, err := s.comments.FindPostByID(ctx, pc.PostID)
	if err != nil {
		return err
	}
	delta := deleteHeatDelta(pc.CreatedAt, count, true)
	if err := s.comments.IncrPostCommentNum(ctx, post.ID, -int(count+1)); err != nil {
		return err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, -delta); err != nil {
		return err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")
	return s.comments.DeletePcomment(ctx, pcommentID)
}

func (s *commentService) DeleteCcomment(ctx context.Context, ccommentID int) error {
	cc, err := s.comments.FindCcommentByID(ctx, ccommentID)
	if err != nil {
		return err
	}
	pc, err := s.comments.FindPcommentByID(ctx, cc.PcommentID)
	if err != nil {
		return err
	}
	post, err := s.comments.FindPostByID(ctx, pc.PostID)
	if err != nil {
		return err
	}
	delta := deleteHeatDelta(cc.CreatedAt, 0, false)
	if err := s.comments.IncrPostCommentNum(ctx, post.ID, -1); err != nil {
		return err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, -delta); err != nil {
		return err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")
	return s.comments.DeleteCcomment(ctx, ccommentID)
}

func (s *commentService) CreatePcomment(ctx context.Context, in domain.PcommentCreateInput) (*domain.PcommentCreateResult, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > 1000 {
		return nil, domain.ErrBadRequest
	}
	if in.UserID == 0 || in.PostID == 0 {
		return nil, domain.ErrBadRequest
	}
	author, err := s.comments.FindUserByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if author.BanEnd.After(time.Now()) {
		return nil, domain.ErrBadRequest
	}
	post, err := s.comments.FindPostByID(ctx, in.PostID)
	if err != nil {
		return nil, err
	}
	if !canViewPost(post, in.UserID) {
		return nil, domain.ErrNotFound
	}

	pc := &domain.Pcomment{
		UserID:    in.UserID,
		PostID:    in.PostID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := s.comments.CreatePcomment(ctx, pc); err != nil {
		return nil, err
	}

	var notifyReceiver *domain.User
	if post.UserID != in.UserID {
		_ = s.comments.CreateNotice(ctx, post.UserID, in.UserID, "pcomment", content, pc.ID)
		notifyReceiver, _ = s.comments.FindUserByID(ctx, post.UserID)
	}

	if err := s.comments.IncrPostCommentNum(ctx, post.ID, 1); err != nil {
		return nil, err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, commentWeight); err != nil {
		return nil, err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")

	subs, err := s.buildSubcomments(ctx, pc.ID, in.UserID, map[int]*domain.User{in.UserID: author})
	if err != nil {
		return nil, err
	}
	return &domain.PcommentCreateResult{
		Pcomment:       pc,
		Author:         author,
		Subcomments:    subs,
		NotifyReceiver: notifyReceiver,
	}, nil
}

func (s *commentService) CreateCcomment(ctx context.Context, in domain.CcommentCreateInput) ([]*domain.CcommentNotifyInfo, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > 1000 {
		return nil, domain.ErrBadRequest
	}
	if in.UserID == 0 || in.PostID == 0 || in.PcommentID == 0 {
		return nil, domain.ErrBadRequest
	}
	author, err := s.comments.FindUserByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if author.BanEnd.After(time.Now()) {
		return nil, domain.ErrBadRequest
	}
	post, err := s.comments.FindPostByID(ctx, in.PostID)
	if err != nil {
		return nil, err
	}
	if !canViewPost(post, in.UserID) {
		return nil, domain.ErrNotFound
	}

	cc := &domain.Ccomment{
		UserID:         in.UserID,
		PcommentID:     in.PcommentID,
		Content:        content,
		CreatedAt:      time.Now(),
		UserTargetName: in.UserTargetName,
	}
	if err := s.comments.CreateCcomment(ctx, cc); err != nil {
		return nil, err
	}

	pc, err := s.comments.FindPcommentByID(ctx, in.PcommentID)
	if err != nil {
		return nil, err
	}

	var notifies []*domain.CcommentNotifyInfo
	if pc.UserID != in.UserID {
		_ = s.comments.CreateNotice(ctx, pc.UserID, in.UserID, "ccomment", content, cc.ID)
		if receiver, err := s.comments.FindUserByID(ctx, pc.UserID); err == nil {
			notifies = append(notifies, &domain.CcommentNotifyInfo{Receiver: receiver})
		}
	}
	if in.UserTargetName != "" && in.ReplyCcommentID > 0 {
		replyCC, err := s.comments.FindCcommentByID(ctx, in.ReplyCcommentID)
		if err != nil {
			return nil, err
		}
		if replyCC.UserID != in.UserID && pc.UserID != replyCC.UserID {
			_ = s.comments.CreateNotice(ctx, replyCC.UserID, in.UserID, "ccomment", content, cc.ID)
			if receiver, err := s.comments.FindUserByID(ctx, replyCC.UserID); err == nil {
				notifies = append(notifies, &domain.CcommentNotifyInfo{Receiver: receiver})
			}
		}
	}

	if err := s.comments.IncrPostCommentNum(ctx, post.ID, 1); err != nil {
		return nil, err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, commentWeight); err != nil {
		return nil, err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")
	return notifies, nil
}

func (s *commentService) TogglePcommentLike(ctx context.Context, userID, pcommentID int) error {
	if err := s.validatePcommentAccess(ctx, pcommentID, userID); err != nil {
		return err
	}
	isLiked, err := s.comments.FindPcommentLike(ctx, userID, pcommentID)
	if err != nil {
		return err
	}
	isDenied, err := s.comments.FindPcommentDeny(ctx, userID, pcommentID)
	if err != nil {
		return err
	}
	if isLiked {
		if err := s.comments.IncrPcommentLikeNum(ctx, pcommentID, -1); err != nil {
			return err
		}
		return s.comments.DeletePcommentLike(ctx, userID, pcommentID)
	}
	if isDenied {
		if err := s.comments.IncrPcommentDenyNum(ctx, pcommentID, -1); err != nil {
			return err
		}
		if err := s.comments.DeletePcommentDeny(ctx, userID, pcommentID); err != nil {
			return err
		}
	}
	if userID != 0 && pcommentID != 0 {
		if err := s.comments.IncrPcommentLikeNum(ctx, pcommentID, 1); err != nil {
			return err
		}
		return s.comments.CreatePcommentLike(ctx, userID, pcommentID)
	}
	return nil
}

func (s *commentService) TogglePcommentDeny(ctx context.Context, userID, pcommentID int) error {
	if err := s.validatePcommentAccess(ctx, pcommentID, userID); err != nil {
		return err
	}
	isLiked, err := s.comments.FindPcommentLike(ctx, userID, pcommentID)
	if err != nil {
		return err
	}
	isDenied, err := s.comments.FindPcommentDeny(ctx, userID, pcommentID)
	if err != nil {
		return err
	}
	if isDenied {
		if err := s.comments.IncrPcommentDenyNum(ctx, pcommentID, -1); err != nil {
			return err
		}
		return s.comments.DeletePcommentDeny(ctx, userID, pcommentID)
	}
	if isLiked {
		if err := s.comments.IncrPcommentLikeNum(ctx, pcommentID, -1); err != nil {
			return err
		}
		if err := s.comments.DeletePcommentLike(ctx, userID, pcommentID); err != nil {
			return err
		}
	}
	if userID != 0 && pcommentID != 0 {
		if err := s.comments.IncrPcommentDenyNum(ctx, pcommentID, 1); err != nil {
			return err
		}
		return s.comments.CreatePcommentDeny(ctx, userID, pcommentID)
	}
	return nil
}

func (s *commentService) ToggleCcommentLike(ctx context.Context, userID, ccommentID int) error {
	if err := s.validateCcommentAccess(ctx, ccommentID, userID); err != nil {
		return err
	}
	isLiked, err := s.comments.FindCcommentLike(ctx, userID, ccommentID)
	if err != nil {
		return err
	}
	isDenied, err := s.comments.FindCcommentDeny(ctx, userID, ccommentID)
	if err != nil {
		return err
	}
	if isLiked {
		if err := s.comments.IncrCcommentLikeNum(ctx, ccommentID, -1); err != nil {
			return err
		}
		return s.comments.DeleteCcommentLike(ctx, userID, ccommentID)
	}
	if isDenied {
		if err := s.comments.IncrCcommentDenyNum(ctx, ccommentID, -1); err != nil {
			return err
		}
		if err := s.comments.DeleteCcommentDeny(ctx, userID, ccommentID); err != nil {
			return err
		}
	}
	if userID != 0 && ccommentID != 0 {
		if err := s.comments.IncrCcommentLikeNum(ctx, ccommentID, 1); err != nil {
			return err
		}
		return s.comments.CreateCcommentLike(ctx, userID, ccommentID)
	}
	return nil
}

func (s *commentService) ToggleCcommentDeny(ctx context.Context, userID, ccommentID int) error {
	if err := s.validateCcommentAccess(ctx, ccommentID, userID); err != nil {
		return err
	}
	isLiked, err := s.comments.FindCcommentLike(ctx, userID, ccommentID)
	if err != nil {
		return err
	}
	isDenied, err := s.comments.FindCcommentDeny(ctx, userID, ccommentID)
	if err != nil {
		return err
	}
	if isDenied {
		if err := s.comments.IncrCcommentDenyNum(ctx, ccommentID, -1); err != nil {
			return err
		}
		return s.comments.DeleteCcommentDeny(ctx, userID, ccommentID)
	}
	if isLiked {
		if err := s.comments.IncrCcommentLikeNum(ctx, ccommentID, -1); err != nil {
			return err
		}
		if err := s.comments.DeleteCcommentLike(ctx, userID, ccommentID); err != nil {
			return err
		}
	}
	if userID != 0 && ccommentID != 0 {
		if err := s.comments.IncrCcommentDenyNum(ctx, ccommentID, 1); err != nil {
			return err
		}
		return s.comments.CreateCcommentDeny(ctx, userID, ccommentID)
	}
	return nil
}

func (s *commentService) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	if phone == "" {
		return nil, domain.ErrNotFound
	}
	return s.comments.FindUserByPhone(ctx, phone)
}

func (s *commentService) FindPcomment(ctx context.Context, pcommentID int) (*domain.Pcomment, error) {
	pc, err := s.comments.FindPcommentByID(ctx, pcommentID)
	if err != nil {
		return nil, err
	}
	if pc.ID == 0 {
		return nil, domain.ErrNotFound
	}
	return pc, nil
}

func (s *commentService) FindCcomment(ctx context.Context, ccommentID int) (*domain.Ccomment, error) {
	cc, err := s.comments.FindCcommentByID(ctx, ccommentID)
	if err != nil {
		return nil, err
	}
	if cc.ID == 0 {
		return nil, domain.ErrNotFound
	}
	return cc, nil
}

func (s *commentService) ValidatePcommentAccess(ctx context.Context, pcommentID, viewerUserID int) error {
	return s.validatePcommentAccess(ctx, pcommentID, viewerUserID)
}

func (s *commentService) ValidateCcommentAccess(ctx context.Context, ccommentID, viewerUserID int) error {
	return s.validateCcommentAccess(ctx, ccommentID, viewerUserID)
}

func (s *commentService) validatePcommentAccess(ctx context.Context, pcommentID, viewerUserID int) error {
	pc, err := s.comments.FindPcommentByID(ctx, pcommentID)
	if err != nil {
		return err
	}
	post, err := s.comments.FindPostByID(ctx, pc.PostID)
	if err != nil {
		return err
	}
	if !canViewPost(post, viewerUserID) {
		return domain.ErrNotFound
	}
	return nil
}

func (s *commentService) validateCcommentAccess(ctx context.Context, ccommentID, viewerUserID int) error {
	cc, err := s.comments.FindCcommentByID(ctx, ccommentID)
	if err != nil {
		return err
	}
	pc, err := s.comments.FindPcommentByID(ctx, cc.PcommentID)
	if err != nil {
		return err
	}
	post, err := s.comments.FindPostByID(ctx, pc.PostID)
	if err != nil {
		return err
	}
	if !canViewPost(post, viewerUserID) {
		return domain.ErrNotFound
	}
	return nil
}

func deleteHeatDelta(commentTime time.Time, subCount int64, isPcomment bool) float64 {
	days := int(time.Since(commentTime).Hours() / 24)
	fmt.Println("days: ", days)
	if isPcomment {
		if days > 0 {
			power := math.Pow(0.5, float64(days))
			return math.Pow(commentWeight, power) + float64(subCount)
		}
		return commentWeight + float64(subCount)*commentWeight
	}
	if days > 0 {
		power := math.Pow(0.5, float64(days))
		return math.Pow(commentWeight, power)
	}
	return commentWeight
}

func authorForComment(m map[int]*domain.User, userID int) *domain.User {
	if userID == 0 {
		return &domain.User{Name: "管理员", Phone: "11111111111"}
	}
	if u, ok := m[userID]; ok && u != nil {
		return u
	}
	return &domain.User{}
}

func intSetToSlice(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}
