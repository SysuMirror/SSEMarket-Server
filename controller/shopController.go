package controller

import (
	"fmt"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
	"net/http"
	"time"
	"unicode/utf8"
	"strings"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

type ProductMsg struct {
	UserID      int
	Price       int
	Title       string
	Content     string
	Photos      []string
	ISAnonymous bool
}

type ProductReponse struct {
	SellerID    int
	ProductID   int
	Seller      string
	Price       int
	Name        string
	Description string
	Photos      []string
	ISAnonymous bool
}

type ShopBrowseMeg struct {
	UserID     int
	Searchinfo string
	Searchsort string //用于分表查询 分为home,history两种
	Limit      int
	Offset     int
}

type ProductIDmsg struct {
	ProductID int
}

func PostProduct(c *gin.Context) {
	db := common.GetDB()
	var requestPostMsg ProductMsg
	c.Bind(&requestPostMsg)
	fmt.Printf("接收到的请求数据: %+v\n", requestPostMsg)
	userID := requestPostMsg.UserID
	price := requestPostMsg.Price
	title := requestPostMsg.Title
	content := requestPostMsg.Content
	photos := requestPostMsg.Photos
	isAnonymous := requestPostMsg.ISAnonymous
	// 验证数据
	if !isUserExist(db, userID) {
		response.Response(c, http.StatusBadRequest, 400, nil, "用户不存在")
		return
	}
	if len(title) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "标题不能为空")
		return
	}
	// 这里不能直接用len()否则会出现中文字符的计算错误
	if utf8.RuneCountInString(title) > 30 {
		response.Response(c, http.StatusBadRequest, 400, nil, "标题最多为30个字")
		return
	}

	if len(content) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "内容不能为空")
		return
	}

	if utf8.RuneCountInString(content) > 10000 {
		response.Response(c, http.StatusBadRequest, 400, nil, "内容最多为10000个字")
		return
	}

	var user model.User
	db.Where("userID = ?", userID).First(&user)

	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.UserID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	newProduct := model.Product{
		UserID:      int(user.UserID),
		Name:        title,
		Description: content,
		PostTime:    time.Now(),
		Photos:      strings.Join(photos, ","),
		ISSold:      false,
		ISAnonymous: isAnonymous,
		Price:       price,
	}
	if err := db.Create(&newProduct).Error; err != nil {
		fmt.Printf("插入数据库时出错: %v\n", err)
		response.Response(c, http.StatusInternalServerError, 500, nil, "数据库插入失败")
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "商品发布成功")
}

func GetProducts(c *gin.Context) {
	db := common.GetDB()
	var requestPostMsg ShopBrowseMeg
	c.Bind(&requestPostMsg)
	userID := GetTokenUserID(c)
	searchinfo := requestPostMsg.Searchinfo
	searchsort := requestPostMsg.Searchsort
	limit := requestPostMsg.Limit
	offset := requestPostMsg.Offset

	var user model.User
	db.Where("userID = ?", userID).First(&user)
	if user.UserID == 0 {
		fmt.Println("requestPostMsg:", requestPostMsg)
		c.JSON(400, "未找到该用户")
		return
	}
	var products []model.Product
	var responses []ProductReponse
	if searchsort == "home" {
		if len(searchinfo) == 0 {
			db.Order("productID DESC").Offset(offset).Limit(limit).
				Where("is_delete = ?", false).
				Find(&products)
		} else {
			db.Order("productID DESC").Offset(offset).Limit(limit).
				Where("(title LIKE ? OR ptext LIKE ? )", "%"+searchinfo+"%", "%"+searchinfo+"%").
				Where("is_delete = ?", false).
				Find(&products)
		}
	} else {
		db.Order("productID DESC").Offset(offset).Limit(limit).
			Where("is_delete = ?", false).
			Where("userID = ?", user.UserID)
			Find(&products)
	}
	for _, product := range products {
		photo := strings.Split(product.Photos, ",")
		for i, photoURL := range photo {
			photo[i] = replaceResizedWithURL(photoURL)
		}
		response := ProductReponse{
			ProductID:   product.ProductID,
			Seller:      user.Name,
			Price:       product.Price,
			Name:        product.Name,
			Description: product.Description,
			Photos:      photo,
			ISAnonymous: product.ISAnonymous,
		}
		responses = append(responses, response)
	}

	c.JSON(http.StatusOK, responses)
}

