package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"loginTest/common"
	"loginTest/model"
	"loginTest/response"
)

const (
	maxVideoUploadBytes    = 200 << 20
	maxVideoDuration       = 30.0
	videoStorageRoot       = "public/videos"
	defaultVideoPublicBase = "https://ssemarket.cn"
)

var allowedVideoUploadExtensions = map[string]bool{
	".mp4":  true,
	".mov":  true,
	".m4v":  true,
	".webm": true,
	".mkv":  true,
}

type videoProbe struct {
	DurationSec     int
	DurationSeconds float64
	Width           int
	Height          int
}

type hlsTranscodeResult struct {
	DurationSec   int
	Width         int
	Height        int
	FileSizeBytes int64
}

var (
	videoDurationPattern = regexp.MustCompile(`Duration:\s*(\d+):(\d+):([0-9.]+)`)
	videoStreamPattern   = regexp.MustCompile(`Stream #.*?: Video: .*?,\s*(\d+)x(\d+)`)
)

func probeVideoMetadata(path string) (videoProbe, error) {
	cmd := exec.Command(
		"ffmpeg", "-hide_banner", "-nostdin", "-i", path,
		"-map", "0:v:0", "-t", "0", "-f", "null", "-",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return videoProbe{}, errors.New("无法识别视频文件，请上传完整的 mp4/mov/m4v/webm/mkv 文件")
	}

	durationMatch := videoDurationPattern.FindStringSubmatch(string(output))
	streamMatch := videoStreamPattern.FindStringSubmatch(string(output))
	if durationMatch == nil || streamMatch == nil {
		return videoProbe{}, errors.New("文件中没有可播放的视频轨")
	}

	hours, _ := strconv.Atoi(durationMatch[1])
	minutes, _ := strconv.Atoi(durationMatch[2])
	seconds, parseErr := strconv.ParseFloat(durationMatch[3], 64)
	if parseErr != nil || seconds < 0 {
		return videoProbe{}, errors.New("无法读取视频时长")
	}
	duration := float64(hours*3600+minutes*60) + seconds
	width, _ := strconv.Atoi(streamMatch[1])
	height, _ := strconv.Atoi(streamMatch[2])
	if duration <= 0 || width <= 0 || height <= 0 {
		return videoProbe{}, errors.New("文件中没有可播放的视频轨")
	}

	return videoProbe{
		DurationSeconds: duration,
		DurationSec:     int(duration),
		Width:           width,
		Height:          height,
	}, nil
}

func scaledDimensions(width, height int) (int, int) {
	shortSide := width
	if height < shortSide {
		shortSide = height
	}
	if shortSide > 720 {
		shortSide = 720
	}
	var scaledWidth, scaledHeight int
	if width <= height {
		scaledWidth = shortSide
		scaledHeight = height * shortSide / width
	} else {
		scaledWidth = width * shortSide / height
		scaledHeight = shortSide
	}
	scaledWidth = (scaledWidth + 1) / 2 * 2
	scaledHeight = (scaledHeight + 1) / 2 * 2
	return scaledWidth, scaledHeight
}

func probeVideoFile(path string) (videoProbe, error) {
	probe, err := probeVideoMetadata(path)
	if err != nil {
		return videoProbe{}, err
	}
	if probe.DurationSeconds > maxVideoDuration {
		return videoProbe{}, fmt.Errorf("视频时长不能超过 %.0f 秒", maxVideoDuration)
	}
	return probe, nil
}

func videoScaleFilter() string {
	return "scale='if(lte(iw,ih),min(720,iw),-2)':'if(gt(iw,ih),min(720,ih),-2)'"
}

func videoEncoderArgs() []string {
	return []string{
		"-c:v", "libx264", "-preset", "veryfast",
		"-b:v", "1M", "-maxrate", "1M", "-bufsize", "2M",
		"-pix_fmt", "yuv420p",
	}
}

func runFFmpeg(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-nostdin", "-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("视频处理失败: %s", detail)
	}
	return nil
}

func transcodeVideoToHLS(input, outputDir string) (hlsTranscodeResult, error) {
	if _, err := os.Stat(outputDir); err == nil {
		return hlsTranscodeResult{}, errors.New("视频输出目录已存在")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return hlsTranscodeResult{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return hlsTranscodeResult{}, err
	}

	playlist := filepath.Join(outputDir, "master.m3u8")
	segment := filepath.Join(outputDir, "segment_%04d.ts")
	ffmpegArgs := []string{
		"-i", input,
		"-vf", videoScaleFilter(),
	}
	ffmpegArgs = append(ffmpegArgs, videoEncoderArgs()...)
	ffmpegArgs = append(ffmpegArgs,
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-force_key_frames", "expr:gte(t,n_forced*4)",
		"-f", "hls", "-hls_time", "4", "-hls_list_size", "0",
		"-hls_playlist_type", "vod", "-hls_flags", "independent_segments",
		"-hls_segment_filename", segment, playlist,
	)
	err := runFFmpeg(ffmpegArgs...)
	if err == nil {
		err = makeVideoPoster(input, filepath.Join(outputDir, "poster.jpg"))
	}
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return hlsTranscodeResult{}, err
	}

	source, err := probeVideoMetadata(input)
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return hlsTranscodeResult{}, err
	}
	width, height := scaledDimensions(source.Width, source.Height)
	size, err := directorySize(outputDir)
	if err != nil {
		_ = os.RemoveAll(outputDir)
		return hlsTranscodeResult{}, err
	}

	return hlsTranscodeResult{
		DurationSec:   source.DurationSec,
		Width:         width,
		Height:        height,
		FileSizeBytes: size,
	}, nil
}

