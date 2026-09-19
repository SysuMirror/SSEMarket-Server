package controller

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
)

const (
	maxVideoUploadBytes = 200 << 20
	defaultMediaBaseURL = "https://image.ssemarket.cn"
)

var allowedVideoUploadExtensions = map[string]bool{
	".mp4":  true,
	".m4v":  true,
	".mov":  true,
	".webm": true,
}

func mediaPublicBaseURL() string {
	return strings.TrimSuffix(strings.TrimSpace(os.Getenv("MEDIA_PUBLIC_BASE_URL")), "/")
}

func mediaAssetURL(assetID uint, filename string) string {
	base := mediaPublicBaseURL()
	if base == "" {
		base = defaultMediaBaseURL
	}
	return fmt.Sprintf("%s/m/videos/%d/%s", base, assetID, filename)
}

func videoFromMediaAsset(
	adminID int,
	asset *MediaUploadResult,
	title, description, coverURL, status string,
	sortWeight int,
	originalFileName string,
	now time.Time,
) model.Video {
	video := model.Video{
		AdminID:          adminID,
		MediaAssetID:     int64(asset.ID),
		Title:            truncateStr(title, 120),
		Description:      truncateStr(description, 2000),
		CoverURL:         truncateStr(coverURL, 2048),
		SourceType:       "hls",
		SourceURL:        mediaAssetURL(asset.ID, "master.m3u8"),
		Status:           normalizeVideoStatus(status),
		SortWeight:       sortWeight,
		OriginalFileName: truncateStr(originalFileName, 255),
		DurationSec:      int(math.Round(float64(asset.DurationMillis) / 1000)),
		FileSizeBytes:    asset.FileSizeBytes,
		Width:            asset.Width,
		Height:           asset.Height,
		CreatedAt:        now,
		UpdatedAt:        now,
		TranscodeStatus:  asset.Status,
		TranscodeError:   asset.ErrorMessage,
	}
	if video.CoverURL == "" {
		video.CoverURL = mediaAssetURL(asset.ID, "poster.jpg")
	}
	if video.Status == model.VideoStatusPublished {
		video.PublishedAt = &now
	}
	return video
}

func AdminUploadVideo(c *gin.Context) {
	admin, ok := adminFromVideoContext(c)
	if !ok || admin.AdminID <= 0 {
		response.Fail(c, nil, "管理员信息无效")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxVideoUploadBytes+(1<<20))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, nil, "视频上传失败或超过 200MB")
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > maxVideoUploadBytes {
		response.Fail(c, nil, "视频文件不能超过 200MB")
		return
	}

	extension := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedVideoUploadExtensions[extension] {
		response.Fail(c, nil, "仅支持 mp4/m4v/mov/webm 视频")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		response.Fail(c, nil, "标题不能为空")
		return
	}
	status := normalizeVideoStatus(c.PostForm("status"))
	sortWeight, _ := strconv.Atoi(c.PostForm("sortWeight"))

	tempFile, err := os.CreateTemp("", "sse-market-video-*"+extension)
	if err != nil {
		response.Fail(c, nil, "保存上传文件失败")
		return
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()
	removeTemp := func() { _ = os.Remove(tempPath) }

	if err := c.SaveUploadedFile(fileHeader, tempPath); err != nil {
		removeTemp()
		response.Fail(c, nil, "保存上传文件失败")
		return
	}

	asset, err := uploadVideoToMedia(tempPath, fileHeader.Filename)
	removeTemp()
	if err != nil {
		response.Fail(c, nil, err.Error())
		return
	}

	now := time.Now()
	video := videoFromMediaAsset(
		admin.AdminID,
		asset,
		title,
		strings.TrimSpace(c.PostForm("description")),
		strings.TrimSpace(c.PostForm("coverURL")),
		status,
		sortWeight,
		filepath.Base(fileHeader.Filename),
		now,
	)
	if err := common.GetDB().Create(&video).Error; err != nil {
		response.Fail(c, nil, "保存视频记录失败")
		return
	}

	response.Success(c, gin.H{"video": video}, "上传成功")
}
