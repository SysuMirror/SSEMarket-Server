package controller

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
)

var bilibiliBVRegexp = regexp.MustCompile(`^/video/(BV[0-9A-Za-z]{8,12})(?:[/?#]|$)`)

func allowedVideoHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "ssemarket.cn" {
		return true
	}
	hosts := os.Getenv("VIDEO_ALLOWED_SELF_HOSTS")
	if hosts == "" {
		hosts = ".myqcloud.com"
	}
	for _, item := range strings.Split(hosts, ",") {
		suffix := strings.ToLower(strings.TrimSpace(item))
		if suffix == "" {
			continue
		}
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func ParseVideoSource(sourceType, rawURL string) (string, string, string, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", "", "", "", errors.New("视频地址必须是 HTTPS URL")
	}
	host := strings.ToLower(u.Hostname())
	path := u.EscapedPath()

	switch sourceType {
	case "bilibili":
		if host != "www.bilibili.com" && host != "m.bilibili.com" {
			return "", "", "", "", errors.New("Bilibili 地址域名不正确")
		}
		m := bilibiliBVRegexp.FindStringSubmatch(path)
		if m == nil {
			return "", "", "", "", errors.New("Bilibili 地址格式不正确")
		}
		return "bilibili", rawURL, m[1], rawURL, nil
	case "mp4":
		if !allowedVideoHost(host) || !strings.HasSuffix(strings.ToLower(path), ".mp4") {
			return "", "", "", "", errors.New("MP4 地址域名不受支持或扩展名不正确")
		}
		return "mp4", rawURL, "", "", nil
	case "hls":
		if !allowedVideoHost(host) || !strings.HasSuffix(strings.ToLower(path), ".m3u8") {
			return "", "", "", "", errors.New("HLS 地址域名不受支持或扩展名不正确")
		}
		return "hls", rawURL, "", "", nil
	case "auto":
		if (host == "www.bilibili.com" || host == "m.bilibili.com") && bilibiliBVRegexp.MatchString(path) {
			return ParseVideoSource("bilibili", rawURL)
		}
		if strings.HasSuffix(strings.ToLower(path), ".m3u8") {
			return ParseVideoSource("hls", rawURL)
		}
		if strings.HasSuffix(strings.ToLower(path), ".mp4") {
			return ParseVideoSource("mp4", rawURL)
		}
		return "", "", "", "", errors.New("无法识别视频来源")
	default:
		return "", "", "", "", fmt.Errorf("不支持的视频来源: %s", sourceType)
	}
}

type videoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	CoverURL    string `json:"coverURL"`
	SourceType  string `json:"sourceType"`
	SourceURL   string `json:"sourceURL"`
	DurationSec int    `json:"durationSec"`
	SortWeight  int    `json:"sortWeight"`
	Status      string `json:"status"`
}

func adminFromVideoContext(c *gin.Context) (*model.Admin, bool) {
	value, exists := c.Get("admin")
	if !exists {
		return nil, false
	}
	admin, ok := value.(*model.Admin)
	return admin, ok && admin.AdminID != 0
}

func normalizeVideoStatus(status string) string {
	if status == model.VideoStatusPublished || status == model.VideoStatusHidden {
		return status
	}
	return model.VideoStatusDraft
}

func publicVideo(v model.Video) gin.H {
	return gin.H{
		"videoID":         v.VideoID,
		"title":           v.Title,
		"description":     v.Description,
		"coverURL":        v.CoverURL,
		"sourceType":      v.SourceType,
		"sourceURL":       v.SourceURL,
		"bilibiliBVID":    v.BilibiliBVID,
		"externalPageURL": v.ExternalPageURL,
		"durationSec":     v.DurationSec,
		"playCount":       v.PlayCount,
		"publishedAt":     v.PublishedAt,
	}
}

