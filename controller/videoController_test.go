package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"loginTest/domain"
	"loginTest/model"
)

func TestAdminFromVideoContextAcceptsAuthAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("admin", &domain.Admin{ID: 7, Account: "marketsse"})

	admin, ok := adminFromVideoContext(ctx)
	if !ok {
		t.Fatal("expected auth admin context to be accepted")
	}
	if admin.AdminID != 7 {
		t.Fatalf("unexpected admin ID: %d", admin.AdminID)
	}
}

func TestParseVideoSource(t *testing.T) {
	st, url, bvid, page, err := ParseVideoSource("auto", "https://www.bilibili.com/video/BV1P1421U775/")
	if err != nil || st != "bilibili" || url != "https://www.bilibili.com/video/BV1P1421U775/" || bvid != "BV1P1421U775" || page != url {
		t.Fatalf("bilibili parse: %q %q %q %q %v", st, url, bvid, page, err)
	}

	st, url, bvid, page, err = ParseVideoSource("mp4", "https://ssemarket.cn/videos/a.mp4")
	if err != nil || st != "mp4" || url != "https://ssemarket.cn/videos/a.mp4" || bvid != "" || page != "" {
		t.Fatalf("mp4 parse: %q %q %q %q %v", st, url, bvid, page, err)
	}

	st, _, _, _, err = ParseVideoSource("hls", "https://ssemarket.cn/videos/master.m3u8")
	if err != nil || st != "hls" {
		t.Fatalf("hls parse: %q %v", st, err)
	}

	for _, tc := range []struct{ kind, raw string }{
		{"auto", "http://ssemarket.cn/a.mp4"},
		{"auto", "https://evil.example/a.mp4"},
		{"bilibili", "https://evil.example/video/BV12345678/"},
		{"hls", "https://ssemarket.cn/a.mp4"},
	} {
		if _, _, _, _, err := ParseVideoSource(tc.kind, tc.raw); err == nil {
			t.Fatalf("expected rejection for %s %s", tc.kind, tc.raw)
		}
	}
}

func TestPublicVideoOmitsPrivateFields(t *testing.T) {
	b, err := json.Marshal(publicVideo(model.Video{
		VideoID: 1, Title: "t", Description: "d", CoverURL: "https://ssemarket.cn/c.jpg",
		SourceType: "mp4", SourceURL: "https://ssemarket.cn/a.mp4", DurationSec: 10,
		StorageKey: "videos/1/original.mp4", PlaybackKey: "videos/1/hls/master.m3u8",
		TranscodeError: "boom",
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"adminID", "storageKey", "playbackKey", "transcodeError"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("public response contains %s", forbidden)
		}
	}
}

func createTestVideo(t *testing.T, path string, seconds int) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	args := []string{
		"-y", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", fmt.Sprintf("%d", seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", path,
	}
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, out)
	}
}

func TestProbeVideoRejectsVideosOverThirtySeconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "too-long.mp4")
	createTestVideo(t, path, 31)

	if _, err := probeVideoFile(path); err == nil {
		t.Fatal("expected a 31-second video to be rejected")
	}
}

func TestTranscodeVideoToHLS(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.mp4")
	createTestVideo(t, input, 2)

	probe, err := probeVideoFile(input)
	if err != nil {
		t.Fatalf("probe short video: %v", err)
	}
	if probe.Width != 320 || probe.Height != 240 || probe.DurationSec != 2 {
		t.Fatalf("unexpected probe: %+v", probe)
	}

	outputDir := filepath.Join(dir, "hls")
	result, err := transcodeVideoToHLS(input, outputDir)
	if err != nil {
		t.Fatalf("transcode: %v", err)
	}
	if result.Width != 320 || result.Height != 240 || result.DurationSec != 2 || result.FileSizeBytes <= 0 {
		t.Fatalf("unexpected transcode result: %+v", result)
	}
	playlist, err := os.ReadFile(filepath.Join(outputDir, "master.m3u8"))
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	if !strings.Contains(string(playlist), "#EXTINF:") {
		t.Fatal("playlist contains no segments")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "poster.jpg")); err != nil {
		t.Fatalf("poster missing: %v", err)
	}
	if time.Duration(result.DurationSec)*time.Second > 30*time.Second {
		t.Fatalf("transcoded duration exceeds limit: %+v", result)
	}
}
