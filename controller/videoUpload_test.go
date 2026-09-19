package controller

import (
	"testing"
	"time"

	"loginTest/model"
)

func TestVideoFromMediaAssetStoresCatalogReference(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.Local)
	video := videoFromMediaAsset(
		7,
		&MediaUploadResult{
			ID:             12,
			Status:         "ready",
			PlaylistKey:    "media/videos/12/master.m3u8",
			PosterKey:      "media/videos/12/poster.jpg",
			DurationMillis: 29480,
			Width:          1280,
			Height:         720,
			FileSizeBytes:  6762720,
		},
		"乐队test2",
		"乐队 test 2",
		"",
		model.VideoStatusPublished,
		0,
		"band-test2.mp4",
		now,
	)

	if video.MediaAssetID != 12 {
		t.Fatalf("media asset ID = %d, want 12", video.MediaAssetID)
	}
	if video.SourceType != "hls" || video.SourceURL != "https://image.ssemarket.cn/m/videos/12/master.m3u8" {
		t.Fatalf("unexpected playback reference: %s %s", video.SourceType, video.SourceURL)
	}
	if video.CoverURL != "https://image.ssemarket.cn/m/videos/12/poster.jpg" {
		t.Fatalf("poster URL = %s", video.CoverURL)
	}
	if video.DurationSec != 29 || video.Width != 1280 || video.Height != 720 || video.FileSizeBytes != 6762720 {
		t.Fatalf("unexpected metadata: %+v", video)
	}
	if video.StorageKey != "" || video.PlaybackKey != "" {
		t.Fatalf("new catalog row duplicated storage paths: %+v", video)
	}
	if video.PublishedAt == nil || video.Status != model.VideoStatusPublished {
		t.Fatalf("published metadata not set: %+v", video)
	}
}
