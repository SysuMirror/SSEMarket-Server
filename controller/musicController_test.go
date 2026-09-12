package controller

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dhowden/tag"
)

// 构造一个带 ID3v2.3 标签（含标题与内嵌封面）的极简 mp3，验证 dhowden/tag 的解析用法
func buildTestMP3() []byte {
	frame := func(id string, payload []byte) []byte {
		head := make([]byte, 0, 10)
		head = append(head, id...)
		size := len(payload)
		head = append(head, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
		head = append(head, 0x00, 0x00) // flags
		return append(head, payload...)
	}

	tit2 := append([]byte{0x00}, []byte("Test Song")...) // ISO-8859-1 编码
	apic := []byte{0x00}                                  // 编码
	apic = append(apic, []byte("image/png")...)
	apic = append(apic, 0x00)          // mime 结束
	apic = append(apic, 0x03)          // 封面类型：front cover
	apic = append(apic, []byte("cover")...)
	apic = append(apic, 0x00) // 描述结束
	apic = append(apic, 0x89, 0x50, 0x4E, 0x47) // 假 PNG 头

	body := append(frame("TIT2", tit2), frame("APIC", apic)...)
	syncsafe := []byte{byte(len(body) >> 21 & 0x7f), byte(len(body) >> 14 & 0x7f), byte(len(body) >> 7 & 0x7f), byte(len(body) & 0x7f)}

	mp3 := append([]byte("ID3"), 0x03, 0x00, 0x00)
	mp3 = append(mp3, syncsafe...)
	mp3 = append(mp3, body...)
	// 一个 MPEG 帧同步头，便于嗅探为音频
	return append(mp3, 0xFF, 0xFB, 0x90, 0x00)
}

func TestReadTagsFromMP3(t *testing.T) {
	m, err := tag.ReadFrom(bytes.NewReader(buildTestMP3()))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if m.Title() != "Test Song" {
		t.Errorf("标题 = %q, 期望 Test Song", m.Title())
	}
	pic := m.Picture()
	if pic == nil {
		t.Fatal("未解析出内嵌封面")
	}
	if pic.Ext != "png" {
		t.Errorf("封面扩展名 = %q, 期望 png", pic.Ext)
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		`../../etc/passwd`: `.._.._etc_passwd`,
		`a/b\c.mp3`:        `a_b_c.mp3`,
		"  ":               "untitled",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestSafeMusicPath(t *testing.T) {
	if _, ok := safeMusicPath(musicFilesDir, "202609/123_a.mp3"); !ok {
		t.Error("正常相对路径应通过校验")
	}
	if _, ok := safeMusicPath(musicFilesDir, "../../etc/passwd"); ok {
		t.Error("目录穿越路径应被拒绝")
	}
}

func TestSaveMusicCover(t *testing.T) {
	// 切到临时目录，避免测试产物落在仓库 public/ 下
	oldWd, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Skipf("无法切换工作目录: %v", err)
	}
	defer os.Chdir(oldWd)

	name := saveMusicCover([]byte("fakepng"), "png")
	if name == "" {
		t.Fatal("封面应保存成功")
	}
	if filepath.Ext(name) != ".png" {
		t.Errorf("封面扩展名 = %q, 期望 .png", filepath.Ext(name))
	}
	if _, err := os.Stat(musicCoverAbsPath(name)); err != nil {
		t.Errorf("封面文件应存在: %v", err)
	}
	if got := saveMusicCover(make([]byte, maxCoverSize+1), "png"); got != "" {
		t.Error("超限封面应被拒绝")
	}
	if got := saveMusicCover([]byte("x"), "exe"); filepath.Ext(got) != ".jpg" {
		t.Errorf("未知扩展名应兜底为 .jpg, got %q", got)
	}
}
