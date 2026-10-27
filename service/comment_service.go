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
	posts    repository.PostRepository
	staff    StaffService
}

func NewCommentService(comments repository.CommentRepository, posts repository.PostRepository, staff StaffService) CommentService {
	return &commentService{comments: comments, posts: posts, staff: staff}
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
	items, err := s.buildCommentResponse(ctx, tree, viewerUserID)
	if err != nil {
		return nil, err
	}
	// 匿名帖：作者在自己帖下的评论/回复也显示为匿名用户，避免暴露发帖人身份
	if post.IsAnonymous {
		anonymizeCommentsByPostAuthor(items, post.UserID)
	}
	return items, nil
}

// anonymizeCommentsByPostAuthor 把匿名帖作者自己发的评论/回复替换为脱敏作者副本，
// 不修改 userMap 中共享的 *domain.User（其他评论者信息不受影响）。
func anonymizeCommentsByPostAuthor(items []*domain.PcommentWithMeta, postAuthorID int) {
	for _, item := range items {
		if item.Pcomment != nil && item.Pcomment.UserID == postAuthorID {
			item.Author = &domain.User{Name: "匿名用户"}
		}
		for _, sc := range item.Subcomments {
			if sc.Ccomment != nil && sc.Ccomment.UserID == postAuthorID {
				sc.Author = &domain.User{Name: "匿名用户"}
			}
		}
	}
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

	// 「以职位身份回复」的评论对外署名是职位小号，前端按 phone 判不出本人；
	// 由服务端判定哪些评论属于当前用户（本人发的，或本人名下职位小号发的），
	// 这样开发人员能像普通用户一样删除自己的职位身份回复。
	ownShells, err := s.staff.AliasShellUserIDsForUser(ctx, viewerUserID)
	if err != nil {
		ownShells = nil // 判定失败不阻断评论列表：退化成「只认本人」
	}

	out := make([]*domain.PcommentWithMeta, 0, len(tree.Pcomments))
	for _, pc := range tree.Pcomments {
		subs, err := s.buildSubcommentsFromCache(tree.Subs[pc.ID], userMap, ccLikeMap, ccDenyMap, viewerUserID, ownShells)
		if err != nil {
			return nil, err
		}
		out = append(out, &domain.PcommentWithMeta{
			Pcomment:    pc,
			Author:      authorForComment(userMap, pc.UserID),
			Subcomments: subs,
			IsLiked:     likeMap[pc.ID],
			IsDenied:    denyMap[pc.ID],
			IsOwn:       isOwnComment(pc.UserID, viewerUserID, ownShells),
		})
	}
	return out, nil
}

// isOwnComment 这条评论对外是否属于当前查看者：本人，或本人「以职位身份」发言时的小号。
func isOwnComment(authorID, viewerUserID int, ownShells map[int]struct{}) bool {
	if authorID == viewerUserID {
		return true
	}
	if len(ownShells) == 0 {
		return false
	}
	_, ok := ownShells[authorID]
	return ok
}

