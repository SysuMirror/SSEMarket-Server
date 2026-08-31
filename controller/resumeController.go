package controller

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"loginTest/api"
	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	resumeMaxUploadSize = 10 << 20 // 10 MB
	resumeMaxPerUser    = 5
	resumeCOSPrefix     = "resume/"
	resumeURLExpire     = 30 * time.Minute
)

var resumeAllowedExt = map[string]bool{
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

func resumeCanPreview(ext string) bool {
	switch strings.ToLower(ext) {
	case ".pdf", ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func resumeContentType(ext, mimeType string) string {
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType
	}
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

type ResumeBrowseMsg struct {
	Searchinfo string `json:"searchinfo"`
	Searchsort string `json:"searchsort"` // home | my
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type ResumeIDMsg struct {
	ResumeID int `json:"resumeID"`
}

type ResumeUpdateMsg struct {
	ResumeID    int    `json:"resumeID"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func validateResumeTitleDescription(title, description string) (string, string, string) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" {
		return "", "", "标题不能为空"
	}
	if utf8.RuneCountInString(title) > 50 {
		return "", "", "标题最多50个字"
	}
	if utf8.RuneCountInString(description) > 500 {
		return "", "", "简介最多500个字"
	}
	return title, description, ""
}

type ResumeResponse struct {
	ResumeID    int    `json:"resumeID"`
	UserID      int    `json:"userID"`
	Uploader    string `json:"uploader"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	MimeType    string `json:"mimeType"`
	FileExt     string `json:"fileExt"`
	DownloadNum int    `json:"downloadNum"`
	ViewNum     int    `json:"viewNum"`
	IsAnonymous bool   `json:"isAnonymous"`
	IsOwner     bool   `json:"isOwner"`
	CanPreview  bool   `json:"canPreview"`
	CreatedAt   string `json:"createdAt"`
}

func requireLoginUser(c *gin.Context) (*domain.User, bool) {
	userID := GetTokenUserID(c)
	if userID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return nil, false
	}
	u, err := service.Users.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户不存在")
		return nil, false
	}
	return u, true
}

func viewerIsOwnerDomain(resume *domain.Resume, viewer *domain.User) bool {
	return resume.UserID == viewer.ID
}

func shouldMaskAnonymousDomain(resume *domain.Resume, viewer *domain.User) bool {
	return resume.IsAnonymous && !viewerIsOwnerDomain(resume, viewer)
}

func downloadDisplayFileNameDomain(resume *domain.Resume, uploader *domain.User) string {
	displayName := ""
	if uploader != nil {
		displayName = strings.TrimSpace(uploader.Name)
	}
	if resume.IsAnonymous {
		displayName = "匿名用户"
	}
	if displayName == "" {
		displayName = "用户"
	}
	displayName = sanitizeDownloadFileSegment(displayName)
	ext := strings.ToLower(filepath.Ext(resume.FileName))
	if ext == "" {
		ext = ".pdf"
	}
	return displayName + "-简历" + ext
}

// anonymousDisplayFileName 对外展示的文件名，避免真实姓名出现在 URL/界面。
func anonymousDisplayFileName(original string) string {
	ext := strings.ToLower(filepath.Ext(original))
	if ext == "" {
		return "简历文件"
	}
	return "简历" + ext
}

func sanitizeDownloadFileSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "用户"
	}
	return out
}

func resumeToResponse(r *domain.Resume, uploader *domain.User, viewer *domain.User) ResumeResponse {
	mask := shouldMaskAnonymousDomain(r, viewer)
	owner := viewerIsOwnerDomain(r, viewer)

	uploaderName := ""
	uploaderID := 0
	if uploader != nil {
		uploaderName = uploader.Name
		uploaderID = uploader.ID
	}
	fileName := anonymousDisplayFileName(r.FileName)
	if mask {
		uploaderName = "匿名用户"
		uploaderID = 0
	}

	ext := strings.ToLower(filepath.Ext(r.FileName))
	fileExt := strings.TrimPrefix(ext, ".")
	canPreview := resumeCanPreview(ext)

	return ResumeResponse{
		ResumeID:    r.ID,
		UserID:      uploaderID,
		Uploader:    uploaderName,
		Title:       r.Title,
		Description: r.Description,
		FileName:    fileName,
		FileSize:    r.FileSize,
		MimeType:    r.MimeType,
		FileExt:     fileExt,
		DownloadNum: r.DownloadNum,
		ViewNum:     r.ViewNum,
		IsAnonymous: r.IsAnonymous,
		IsOwner:     owner,
		CanPreview:  canPreview,
		CreatedAt:   r.CreatedAt.Format("2006-01-02 15:04"),
	}
}

func newResumeCosKey(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s/%d%s", resumeCOSPrefix, hex.EncodeToString(b), time.Now().UnixNano(), ext), nil
}

