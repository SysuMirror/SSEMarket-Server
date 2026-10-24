package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// 用户自定义皮肤：上传（桌面/手机两张图）、皮肤库、公开搜索、分享链接。
//
// GET  /api/auth/skins/mine          我的皮肤库
// POST /api/auth/skins/create        新建自定义皮肤（默认私密）
// GET  /api/auth/skins/search?q=     搜索公开皮肤
// POST /api/auth/skins/visibility    作者公开/私密
// POST /api/auth/skins/delete        作者删除
// POST /api/auth/skins/add           添加公开皮肤到皮肤库
// POST /api/auth/skins/remove        从皮肤库移除
// POST /api/auth/skins/addByToken    分享链接添加
// GET  /api/skins/share/:token       分享链接预览（公开）

func userSkinData(s domain.UserSkin) gin.H {
	data := gin.H{
		"skinID":      s.ID,
		"name":        s.Name,
		"description": s.Description,
		"desktopUrl":  s.DesktopURL,
		"mobileUrl":   s.MobileURL,
		"isPublic":    s.IsPublic,
		"ownerID":     s.OwnerID,
		"ownerName":   s.OwnerName,
		"isOwner":     s.IsOwner,
		"inLibrary":   s.InLibrary,
	}
	if !s.CreatedAt.IsZero() {
		data["createdAt"] = s.CreatedAt.Format(time.RFC3339)
	}
	// 公开皮肤允许任何皮肤库用户分享；私密皮肤只有作者自己能拿到 token（但链接添加要求公开）。
	if s.IsPublic || s.IsOwner {
		data["shareToken"] = s.ShareToken
	}
	return data
}

func userSkinListData(skins []domain.UserSkin) []gin.H {
	items := make([]gin.H, 0, len(skins))
	for _, s := range skins {
		items = append(items, userSkinData(s))
	}
	return items
}

func writeUserSkinError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, domain.ErrUserSkinNotFound):
		response.Response(c, http.StatusNotFound, 404, nil, err.Error())
	case errors.Is(err, domain.ErrUserSkinNotOwner):
		response.Response(c, http.StatusForbidden, 403, nil, err.Error())
	case errors.Is(err, domain.ErrUserSkinNotPublic):
		response.Response(c, http.StatusForbidden, 403, nil, err.Error())
	case errors.Is(err, domain.ErrUserSkinNotInLibrary):
		response.Response(c, http.StatusForbidden, 403, nil, err.Error())
	case errors.Is(err, domain.ErrUserSkinAlreadyOwned):
		response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
	case errors.Is(err, domain.ErrUserSkinNameInvalid),
		errors.Is(err, domain.ErrUserSkinImageInvalid):
		response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
	default:
		apperr.WriteMsg(c, err, fallback)
	}
}

func MyUserSkins(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	skins, err := service.UserSkins.ListLibrary(c.Request.Context(), user.ID)
	if err != nil {
		writeUserSkinError(c, err, "读取皮肤库失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"skins": userSkinListData(skins)}, "")
}

type userSkinCreateReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DesktopURL  string `json:"desktopUrl"`
	MobileURL   string `json:"mobileUrl"`
}

func CreateUserSkin(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数格式错误")
		return
	}
	skin, err := service.UserSkins.Create(c.Request.Context(), user.ID, domain.UserSkinInput{
		Name:        req.Name,
		Description: req.Description,
		DesktopURL:  req.DesktopURL,
		MobileURL:   req.MobileURL,
	})
	if err != nil {
		writeUserSkinError(c, err, "创建皮肤失败")
		return
	}
	response.Success(c, gin.H{"skin": userSkinData(*skin)}, "自定义皮肤已创建")
}

func SearchUserSkins(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	skins, err := service.UserSkins.SearchPublic(c.Request.Context(), user.ID, c.Query("q"), limit)
	if err != nil {
		writeUserSkinError(c, err, "搜索皮肤失败")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"skins": userSkinListData(skins)}, "")
}

type userSkinIDReq struct {
	SkinID int `json:"skinID"`
}

type userSkinVisibilityReq struct {
	SkinID   int  `json:"skinID"`
	IsPublic bool `json:"isPublic"`
}

func SetUserSkinVisibility(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinVisibilityReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SkinID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	skin, err := service.UserSkins.SetPublic(c.Request.Context(), user.ID, req.SkinID, req.IsPublic)
	if err != nil {
		writeUserSkinError(c, err, "更新公开状态失败")
		return
	}
	msg := "已设为公开，可以分享给其他人"
	if !req.IsPublic {
		msg = "已设为私密，其他人无法搜索或通过链接添加"
	}
	response.Success(c, gin.H{"skin": userSkinData(*skin)}, msg)
}

func DeleteUserSkin(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinIDReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SkinID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	if err := service.UserSkins.Delete(c.Request.Context(), user.ID, req.SkinID); err != nil {
		writeUserSkinError(c, err, "删除皮肤失败")
		return
	}
	response.Success(c, nil, "皮肤已删除")
}

func AddUserSkin(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinIDReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SkinID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	skin, err := service.UserSkins.AddToLibrary(c.Request.Context(), user.ID, req.SkinID)
	if err != nil {
		writeUserSkinError(c, err, "添加皮肤失败")
		return
	}
	response.Success(c, gin.H{"skin": userSkinData(*skin)}, "已添加到皮肤库")
}

func RemoveUserSkin(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinIDReq
	if err := c.ShouldBindJSON(&req); err != nil || req.SkinID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数无效")
		return
	}
	if err := service.UserSkins.RemoveFromLibrary(c.Request.Context(), user.ID, req.SkinID); err != nil {
		writeUserSkinError(c, err, "移除皮肤失败")
		return
	}
	response.Success(c, nil, "已从皮肤库移除")
}

type userSkinTokenReq struct {
	Token string `json:"token"`
}

func AddUserSkinByToken(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return
	}
	var req userSkinTokenReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		response.Response(c, http.StatusBadRequest, 400, nil, "链接无效")
		return
	}
	skin, err := service.UserSkins.AddByShareToken(c.Request.Context(), user.ID, req.Token)
	if err != nil {
		writeUserSkinError(c, err, "添加皮肤失败")
		return
	}
	response.Success(c, gin.H{"skin": userSkinData(*skin)}, "已添加到皮肤库")
}

// ShareSkinInfo 分享链接预览：公开皮肤才可访问。
func ShareSkinInfo(c *gin.Context) {
	skin, err := service.UserSkins.ResolveShareToken(c.Request.Context(), c.Param("token"))
	if err != nil {
		writeUserSkinError(c, err, "读取分享皮肤失败")
		return
	}
	if viewer, ok := viewerFromAuth(c); ok {
		skin.IsOwner = skin.OwnerID == viewer.ID
		if _, err := service.UserSkins.GetActiveForUser(c.Request.Context(), viewer.ID, skin.ID); err == nil {
			skin.InLibrary = true
		}
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"skin": userSkinData(*skin)}, "")
}
