package controller

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"loginTest/api"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
)

// 音乐存储根目录在容器内 public/music 下，宿主机对应
// /root/SSE_Market/sse_market_server_resource/music/（bind-mount，容器重建不丢）
const (
	musicFilesDir  = "public/music/files"
	musicCoversDir = "public/music/covers"
	maxMusicSize   = 60 << 20 // 60MB
	maxCoverSize   = 10 << 20 // 10MB
	cosMusicPrefix = "/src/music/"
)

// 允许的音频格式：扩展名 -> 播放时使用的 Content-Type
var musicFormats = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
}

// 允许的封面图片格式
var coverTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// 去掉路径分隔符与控制字符，防止目录穿越
var unsafeChars = regexp.MustCompile(`[\\/\x00-\x1f]`)

func sanitizeFilename(name string) string {
	name = unsafeChars.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	if name == "" {
		name = "untitled"
	}
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	return name
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}

func musicFileAbsPath(relName string) string {
	return filepath.Join(filepath.FromSlash(musicFilesDir), filepath.FromSlash(relName))
}

func musicCoverAbsPath(relName string) string {
	return filepath.Join(filepath.FromSlash(musicCoversDir), filepath.FromSlash(relName))
}

// 校验相对 key 没有逃出存储目录（创建时已安全化，双保险）
func safeMusicPath(baseDir, relName string) (string, bool) {
	abs := filepath.Join(baseDir, filepath.FromSlash(relName))
	rel, err := filepath.Rel(baseDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

// mirrorMusicToCOS 把本地文件双写一份到 COS 做备份（本地盘是主存储，失败只记日志不阻断业务）
func mirrorMusicToCOS(relName string, isCover bool) {
	subDir := "files"
	localPath := musicFileAbsPath(relName)
	if isCover {
		subDir = "covers"
		localPath = musicCoverAbsPath(relName)
	}
	f, err := os.Open(localPath)
	if err != nil {
		log.Println("COS 备份打开文件失败：", err)
		return
	}
	defer f.Close()
	if err := api.UploadFile(cosMusicPrefix+subDir+"/"+filepath.ToSlash(relName), f); err != nil {
		log.Println("COS 备份上传失败（本地文件已保存）：", err)
	}
}

// musicSummary 拼给前端的曲目信息（文件/封面 URL 由前端用 VITE_API_BASE_URL 自行拼接）
func musicSummary(m model.Music, uploaderName string, isOwner bool) gin.H {
	return gin.H{
		"musicID":      m.MusicID,
		"title":        m.Title,
		"artist":       m.Artist,
		"album":        m.Album,
		"format":       m.Format,
		"durationSec":  m.DurationSec,
		"sizeBytes":    m.SizeBytes,
		"playCount":    m.PlayCount,
		"hasCover":     m.CoverName != "",
		"uploaderID":   m.UserID,
		"uploaderName": uploaderName,
		"isOwner":      isOwner,
		"postTime":     m.PostTime.Format("2006-01-02 15:04:05"),
	}
}

// UploadMusic 上传音乐（两段式第一步）：保存文件、解析元数据与内嵌封面，
// 建立未发布记录并返回预览信息，等前端确认后调 publishMusic 发布
func UploadMusic(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, nil, "文件上传失败："+err.Error())
		return
	}
	if file.Size > maxMusicSize {
		response.Fail(c, nil, "文件太大，不能超过60MB")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if _, ok := musicFormats[ext]; !ok {
		response.Fail(c, nil, "仅支持 mp3/flac/ogg/wav/m4a 格式")
		return
	}

	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}

	// 嗅探文件真实类型，防止改扩展名上传非音频文件
	src, err := file.Open()
	if err != nil {
		response.Fail(c, nil, "无法读取文件")
		return
	}
	defer src.Close()
	buffer := make([]byte, 512)
	n, err := src.Read(buffer)
	if err != nil && err != io.EOF {
		response.Fail(c, nil, "无法读取文件")
		return
	}
	sniffed := http.DetectContentType(buffer[:n])
	if !strings.HasPrefix(sniffed, "audio/") && !strings.HasPrefix(sniffed, "application/ogg") {
		response.Fail(c, nil, "文件内容不是音频")
		return
	}

	// 保存文件：public/music/files/<yyyymm>/<unixnano>_<安全文件名>
	relName := fmt.Sprintf("%s/%d_%s", time.Now().Format("200601"), time.Now().UnixNano(), sanitizeFilename(file.Filename))
	absPath, ok := safeMusicPath(musicFilesDir, relName)
	if !ok {
		response.Fail(c, nil, "文件名非法")
		return
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		response.Fail(c, nil, "创建目录失败："+err.Error())
		return
	}
	if err := c.SaveUploadedFile(file, absPath); err != nil {
		response.Fail(c, nil, "文件保存失败："+err.Error())
		return
	}

	// 解析内嵌元数据与封面（解析失败不阻断，字段留空）
	title, artist, album := "", "", ""
	coverName := ""
	f, err := os.Open(absPath)
	if err == nil {
		if metadata, err := tag.ReadFrom(f); err == nil {
			title = metadata.Title()
			artist = metadata.Artist()
			album = metadata.Album()
			if picture := metadata.Picture(); picture != nil {
				coverName = saveMusicCover(picture.Data, picture.Ext)
			}
		} else {
			log.Println("音乐元数据解析失败：", err)
		}
		f.Close()
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(file.Filename), filepath.Ext(file.Filename))
	}

	// 时长由浏览器在上传前读取音频元数据得到（秒）
	durationSec := 0
	if d := c.PostForm("duration"); d != "" {
		if v, err := strconv.Atoi(strings.Split(d, ".")[0]); err == nil && v > 0 && v < 86400 {
			durationSec = v
		}
	}

	music := model.Music{
		UserID:      user.ID,
		Title:       truncateStr(title, 100),
		Artist:      truncateStr(artist, 100),
		Album:       truncateStr(album, 100),
		FileName:    relName,
		CoverName:   coverName,
		Format:      strings.TrimPrefix(ext, "."),
		SizeBytes:   file.Size,
		DurationSec: durationSec,
		PostTime:    time.Now(),
	}
	db := common.GetDB()
	if err := db.Create(&music).Error; err != nil {
		os.Remove(absPath)
		if coverName != "" {
			os.Remove(musicCoverAbsPath(coverName))
		}
		response.Fail(c, nil, "音乐记录创建失败："+err.Error())
		return
	}

	// 本地为主存储，COS 双写备份
	go mirrorMusicToCOS(relName, false)
	if coverName != "" {
		go mirrorMusicToCOS(coverName, true)
	}

	response.Success(c, gin.H{"music": musicSummary(music, user.Name, true)}, "上传成功，请确认信息后发布")
}

