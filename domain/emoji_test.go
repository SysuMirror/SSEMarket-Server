package domain

import (
	"strings"
	"testing"
)

func TestStripEmojiTokens(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"[emoji:1]", "[表情]"},
		{"你好[emoji:12]呀[emoji:3]", "你好[表情]呀[表情]"},
		{"纯文字不被动", "纯文字不被动"},
		{"[emoji:abc] 不是合法 token", "[emoji:abc] 不是合法 token"},
		{"", ""},
	}
	for _, c := range cases {
		if got := StripEmojiTokens(c.in); got != c.want {
			t.Errorf("StripEmojiTokens(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeEmojiName(t *testing.T) {
	if got := NormalizeEmojiName("  乖巧  "); got != "乖巧" {
		t.Errorf("trim 失败: %q", got)
	}
	if got := NormalizeEmojiName(""); got != "表情" {
		t.Errorf("空名应回落: %q", got)
	}
	long := strings.Repeat("啊", EmojiNameMaxLen+10)
	if got := NormalizeEmojiName(long); len([]rune(got)) != EmojiNameMaxLen {
		t.Errorf("应按 rune 截断到 %d, got %d", EmojiNameMaxLen, len([]rune(got)))
	}
}

func TestBuiltinEmojiFilesShape(t *testing.T) {
	seen := map[string]bool{}
	for _, file := range BuiltinEmojiFiles {
		name := BuiltinEmojiName(file)
		if name == "" || name == file {
			t.Errorf("内置表情 %q 应有含义名", file)
		}
		if !strings.HasSuffix(file, ".png") && !strings.HasSuffix(file, ".jpg") {
			t.Errorf("内置表情 %q 扩展名异常", file)
		}
		if seen[name] {
			t.Errorf("内置表情含义重复: %q", name)
		}
		seen[name] = true
	}
	if len(BuiltinEmojiFiles) == 0 {
		t.Fatal("内置表情清单不能为空")
	}
}

// TestBuiltinPackTitles 锁住两个组名：内置组叫「小汐（SSE娘）」，
// 个人表情组默认叫「我的表情」。改名要连前端文案一起改，所以两侧都有测试盯着。
func TestBuiltinPackTitles(t *testing.T) {
	if BuiltinPackTitle == "" {
		t.Fatal("内置组标题不能为空")
	}
	if !strings.Contains(BuiltinPackTitle, "SSE娘") {
		t.Errorf("内置组标题应包含「SSE娘」, got %q", BuiltinPackTitle)
	}
	if PersonalPackTitle == "" {
		t.Fatal("个人表情组标题不能为空")
	}
}
