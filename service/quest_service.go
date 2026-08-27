package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/repository"
)

type QuestService interface {
	List(ctx context.Context, filter domain.QuestListFilter) ([]domain.QuestPostItem, int64, error)
	Detail(ctx context.Context, postID, viewerUserID int) (*domain.QuestPostItem, error)
	DetailFull(ctx context.Context, postID, viewerUserID int) (*domain.QuestPostItem, error)
	Create(ctx context.Context, in domain.QuestPostCreateInput) (*domain.QuestPostItem, error)
	UpdateStatus(ctx context.Context, postID, userID int, status string) error
	Me(ctx context.Context, userID int) (*domain.QuestMeInfo, error)
	LevelRules(ctx context.Context, userID int) (*domain.QuestLevelInfo, error)
	AcceptOrder(ctx context.Context, postID, workerID int) (*domain.QuestOrderSummary, error)
	SubmitComplete(ctx context.Context, postID, workerID int, note string) error
	ConfirmComplete(ctx context.Context, postID, publisherID int) error
}

type questService struct {
	quest repository.QuestRepository
	posts repository.PostRepository
	users repository.UserRepository
}

func NewQuestService(quest repository.QuestRepository, posts repository.PostRepository, users repository.UserRepository) QuestService {
	return &questService{quest: quest, posts: posts, users: users}
}

func (s *questService) canQuestPost(user *domain.User) bool {
	if user == nil || user.ID <= 0 {
		return false
	}
	if config.QuestTestAllowAllPost() {
		return true
	}
	if user.UserType == "external" {
		return true
	}
	for _, id := range config.QuestExternalUserIDs() {
		if id == user.ID {
			return true
		}
	}
	return false
}

func (s *questService) Me(ctx context.Context, userID int) (*domain.QuestMeInfo, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, domain.ErrNotFound
	}
	ext := user.UserType == "external"
	for _, id := range config.QuestExternalUserIDs() {
		if id == user.ID {
			ext = true
			break
		}
	}
	return &domain.QuestMeInfo{
		UserID:     user.ID,
		Name:       user.Name,
		Phone:      user.Phone,
		AvatarURL:  user.AvatarURL,
		UserType:   user.UserType,
		Score:      user.Score,
		CanPost:    s.canQuestPost(user),
		IsExternal: ext,
	}, nil
}

func (s *questService) LevelRules(ctx context.Context, userID int) (*domain.QuestLevelInfo, error) {
	score := 0
	if userID > 0 {
		u, err := s.users.FindByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		if u != nil {
			score = u.Score
		}
	}
	lv, next, need := domain.QuestLevelForScore(score)
	return &domain.QuestLevelInfo{
		Score:      score,
		Level:      lv,
		NextLevel:  next,
		NeedPoints: need,
		Rules:      questMarketScoreRules(),
	}, nil
}

func questMarketScoreRules() []domain.QuestScoreRule {
	return []domain.QuestScoreRule{
		{Action: "发布一条帖子", Points: 10},
		{Action: "发表一条评论", Points: 5},
		{Action: "帖子/评论获得点赞", Points: 1},
		{Action: "帖子被收藏", Points: 3},
		{Action: "帖子被评论或回复", Points: 2},
		{Action: "申诉成功", Points: 20},
		{Action: "受到处罚", Points: -20},
	}
}

func (s *questService) List(ctx context.Context, filter domain.QuestListFilter) ([]domain.QuestPostItem, int64, error) {
	posts, total, err := s.quest.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.QuestPostItem, 0, len(posts))
	for _, p := range posts {
		item, err := s.toItem(ctx, p, filter.ViewerUserID, true)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, nil
}

func (s *questService) Detail(ctx context.Context, postID, viewerUserID int) (*domain.QuestPostItem, error) {
	return s.detail(ctx, postID, viewerUserID, true)
}

func (s *questService) DetailFull(ctx context.Context, postID, viewerUserID int) (*domain.QuestPostItem, error) {
	return s.detail(ctx, postID, viewerUserID, false)
}

