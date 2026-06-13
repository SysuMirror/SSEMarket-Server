package gormrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type shopRepository struct {
	db *gorm.DB
}

func NewShopRepository(db *gorm.DB) repository.ShopRepository {
	return &shopRepository{db: db}
}

func (r *shopRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *shopRepository) applyFilter(q *gorm.DB, filter domain.ProductFilter) *gorm.DB {
	q = q.Where("is_delete = ?", false)
	searchinfo := strings.TrimSpace(filter.SearchInfo)
	if filter.SearchSort == "history" {
		q = q.Where("userID = ?", filter.ViewerUserID)
	} else if searchinfo != "" {
		like := "%" + searchinfo + "%"
		q = q.Where("(product_name LIKE ? OR description LIKE ?)", like, like)
	}
	return q
}

func (r *shopRepository) Create(ctx context.Context, product *domain.Product) error {
	m := model.Product{
		UserID:      product.UserID,
		Name:        product.Name,
		Description: product.Description,
		PostTime:    product.PostTime,
		Photos:      product.Photos,
		ISSold:      product.IsSold,
		Price:       product.Price,
		ISAnonymous: product.IsAnonymous,
	}
	if err := r.session(ctx).Create(&m).Error; err != nil {
		return err
	}
	product.ID = m.ProductID
	return nil
}

func (r *shopRepository) CreateWithFeedPost(ctx context.Context, product *domain.Product, user *domain.User, feedTitle string) error {
	return r.session(ctx).Transaction(func(tx *gorm.DB) error {
		m := model.Product{
			UserID:      product.UserID,
			Name:        product.Name,
			Description: product.Description,
			PostTime:    product.PostTime,
			Photos:      product.Photos,
			ISSold:      product.IsSold,
			Price:       product.Price,
			ISAnonymous: product.IsAnonymous,
		}
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		product.ID = m.ProductID
		feedContent := productFeedPostContent(product)
		if err := common.CreateHomeFeedPost(tx, userToModel(user), feedTitle, feedContent); err != nil {
			return domain.ErrBadRequest
		}
		return nil
	})
}

func productFeedPostContent(product *domain.Product) string {
	link := fmt.Sprintf("%s/shop/productdetail/%d", common.NewFrontendBaseURL, product.ID)
	var b strings.Builder
	b.WriteString("商城有新商品上架「")
	b.WriteString(product.Name)
	b.WriteString(fmt.Sprintf("」，标价 ¥%d。\n\n", product.Price))
	b.WriteString("商品链接：\n")
	b.WriteString(link)
	b.WriteString("\n")
	desc := strings.TrimSpace(product.Description)
	if desc != "" {
		if utf8.RuneCountInString(desc) > 400 {
			runes := []rune(desc)
			desc = string(runes[:400]) + "…"
		}
		b.WriteString("\n")
		b.WriteString(desc)
		b.WriteString("\n")
	}
	return b.String()
}

func (r *shopRepository) FindByID(ctx context.Context, id int) (*domain.Product, error) {
	var m model.Product
	err := r.session(ctx).Where("productID = ? AND is_delete = ?", id, false).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return productToDomain(&m), nil
}

func (r *shopRepository) FindRawByID(ctx context.Context, id int) (*domain.Product, error) {
	var m model.Product
	err := r.session(ctx).Where("productID = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return productToDomain(&m), nil
}

func (r *shopRepository) List(ctx context.Context, filter domain.ProductFilter) ([]*domain.Product, error) {
	q := r.applyFilter(r.session(ctx).Model(&model.Product{}), filter)
	limit, offset := filter.Limit, filter.Offset
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var list []model.Product
	err := q.Order("is_sold ASC, productID DESC").Offset(offset).Limit(limit).Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Product, 0, len(list))
	for i := range list {
		out = append(out, productToDomain(&list[i]))
	}
	return out, nil
}

func (r *shopRepository) Count(ctx context.Context, filter domain.ProductFilter) (int64, error) {
	var n int64
	q := r.applyFilter(r.session(ctx).Model(&model.Product{}), filter)
	err := q.Count(&n).Error
	return n, err
}

func (r *shopRepository) SoftDelete(ctx context.Context, id int) error {
	res := r.session(ctx).Model(&model.Product{}).Where("productID = ?", id).Update("is_delete", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *shopRepository) MarkSold(ctx context.Context, id int) error {
	res := r.session(ctx).Model(&model.Product{}).Where("productID = ?", id).Update("is_sold", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *shopRepository) ListCarouselCandidates(ctx context.Context, limit int) ([]*domain.Product, error) {
	var list []model.Product
	err := r.session(ctx).
		Where("is_delete = ? AND is_sold = ?", false, false).
		Where("photos != ?", "").
		Order("productID DESC").
		Limit(limit).
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Product, 0, len(list))
	for i := range list {
		out = append(out, productToDomain(&list[i]))
	}
	return out, nil
}