func AdminListVideos(c *gin.Context) {
	if _, ok := adminFromVideoContext(c); !ok {
		response.Fail(c, nil, "管理员信息无效")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if size < 1 || size > 100 {
		size = 20
	}
	db := common.GetDB().Model(&model.Video{})
	if s := c.Query("status"); s != "" {
		db = db.Where("status = ?", s)
	}
	if s := c.Query("sourceType"); s != "" {
		db = db.Where("source_type = ?", s)
	}
	if q := strings.TrimSpace(c.Query("keyword")); q != "" {
		db = db.Where("title like ?", "%"+q+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		response.Fail(c, nil, "读取视频失败")
		return
	}
	var rows []model.Video
	if err := db.Order("sort_weight desc, videoID desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		response.Fail(c, nil, "读取视频失败")
		return
	}
	response.Success(c, gin.H{"list": rows, "total": total, "page": page, "pageSize": size}, "ok")
}

func AdminParseVideoSource(c *gin.Context) {
	var req videoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	kind, url, bvid, page, err := ParseVideoSource(req.SourceType, req.SourceURL)
	if err != nil {
		response.Fail(c, nil, err.Error())
		return
	}
	response.Success(c, gin.H{"sourceType": kind, "sourceURL": url, "bilibiliBVID": bvid, "externalPageURL": page}, "ok")
}

func AdminCreateVideo(c *gin.Context) {
	admin, ok := adminFromVideoContext(c)
	if !ok {
		response.Fail(c, nil, "管理员信息无效")
		return
	}
	var req videoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	kind, url, bvid, page, err := ParseVideoSource(req.SourceType, req.SourceURL)
	if err != nil {
		response.Fail(c, nil, err.Error())
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		response.Fail(c, nil, "标题不能为空")
		return
	}
	now := time.Now()
	status := normalizeVideoStatus(req.Status)
	v := model.Video{
		AdminID: admin.AdminID, Title: req.Title, Description: req.Description,
		CoverURL: req.CoverURL, SourceType: kind, SourceURL: url,
		BilibiliBVID: bvid, ExternalPageURL: page, Status: status,
		DurationSec: req.DurationSec, SortWeight: req.SortWeight,
		CreatedAt: now, UpdatedAt: now, TranscodeStatus: "ready",
	}
	if status == model.VideoStatusPublished {
		v.PublishedAt = &now
	}
	if err := common.GetDB().Create(&v).Error; err != nil {
		response.Fail(c, nil, "保存视频失败")
		return
	}
	response.Success(c, gin.H{"video": v}, "已保存")
}

func AdminGetVideo(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("videoID"), 10, 64)
	var v model.Video
	if id <= 0 || common.GetDB().First(&v, id).Error != nil {
		response.Fail(c, nil, "视频不存在")
		return
	}
	response.Success(c, gin.H{"video": v}, "ok")
}

func AdminUpdateVideo(c *gin.Context) {
	admin, ok := adminFromVideoContext(c)
	if !ok || admin.AdminID <= 0 {
		response.Fail(c, nil, "管理员信息无效")
		return
	}
	id, _ := strconv.ParseInt(c.Param("videoID"), 10, 64)
	var v model.Video
	db := common.GetDB()
	if id <= 0 || db.First(&v, id).Error != nil {
		response.Fail(c, nil, "视频不存在")
		return
	}
	var req videoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	kind, url, bvid, page, err := ParseVideoSource(req.SourceType, req.SourceURL)
	if err != nil {
		response.Fail(c, nil, err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		response.Fail(c, nil, "标题不能为空")
		return
	}
	status := normalizeVideoStatus(req.Status)
	updates := map[string]any{
		"adminID": admin.AdminID, "title": strings.TrimSpace(req.Title),
		"description": req.Description, "cover_url": req.CoverURL,
		"source_type": kind, "source_url": url, "bilibili_bvid": bvid,
		"external_page_url": page, "duration_sec": req.DurationSec,
		"sort_weight": req.SortWeight, "status": status, "updated_at": time.Now(),
	}
	if status == model.VideoStatusPublished && v.PublishedAt == nil {
		updates["published_at"] = time.Now()
	}
	if err := db.Model(&v).Updates(updates).Error; err != nil {
		response.Fail(c, nil, "更新失败")
		return
	}
	response.Success(c, gin.H{"video": v}, "已更新")
}

func AdminDeleteVideo(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("videoID"), 10, 64)
	if id <= 0 || common.GetDB().Delete(&model.Video{}, id).Error != nil {
		response.Fail(c, nil, "删除失败")
		return
	}
	response.Success(c, nil, "已删除")
}

func PublicVideoList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "12"))
	if size < 1 || size > 48 {
		size = 12
	}
	db := common.GetDB().Where("status = ?", model.VideoStatusPublished)
	if q := strings.TrimSpace(c.Query("keyword")); q != "" {
		db = db.Where("title like ?", "%"+q+"%")
	}
	var total int64
	if db.Model(&model.Video{}).Count(&total).Error != nil {
		response.Fail(c, nil, "读取视频失败")
		return
	}
	var rows []model.Video
	if err := db.Order("sort_weight desc, published_at desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		response.Fail(c, nil, "读取视频失败")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, v := range rows {
		out = append(out, publicVideo(v))
	}
	response.Success(c, gin.H{"list": out, "total": total, "page": page, "pageSize": size}, "ok")
}

func PublicVideoDetail(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("videoID"), 10, 64)
	var v model.Video
	if id <= 0 || common.GetDB().Where("status = ?", model.VideoStatusPublished).First(&v, id).Error != nil {
		response.Fail(c, nil, "视频不存在")
		return
	}
	common.GetDB().Model(&model.Video{}).Where("videoID = ?", id).UpdateColumn("view_count", v.ViewCount+1)
	response.Success(c, gin.H{"video": publicVideo(v)}, "ok")
}

func RecordVideoPlayed(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("videoID"), 10, 64)
	if id <= 0 {
		response.Fail(c, nil, "视频不存在")
		return
	}
	result := common.GetDB().Exec(
		"UPDATE videos SET play_count = COALESCE(play_count, 0) + 1 WHERE videoID = ? AND status = ?",
		id, model.VideoStatusPublished,
	)
	if result.Error != nil || result.RowsAffected == 0 {
		response.Fail(c, nil, "记录播放失败")
		return
	}
	response.Success(c, nil, "ok")
}
