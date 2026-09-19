# Main-site Video Catalog Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make admin uploads register unified media assets and make `/video` show the published catalog.

**Architecture:** The main site accepts the administrator multipart upload, validates catalog fields, and streams the file to the image Pod media API with an internal bearer token. It stores the returned `media_asset_id`, public HLS/poster URLs, and catalog metadata in `videos`. The public frontend keeps using the existing `/api/auth/videos` endpoints and Artplayer + hls.js player.

**Tech Stack:** Go 1.25, Gin, GORM, MySQL, Vue 3, TypeScript, Element Plus, Artplayer, hls.js.

**Spec:** docs/superpowers/specs/2026-09-19-main-site-video-catalog.md

## Global Constraints

- Source videos: `.mp4`, `.m4v`, `.mov`, `.webm`; at most 200 MB and 30 seconds.
- Output: 720p HLS, about 1 Mbps video and 128 kbps AAC, 4-second segments.
- Public playback URL: `https://image.ssemarket.cn/m/videos/{id}/master.m3u8`.
- Do not expose or log `MEDIA_INTERNAL_TOKEN`.
- Do not delete existing server media, backups, build outputs, or test artifacts.

### Task 1: Media Upload Client

**Files:**
- Create: `controller/mediaClient.go`
- Test: `controller/mediaClient_test.go`
- Modify: `controller/videoUpload.go`

**Interfaces:**
- Consumes: image Pod `POST /api/media/videos`, response `{code:0,data:MediaAsset}`.
- Produces: `type MediaUploadResult struct` and `func uploadVideoToMedia(path, filename string) (*MediaUploadResult, error)`.

- [ ] **Step 1: Write the failing client test**

Use an `httptest.Server` that checks the bearer token and multipart filename, then returns asset ID 12 and `status=ready`. Set both environment variables with `t.Setenv`, write a tiny temporary file, and assert the parsed asset.

- [ ] **Step 2: Run the test and verify RED**

Run: `cd /root/SSE_Market/worktrees/video-player/server && /usr/local/go/bin/go test ./controller -run TestUploadVideoToMedia -count=1`
Expected: compile failure because `uploadVideoToMedia` is undefined.

- [ ] **Step 3: Implement streaming multipart upload**

Use `io.Pipe` and `multipart.Writer`; never buffer the video in memory. Send `Authorization: Bearer $MEDIA_INTERNAL_TOKEN` to `$MEDIA_SERVICE_BASE_URL/api/media/videos`. Reject non-2xx or `code != 0`. Return only media asset fields.

- [ ] **Step 4: Run client tests**

Run: `/usr/local/go/bin/go test ./controller -run TestUploadVideoToMedia -count=1`
Expected: PASS.

### Task 2: Media-backed Business Record

**Files:**
- Modify: `model/video.go`
- Modify: `controller/videoUpload.go`
- Test: `controller/videoUpload_test.go`

**Interfaces:**
- Consumes: `MediaUploadResult`.
- Produces: `func videoFromMediaAsset(adminID int, asset *MediaUploadResult, title, description, coverURL, status string, sortWeight int, originalName string, now time.Time) model.Video`.

- [ ] **Step 1: Write the failing model-conversion test**

Assert that asset ID 12 produces `media_asset_id=12`, `source_type=hls`, `source_url=https://image.ssemarket.cn/m/videos/12/master.m3u8`, poster URL `.../poster.jpg`, rounded duration, dimensions and size. Assert `StorageKey` and `PlaybackKey` remain empty for new media-backed rows.

- [ ] **Step 2: Run test and verify RED**

Run: `/usr/local/go/bin/go test ./controller -run TestVideoFromMediaAsset -count=1`
Expected: compile failure because the conversion function is undefined.

- [ ] **Step 3: Add MediaAssetID and implement conversion**

Add `MediaAssetID int64` as `column:media_asset_id` on `model.Video`. Build public URLs from `MEDIA_PUBLIC_BASE_URL`, defaulting to `https://image.ssemarket.cn`.

- [ ] **Step 4: Run model and upload tests**

Run: `/usr/local/go/bin/go test ./model ./controller -run "TestVideo|TestUploadVideo|TestMedia" -count=1`
Expected: PASS.

### Task 3: Replace Local Transcoding Upload Path

**Files:**
- Modify: `controller/videoUpload.go`
- Modify: `controller/videoController_test.go`
- Modify: `controller/videoUpload_test.go`
- Modify: `common/database.go`

- [ ] **Step 1: Preserve a failing regression test**

Change old local-transcode tests into a catalog test: `videoFromMediaAsset` must set published time and no local storage reference. Remove tests that directly execute ffmpeg through the main backend.

- [ ] **Step 2: Replace upload flow**

Validate extension, size, title, status, and sort weight. Save the upload to `os.CreateTemp`, call `uploadVideoToMedia`, remove the temporary source with `defer`, then call `videoFromMediaAsset` and insert it. On database failure return an error without deleting the media asset.

- [ ] **Step 3: Add `model.Video` to AutoMigrate**

Confirm `common/database.go` already contains `db.AutoMigrate(&model.Video{})`; retain it.

- [ ] **Step 4: Run the focused Go suite**

Run: `/usr/local/go/bin/go test ./model ./controller -count=1`
Expected: PASS.

### Task 4: Admin UI Compatibility

**Files:**
- Modify: `../admin/src/views/VideosView.vue`
- Test: `../admin/src/views/VideosView.test.ts`

- [ ] **Step 1: Update accepted extensions test/copy**

Remove `.mkv` from accepted list because the unified media service does not currently accept it; display `.mp4 / .m4v / .mov / .webm`.

- [ ] **Step 2: Run admin tests and typecheck**

Run: `cd /root/SSE_Market/worktrees/video-player/admin && pnpm test && pnpm typecheck`
Expected: PASS.

### Task 5: Deployment Verification

**Files:**
- No new source files.

- [ ] **Step 1: Build and start a test backend container**

Use a unique image tag and port; do not remove existing containers.

- [ ] **Step 2: Verify API and public page**

Call `/health`, `/ready`, and `/api/auth/videos` against the test container. Build both frontends and verify `/new/video` returns the SPA shell.

- [ ] **Step 3: Production cutover with rollback identity**

Record the current production image and start the new container on the existing network alias. Do not remove the old container/image. Re-check `/health`, `/ready`, `/api/auth/videos`, `/new/video`, and one direct media playlist.

- [ ] **Step 4: Commit source changes only**

Commit backend, admin source, lockfile changes, and plan documents. Exclude build output directories.
