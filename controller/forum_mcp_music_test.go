package controller

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestForumMusicIDsBoundedAndUnique(t *testing.T) {
	got := forumMusicIDs("::music 27:: ::music 27:: ::music 0:: ::music -1:: ::music 9999999999:: ::music 28::")
	if !reflect.DeepEqual(got, []int{27, 28}) {
		t.Fatalf("unexpected IDs: %v", got)
	}
	if len(forumMusicIDs(strings.Repeat("::music 1::", 1000))) != 1 {
		t.Fatal("duplicate music IDs")
	}
	if rows, status := forumPostMusic(context.Background(), "no music"); len(rows) != 0 || status != "none" {
		t.Fatal("plain posts should not query music")
	}
}

func TestForumMusicWhitelist(t *testing.T) {
	b, _ := json.Marshal(forumMusic{27, "song", "artist", "album", 123})
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	if len(fields) != 5 || fields["music_id"] != float64(27) || fields["artist"] != "artist" {
		t.Fatalf("unexpected metadata projection %s", b)
	}
}
