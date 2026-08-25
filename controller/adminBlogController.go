package controller

import (
	"bytes"
	"fmt"
	"io"
	"loginTest/response"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminBlogSaveReq struct {
	Filename   string   `json:"filename"`
	Title      string   `json:"title"`
	Date       string   `json:"date"`
	Tags       []string `json:"tags"`
	Categories []string `json:"categories"`
	Content    string   `json:"content"`
	Source     string   `json:"source"`
	Publish    *bool    `json:"publish"`
}

func hexoRoot() string {
	if v := os.Getenv("HEXO_ROOT"); v != "" {
		return v
	}
	return "/app/hexo_blog"
}

func hexoPostsDir() string {
	return filepath.Join(hexoRoot(), "source", "_posts")
}

func sanitizeBlogFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "..", "")
	re := regexp.MustCompile(`[<>:"|?*]`)
	name = re.ReplaceAllString(name, "-")
	if name == "" {
		name = "untitled-" + time.Now().Format("20060102150405")
	}
	return name + ".md"
}

func parseFrontTitle(raw string) (title, date string) {
	if !strings.HasPrefix(raw, "---") {
		return "", ""
	}
	parts := strings.SplitN(raw, "---", 3)
	if len(parts) < 3 {
		return "", ""
	}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.Trim(strings.TrimPrefix(line, "title:"), `"'`))
		}
		if strings.HasPrefix(line, "date:") {
			date = strings.TrimSpace(strings.TrimPrefix(line, "date:"))
		}
	}
	return title, date
}

func AdminListBlogPosts(c *gin.Context) {
	entries, err := os.ReadDir(hexoPostsDir())
	if err != nil {
		response.Fail(c, nil, "无法读取博客目录，请确认已挂载 hexo_blog")
		return
	}
	type item struct {
		Filename string `json:"filename"`
		Title    string `json:"title"`
		Date     string `json:"date"`
		Updated  string `json:"updated"`
	}
	list := make([]item, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(hexoPostsDir(), e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		title, date := parseFrontTitle(string(raw))
		if title == "" {
			title = strings.TrimSuffix(e.Name(), ".md")
		}
		info, _ := e.Info()
		updated := ""
		if info != nil {
			updated = info.ModTime().Format("2006-01-02 15:04:05")
		}
		list = append(list, item{Filename: e.Name(), Title: title, Date: date, Updated: updated})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Updated > list[j].Updated })
	response.Success(c, gin.H{"posts": list}, "ok")
}

func AdminGetBlogPost(c *gin.Context) {
	name, _ := url.PathUnescape(c.Param("filename"))
	name = sanitizeBlogFilename(name)
	raw, err := os.ReadFile(filepath.Join(hexoPostsDir(), name))
	if err != nil {
		response.Fail(c, nil, "文章不存在")
		return
	}
	title, date := parseFrontTitle(string(raw))
	response.Success(c, gin.H{"filename": name, "title": title, "date": date, "raw": string(raw)}, "ok")
}

func buildHexoMarkdown(req adminBlogSaveReq) string {
	title := strings.TrimSpace(req.Title)
	date := strings.TrimSpace(req.Date)
	if date == "" {
		date = time.Now().Format("2006-01-02 15:04:05")
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + title + "\n")
	b.WriteString("date: " + date + "\n")
	b.WriteString("tags:\n")
	for _, t := range req.Tags {
		t = strings.TrimSpace(t)
		if t != "" {
			b.WriteString("  - " + t + "\n")
		}
	}
	b.WriteString("categories:\n")
	for _, t := range req.Categories {
		t = strings.TrimSpace(t)
		if t != "" {
			b.WriteString("  - " + t + "\n")
		}
	}
	if strings.TrimSpace(req.Source) != "" {
		b.WriteString("source: " + strings.TrimSpace(req.Source) + "\n")
	}
	b.WriteString("---\n")
	b.WriteString(req.Content)
	if !strings.HasSuffix(req.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func triggerHexoGenerate() error {
	url := os.Getenv("HEXO_PUBLISH_URL")
	if url == "" {
		url = "http://hexo_publish:3951/generate"
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("hexo generate failed: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func AdminSaveBlogPost(c *gin.Context) {
	var req adminBlogSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, nil, "请求格式错误")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		response.Fail(c, nil, "标题不能为空")
		return
	}
	filename := req.Filename
	if filename == "" {
		filename = req.Title
	}
	filename = sanitizeBlogFilename(filename)
	if err := os.MkdirAll(hexoPostsDir(), 0755); err != nil {
		response.Fail(c, nil, "无法创建文章目录")
		return
	}
	raw := strings.TrimSpace(req.Content)
	// 若正文已含 front-matter，则原样保存；否则由后台组装。
	if !strings.HasPrefix(raw, "---") {
		raw = buildHexoMarkdown(req)
	} else if !strings.Contains(raw, "title:") {
		raw = buildHexoMarkdown(req)
	}
	path := filepath.Join(hexoPostsDir(), filename)
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		response.Fail(c, nil, "写入 Markdown 失败")
		return
	}
	published := false
	publishErr := ""
	if req.Publish == nil || *req.Publish {
		if err := triggerHexoGenerate(); err != nil {
			publishErr = err.Error()
		} else {
			published = true
		}
	}
	response.Success(c, gin.H{
		"filename":  filename,
		"published": published,
		"error":     publishErr,
	}, "已保存到 source/_posts")
}

func AdminDeleteBlogPost(c *gin.Context) {
	name, _ := url.PathUnescape(c.Param("filename"))
	name = sanitizeBlogFilename(name)
	path := filepath.Join(hexoPostsDir(), name)
	if err := os.Remove(path); err != nil {
		response.Fail(c, nil, "删除失败")
		return
	}
	_ = triggerHexoGenerate()
	response.Success(c, gin.H{"filename": name}, "已删除并尝试重新生成站点")
}

func AdminPublishBlog(c *gin.Context) {
	if err := triggerHexoGenerate(); err != nil {
		response.Fail(c, gin.H{"error": err.Error()}, "Hexo 生成失败")
		return
	}
	response.Success(c, nil, "已重新生成并发布到站点根路径")
}
