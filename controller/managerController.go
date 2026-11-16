package controller

import (
	"errors"
	"fmt"
	"loginTest/apperr"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/dto"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type User struct {
	UserID    int    `json:"-"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Password  string `json:"-"`
	Name      string `json:"name"`
	Num       int    `json:"num"`
	Profile   string `json:"-"`
	Intro     string `json:"-"`
	IDpass    bool   `json:"IDpass"`
	Ban       bool   `json:"ban"`
	Punishnum int    `json:"punishnum"`
}

type Username struct {
	Name string
}

type Check struct {
	Name   string
	Phone  string
	IdPass int
}

func AdminInfo(c *gin.Context) {
	admin, _ := c.Get("admin")
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{"admin": dto.ToAdminDto(*admin.(*domain.Admin))}})
}

func ShowFilterUsers(ctx *gin.Context) {
	var requestInfo Check
	ctx.Bind(&requestInfo)

	items, err := service.Admins.FilterUsers(ctx.Request.Context(), domain.AdminUserFilter{
		Name:   requestInfo.Name,
		Phone:  requestInfo.Phone,
		IDPass: requestInfo.IdPass,
	})
	if err != nil {
		apperr.Write(ctx, err)
		return
	}
	userList := make([]User, 0, len(items))
	for _, item := range items {
		userList = append(userList, User{
			Phone:     item.Phone,
			Email:     item.Email,
			Name:      item.Name,
			Punishnum: item.PunishNum,
		})
	}
	response.Success(ctx, gin.H{"data": userList}, "Successfully show all users")
}

func DeleteUser(ctx *gin.Context) {
	var user Username
	ctx.Bind(&user)

	fmt.Println(user)
	fmt.Println(user.Name)
	err := service.Admins.DeleteUser(ctx.Request.Context(), user.Name)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "未找到该用户")
			return
		}
		apperr.Write(ctx, err)
		return
	}
	response.Response(ctx, http.StatusOK, 200, nil, "成功删除该用户")
}

func AdminPost(c *gin.Context) {
	var requestPostMsg PostMsg
	c.Bind(&requestPostMsg)
	userTelephone := requestPostMsg.UserTelephone
	title := requestPostMsg.Title
	content := requestPostMsg.Content
	partition := requestPostMsg.Partition
	photos := requestPostMsg.Photos
	tagList := requestPostMsg.TagList
	tags := strings.Split(tagList, "|")
	tagString := strings.Join(tags, ",")

	if len(userTelephone) == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "返回的手机号为空")
		return
	}

	err := service.Admins.CreatePost(c.Request.Context(), domain.AdminPostInput{
		Title:     title,
		Content:   content,
		Partition: partition,
		Photos:    photos,
		Tags:      tagString,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(c, http.StatusBadRequest, 400, nil, adminPostMsg(title, content, partition))
		default:
			apperr.Write(c, err)
		}
		return
	}
	response.Response(c, http.StatusOK, 200, nil, "发帖成功")
}

func adminPostMsg(title, content, partition string) string {
	if len(strings.TrimSpace(title)) == 0 {
		return "标题不能为空"
	}
	if utf8.RuneCountInString(title) > 15 {
		return "标题最多为15个字"
	}
	if len(strings.TrimSpace(content)) == 0 {
		return "内容不能为空"
	}
	if utf8.RuneCountInString(content) > config.PostContentMaxRunes {
		return fmt.Sprintf("内容最多为%d个字", config.PostContentMaxRunes)
	}
	if len(strings.TrimSpace(partition)) == 0 {
		return "分区不能为空"
	}
	return "参数错误"
}
