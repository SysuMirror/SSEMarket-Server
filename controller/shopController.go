package controller

import (
	"errors"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type ProductMsg struct {
	UserID       int      `json:"UserID"`
	Price        int      `json:"Price"`
	Title        string   `json:"Title"`
	Content      string   `json:"Content"`
	Photos       []string `json:"Photos"`
	ISAnonymous  bool     `json:"ISAnonymous"`
	SyncFeedPost *bool    `json:"syncFeedPost"`
}

type ProductReponse struct {
	SellerID    int       `json:"SellerID"`
	ProductID   int       `json:"ProductID"`
	Seller      string    `json:"Seller"`
	Price       int       `json:"Price"`
	Name        string    `json:"Name"`
	Description string    `json:"Description"`
	Photos      []string  `json:"Photos"`
	ISSold      bool      `json:"ISSold"`
	ISAnonymous bool      `json:"ISAnonymous"`
	PostTime    time.Time `json:"PostTime"`
}

type ShopBrowseMeg struct {
	UserID     int    `json:"UserID"`
	Searchinfo string `json:"searchinfo"`
	Searchsort string `json:"searchsort"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type ProductIDmsg struct {
	ProductID int `json:"productID"`
}

func shopBrowseFilter(msg ShopBrowseMeg, viewerID int) domain.ProductFilter {
	return domain.ProductFilter{
		ViewerUserID: viewerID,
		SearchInfo:   msg.Searchinfo,
		SearchSort:   msg.Searchsort,
		Limit:        msg.Limit,
		Offset:       msg.Offset,
	}
}

func productToResponse(product *domain.Product, seller *domain.User, viewerID int, viewerIsOwner bool) ProductReponse {
	photos := splitProductPhotos(product.Photos)
	sellerID, sellerName := 0, "匿名卖家"
	if !product.IsAnonymous || viewerIsOwner {
		if seller != nil && seller.ID != 0 {
			sellerID = seller.ID
			sellerName = seller.Name
		}
	}
	return ProductReponse{
		ProductID:   product.ID,
		SellerID:    sellerID,
		Seller:      sellerName,
		Price:       product.Price,
		Name:        product.Name,
		Description: product.Description,
		Photos:      photos,
		ISSold:      product.IsSold,
		ISAnonymous: product.IsAnonymous,
		PostTime:    product.PostTime,
	}
}

func PostProduct(c *gin.Context) {
	var msg ProductMsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != msg.UserID {
		response.Response(c, http.StatusUnprocessableEntity, 403, nil, "权限不足")
		return
	}
	syncFeed := true
	if msg.SyncFeedPost != nil {
		syncFeed = *msg.SyncFeedPost
	}
	id, err := service.Shops.Create(c.Request.Context(), domain.ProductCreateInput{
		UserID:       msg.UserID,
		Title:        msg.Title,
		Content:      msg.Content,
		Photos:       msg.Photos,
		Price:        msg.Price,
		IsAnonymous:  msg.ISAnonymous,
		SyncFeedPost: syncFeed,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, shopCreateMsg(msg))
		default:
			apperr.WriteMsg(c, err, "发布失败")
		}
		return
	}
	response.Response(c, http.StatusOK, 200, gin.H{"ProductID": id}, "商品发布成功")
}

func shopCreateMsg(msg ProductMsg) string {
	title := strings.TrimSpace(msg.Title)
	content := strings.TrimSpace(msg.Content)
	if title == "" {
		return "标题不能为空"
	}
	if utf8.RuneCountInString(title) > 30 {
		return "标题最多为30个字"
	}
	if content == "" {
		return "内容不能为空"
	}
	if utf8.RuneCountInString(content) > 2000 {
		return "描述最多2000字"
	}
	if msg.Price < 0 || msg.Price > 1000000 {
		return "价格无效"
	}
	return "用户不存在"
}

func GetProducts(c *gin.Context) {
	var msg ShopBrowseMeg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	ctx := c.Request.Context()
	tokenUserID := GetTokenUserID(c)
	filter := shopBrowseFilter(msg, tokenUserID)
	products, err := service.Shops.List(ctx, filter)
	if err != nil {
		apperr.Write(c, err)
		return
	}
	out := make([]ProductReponse, 0, len(products))
	for _, p := range products {
		seller, _ := service.Shops.SellerInfo(ctx, p.UserID)
		out = append(out, productToResponse(p, seller, tokenUserID, msg.Searchsort == "history"))
	}
	c.JSON(http.StatusOK, out)
}

func GetProductDetail(c *gin.Context) {
	var msg ProductIDmsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	ctx := c.Request.Context()
	product, err := service.Shops.GetByID(ctx, msg.ProductID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "商品不存在")
			return
		}
		apperr.Write(c, err)
		return
	}
	tokenUserID := GetTokenUserID(c)
	seller, _ := service.Shops.SellerInfo(ctx, product.UserID)
	c.JSON(http.StatusOK, productToResponse(product, seller, tokenUserID, product.UserID == tokenUserID))
}

func GetProductNum(c *gin.Context) {
	var msg ShopBrowseMeg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	count, err := service.Shops.Count(c.Request.Context(), shopBrowseFilter(msg, GetTokenUserID(c)))
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"Productcount": count})
}

func DeleteProduct(c *gin.Context) {
	var msg ProductIDmsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	err := service.Shops.Delete(c.Request.Context(), msg.ProductID, GetTokenUserID(c))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusBadRequest, 400, nil, "商品不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusUnprocessableEntity, 403, nil, "权限不足")
		default:
			apperr.Write(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "商品删除成功"})
}

func SaleProduct(c *gin.Context) {
	var msg ProductIDmsg
	if err := c.ShouldBindBodyWith(&msg, binding.JSON); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	err := service.Shops.MarkSold(c.Request.Context(), msg.ProductID, GetTokenUserID(c))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusBadRequest, 400, nil, "商品不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusUnprocessableEntity, 403, nil, "权限不足")
		default:
			apperr.Write(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "商品出售成功"})
}

func GetCarouselImg(c *gin.Context) {
	urls, err := service.Shops.CarouselPhotos(c.Request.Context())
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, urls)
}

func splitProductPhotos(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, replaceResizedWithURL(p))
	}
	return out
}

func replaceResizedWithURL(photoURL string) string {
	return strings.ReplaceAll(photoURL, "/resized/", "/uploads/")
}