func resumeFilePayloadDomain(resume *domain.Resume, uploader *domain.User, inline bool) (gin.H, error) {
	var fileName string
	var attachmentName string
	if inline {
		fileName = anonymousDisplayFileName(resume.FileName)
		attachmentName = ""
	} else {
		fileName = downloadDisplayFileNameDomain(resume, uploader)
		attachmentName = fileName
	}
	fileURL, err := resumeAccessURL(resume.CosKey, inline, attachmentName)
	if err != nil {
		return nil, err
	}
	return gin.H{
		"fileURL":    fileURL,
		"fileName":   fileName,
		"canPreview": resumeCanPreview(filepath.Ext(resume.FileName)),
	}, nil
}

func resumeAccessURL(cosKey string, inline bool, attachmentFilename string) (string, error) {
	url, err := api.GetPresignedURL(cosKey, resumeURLExpire, inline, attachmentFilename)
	if err != nil {
		return api.GetUrl(cosKey), nil
	}
	return url, nil
}

func ResumeUpload(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	if err := service.Resumes.CanUpload(ctx, user.ID); err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			response.Response(c, http.StatusBadRequest, 400, nil, fmt.Sprintf("每人最多上传%d份简历", resumeMaxPerUser))
			return
		}
		apperr.Write(c, err)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "请选择要上传的文件")
		return
	}
	if file.Size > resumeMaxUploadSize {
		response.Response(c, http.StatusBadRequest, 400, nil, "文件不能超过10MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !resumeAllowedExt[ext] {
		response.Response(c, http.StatusBadRequest, 400, nil, "仅支持 PDF、Word、JPG、PNG、WebP、GIF 格式")
		return
	}

	title, description, errMsg := validateResumeTitleDescription(c.PostForm("title"), c.PostForm("description"))
	if errMsg != "" {
		response.Response(c, http.StatusBadRequest, 400, nil, errMsg)
		return
	}
	isAnonymous := c.PostForm("isAnonymous") == "true" || c.PostForm("isAnonymous") == "1"

	f, err := file.Open()
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "无法读取文件")
		return
	}
	defer f.Close()

	buffer := make([]byte, 512)
	n, _ := f.Read(buffer)
	mimeType := http.DetectContentType(buffer[:n])
	if !isAllowedResumeMime(mimeType, ext) {
		response.Response(c, http.StatusBadRequest, 400, nil, "文件格式不正确")
		return
	}

	safeName := filepath.Base(file.Filename)
	safeName = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, safeName)

	cosKey, err := newResumeCosKey(ext)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "生成存储路径失败")
		return
	}
	if _, err = f.Seek(0, 0); err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "无法读取文件")
		return
	}
	if err = api.UploadFile(cosKey, f); err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "文件上传失败")
		return
	}

	resumeID, err := service.Resumes.Create(c.Request.Context(), domain.ResumeCreateInput{
		UserID:      user.ID,
		Title:       title,
		Description: description,
		FileName:    safeName,
		CosKey:      cosKey,
		FileSize:    file.Size,
		MimeType:    mimeType,
		IsAnonymous: isAnonymous,
	})
	if err != nil {
		_ = api.FileDelete(cosKey)
		apperr.WriteMsg(c, err, "保存失败")
		return
	}

	response.Success(c, gin.H{"resumeID": resumeID}, "简历上传成功")
}

func isAllowedResumeMime(mimeType, ext string) bool {
	switch ext {
	case ".pdf":
		return strings.Contains(mimeType, "pdf") || mimeType == "application/octet-stream"
	case ".doc":
		return strings.Contains(mimeType, "msword") || mimeType == "application/octet-stream"
	case ".docx":
		return strings.Contains(mimeType, "wordprocessingml") || strings.Contains(mimeType, "zip") || mimeType == "application/octet-stream"
	case ".jpg", ".jpeg":
		return strings.Contains(mimeType, "jpeg") || mimeType == "application/octet-stream"
	case ".png":
		return strings.Contains(mimeType, "png") || mimeType == "application/octet-stream"
	case ".webp":
		return strings.Contains(mimeType, "webp") || mimeType == "application/octet-stream"
	case ".gif":
		return strings.Contains(mimeType, "gif") || mimeType == "application/octet-stream"
	default:
		return false
	}
}

func ResumeList(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}

	var req ResumeBrowseMsg
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	ctx := c.Request.Context()
	resumes, err := service.Resumes.List(ctx, domain.ResumeFilter{
		UserID:     user.ID,
		SearchInfo: req.Searchinfo,
		SearchSort: req.Searchsort,
		Limit:      req.Limit,
		Offset:     req.Offset,
	})
	if err != nil {
		apperr.Write(c, err)
		return
	}

	userIDs := make([]int, 0, len(resumes))
	for _, r := range resumes {
		userIDs = append(userIDs, r.UserID)
	}
	userMap, err := service.Resumes.UploadersByIDs(ctx, userIDs)
	if err != nil {
		apperr.Write(c, err)
		return
	}

	responses := make([]ResumeResponse, 0, len(resumes))
	for _, r := range resumes {
		responses = append(responses, resumeToResponse(r, userMap[r.UserID], user))
	}
	c.JSON(http.StatusOK, responses)
}