func GetProductDetail(c *gin.Context) {
	db := common.GetDB()
	var requestMsg ProductIDmsg
	c.Bind(&requestMsg)
	var product model.Product
	db.Where("productID = ?", requestMsg.ProductID).First(&product)
	var user model.User
	db.Where("userID = ?", product.UserID).First(&user)
	photo := strings.Split(product.Photos, ",")
	for i, photoURL := range photo {
        photo[i] = replaceResizedWithURL(photoURL)
    }
	response := ProductReponse{
		ProductID:   product.ProductID,
		SellerID:    user.UserID,
		Seller:      user.Name,
		Price:       product.Price,
		Name:        product.Name,
		Description: product.Description,
		Photos:      photo,
		ISAnonymous: product.ISAnonymous,
	}
	c.JSON(http.StatusOK, response)
}

func GetProductNum(c *gin.Context) {
	db := common.GetDB()
	var requestPostMsg ShopBrowseMeg
	c.Bind(&requestPostMsg)
	userID := requestPostMsg.UserID
	searchinfo := requestPostMsg.Searchinfo
	searchsort := requestPostMsg.Searchsort
	var count int
	var user model.User
	db.Where("userID = ?", userID).First(&user)
	if user.UserID == 0 {
		return
	}

	if searchsort == "home" {
		if len(searchinfo) == 0 {
			db.Model(&model.Product{}).Where("is_delete = ?", false).Count(&count)
		} else {
			db.Model(&model.Product{}).
				Where("(title LIKE ? OR ptext LIKE ? )", "%"+searchinfo+"%", "%"+searchinfo+"%").
				Where("is_delete = ?", false).
				Count(&count)
		}
	} else {
		db.Model(&model.Product{}).
			Where("userID = ?", userID).
			Where("is_delete = ?", false).
			Count(&count)
	}

	c.JSON(http.StatusOK, gin.H{
		"Productcount": count,
	})
}

func DeleteProduct(c *gin.Context) {
	db := common.GetDB()
	var productID ProductIDmsg
	c.Bind(&productID)
	var product model.Product
	db.Where("productID = ?", productID.ProductID).First(&product)
	if product.ProductID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "需要出售商品不存在")
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != product.UserID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	db.Model(&product).Update("is_delete", true)
	c.JSON(http.StatusOK, gin.H{"message": "商品删除成功"})
}

func SaleProduct(c *gin.Context) {
	db := common.GetDB()
	var productID ProductIDmsg
	c.Bind(&productID)
	var product model.Product
	db.Where("productID = ?", productID).First(&product)
	if product.ProductID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "需要删除的商品不存在")
		return
	}
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != product.UserID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	db.Model(&product).Update("is_sold", true)
	c.JSON(http.StatusOK, gin.H{"message": "商品出售成功"})
}

func GetCarouselImg(c *gin.Context) {
	db := common.GetDB()
	var products []model.Product
	db.Order("productID desc").Where("is_delete = ?", false).Limit(5).Find(&products)
	var CarouselImg string
	//懒得写了
	c.JSON(http.StatusOK, CarouselImg)
}

func isUserExist(db *gorm.DB, user_id int) bool {
	var user model.User
	db.Where("userID = ?", user_id).First(&user)
	if user.UserID != 0 {
		return true
	}
	return false
}

func replaceResizedWithURL(photoURL string) string {
    return strings.ReplaceAll(photoURL, "/resized/", "/uploads/")
}