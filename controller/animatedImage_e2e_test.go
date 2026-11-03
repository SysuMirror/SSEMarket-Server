package controller

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// countGIFFrames 数 GIF 里的图形控制扩展块（21 F9 04），作为帧数的近似。
func countGIFFrames(b []byte) int {
	return bytes.Count(b, []byte{0x21, 0xF9, 0x04})
}

// ffmpeg 保帧压缩冒烟测试：只需要 ANIM_TEST_GIF（本机需有 ffmpeg，缺则跳过）。
//
//	COMPRESS_TEST_GIF=/tmp/x.gif go test ./controller -run TestCompressAnimatedGIF
func TestCompressAnimatedGIF(t *testing.T) {
	fixture := os.Getenv("COMPRESS_TEST_GIF")
	if fixture == "" {
		t.Skip("未设置 COMPRESS_TEST_GIF，跳过")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("本机没有 ffmpeg，跳过")
	}
	orig, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("读取样例动图失败: %v", err)
	}
	out, err := compressAnimatedGIF(fixture, 1920)
	if err != nil {
		t.Fatalf("压缩报错: %v", err)
	}
	if out == "" {
		t.Logf("样例太小压不动（%d B），回退直传属正常", len(orig))
		return
	}
	defer os.Remove(out)
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("读取产物失败: %v", err)
	}
	if !bytes.HasPrefix(got, []byte("GIF8")) {
		t.Fatalf("产物不是 GIF: %q", got[:min(6, len(got))])
	}
	origFrames, gotFrames := countGIFFrames(orig), countGIFFrames(got)
	if gotFrames != origFrames {
		t.Fatalf("帧数变了: %d -> %d", origFrames, gotFrames)
	}
	if len(got) >= len(orig) {
		t.Fatalf("产物反而更大: %d >= %d", len(got), len(orig))
	}
	t.Logf("压缩 %d B -> %d B (%.0f%%), 帧数 %d 保持",
		len(orig), len(got), float64(len(got))/float64(len(orig))*100, origFrames)
}

// 真实图床集成测试：需要 IMAGEBED_API_TOKEN/IMAGEBED_UPLOAD_URL 与一个动图样例文件。
// 动图 GIF 现在会先经 ffmpeg 保帧压缩再上传（容器/主机需有 ffmpeg；没有则直传原文件），
// 因此这里校验的是“产物仍是 GIF 且帧数不减”，而非字节相等。
//
//	用法:  IMAGEBED_API_TOKEN=... IMAGEBED_UPLOAD_URL=... ANIM_TEST_GIF=/tmp/x.gif \
//	       go test ./controller -run TestAnimatedImageBed
func TestAnimatedImageBed(t *testing.T) {
	token, uploadURL, fixture := os.Getenv("IMAGEBED_API_TOKEN"), os.Getenv("IMAGEBED_UPLOAD_URL"), os.Getenv("ANIM_TEST_GIF")
	if token == "" || uploadURL == "" || fixture == "" {
		t.Skip("未设置 IMAGEBED_API_TOKEN/IMAGEBED_UPLOAD_URL/ANIM_TEST_GIF，跳过")
	}
	orig, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("读取样例动图失败: %v", err)
	}
	viper.Set("imagebed.upload_url", uploadURL)
	viper.Set("imagebed.token", token)
	viper.Set("imagebed.public_base", os.Getenv("IMAGEBED_PUBLIC_BASE"))

	url, err := processAnimatedImage(fixture, 1920)
	if err != nil {
		t.Fatalf("动图上传失败: %v", err)
	}
	if url == "" {
		t.Fatalf("样例未识别为动图: %s", fixture)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("回读 %s 失败: %v", url, err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if !bytes.HasPrefix(got, []byte("GIF8")) {
		t.Fatalf("图床返回的不是 GIF: %q", got[:min(6, len(got))])
	}
	origFrames := countGIFFrames(orig)
	gotFrames := countGIFFrames(got)
	if gotFrames < origFrames {
		t.Fatalf("帧数丢失: 原 %d 帧 -> 上传后 %d 帧", origFrames, gotFrames)
	}
	t.Logf("动图上传验证通过: %s 原 %d B/%d 帧 -> 存 %d B/%d 帧", url, len(orig), origFrames, len(got), gotFrames)
}
