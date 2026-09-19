package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadVideoToMediaReturnsAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/media/videos" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.NotFound(w, r)
			return
		}
		contentType := r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(contentType, "multipart/form-data") || !strings.Contains(string(body), `filename="clip.mp4"`) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":{"id":12,"status":"ready","playlist_key":"media/videos/x/master.m3u8","poster_key":"media/videos/x/poster.jpg","duration_millis":29480,"width":1280,"height":720,"file_size_bytes":4096000}}`))
	}))
	defer server.Close()

	t.Setenv("MEDIA_SERVICE_BASE_URL", server.URL)
	t.Setenv("MEDIA_INTERNAL_TOKEN", "test-token")
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	asset, err := uploadVideoToMedia(path, "clip.mp4")
	if err != nil {
		t.Fatalf("uploadVideoToMedia() error = %v", err)
	}
	if asset.ID != 12 || asset.Status != "ready" || asset.PlaylistKey != "media/videos/x/master.m3u8" {
		t.Fatalf("unexpected asset: %+v", asset)
	}
	if asset.DurationMillis != 29480 || asset.Width != 1280 || asset.Height != 720 || asset.FileSizeBytes != 4096000 {
		t.Fatalf("unexpected media metadata: %+v", asset)
	}
}

func TestUploadVideoToMediaReturnsServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"invalid token"}`))
	}))
	defer server.Close()

	t.Setenv("MEDIA_SERVICE_BASE_URL", server.URL)
	t.Setenv("MEDIA_INTERNAL_TOKEN", "wrong")
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := uploadVideoToMedia(path, "clip.mp4"); err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("expected service error, got %v", err)
	}
}
