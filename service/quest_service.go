package service

import (
	"context"
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
	return s.quest.UpdateQuestStatus(ctx, postID, userID, status)
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
	if kind == domain.QuestPostKindInquiry && in.QuotePrice != nil {
		return domain.ErrBadRequest
	}
	if kind == domain.QuestPostKindBounty && in.QuotePrice != nil && *in.QuotePrice < 0 {
		return domain.ErrBadRequest
	}
	return nil
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
	status := post.QuestStatus
	if status == "" {
		if post.PostKind == domain.QuestPostKindBounty {
			status = domain.QuestStatusInProgress
		} else {
			status = domain.QuestStatusOpen
		}
	}
	isSaved := false
	if viewerUserID > 0 {
		isSaved, _ = s.posts.IsSaved(ctx, viewerUserID, post.ID)
	}
	return &domain.QuestPostItem{
		PostID:       post.ID,
		PostKind:     post.PostKind,
		QuestStatus:  status,
		Title:        post.Title,
		Content:      content,
		QuotePrice:   post.QuotePrice,
		CommentNum:   post.CommentNum,
		PostTime:     post.PostTime,
		AuthorID:     post.UserID,
		AuthorName:   name,
		AuthorAvatar: avatar,
		AuthorScore:  score,
		IsSaved:      isSaved,
	}, nil
}
