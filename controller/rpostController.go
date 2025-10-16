package controller

import (
	"fmt"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RbrowseMeg struct {
	UserTelephone string
	Partition     string
	Searchinfo    string
	Tag           string
	Searchsort    string //用于分表查询 分为home,history,save三种
	Limit         int
	Offset        int
}

type RatingMsg struct {
	UserTelephone string
	PostID        uint
	Rating        int
}

func UpdateRating(c *gin.Context) {
	db := common.GetDB()
	var requestRatingMsg RatingMsg
	c.Bind(&requestRatingMsg)
	userTelephone := requestRatingMsg.UserTelephone
	postID := requestRatingMsg.PostID
	rating := requestRatingMsg.Rating

	// Find the user by telephone
	var user model.User
	db.Where("phone = ?", userTelephone).First(&user)
	if user.UserID == 0 {
		return
	}
	var post model.Post
	// 获取token中的用户标识符
	tokenUserID := GetTokenUserID(c)
	if tokenUserID != user.UserID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	db.Where("postID = ?", postID).First(&post)
	if post.PostID == 0 {
		return
	}

	var rstar model.Rstar

	err := db.Model(&model.Rstar{}).
		Where("postID = ? AND userID = ?", post.PostID, user.UserID).
		First(&rstar).Error
	if err != nil {
		fmt.Println(err)
		rstar = model.Rstar{
			PostID:  post.PostID,
			UserID:  user.UserID,
			StarNum: rating,
		}

		db.Create(&rstar)
	} else {
		db.Model(&rstar).Update("star_num", rating)
	}
	response.Response(c, http.StatusOK, 200, nil, "评分成功")
}

type RatingGetMsg struct {
	PostID uint
}

func GetAverageRating(c *gin.Context) {
	var requestRatingMsg RatingGetMsg
	c.Bind(&requestRatingMsg)
	postID := requestRatingMsg.PostID
	if postID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "需要获取的帖子不存在")
	}
	// 计算平均评分
	averageRating := getAverageRating(postID)
	// 格式化平均评分，保留两位小数
	formattedRating := fmt.Sprintf("%.2f", averageRating)
	// 将格式化后的字符串转换为 float64 类型
	var averageRatingFloat float64
	_, err := fmt.Sscanf(formattedRating, "%f", &averageRatingFloat)
	if err != nil {
		// 处理转换错误
	}
	c.JSON(http.StatusOK, gin.H{
		"averageRating": averageRatingFloat,
	})
}

func GetStars(c *gin.Context) {
	var requestRatingMsg RatingGetMsg
	c.Bind(&requestRatingMsg)
	postID := requestRatingMsg.PostID
	db := common.GetDB()
	var star1 int
	db.Model(&model.Rstar{}).Where("postID = ? AND star_num = ?", postID, 1).Count(&star1)
	var star2 int
	db.Model(&model.Rstar{}).Where("postID = ? AND star_num = ?", postID, 2).Count(&star2)
	var star3 int
	db.Model(&model.Rstar{}).Where("postID = ? AND star_num = ?", postID, 3).Count(&star3)
	var star4 int
	db.Model(&model.Rstar{}).Where("postID = ? AND star_num = ?", postID, 4).Count(&star4)
	var star5 int
	db.Model(&model.Rstar{}).Where("postID = ? AND star_num = ?", postID, 5).Count(&star5)
	c.JSON(http.StatusOK, gin.H{
		"star1": star1,
		"star2": star2,
		"star3": star3,
		"star4": star4,
		"star5": star5,
	})
}

type PostCommentMsg struct {
	PostID  uint   `json:"postID"`
	UserID  uint   `json:"userID"`
	Content string `json:"content"`
	Rating  int    `json:"rating"`
}

func GetUserPostRating(c *gin.Context) {
	var requestRatingMsg RatingGetMsg
	c.Bind(&requestRatingMsg)
	postID := requestRatingMsg.PostID
	db := common.GetDB()
	var rstar model.Rstar
	db.Where("postID = ? AND userID = ?", postID, GetTokenUserID(c)).First(&rstar)
	if rstar.StarNum == 0 {
		response.Response(c, http.StatusOK, 200, nil, "该用户未对该帖子评分")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rating": rstar.StarNum,
	})
}