func ResumeDetail(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}

	var req ResumeIDMsg
	if err := c.ShouldBindJSON(&req); err != nil || req.ResumeID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	ctx := c.Request.Context()
	resume, err := service.Resumes.Detail(ctx, req.ResumeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
			return
		}
		apperr.Write(c, err)
		return
	}

	uploaderMap, err := service.Resumes.UploadersByIDs(ctx, []int{resume.UserID})
	if err != nil {
		apperr.Write(c, err)
		return
	}
	c.JSON(http.StatusOK, resumeToResponse(resume, uploaderMap[resume.UserID], user))
}

func ResumePreview(c *gin.Context) {
	if _, ok := requireLoginUser(c); !ok {
		return
	}

	var req ResumeIDMsg
	if err := c.ShouldBindJSON(&req); err != nil || req.ResumeID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	ctx := c.Request.Context()
	resume, err := service.Resumes.GetByID(ctx, req.ResumeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
			return
		}
		apperr.Write(c, err)
		return
	}

	if !resumeCanPreview(filepath.Ext(resume.FileName)) {
		response.Response(c, http.StatusBadRequest, 400, nil, "当前文件类型不支持在线预览，请下载查看")
		return
	}

	uploaderMap, _ := service.Resumes.UploadersByIDs(ctx, []int{resume.UserID})
	payload, err := resumeFilePayloadDomain(resume, uploaderMap[resume.UserID], true)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "获取预览链接失败")
		return
	}
	response.Success(c, payload, "获取预览链接成功")
}

// ResumePreviewStream 经服务端代理 PDF 流，响应 Content-Disposition: inline，供前端 blob 内嵌预览。
func ResumePreviewStream(c *gin.Context) {
	if _, ok := requireLoginUser(c); !ok {
		return
	}

	resumeID, err := strconv.Atoi(c.Param("resumeID"))
	if err != nil || resumeID <= 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	resume, err := service.Resumes.GetByID(c.Request.Context(), resumeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
			return
		}
		apperr.Write(c, err)
		return
	}

	ext := strings.ToLower(filepath.Ext(resume.FileName))
	if !resumeCanPreview(ext) {
		response.Response(c, http.StatusBadRequest, 400, nil, "当前文件类型不支持在线预览")
		return
	}

	body, contentType, err := api.GetObject(resume.CosKey)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "读取文件失败")
		return
	}
	defer body.Close()

	c.Header("Content-Type", resumeContentType(ext, contentType))
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "SAMEORIGIN")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, body)
}

func ResumeDownload(c *gin.Context) {
	if _, ok := requireLoginUser(c); !ok {
		return
	}

	var req ResumeIDMsg
	if err := c.ShouldBindJSON(&req); err != nil || req.ResumeID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	ctx := c.Request.Context()
	resume, err := service.Resumes.GetByID(ctx, req.ResumeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
			return
		}
		apperr.Write(c, err)
		return
	}

	uploaderMap, _ := service.Resumes.UploadersByIDs(ctx, []int{resume.UserID})
	if err := service.Resumes.RecordDownload(ctx, req.ResumeID); err != nil {
		apperr.Write(c, err)
		return
	}
	payload, err := resumeFilePayloadDomain(resume, uploaderMap[resume.UserID], false)
	if err != nil {
		response.Response(c, http.StatusInternalServerError, 500, nil, "获取下载链接失败")
		return
	}
	response.Success(c, gin.H{
		"downloadURL": payload["fileURL"],
		"fileName":    payload["fileName"],
	}, "获取下载链接成功")
}

func ResumeUpdate(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}

	var req ResumeUpdateMsg
	if err := c.ShouldBindJSON(&req); err != nil || req.ResumeID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	title, description, errMsg := validateResumeTitleDescription(req.Title, req.Description)
	if errMsg != "" {
		response.Response(c, http.StatusBadRequest, 400, nil, errMsg)
		return
	}

	ctx := c.Request.Context()
	resume, err := service.Resumes.Update(ctx, domain.ResumeUpdateInput{
		ResumeID:    req.ResumeID,
		UserID:      user.ID,
		Title:       title,
		Description: description,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "无权编辑该简历")
		default:
			apperr.WriteMsg(c, err, "保存失败")
		}
		return
	}

	uploaderMap, _ := service.Resumes.UploadersByIDs(ctx, []int{resume.UserID})
	c.JSON(http.StatusOK, resumeToResponse(resume, uploaderMap[resume.UserID], user))
}

func ResumeDelete(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}

	var req ResumeIDMsg
	if err := c.ShouldBindJSON(&req); err != nil || req.ResumeID == 0 {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数错误")
		return
	}

	cosKey, err := service.Resumes.Delete(c.Request.Context(), req.ResumeID, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			response.Response(c, http.StatusNotFound, 404, nil, "简历不存在")
		case errors.Is(err, domain.ErrForbidden):
			response.Response(c, http.StatusForbidden, 403, nil, "无权删除该简历")
		default:
			apperr.Write(c, err)
		}
		return
	}

	_ = api.FileDelete(cosKey)
	response.Success(c, nil, "简历已删除")
}
