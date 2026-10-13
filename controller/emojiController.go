package controller

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"loginTest/api"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mime/multipart"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

// 表情图片上传口径：单张 ≤ 2MB，静态图缩到 500×500 以内，GIF 原样保留（动画不能缩）。
const emojiImageMaxSize = 2 << 20

func emojiCurrentUser(c *gin.Context) (*domain.User, bool) {
	value, ok := c.Get("user")
	if !ok {
		return nil, false
	}
	user, ok := value.(*domain.User)
	if !ok || user.ID == 0 {
		return nil, false
	}
	return user, true
}

type emojiResp struct {
	EmojiID     int    `json:"emojiID"`
	PackID      int    `json:"packID"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	IsBuiltin   bool   `json:"isBuiltin"`
	OwnerUserID int    `json:"ownerUserID"`
}

type emojiPackResp struct {
	PackID       int         `json:"packID"`
	Title        string      `json:"title"`
	OwnerUserID  int         `json:"ownerUserID"`
	IsPersonal   bool        `json:"isPersonal"`
	IsPublic     bool        `json:"isPublic"`
	CoverEmojiID int         `json:"coverEmojiID"`
	StickerCount int         `json:"stickerCount"`
	AddedByMe    bool        `json:"addedByMe"`
	Preview      []emojiResp `json:"preview"`
	Stickers     []emojiResp `json:"stickers,omitempty"`
}

func emojiToResp(e *domain.Emoji) emojiResp {
	return emojiResp{
		EmojiID:     e.ID,
		PackID:      e.PackID,
		Name:        e.Name,
		URL:         e.URL,
		IsBuiltin:   e.IsBuiltin,
		OwnerUserID: e.OwnerUserID,
	}
}

func emojisToResp(list []*domain.Emoji) []emojiResp {
	out := make([]emojiResp, 0, len(list))
	for _, e := range list {
		out = append(out, emojiToResp(e))
	}
	return out
}

func emojiPackToResp(p *domain.EmojiPack, stickers []*domain.Emoji) emojiPackResp {
	return emojiPackResp{
		PackID:       p.ID,
		Title:        p.Title,
		OwnerUserID:  p.OwnerUserID,
		IsPersonal:   p.IsPersonal,
		IsPublic:     p.IsPublic,
		CoverEmojiID: p.CoverEmojiID,
		StickerCount: p.StickerCount,
		AddedByMe:    p.AddedByMe,
		Preview:      emojisToResp(p.Preview),
		Stickers:     emojisToResp(stickers),
	}
}

// EmojiPanel 面板初始数据：内置组（小汐/SSE娘，始终展示）+ 个人表情组（默认空）
// + 我添加/自建的分享组（组带全量表情）。
func EmojiPanel(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	panel, err := service.Emojis.GetPanel(c.Request.Context(), user.ID)
	if err != nil {
		log.Printf("[emoji] panel failed: %v", err)
		response.Fail(c, gin.H{}, "表情面板加载失败，请重试")
		return
	}
	packs := make([]emojiPackResp, 0, len(panel.Packs))
	for _, pack := range panel.Packs {
		packs = append(packs, emojiPackToResp(pack, panel.PackItems[pack.ID]))
	}
	response.Success(c, gin.H{
		"builtinPack":  emojiPackToResp(panel.BuiltinPack, panel.Builtin),
		"personalPack": emojiPackToResp(panel.PersonalPack, panel.Personal),
		"packs":        packs,
	}, "获取表情面板成功")
}

// sanitizeEmojiFilename 去掉路径分隔符等危险字符，只留一个安全的文件名。
func sanitizeEmojiFilename(name string) string {
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, " ", "")
	if name == "." || name == ".." || name == "" {
		name = "emoji"
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

// uploadEmojiImage 校验并保存一张表情图片，返回图床 URL。
// 与 UploadPhotos 同一条链路：本地落盘 → api.UploadImage 推图床。
func uploadEmojiImage(file *multipart.FileHeader) (string, error) {
	if file.Size > emojiImageMaxSize {
		return "", errors.New("单张表情不能超过 2MB")
	}
	src, err := file.Open()
	if err != nil {
		return "", errors.New("无法读取文件")
	}
	defer src.Close()
	img, format, err := image.Decode(src)
	if err != nil {
		return "", errors.New("只支持 PNG / JPG / GIF 图片")
	}
	ext := strings.ToLower(path.Ext(file.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif":
	default:
		ext = "." + format
	}
	if format != "gif" && format != "png" && format != "jpeg" {
		return "", errors.New("只支持 PNG / JPG / GIF 图片")
	}

	dir := "public/uploads/emoji"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", errors.New("表情目录创建失败")
	}
	filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeEmojiFilename(file.Filename))
	localPath := filepath.Join(dir, filename)

	if format == "gif" {
		// GIF 直接复制原始字节，缩放会毁掉动画。
		src.Seek(0, 0)
		data, err := io.ReadAll(io.LimitReader(src, emojiImageMaxSize))
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(localPath, data, 0o644); err != nil {
			return "", errors.New("表情保存失败")
		}
	} else {
		resized := imaging.Fit(img, 500, 500, imaging.Lanczos)
		if err := imaging.Save(resized, localPath); err != nil {
			return "", errors.New("表情保存失败")
		}
	}

	fileURL, err := api.UploadImage("emoji/"+filename, localPath)
	if err != nil {
		log.Printf("[emoji] imagebed upload failed: path=%s err=%v", localPath, err)
		return "", errors.New("表情上传失败，请重试")
	}
	return fileURL, nil
}

// collectEmojiUploads 处理 multipart 里的全部图片，产出 service 入参。
func collectEmojiUploads(c *gin.Context) ([]domain.EmojiStickerInput, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, errors.New("文件上传失败")
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		return nil, errors.New("没有收到表情图片")
	}
	items := make([]domain.EmojiStickerInput, 0, len(files))
	for _, file := range files {
		fileURL, err := uploadEmojiImage(file)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(sanitizeEmojiFilename(file.Filename), path.Ext(file.Filename))
		items = append(items, domain.EmojiStickerInput{Name: name, URL: fileURL})
	}
	return items, nil
}

// EmojiUpload 上传单张（或多张）表情，直接进「我的表情」。
func EmojiUpload(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	items, err := collectEmojiUploads(c)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	created, err := service.Emojis.UploadStickers(c.Request.Context(), user.ID, items)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{"stickers": emojisToResp(created)}, "表情上传成功")
}

// EmojiPackCreate 上传一组表情并建组；isPublic=true 时别人可以搜索并添加。
func EmojiPackCreate(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	items, err := collectEmojiUploads(c)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	title := c.PostForm("title")
	isPublic := c.PostForm("isPublic") == "true" || c.PostForm("isPublic") == "1"
	pack, created, err := service.Emojis.CreatePack(c.Request.Context(), user.ID, title, isPublic, items)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{
		"pack":     emojiPackToResp(pack, created),
		"stickers": emojisToResp(created),
	}, "表情包组创建成功")
}

type emojiPackIDReq struct {
	PackID int `json:"packID" binding:"required"`
}

type emojiStickerIDReq struct {
	EmojiID int `json:"emojiID" binding:"required"`
}

type emojiSearchReq struct {
	Keyword string `json:"keyword"`
}

type emojiResolveReq struct {
	IDs []int `json:"ids" binding:"required"`
}

// EmojiPackAdd 把搜索到的组添加进自己的面板。
func EmojiPackAdd(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiPackIDReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Emojis.AddPack(c.Request.Context(), user.ID, req.PackID); err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{}, "已添加表情包组")
}

// EmojiPackRemove 从面板移除一个组（不删除组本身）。
func EmojiPackRemove(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiPackIDReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Emojis.RemovePack(c.Request.Context(), user.ID, req.PackID); err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{}, "已移除表情包组")
}

// EmojiPackDelete 删除自己创建的表情包组（软删除，历史消息不受影响）。
func EmojiPackDelete(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiPackIDReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Emojis.DeletePack(c.Request.Context(), user.ID, req.PackID); err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{}, "表情包组已删除")
}

// EmojiStickerDelete 删除自己的一张表情（软删除）：个人表情组里的表情从此处移出，
// 分享组里自己上传的也走这里；内置原件与别人的表情不允许删。
func EmojiStickerDelete(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiStickerIDReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	if err := service.Emojis.DeleteSticker(c.Request.Context(), user.ID, req.EmojiID); err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{}, "表情已删除")
}

// EmojiAddFromChat 长按发出来的表情 → 加入自己的表情包。
func EmojiAddFromChat(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiStickerIDReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	sticker, err := service.Emojis.AddFromChat(c.Request.Context(), user.ID, req.EmojiID)
	if err != nil {
		response.Fail(c, gin.H{}, err.Error())
		return
	}
	response.Success(c, gin.H{"sticker": emojiToResp(sticker)}, "已加入表情包")
}

// EmojiSearch 搜表情与组（内置 / 公开 / 自己的）。
func EmojiSearch(c *gin.Context) {
	user, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiSearchReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	result, err := service.Emojis.Search(c.Request.Context(), user.ID, req.Keyword)
	if err != nil {
		log.Printf("[emoji] search failed: %v", err)
		response.Fail(c, gin.H{}, "搜索失败，请重试")
		return
	}
	packs := make([]emojiPackResp, 0, len(result.Packs))
	for _, pack := range result.Packs {
		packs = append(packs, emojiPackToResp(pack, nil))
	}
	response.Success(c, gin.H{
		"stickers": emojisToResp(result.Stickers),
		"packs":    packs,
	}, "搜索成功")
}

// EmojiResolve 按消息正文里的 [emoji:<id>] token 批量取表情元数据。
func EmojiResolve(c *gin.Context) {
	_, ok := emojiCurrentUser(c)
	if !ok {
		response.Fail(c, gin.H{}, "检测不到用户，无法访问")
		return
	}
	req := emojiResolveReq{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, gin.H{}, "无效的请求参数")
		return
	}
	ids := make([]int, 0, len(req.IDs))
	seen := make(map[int]bool, len(req.IDs))
	for _, id := range req.IDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	stickers, err := service.Emojis.Resolve(c.Request.Context(), ids)
	if err != nil {
		log.Printf("[emoji] resolve failed: %v", err)
		response.Fail(c, gin.H{}, "表情加载失败，请重试")
		return
	}
	response.Success(c, gin.H{"stickers": emojisToResp(stickers)}, "表情加载成功")
}