func makeVideoPoster(input, output string) error {
	err := runFFmpeg("-ss", "1", "-i", input, "-frames:v", "1", "-q:v", "3", output)
	if err == nil {
		return nil
	}
	return runFFmpeg("-i", input, "-frames:v", "1", "-q:v", "3", output)
}

func directorySize(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func newVideoStorageKey() (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return filepath.Join(time.Now().Format("200601"), hex.EncodeToString(random)), nil
}

func videoPublicBaseURL() string {
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("VIDEO_PUBLIC_BASE_URL")), "/")
	if base == "" {
		return defaultVideoPublicBase
	}
	return base
}

func videoMediaURL(key, filename string) string {
	return fmt.Sprintf("%s/media/videos/%s/%s", videoPublicBaseURL(), filepath.ToSlash(key), filename)
}

func AdminUploadVideo(c *gin.Context) {
	admin, ok := adminFromVideoContext(c)
	if !ok || admin.AdminID <= 0 {
		response.Fail(c, nil, "管理员信息无效")
		return
	}

	c.Request.Body = http.MaxBytesReader(
		c.Writer, c.Request.Body, maxVideoUploadBytes+(1<<20),
	)
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
		response.Fail(c, nil, "仅支持 mp4/mov/m4v/webm/mkv 视频")
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		response.Fail(c, nil, "标题不能为空")
		return
	}
	status := normalizeVideoStatus(c.PostForm("status"))
	sortWeight, _ := strconv.Atoi(c.PostForm("sortWeight"))

	uploadDir := filepath.Join(videoStorageRoot, ".uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		response.Fail(c, nil, "创建上传目录失败")
		return
	}
	tempFile, err := os.CreateTemp(uploadDir, "upload-*"+extension)
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

	if _, err := probeVideoFile(tempPath); err != nil {
		removeTemp()
		response.Fail(c, nil, err.Error())
		return
	}

	key, err := newVideoStorageKey()
	if err != nil {
		removeTemp()
		response.Fail(c, nil, "生成视频存储路径失败")
		return
	}
	outputDir := filepath.Join(videoStorageRoot, key)
	result, err := transcodeVideoToHLS(tempPath, outputDir)
	removeTemp()
	if err != nil {
		response.Fail(c, nil, err.Error())
		return
	}

	coverURL := strings.TrimSpace(c.PostForm("coverURL"))
	if coverURL == "" {
		coverURL = videoMediaURL(key, "poster.jpg")
	}
	now := time.Now()
	video := uploadedVideoModel(
		admin.AdminID,
		result,
		title,
		strings.TrimSpace(c.PostForm("description")),
		coverURL,
		status,
		sortWeight,
		key,
		filepath.Base(fileHeader.Filename),
		now,
	)

	if err := common.GetDB().Create(&video).Error; err != nil {
		_ = os.RemoveAll(outputDir)
		response.Fail(c, nil, "保存视频记录失败")
		return
	}

	response.Success(c, gin.H{"video": video}, "上传成功")
}

func uploadedVideoModel(
	adminID int,
	result hlsTranscodeResult,
	title, description, coverURL, status string,
	sortWeight int,
	key, originalFileName string,
	now time.Time,
) model.Video {
	video := model.Video{
		AdminID:          adminID,
		Title:            truncateStr(title, 120),
		Description:      truncateStr(description, 2000),
		CoverURL:         truncateStr(coverURL, 2048),
		SourceType:       "hls",
		SourceURL:        videoMediaURL(key, "master.m3u8"),
		Status:           status,
		SortWeight:       sortWeight,
		StorageKey:       filepath.ToSlash(key),
		PlaybackKey:      filepath.ToSlash(filepath.Join(key, "master.m3u8")),
		OriginalFileName: truncateStr(originalFileName, 255),
		DurationSec:      result.DurationSec,
		FileSizeBytes:    result.FileSizeBytes,
		Width:            result.Width,
		Height:           result.Height,
		CreatedAt:        now,
		UpdatedAt:        now,
		TranscodeStatus:  "ready",
	}
	if status == model.VideoStatusPublished {
		video.PublishedAt = &now
	}
	return video
}
