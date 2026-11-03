package controller

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"loginTest/api"
)

// Go 标准库 image.Decode 对动图只解第一帧，走“解码→缩放→保存”管线会把
// GIF/动图WebP/APNG 压成静图。图床按原始字节透传存储，因此检测到动图格式时
// 跳过常规缩图管线：动图 GIF 交给容器内 ffmpeg 全帧缩放重编码（palettegen/
// paletteuse），压不动或失败就原文件直通；动图 WebP/APNG 没有合适的重编码
// 工具，保持原文件直通。
const (
	animatedHeadBytes      = 64 << 10
	animatedGIFTimeout     = 60 * time.Second
	animatedGIFSuffixScale = ".anim-scaled.gif"
	animatedGIFSuffixLossy = ".anim.gif"
)

// isLikelyAnimatedRaster 依据文件头判断是否为“解码会丢动画”的格式：
// GIF（含单帧，重编码没有收益）、动图 WebP（VP8X 首块且 ANIM 位 0x02）、
// APNG（IDAT 之前出现 acTL 块）。
func isLikelyAnimatedRaster(head []byte) bool {
	if len(head) < 16 {
		return false
	}
	switch {
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return true
	case len(head) >= 21 && bytes.HasPrefix(head, []byte("RIFF")) &&
		bytes.Equal(head[8:12], []byte("WEBP")) && bytes.Equal(head[12:16], []byte("VP8X")):
		return head[20]&0x02 != 0
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		// 顺次走顶层块；遇到 acTL 判动图，走到 IDAT/IEND 或头读完则按静图处理。
		for p := 8; p+12 <= len(head); {
			length := int(binary.BigEndian.Uint32(head[p : p+4]))
			typ := head[p+4 : p+8]
			if bytes.Equal(typ, []byte("acTL")) {
				return true
			}
			if bytes.Equal(typ, []byte("IDAT")) || bytes.Equal(typ, []byte("IEND")) || length < 0 {
				return false
			}
			p += 12 + length
		}
	}
	return false
}

// isAnimatedGIFHead 文件头是否为 GIF（含单帧）。
func isAnimatedGIFHead(head []byte) bool {
	return bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a"))
}

// readHeadBytes 读取文件头用于格式判断，不足 maxBytes 时按实际长度返回。
func readHeadBytes(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

// compressAnimatedGIF 对动图 GIF 做保帧压缩：第一段 ffmpeg 按 maxDim 封顶
// 长边（不放大）lanczos 缩放 + stats_mode=diff 调色板重编码；第二段 gifsicle
// -O3 --lossy 有损优化（工具缺失或失败自动跳过，不影响第一段产物）。
// ffmpeg 缺失、超时、失败，或最终产物不是合法 GIF、不比原文件小，返回空串
// （调用方回退直传原文件）。返回的临时文件由调用方负责删除。
func compressAnimatedGIF(localPath string, maxDim int) (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg 不可用: %w", err)
	}
	scaledPath := localPath + animatedGIFSuffixScale
	outPath := localPath + animatedGIFSuffixLossy
	// min(cap,iw/ih) + force_original_aspect_ratio=decrease 等价于静图管线的
	// imaging.Fit：只缩小、不放大、保持比例。
	vf := fmt.Sprintf(
		"scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos,"+
			"split[s0][s1];[s0]palettegen=stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle",
		maxDim, maxDim)

	ctx, cancel := context.WithTimeout(context.Background(), animatedGIFTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", localPath, "-vf", vf, "-loop", "0", scaledPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(scaledPath)
		return "", fmt.Errorf("ffmpeg 重编码失败: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	outHead, err := readHeadBytes(scaledPath, 6)
	if err != nil || !isAnimatedGIFHead(outHead) {
		os.Remove(scaledPath)
		return "", fmt.Errorf("ffmpeg 产物不是合法 GIF")
	}
	finalPath := scaledPath

	// gifsicle 有损优化：对 GIF 体积影响最大的通常是逐帧冗余而非像素精度，
	// lossy=60 在视觉上基本无感。失败仅退回第一段产物。
	// 中间/最终文件名都必须以 .gif 结尾——UploadImage 按文件名交给图床识别类型。
	if gifsiclePath, lerr := exec.LookPath("gifsicle"); lerr == nil {
		lossCtx, lossCancel := context.WithTimeout(context.Background(), animatedGIFTimeout)
		defer lossCancel()
		lcmd := exec.CommandContext(lossCtx, gifsiclePath,
			"--no-warnings", "-O3", "--lossy=60", scaledPath, "-o", outPath)
		var lstderr bytes.Buffer
		lcmd.Stderr = &lstderr
		if lerr := lcmd.Run(); lerr != nil {
			log.Printf("[compressAnimatedGIF] gifsicle 失败，用 ffmpeg 产物: %v (%s)", lerr, strings.TrimSpace(lstderr.String()))
			os.Remove(outPath)
		} else if lh, herr := readHeadBytes(outPath, 6); herr != nil || !isAnimatedGIFHead(lh) {
			os.Remove(outPath)
		} else {
			os.Remove(scaledPath)
			finalPath = outPath
		}
	}

	origInfo, err1 := os.Stat(localPath)
	outInfo, err2 := os.Stat(finalPath)
	if err1 != nil || err2 != nil || outInfo.Size() >= origInfo.Size() {
		// 小图重编码可能反而变大，不值得保留
		os.Remove(finalPath)
		return "", nil
	}
	log.Printf("[compressAnimatedGIF] %d -> %d bytes (maxDim=%d)", origInfo.Size(), outInfo.Size(), maxDim)
	return finalPath, nil
}

// processAnimatedImage 若 localPath 是动图：GIF 先按 maxDim 保帧压缩（失败
// 则直通原文件），WebP/APNG 直通原文件，上传图床并返回 URL；不是动图（或
// 文件头读取失败）返回空串，调用方继续走常规缩图管线。
func processAnimatedImage(localPath string, maxDim int) (string, error) {
	head, err := readHeadBytes(localPath, animatedHeadBytes)
	if err != nil {
		return "", nil
	}
	if !isLikelyAnimatedRaster(head) {
		return "", nil
	}
	if isAnimatedGIFHead(head) {
		if compressed, cerr := compressAnimatedGIF(localPath, maxDim); cerr != nil {
			log.Printf("[processAnimatedImage] GIF 压缩失败，回退直传: path=%s err=%v", localPath, cerr)
		} else if compressed != "" {
			defer os.Remove(compressed)
			return api.UploadImage("", compressed)
		}
	}
	return api.UploadImage("", localPath)
}