func (s *commentService) buildSubcommentsFromCache(ccomments []*domain.Ccomment, userMap map[int]*domain.User, ccLikeMap, ccDenyMap map[int]bool, viewerUserID int, ownShells map[int]struct{}) ([]*domain.CcommentWithMeta, error) {
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
			IsOwn:    isOwnComment(cc.UserID, viewerUserID, ownShells),
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
			// 创建路径里 viewerUserID 就是本次发言的对外身份（本人或职位小号），
			// 这些子评论都是它发的
			IsOwn: cc.UserID == viewerUserID,
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

	// 「以职位身份回复」：被 @ 的开发者可选用职位小号对外发言（对外只见职位，不见本人）。
	authorID, effectiveAuthor, err := s.resolveEffectiveAuthor(ctx, in.UserID, in.PostID, in.AsStaffUserID, author)
	if err != nil {
		return nil, err
	}

	pc := &domain.Pcomment{
		UserID:    authorID,
		PostID:    in.PostID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := s.comments.CreatePcomment(ctx, pc); err != nil {
		return nil, err
	}

	var notifyReceiver *domain.User
	if post.UserID != in.UserID {
		// 通知的发起人一律用对外身份（职位小号），避免把真实开发者暴露给收信人
		_ = s.comments.CreateNotice(ctx, post.UserID, authorID, "pcomment", content, pc.ID)
		notifyReceiver, _ = s.comments.FindUserByID(ctx, post.UserID)
	}
	skipNotify := map[int]struct{}{in.UserID: {}, authorID: {}}
	if post.UserID != in.UserID {
		skipNotify[post.UserID] = struct{}{}
	}
	s.notifyMentions(ctx, authorID, content, "pcomment", pc.ID, skipNotify, "", 0)
	postWatchReceivers := s.notifyPostWatchers(ctx, post.ID, authorID, content, skipNotify)

	if err := s.comments.IncrPostCommentNum(ctx, post.ID, 1); err != nil {
		return nil, err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, commentWeight); err != nil {
		return nil, err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")

	// 抽奖参与是个人行为，挂在真实账号下（由 controller 判断），职位身份发言不参与

	subs, err := s.buildSubcomments(ctx, pc.ID, authorID, map[int]*domain.User{authorID: effectiveAuthor})
	if err != nil {
		return nil, err
	}
	return &domain.PcommentCreateResult{
		Pcomment:                pc,
		Author:                  effectiveAuthor,
		Subcomments:             subs,
		NotifyReceiver:          notifyReceiver,
		PostWatchEmailReceivers: postWatchReceivers,
	}, nil
}

// resolveEffectiveAuthor 解析本次发言的对外身份：
// AsStaffUserID==0 时就是本人；否则校验「以职位身份回复」并改用职位小号。
// 返回 (对外 userID, 对外 user 实体, error)。
func (s *commentService) resolveEffectiveAuthor(ctx context.Context, userID, postID, asStaffUserID int, author *domain.User) (int, *domain.User, error) {
	if asStaffUserID == 0 || asStaffUserID == userID {
		return userID, author, nil
	}
	alias, err := s.staff.AuthorizeAliasReply(ctx, userID, postID, asStaffUserID)
	if err != nil {
		return 0, nil, domain.ErrForbidden
	}
	shell, err := s.comments.FindUserByID(ctx, alias.AliasUserID)
	if err != nil || shell == nil {
		return 0, nil, domain.ErrForbidden
	}
	return alias.AliasUserID, shell, nil
}

func (s *commentService) CreateCcomment(ctx context.Context, in domain.CcommentCreateInput) (*domain.CcommentCreateResult, error) {
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

	// 「以职位身份回复」：被 @ 的开发者可选用职位小号对外发言（对外只见职位，不见本人）。
	authorID, _, err := s.resolveEffectiveAuthor(ctx, in.UserID, in.PostID, in.AsStaffUserID, author)
	if err != nil {
		return nil, err
	}

	// Resolve the entire target chain before inserting. A forged parent or
	// reply ID must not create an orphan/cross-post reply before validation.
	pc, err := s.comments.FindPcommentByID(ctx, in.PcommentID)
	if err != nil {
		return nil, err
	}
	if pc.PostID != in.PostID {
		return nil, domain.ErrBadRequest
	}
	if in.ReplyCcommentID > 0 {
		reply, err := s.comments.FindCcommentByID(ctx, in.ReplyCcommentID)
		if err != nil {
			return nil, err
		}
		if reply.PcommentID != in.PcommentID {
			return nil, domain.ErrBadRequest
		}
	}
	cc := &domain.Ccomment{
		UserID:         authorID,
		PcommentID:     in.PcommentID,
		Content:        content,
		CreatedAt:      time.Now(),
		UserTargetName: in.UserTargetName,
	}
	if err := s.comments.CreateCcomment(ctx, cc); err != nil {
		return nil, err
	}

	var notifies []*domain.CcommentNotifyInfo
	// 自查口径用对外身份：开发者以职位小号回复自己职位的一级评论时不算「回复了别人」
	if pc.UserID != in.UserID && pc.UserID != authorID {
		// 通知的发起人一律用对外身份（职位小号），避免把真实开发者暴露给收信人
		_ = s.comments.CreateNotice(ctx, pc.UserID, authorID, "ccomment", content, cc.ID)
		if receiver, err := s.comments.FindUserByID(ctx, pc.UserID); err == nil {
			notifies = append(notifies, &domain.CcommentNotifyInfo{Receiver: receiver})
		}
	}
	skipNotify := map[int]struct{}{in.UserID: {}, authorID: {}, pc.UserID: {}}
	if post.UserID != 0 {
		skipNotify[post.UserID] = struct{}{}
	}
	if in.ReplyCcommentID > 0 {
		replyCC, err := s.comments.FindCcommentByID(ctx, in.ReplyCcommentID)
		if err != nil {
			return nil, err
		}
		skipNotify[replyCC.UserID] = struct{}{}
		if replyCC.UserID != in.UserID && replyCC.UserID != authorID && pc.UserID != replyCC.UserID {
			_ = s.comments.CreateNotice(ctx, replyCC.UserID, authorID, "ccomment", content, cc.ID)
			if receiver, err := s.comments.FindUserByID(ctx, replyCC.UserID); err == nil {
				notifies = append(notifies, &domain.CcommentNotifyInfo{Receiver: receiver})
			}
		}
	}
	// 回复场景：前端携带被回复人 uid，用于正文 @昵称 同名时精确通知被回复人
	if in.TargetUserID != 0 {
		skipNotify[in.TargetUserID] = struct{}{}
	}
	preciseName, preciseUserID := "", in.TargetUserID
	if preciseUserID > 0 {
		if tu, err := s.comments.FindUserByID(ctx, preciseUserID); err == nil && tu != nil {
			preciseName = tu.Name
		}
	}
	s.notifyMentions(ctx, authorID, content, "ccomment", cc.ID, skipNotify, preciseName, preciseUserID)
	postWatchReceivers := s.notifyPostWatchers(ctx, post.ID, authorID, content, skipNotify)

	if err := s.comments.IncrPostCommentNum(ctx, post.ID, 1); err != nil {
		return nil, err
	}
	if err := s.comments.AdjustPostHeat(ctx, post.ID, commentWeight); err != nil {
		return nil, err
	}
	InvalidatePostDetail(post.ID)
	InvalidateCommentList(post.ID, "post")
	return &domain.CcommentCreateResult{
		Ccomment:                cc,
		Notifies:                notifies,
		PostWatchEmailReceivers: postWatchReceivers,
	}, nil
}

func (s *commentService) invalidateCommentListByPcommentID(ctx context.Context, pcommentID int) {
	pc, err := s.comments.FindPcommentByID(ctx, pcommentID)
	if err != nil || pc == nil || pc.PostID == 0 {
		return
	}
	InvalidateCommentList(pc.PostID, "post")
}

func (s *commentService) invalidateCommentListByCcommentID(ctx context.Context, ccommentID int) {
	cc, err := s.comments.FindCcommentByID(ctx, ccommentID)
	if err != nil || cc == nil {
		return
	}
	s.invalidateCommentListByPcommentID(ctx, cc.PcommentID)
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
		if err := s.comments.DeletePcommentLike(ctx, userID, pcommentID); err != nil {
			return err
		}
		s.invalidateCommentListByPcommentID(ctx, pcommentID)
		return nil
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
		if err := s.comments.CreatePcommentLike(ctx, userID, pcommentID); err != nil {
			return err
		}
		s.invalidateCommentListByPcommentID(ctx, pcommentID)
		return nil
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
		if err := s.comments.DeleteCcommentLike(ctx, userID, ccommentID); err != nil {
			return err
		}
		s.invalidateCommentListByCcommentID(ctx, ccommentID)
		return nil
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
		if err := s.comments.CreateCcommentLike(ctx, userID, ccommentID); err != nil {
			return err
		}
		s.invalidateCommentListByCcommentID(ctx, ccommentID)
		return nil
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

// notifyMentions 解析正文 @昵称 并向匹配用户发通知（昵称须与 name 全量相等，后接空格或行末）。
// 命中职位账号（开发人员匿名 @）时通知转发给真实开发者，小号本身不收通知。
// preciseName/preciseUserID 用于回复场景：当正文 @ 的就是被回复人昵称且重名时，只通知被回复人，不广播给其他同名者。
func (s *commentService) notifyMentions(ctx context.Context, authorID int, content, noticeType string, targetID int, skip map[int]struct{}, preciseName string, preciseUserID int) {
	tokens := util.ParseMentionTokens(content)
	if len(tokens) == 0 {
		return
	}
	users, err := s.comments.FindUsersByExactNames(ctx, tokens)
	if err != nil {
		return
	}
	usersByID := make(map[int]*domain.User, len(users))
	for _, u := range users {
		if u != nil && u.ID != 0 {
			usersByID[u.ID] = u
		}
	}
	// 职位账号：小号按普通用户踢出通知列表，改为通知其背后的真实开发者
	if s.staff != nil {
		if aliases, err := s.staff.MatchedAliasesForTokens(ctx, tokens); err == nil {
			for _, a := range aliases {
				delete(usersByID, a.AliasUserID)
				if a.StaffUserID != authorID {
					if _, ok := skip[a.StaffUserID]; !ok {
						_ = s.comments.CreateNotice(ctx, a.StaffUserID, authorID, noticeType, content, targetID)
					}
				}
				skip[a.StaffUserID] = struct{}{}
				skip[a.AliasUserID] = struct{}{}
			}
		}
	}
	for _, u := range usersByID {
		if u == nil || u.ID == 0 {
			continue
		}
		// 回复场景：该 @ 恰为被回复人昵称且用户不是被回复人本人 → 跳过，避免误发给同名者
		if preciseUserID > 0 && u.Name == preciseName && u.ID != preciseUserID {
			continue
		}
		if u.ID == authorID {
			continue
		}
		if _, ok := skip[u.ID]; ok {
			continue
		}
		_ = s.comments.CreateNotice(ctx, u.ID, authorID, noticeType, content, targetID)
		skip[u.ID] = struct{}{}
	}
}

func (s *commentService) notifyPostWatchers(ctx context.Context, postID, authorID int, content string, skip map[int]struct{}) []*domain.User {
	if postID == 0 || s.posts == nil {
		return nil
	}
	watcherIDs, err := s.posts.ListWatcherUserIDsByPostID(ctx, postID)
	if err != nil || len(watcherIDs) == 0 {
		return nil
	}
	text := content
	if utf8.RuneCountInString(text) > 200 {
		text = string([]rune(text)[:200]) + "…"
	}
	var receivers []*domain.User
	for _, uid := range watcherIDs {
		if uid == authorID {
			continue
		}
		if _, ok := skip[uid]; ok {
			continue
		}
		_ = s.comments.CreateNotice(ctx, uid, authorID, "post_watch", text, postID)
		if receiver, err := s.comments.FindUserByID(ctx, uid); err == nil && receiver != nil && receiver.ID != 0 {
			receivers = append(receivers, receiver)
		}
	}
	return receivers
}

func (s *commentService) ResolveMentionUsers(ctx context.Context, names []string) ([]*domain.User, error) {
	if len(names) == 0 {
		return nil, nil
	}
	users, err := s.comments.FindUsersByExactNames(ctx, names)
	if err != nil {
		return nil, err
	}
	// 职位账号（开发人员匿名 @）：把 @职位名 解析成职位小号（小号没建好时现场派生）。
	// 返回的是小号实体，前端 @ 渲染成链接、点进主页都是职位身份。
	if s.staff != nil {
		shells, err := s.staff.ShellsForNames(ctx, names)
		if err == nil && len(shells) > 0 {
			seen := make(map[int]struct{}, len(users))
			for _, u := range users {
				if u != nil {
					seen[u.ID] = struct{}{}
				}
			}
			for _, shell := range shells {
				if shell == nil || shell.ID == 0 {
					continue
				}
				if _, ok := seen[shell.ID]; ok {
					continue
				}
				users = append(users, shell)
			}
		}
	}
	return users, nil
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
