# Main-site Video Catalog Requirements

## Scope

- Administrators can upload short videos and publish them.
- Ordinary users can view and play published videos, but cannot upload.
- Posts do not need to embed videos in this iteration.
- Uploaded videos are transferred to the unified media service on the image Pod.
- The media service owns physical storage, HLS slicing, and media asset identity.
- The main site stores only catalog/business data and references `media_asset_id`.
- Public playback uses `https://image.ssemarket.cn/m/videos/{id}/master.m3u8`.
- The browser must not receive the media service internal bearer token.
- Uploaded source video is at most 200 MB and 30 seconds; output is 720p HLS at about 1 Mbps.
- Existing media and server files must not be deleted.

## User-visible Result

`https://ssemarket.cn/video` shows a real list of published catalog records. Selecting one opens the HLS player at `/video/{id}` and plays from the media Pod public route.
