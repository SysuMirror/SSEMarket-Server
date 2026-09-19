package controller

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

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
