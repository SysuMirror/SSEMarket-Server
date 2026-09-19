package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultMediaServiceBaseURL = "https://image.ssemarket.cn"

type MediaUploadResult struct {
	ID             uint   `json:"id"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"error_message,omitempty"`
	StorageKey     string `json:"storage_key"`
	PlaylistKey    string `json:"playlist_key,omitempty"`
	PosterKey      string `json:"poster_key,omitempty"`
	DurationMillis int64  `json:"duration_millis"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	FileSizeBytes  int64  `json:"file_size_bytes"`
}

type mediaAPIResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Data    MediaUploadResult `json:"data"`
}

func uploadVideoToMedia(path, filename string) (*MediaUploadResult, error) {
	baseURL := strings.TrimSuffix(strings.TrimSpace(os.Getenv("MEDIA_SERVICE_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = defaultMediaServiceBaseURL
	}
	token := strings.TrimSpace(os.Getenv("MEDIA_INTERNAL_TOKEN"))
	if token == "" {
		return nil, fmt.Errorf("媒体服务未配置")
	}

	source, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取上传视频失败")
	}
	defer source.Close()

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	copyDone := make(chan error, 1)
	go func() {
		var copyErr error
		defer func() { copyDone <- copyErr }()
		part, partErr := writer.CreateFormFile("file", filepath.Base(filename))
		if partErr == nil {
			_, copyErr = io.Copy(part, source)
		}
		if closeErr := writer.Close(); copyErr == nil {
			copyErr = closeErr
		}
		_ = pipeWriter.CloseWithError(copyErr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/media/videos", pipeReader)
	if err != nil {
		_ = pipeWriter.CloseWithError(err)
		<-copyDone
		return nil, fmt.Errorf("创建媒体上传请求失败")
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		<-copyDone
		return nil, fmt.Errorf("媒体服务不可用")
	}
	defer response.Body.Close()
	if err := <-copyDone; err != nil {
		return nil, fmt.Errorf("上传视频失败")
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取媒体服务响应失败")
	}
	var body mediaAPIResponse
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("媒体服务响应异常")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || body.Code != 0 {
		message := body.Message
		if message == "" {
			message = "视频上传失败"
		}
		return nil, fmt.Errorf("%s", message)
	}
	if body.Data.ID == 0 || body.Data.Status != "ready" {
		return nil, fmt.Errorf("视频未准备好")
	}
	return &body.Data, nil
}
