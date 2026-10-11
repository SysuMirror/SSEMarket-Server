package domain_test

import (
	"testing"

	"loginTest/domain"
)

func TestSplitNovelParagraphsDropsBlankLines(t *testing.T) {
	content := "第一段\r\n\n第二段  \n\n\n第三段"
	got := domain.SplitNovelParagraphs(content)
	want := []string{"第一段", "第二段", "第三段"}
	if len(got) != len(want) {
		t.Fatalf("分段数 = %d (%v), 期望 %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 段 = %q, 期望 %q", i+1, got[i], want[i])
		}
	}
}

func TestNovelParagraphImage(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"markdown 图片", "![封面](https://cdn.example/a.png)", "https://cdn.example/a.png"},
		{"markdown 空说明", "![](https://cdn.example/a.jpg)", "https://cdn.example/a.jpg"},
		{"markdown 带标题", "![x](https://cdn.example/a.webp \"标题\")", "https://cdn.example/a.webp"},
		{"裸直链", "https://cdn.example/dir/photo.PNG", "https://cdn.example/dir/photo.PNG"},
		{"带 query 的图床链接", "https://cdn.example/a.png?x-oss-process=resize", "https://cdn.example/a.png?x-oss-process=resize"},
		{"协议相对链接", "//cdn.example/a.gif", "//cdn.example/a.gif"},
		{"正文不是图片", "他看着窗外的雨，久久没有说话。", ""},
		{"文字里夹了链接（整段不是纯图片）", "看图 https://cdn.example/a.png", ""},
		{"非图片扩展名", "![附件](https://cdn.example/a.pdf)", ""},
		{"markdown 明确非图片（视频）", "![录像](https://cdn.example/a.mp4)", ""},
		{"markdown 无扩展名的图床链接", "![配图](https://image.example/i/1728abc)", "https://image.example/i/1728abc"},
		{"裸链接无扩展名（自动识别不认）", "https://image.example/i/1728abc", ""},
		{"markdown 语法不完整", "![缺括号](https://cdn.example/a.png", ""},
		{"空段落", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.NovelParagraphImage(tc.text); got != tc.want {
				t.Errorf("NovelParagraphImage(%q) = %q, 期望 %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestImageParagraphIsItsOwnParagraph(t *testing.T) {
	// 「图片上下各留一个空行」在分段规则里天然就是一段独立段落。
	content := "正文一\n\n![](https://cdn.example/a.png)\n\n正文二"
	paragraphs := domain.SplitNovelParagraphs(content)
	if len(paragraphs) != 3 {
		t.Fatalf("分段数 = %d (%v), 期望 3", len(paragraphs), paragraphs)
	}
	if domain.NovelParagraphImage(paragraphs[1]) != "https://cdn.example/a.png" {
		t.Errorf("第 2 段应为图片段落，实际 %q", paragraphs[1])
	}
	if domain.NovelParagraphImage(paragraphs[0]) != "" || domain.NovelParagraphImage(paragraphs[2]) != "" {
		t.Errorf("文字段落不应被判为图片段落")
	}
}

func TestNovelContentWordCountSkipsImageParagraphs(t *testing.T) {
	// 图片链接不计入字数；文字按非空白 rune 计数。
	text := "abcd\n\n![](https://cdn.example/aaaaaaaaaaaaaaaaaaaa.png)\n\nefgh"
	if got := domain.NovelContentWordCount(text); got != 8 {
		t.Errorf("字数 = %d, 期望 8（图片段落不计入）", got)
	}
	if got := domain.NovelWordCount(text); got <= 8 {
		t.Errorf("NovelWordCount 仍按原始口径统计，实际 %d", got)
	}
}

func TestNovelParagraphExcerptForImage(t *testing.T) {
	if got := domain.NovelParagraphExcerpt("![](https://cdn.example/a.png)", 40); got != "[图片]" {
		t.Errorf("摘要 = %q, 期望 [图片]", got)
	}
	long := "一二三四五六七八九十"
	if got := domain.NovelParagraphExcerpt(long, 5); got != "一二三四五…" {
		t.Errorf("摘要 = %q, 期望「一二三四五…」", got)
	}
}
