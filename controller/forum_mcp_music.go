package controller

import (
	"context"
	"regexp"
	"strconv"

	"loginTest/common"
	"loginTest/model"
)

var forumMusicMarker = regexp.MustCompile(`::music\s+([0-9]{1,10})\s*::`)

type forumMusic struct {
	MusicID     int    `json:"music_id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	DurationSec int    `json:"duration_seconds"`
}

func forumMusicIDs(content string) []int {
	ids := make([]int, 0, 8)
	seen := map[int]bool{}
	for _, match := range forumMusicMarker.FindAllStringSubmatch(content, -1) {
		id, err := strconv.ParseInt(match[1], 10, 32)
		if err == nil && id > 0 && !seen[int(id)] {
			seen[int(id)] = true
			ids = append(ids, int(id))
			if len(ids) == 8 {
				break
			}
		}
	}
	return ids
}

// Metadata only: published, nondeleted music referenced by an already-visible post.
// No files, storage keys, uploader identity, playback increments or audio downloads.
func forumPostMusic(ctx context.Context, content string) ([]forumMusic, string) {
	ids := forumMusicIDs(content)
	result := make([]forumMusic, 0, len(ids))
	if len(ids) == 0 {
		return result, "none"
	}
	db := common.GetDB()
	if db == nil {
		return result, "unavailable"
	}
	var rows []model.Music
	if err := db.WithContext(ctx).Select("musicID, title, artist, album, duration_sec").
		Where("musicID IN (?) AND is_published = 1 AND is_delete = 0", ids).
		Limit(8).Find(&rows).Error; err != nil {
		return result, "unavailable"
	}
	byID := map[int]model.Music{}
	for _, row := range rows {
		byID[row.MusicID] = row
	}
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			result = append(result, forumMusic{row.MusicID, row.Title, row.Artist, row.Album, row.DurationSec})
		}
	}
	return result, "resolved"
}
