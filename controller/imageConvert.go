package controller

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// 手机相册常见 HEIC/HEIF（以及 AVIF）不在 Go 标准库的 image.Decode 支持范围内，
// 直接解码会报 “image: unknown format”。容器里已有 ffmpeg，解码失败时先用它
// 转成 JPEG，再回到原来的“缩放 → 落盘 → 图床”管线。
const convertImageTimeout = 60 * time.Second

// convertImageToJPEG 用 ffmpeg 把任意可解码的图片转成临时 JPEG，返回临时文件路径。
// 调用方负责 os.Remove 返回值。
func convertImageToJPEG(srcPath string) (string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg 不可用: %w", err)
	}
	outPath := srcPath + ".converted.jpg"
	ctx, cancel := context.WithTimeout(context.Background(), convertImageTimeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", srcPath,
		"-frames:v", "1",
		"-q:v", "2",
		outPath,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(outPath)
		return "", fmt.Errorf("ffmpeg 转码失败: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		os.Remove(outPath)
		return "", fmt.Errorf("ffmpeg 转码产物为空")
	}
	return outPath, nil
}
