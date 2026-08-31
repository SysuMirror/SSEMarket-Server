package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/repository"
)

type shopService struct {
	shops repository.ShopRepository
	users repository.UserRepository
}

func NewShopService(shops repository.ShopRepository, users repository.UserRepository) ShopService {
	return &shopService{shops: shops, users: users}
}

func (s *shopService) Create(ctx context.Context, in domain.ProductCreateInput) (int, error) {
	if _, err := s.users.FindByID(ctx, in.UserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return 0, domain.ErrBadRequest
		}
		return 0, err
	}
	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if title == "" || content == "" {
		return 0, domain.ErrBadRequest
	}
	if utf8.RuneCountInString(title) > 30 || utf8.RuneCountInString(content) > 2000 {
		return 0, domain.ErrBadRequest
	}
	if in.Price < 0 || in.Price > 1000000 {
		return 0, domain.ErrBadRequest
	}
	photos := make([]string, 0, len(in.Photos))
	for _, p := range in.Photos {
		if t := strings.TrimSpace(p); t != "" {
			photos = append(photos, t)
		}
	}
	product := &domain.Product{
		UserID:      in.UserID,
		Name:        title,
		Description: content,
		PostTime:    time.Now(),
		Photos:      strings.Join(photos, ","),
		IsSold:      false,
		Price:       in.Price,
		IsAnonymous: in.IsAnonymous,
	}
	if in.SyncFeedPost {
		user, err := s.users.FindByID(ctx, in.UserID)
		if err != nil {
			return 0, err
		}
		feedTitle := common.FeedTitleWithPrefix("新上架-", product.Name)
		if err := s.shops.CreateWithFeedPost(ctx, product, user, feedTitle); err != nil {
			return 0, err
		}
	} else if err := s.shops.Create(ctx, product); err != nil {
		return 0, err
	}
	common.BloomFilter.Add([]byte(strconv.FormatUint(uint64(product.ID), 10)))
	return product.ID, nil
}

func (s *shopService) List(ctx context.Context, filter domain.ProductFilter) ([]*domain.Product, error) {
	return s.shops.List(ctx, filter)
}

func (s *shopService) Count(ctx context.Context, filter domain.ProductFilter) (int64, error) {
	return s.shops.Count(ctx, filter)
}

func (s *shopService) GetByID(ctx context.Context, id int) (*domain.Product, error) {
	return s.shops.FindByID(ctx, id)
}

func (s *shopService) GetRawByID(ctx context.Context, id int) (*domain.Product, error) {
	return s.shops.FindRawByID(ctx, id)
}

func (s *shopService) Delete(ctx context.Context, id, userID int) error {
	p, err := s.shops.FindRawByID(ctx, id)
	if err != nil {
		return err
	}
	if p.IsDelete {
		return domain.ErrNotFound
	}
	if p.UserID != userID {
		return domain.ErrForbidden
	}
	return s.shops.SoftDelete(ctx, id)
}

func (s *shopService) MarkSold(ctx context.Context, id, userID int) error {
	p, err := s.shops.FindRawByID(ctx, id)
	if err != nil {
		return err
	}
	if p.IsDelete {
		return domain.ErrNotFound
	}
	if p.UserID != userID {
		return domain.ErrForbidden
	}
	return s.shops.MarkSold(ctx, id)
}

func (s *shopService) CarouselPhotos(ctx context.Context) ([]string, error) {
	products, err := s.shops.ListCarouselCandidates(ctx, 8)
	if err != nil {
		return nil, err
	}
	urls := make([]string, 0, 5)
	for _, p := range products {
		for _, u := range strings.Split(p.Photos, ",") {
			u = strings.TrimSpace(u)
			if u == "" {
				continue
			}
			urls = append(urls, strings.ReplaceAll(u, "/resized/", "/uploads/"))
			break
		}
		if len(urls) >= 5 {
			break
		}
	}
	return urls, nil
}

func (s *shopService) SellerInfo(ctx context.Context, userID int) (*domain.User, error) {
	return s.users.FindByID(ctx, userID)
}

