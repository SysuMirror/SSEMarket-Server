package controller

import (
	"testing"
	"time"

	"loginTest/model"
)

func TestUploadedVideoModelStoresTranscodeMetadata(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.Local)
	video := uploadedVideoModel(
		7,
		hlsTranscodeResult{DurationSec: 28, Width: 1280, Height: 720, FileSizeBytes: 6762720},
		"乐队test2",
		"乐队 test 2",
		"https://ssemarket.cn/media/videos/test/poster.jpg",
		model.VideoStatusPublished,
		0,
		"202609/test",
		"band-test2.mp4",
		now,
	)

	if video.DurationSec != 28 {
		t.Fatalf("duration = %d, want 28", video.DurationSec)
	}
	if video.Width != 1280 || video.Height != 720 || video.FileSizeBytes != 6762720 {
		t.Fatalf("unexpected metadata: %+v", video)
	}
	if video.PublishedAt == nil || video.Status != model.VideoStatusPublished {
		t.Fatalf("published metadata not set: %+v", video)
	}
}