// saveMusicCover 把封面图片字节写入 covers 目录，返回相对文件名
func saveMusicCover(data []byte, ext string) string {
	ext = "." + strings.ToLower(strings.TrimPrefix(ext, "."))
	if _, ok := coverTypes[ext]; !ok {
		ext = ".jpg" // 内嵌封面未知扩展名时按 jpg 兜底
	}
	if len(data) == 0 || len(data) > maxCoverSize {
		return ""
	}
	relName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	absPath := musicCoverAbsPath(relName)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		log.Println("封面保存失败：", err)
		return ""
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		log.Println("封面保存失败：", err)
		return ""
	}
	return relName
}

// PublishMusic 发布音乐（两段式第二步）：可覆盖标题/歌手/专辑，可上传自定义封面替换内嵌封面
func PublishMusic(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}
	musicID, err := strconv.Atoi(c.PostForm("musicID"))
	if err != nil || musicID <= 0 {
		response.Fail(c, nil, "musicID 不合法")
		return
	}

	db := common.GetDB()
	var music model.Music
	db.Where("musicID = ? AND is_delete = 0", musicID).First(&music)
	if music.MusicID == 0 {
		response.Fail(c, nil, "音乐不存在")
		return
	}
	if music.UserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}

	updates := map[string]interface{}{"is_published": true}
	if title := strings.TrimSpace(c.PostForm("title")); title != "" {
		updates["title"] = truncateStr(title, 100)
	}
	if artist := strings.TrimSpace(c.PostForm("artist")); artist != "" {
		updates["artist"] = truncateStr(artist, 100)
	}
	if album := strings.TrimSpace(c.PostForm("album")); album != "" {
		updates["album"] = truncateStr(album, 100)
	}

	// 用户自传封面（优先于内嵌封面）
	if cover, err := c.FormFile("cover"); err == nil {
		if cover.Size > maxCoverSize {
			response.Fail(c, nil, "封面图片太大，不能超过10MB")
			return
		}
		ext := strings.ToLower(filepath.Ext(cover.Filename))
		if _, ok := coverTypes[ext]; !ok {
			response.Fail(c, nil, "封面仅支持 jpg/png/webp 格式")
			return
		}
		src, err := cover.Open()
		if err != nil {
			response.Fail(c, nil, "无法读取封面")
			return
		}
		defer src.Close()
		buffer := make([]byte, 512)
		n, _ := src.Read(buffer)
		sniffed := http.DetectContentType(buffer[:n])
		if !strings.HasPrefix(sniffed, "image/") {
			response.Fail(c, nil, "封面文件内容不是图片")
			return
		}
		relName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		absPath := musicCoverAbsPath(relName)
		if err := c.SaveUploadedFile(cover, absPath); err != nil {
			response.Fail(c, nil, "封面保存失败："+err.Error())
			return
		}
		// 替换旧封面文件（内嵌封面文件是本条记录独占的，可安全删除）
		if music.CoverName != "" {
			os.Remove(musicCoverAbsPath(music.CoverName))
		}
		updates["cover_name"] = relName
		go mirrorMusicToCOS(relName, true)
	}

	if err := db.Model(&music).Updates(updates).Error; err != nil {
		response.Fail(c, nil, "发布失败："+err.Error())
		return
	}
	db.Where("musicID = ?", musicID).First(&music)

	response.Success(c, gin.H{"music": musicSummary(music, user.Name, true)}, "发布成功")
}

