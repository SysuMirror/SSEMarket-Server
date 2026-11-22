package service

import (
	"testing"

	"loginTest/config"
)

func TestCommentContentLimit(t *testing.T) {
	t.Setenv("XQ_BOT_UID", "42")

	if got := commentContentLimit(7); got != config.CommentContentMaxRunes {
		t.Fatalf("普通用户上限 = %d, want %d", got, config.CommentContentMaxRunes)
	}
	if got := commentContentLimit(42); got != 2800 {
		t.Fatalf("小秋 bot 上限 = %d, want 2800", got)
	}

	t.Setenv("XQ_BOT_UID", "")
	if got := commentContentLimit(42); got != config.CommentContentMaxRunes {
		t.Fatalf("未配置 bot 时上限 = %d, want %d", got, config.CommentContentMaxRunes)
	}
}
