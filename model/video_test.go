package model

import "testing"

func TestVideoTableName(t *testing.T) {
	if got := (Video{}).TableName(); got != "videos" {
		t.Fatalf("TableName() = %q, want videos", got)
	}
	if VideoStatusDraft != "draft" || VideoStatusPublished != "published" || VideoStatusHidden != "hidden" {
		t.Fatal("video status constants changed")
	}
}