type getMusicListRequest struct {
	Search string `json:"search"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	// ids 用于刷新后按已保存的播放列表恢复曲目
	IDs []int `json:"ids"`
	// mine=只看自己上传的（发帖插入音乐的选歌面板用）
	Mine bool `json:"mine"`
}

// GetMusicList 获取音乐列表（分页 + 搜索；传 ids 时按 ids 返回用于恢复播放列表）
func GetMusicList(c *gin.Context) {
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}

	var req getMusicListRequest
	if err := c.BindJSON(&req); err != nil {
		response.Fail(c, nil, "请求参数错误")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	db := common.GetDB()
	var musics []model.Music
	if len(req.IDs) > 0 {
		if err := db.Where("is_published = 1 AND is_delete = 0 AND musicID IN (?)", req.IDs).Find(&musics).Error; err != nil {
			response.Fail(c, nil, "列表获取失败："+err.Error())
			return
		}
	} else {
		query := db.Where("is_published = 1 AND is_delete = 0")
		if req.Mine {
			query = query.Where("userID = ?", user.ID)
		}
		if search := strings.TrimSpace(req.Search); search != "" {
			like := "%" + search + "%"
			query = query.Where("title LIKE ? OR artist LIKE ? OR album LIKE ?", like, like, like)
		}
		if err := query.Order("post_time DESC").Limit(req.Limit).Offset(req.Offset).Find(&musics).Error; err != nil {
			response.Fail(c, nil, "列表获取失败："+err.Error())
			return
		}
	}

	// 批量查上传者昵称，避免逐条 Preload 整个 User
	userIDs := make([]int, 0, len(musics))
	for _, m := range musics {
		userIDs = append(userIDs, m.UserID)
	}
	nameByID := map[int]string{}
	if len(userIDs) > 0 {
		var users []model.User
		db.Select("userID, name").Where("userID IN (?)", userIDs).Find(&users)
		for _, u := range users {
			nameByID[u.UserID] = u.Name
		}
	}

	list := make([]gin.H, 0, len(musics))
	for _, m := range musics {
		list = append(list, musicSummary(m, nameByID[m.UserID], m.UserID == user.ID))
	}
	response.Success(c, gin.H{"list": list}, "列表获取成功")
}

// DeleteMusic 删除音乐（软删除，仅上传者本人）
func DeleteMusic(c *gin.Context) {
	var req struct {
		MusicID int `json:"musicID"`
	}
	if err := c.BindJSON(&req); err != nil || req.MusicID <= 0 {
		response.Fail(c, nil, "请求参数错误")
		return
	}
	user, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusBadRequest, 400, nil, "检测不到用户，无法访问")
		return
	}
	db := common.GetDB()
	var music model.Music
	db.Where("musicID = ? AND is_delete = 0", req.MusicID).First(&music)
	if music.MusicID == 0 {
		response.Fail(c, nil, "音乐不存在")
		return
	}
	if music.UserID != user.ID {
		response.Response(c, http.StatusUnprocessableEntity, 400, nil, "权限不足")
		return
	}
	if err := db.Model(&music).Update("is_delete", true).Error; err != nil {
		response.Fail(c, nil, "删除失败："+err.Error())
		return
	}
	response.Success(c, nil, "删除成功")
}

// MusicPlayed 播放数 +1
func MusicPlayed(c *gin.Context) {
	var req struct {
		MusicID int `json:"musicID"`
	}
	if err := c.BindJSON(&req); err != nil || req.MusicID <= 0 {
		response.Fail(c, nil, "请求参数错误")
		return
	}
	db := common.GetDB()
	if err := db.Model(&model.Music{}).
		Where("musicID = ? AND is_delete = 0", req.MusicID).
		Update("play_count", gorm.Expr("play_count + 1")).Error; err != nil {
		response.Fail(c, nil, "播放数更新失败")
		return
	}
	response.Success(c, nil, "ok")
}

// MusicFile 音乐文件流式播放端点（noauth：<audio> 媒体请求带不了 Authorization 头）
// http.ServeContent 原生支持 Range/206/Seek
func MusicFile(c *gin.Context) {
	musicID, err := strconv.Atoi(c.Param("musicID"))
	if err != nil || musicID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "musicID 不合法"})
		return
	}
	db := common.GetDB()
	var music model.Music
	db.Where("musicID = ? AND is_delete = 0", musicID).First(&music)
	if music.MusicID == 0 {
		c.JSON(http.StatusNotFound, gin.H{"msg": "音乐不存在"})
		return
	}
	absPath, ok := safeMusicPath(musicFilesDir, music.FileName)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "文件路径非法"})
		return
	}
	f, err := os.Open(absPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"msg": "文件不存在"})
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"msg": "文件不存在"})
		return
	}
	// 音频文件上传后不可变，可长缓存
	if ct, ok := musicFormats[strings.ToLower(filepath.Ext(music.FileName))]; ok {
		c.Header("Content-Type", ct)
	}
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%d.%s", music.MusicID, music.Format))
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(c.Writer, c.Request, music.FileName, st.ModTime(), f)
}

// MusicCover 封面图端点（noauth，同上）；无封面返回 404，前端兜底默认封面
func MusicCover(c *gin.Context) {
	musicID, err := strconv.Atoi(c.Param("musicID"))
	if err != nil || musicID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "musicID 不合法"})
		return
	}
	db := common.GetDB()
	var music model.Music
	db.Where("musicID = ? AND is_delete = 0", musicID).First(&music)
	if music.MusicID == 0 || music.CoverName == "" {
		c.JSON(http.StatusNotFound, gin.H{"msg": "封面不存在"})
		return
	}
	absPath, ok := safeMusicPath(musicCoversDir, music.CoverName)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"msg": "文件路径非法"})
		return
	}
	f, err := os.Open(absPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"msg": "封面不存在"})
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		c.JSON(http.StatusNotFound, gin.H{"msg": "封面不存在"})
		return
	}
	if ct, ok := coverTypes[strings.ToLower(filepath.Ext(music.CoverName))]; ok {
		c.Header("Content-Type", ct)
	}
	// 封面可能在发布时被替换，不做 immutable，靠 Last-Modified 协商缓存
	c.Header("Cache-Control", "public, max-age=86400")
	http.ServeContent(c.Writer, c.Request, music.CoverName, st.ModTime(), f)
}

// CleanUnpublishedMusic 清理放弃发布留下的草稿（未发布且超过24小时）及其文件
func CleanUnpublishedMusic() {
	db := common.GetDB()
	var stale []model.Music
	db.Where("is_published = 0 AND is_delete = 0 AND post_time < ?", time.Now().Add(-24*time.Hour)).Find(&stale)
	for _, music := range stale {
		if abs, ok := safeMusicPath(musicFilesDir, music.FileName); ok {
			os.Remove(abs)
		}
		if music.CoverName != "" {
			if abs, ok := safeMusicPath(musicCoversDir, music.CoverName); ok {
				os.Remove(abs)
			}
		}
		db.Delete(&music)
		log.Printf("已清理未发布音乐草稿 musicID=%d title=%s", music.MusicID, music.Title)
	}
}