func (s *questService) detail(ctx context.Context, postID, viewerUserID int, truncate bool) (*domain.QuestPostItem, error) {
	post, err := s.quest.GetQuestPost(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post.IsPrivate && post.UserID != viewerUserID {
		return nil, domain.ErrNotFound
	}
	return s.toItem(ctx, post, viewerUserID, truncate)
}

func (s *questService) Create(ctx context.Context, in domain.QuestPostCreateInput) (*domain.QuestPostItem, error) {
	user, err := s.users.FindByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if !s.canQuestPost(user) {
		return nil, domain.ErrForbidden
	}
	if user.BanEnd.After(time.Now()) {
		return nil, domain.ErrForbidden
	}
	if err := validateQuestCreate(in); err != nil {
		return nil, err
	}

	post, err := s.quest.CreateQuestPost(ctx, in)
	if err != nil {
		return nil, err
	}
	common.AddPostID(post.ID)
	InvalidateHomeBrowse()
	return s.toItem(ctx, post, in.UserID, false)
}

func (s *questService) UpdateStatus(ctx context.Context, postID, userID int, status string) error {
	return s.quest.UpdateQuestStatusByAuthor(ctx, postID, userID, status)
}

func (s *questService) AcceptOrder(ctx context.Context, postID, workerID int) (*domain.QuestOrderSummary, error) {
	post, err := s.quest.GetQuestPost(ctx, postID)
	if err != nil {
		return nil, err
	}
	if post.UserID == workerID {
		return nil, domain.ErrBadRequest
	}
	status := effectiveQuestStatus(post)
	if status != domain.QuestStatusOpen {
		return nil, domain.ErrBadRequest
	}
	existing, err := s.quest.GetOrderByPostID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrBadRequest
	}
	now := time.Now()
	order := &domain.QuestOrder{
		PostID:      postID,
		PublisherID: post.UserID,
		WorkerID:    workerID,
		Status:      domain.QuestOrderAccepted,
		AcceptedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.quest.CreateOrder(ctx, order); err != nil {
		return nil, err
	}
	if err := s.quest.UpdateQuestStatus(ctx, postID, domain.QuestStatusInProgress); err != nil {
		return nil, err
	}
	return s.orderSummary(ctx, order)
}

func (s *questService) SubmitComplete(ctx context.Context, postID, workerID int, note string) error {
	order, err := s.quest.GetOrderByPostID(ctx, postID)
	if err != nil {
		return err
	}
	if order == nil || order.WorkerID != workerID {
		return domain.ErrForbidden
	}
	if order.Status != domain.QuestOrderAccepted {
		return domain.ErrBadRequest
	}
	now := time.Now()
	order.Status = domain.QuestOrderSubmitted
	order.SubmittedAt = &now
	order.SubmitNote = strings.TrimSpace(note)
	order.UpdatedAt = now
	if err := s.quest.UpdateOrder(ctx, order); err != nil {
		return err
	}
	return s.quest.UpdateQuestStatus(ctx, postID, domain.QuestStatusPendingConfirm)
}

func (s *questService) ConfirmComplete(ctx context.Context, postID, publisherID int) error {
	post, err := s.quest.GetQuestPost(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != publisherID {
		return domain.ErrForbidden
	}
	order, err := s.quest.GetOrderByPostID(ctx, postID)
	if err != nil {
		return err
	}
	if order == nil || order.Status != domain.QuestOrderSubmitted {
		return domain.ErrBadRequest
	}
	now := time.Now()
	order.Status = domain.QuestOrderCompleted
	order.CompletedAt = &now
	order.UpdatedAt = now
	if err := s.quest.UpdateOrder(ctx, order); err != nil {
		return err
	}
	return s.quest.UpdateQuestStatus(ctx, postID, domain.QuestStatusCompleted)
}

func validateQuestCreate(in domain.QuestPostCreateInput) error {
	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if title == "" || content == "" {
		return domain.ErrBadRequest
	}
	if utf8.RuneCountInString(title) > 30 {
		return domain.ErrBadRequest
	}
	if utf8.RuneCountInString(content) > 5000 {
		return domain.ErrBadRequest
	}
	kind := strings.TrimSpace(in.PostKind)
	if !domain.IsQuestPostKind(kind) {
		return domain.ErrBadRequest
	}
	priceType := in.PriceType
	if priceType == "" {
		priceType = domain.QuestPriceNegotiable
	}
	if !domain.IsValidQuestPriceType(priceType) {
		return domain.ErrBadRequest
	}
	if kind == domain.QuestPostKindInquiry {
		if priceType != domain.QuestPriceNegotiable || in.QuotePrice != nil || in.QuotePriceMin != nil || in.QuotePriceMax != nil {
			return domain.ErrBadRequest
		}
		return nil
	}
	switch priceType {
	case domain.QuestPriceFixed:
		if in.QuotePrice == nil || *in.QuotePrice < 0 {
			return domain.ErrBadRequest
		}
	case domain.QuestPriceRange:
		if in.QuotePriceMin == nil || in.QuotePriceMax == nil || *in.QuotePriceMin < 0 || *in.QuotePriceMax < *in.QuotePriceMin {
			return domain.ErrBadRequest
		}
	case domain.QuestPriceNegotiable:
		if in.QuotePrice != nil || in.QuotePriceMin != nil || in.QuotePriceMax != nil {
			return domain.ErrBadRequest
		}
	}
	if len(in.Attachments) > 9 {
		return domain.ErrBadRequest
	}
	return nil
}

func effectiveQuestStatus(post *domain.Post) string {
	if post == nil {
		return domain.QuestStatusOpen
	}
	if post.QuestStatus != "" {
		return post.QuestStatus
	}
	return domain.QuestStatusOpen
}

func parseQuestAttachments(raw string) []domain.QuestAttachment {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []domain.QuestAttachment
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (s *questService) orderSummary(ctx context.Context, order *domain.QuestOrder) (*domain.QuestOrderSummary, error) {
	if order == nil {
		return nil, nil
	}
	sum := &domain.QuestOrderSummary{
		OrderID:     order.ID,
		PostID:      order.PostID,
		PublisherID: order.PublisherID,
		WorkerID:    order.WorkerID,
		Status:      order.Status,
		AcceptedAt:  order.AcceptedAt,
		SubmittedAt: order.SubmittedAt,
		CompletedAt: order.CompletedAt,
		SubmitNote:  order.SubmitNote,
	}
	w, err := s.users.FindByID(ctx, order.WorkerID)
	if err == nil && w != nil {
		sum.WorkerName = w.Name
		sum.WorkerAvatar = w.AvatarURL
	}
	return sum, nil
}

func (s *questService) toItem(ctx context.Context, post *domain.Post, viewerUserID int, truncate bool) (*domain.QuestPostItem, error) {
	if post == nil {
		return nil, errors.New("nil post")
	}
	author, err := s.users.FindByID(ctx, post.UserID)
	if err != nil {
		return nil, err
	}
	name, avatar, score := "", "", 0
	if author != nil {
		name = author.Name
		avatar = author.AvatarURL
		score = author.Score
	}
	content := post.Content
	if truncate && utf8.RuneCountInString(content) > 120 {
		content = string([]rune(content)[:120]) + "…"
	}
	status := effectiveQuestStatus(post)
	priceType := post.PriceType
	if priceType == "" {
		priceType = domain.QuestPriceNegotiable
	}
	isSaved := false
	if viewerUserID > 0 {
		isSaved, _ = s.posts.IsSaved(ctx, viewerUserID, post.ID)
	}
	order, err := s.quest.GetOrderByPostID(ctx, post.ID)
	if err != nil {
		return nil, err
	}
	var orderSum *domain.QuestOrderSummary
	if order != nil {
		orderSum, err = s.orderSummary(ctx, order)
		if err != nil {
			return nil, err
		}
	}
	return &domain.QuestPostItem{
		PostID:         post.ID,
		PostKind:       post.PostKind,
		QuestStatus:    status,
		Title:          post.Title,
		Content:        content,
		PriceType:      priceType,
		QuotePrice:     post.QuotePrice,
		QuotePriceMin:  post.QuotePriceMin,
		QuotePriceMax:  post.QuotePriceMax,
		QuestDeadline:  post.QuestDeadline,
		Attachments:    parseQuestAttachments(post.QuestAttachments),
		CommentNum:     post.CommentNum,
		PostTime:       post.PostTime,
		AuthorID:       post.UserID,
		AuthorName:     name,
		AuthorAvatar:   avatar,
		AuthorScore:    score,
		IsSaved:        isSaved,
		Order:          orderSum,
	}, nil
}
