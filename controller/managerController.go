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
	"log"
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

type modifyUser struct {
	Account   string
	Password1 string
	Password2 string
}

type Check struct {
	Name   string
	Phone  string
	IdPass int
}

func AdminLogin(ctx *gin.Context) {
	var body struct {
		Account  string `json:"account"`
		Password string `json:"password"`
		Username string `json:"username"`
	}
	_ = ctx.ShouldBindJSON(&body)
	account := strings.TrimSpace(body.Account)
	if account == "" {
		account = strings.TrimSpace(body.Username)
	}

	token, err := service.Admins.Login(ctx.Request.Context(), account, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "管理员账号不存在")
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "密码错误")
		default:
			response.Response(ctx, http.StatusInternalServerError, 400, nil, "系统异常")
			log.Printf("token generate error: %v", err)
		}
		return
	}
	response.Success(ctx, gin.H{"token": token}, "登录成功")
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

func AddAdmin(ctx *gin.Context) {
	fmt.Println("Start to add")
	var newAdmin modifyUser
	ctx.Bind(&newAdmin)

	admin, err := service.Admins.AddAdmin(ctx.Request.Context(), newAdmin.Account, newAdmin.Password1, newAdmin.Password2)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, adminAddMsg(newAdmin))
		case errors.Is(err, domain.ErrConflict):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "管理员账号已存在")
		default:
			apperr.Write(ctx, err)
		}
		return
	}
	response.Success(ctx, gin.H{"data": domain.Admin{
		ID:       admin.ID,
		Account:  admin.Account,
		Password: admin.Password,
	}}, "添加管理员成功")
}

func adminAddMsg(in modifyUser) string {
	if in.Account == "" {
		return "账号不能为空"
	}
	if in.Password1 == "" || in.Password2 == "" {
		return "密码不能为空"
	}
	return "两次密码不同，请重新输入"
}

func ChangeAdminPassword(ctx *gin.Context) {
	var admin modifyUser
	ctx.Bind(&admin)

	updated, err := service.Admins.ChangePassword(ctx.Request.Context(), admin.Account, admin.Password1, admin.Password2)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBadRequest):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "两次密码不同，请重新输入")
		case errors.Is(err, domain.ErrNotFound):
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "管理员不存在")
		default:
			apperr.Write(ctx, err)
		}
		return
	}
	response.Success(ctx, gin.H{"data": domain.Admin{
		ID:       updated.ID,
		Account:  updated.Account,
		Password: updated.Password,
	}}, "成功修改管理员密码")
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

func DeleteAdmin(ctx *gin.Context) {
	var user domain.Admin
	ctx.Bind(&user)

	err := service.Admins.DeleteAdmin(ctx.Request.Context(), user.Account)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(ctx, http.StatusUnprocessableEntity, 400, nil, "未找到该管理员")
			return
		}
		apperr.Write(ctx, err)
		return
	}
	response.Success(ctx, nil, "成功删除该管理员")
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
