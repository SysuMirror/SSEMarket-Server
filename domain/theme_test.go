package domain_test

import (
	"strings"
	"testing"
	"time"

	"loginTest/domain"
)

func TestProgrammersDay2026Window(t *testing.T) {
	event, ok := domain.FindThemeEvent(domain.ProgrammersDay2026ThemeID)
	if !ok {
		t.Fatalf("未注册 %s", domain.ProgrammersDay2026ThemeID)
	}
	cst := time.FixedZone("CST", 8*60*60)
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"前一天深夜", time.Date(2026, 10, 23, 23, 59, 59, 0, cst), false},
		{"活动开始", time.Date(2026, 10, 24, 0, 0, 0, 0, cst), true},
		{"活动当天中午", time.Date(2026, 10, 24, 12, 0, 0, 0, cst), true},
		{"活动结束前 1 秒", time.Date(2026, 10, 24, 23, 59, 59, 0, cst), true},
		{"活动结束（次日 0 点）", time.Date(2026, 10, 25, 0, 0, 0, 0, cst), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := event.ActiveAt(tc.now); got != tc.want {
				t.Errorf("ActiveAt(%s) = %v, 期望 %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestFeaturedThemeEventActiveFlag(t *testing.T) {
	cst := time.FixedZone("CST", 8*60*60)
	event, active := domain.FeaturedThemeEvent(time.Date(2026, 10, 24, 9, 0, 0, 0, cst))
	if event.ThemeID != domain.ProgrammersDay2026ThemeID || !active {
		t.Fatalf("活动当天应返回程序员节皮肤且 active=true，实际 %q active=%v", event.ThemeID, active)
	}
	event, active = domain.FeaturedThemeEvent(time.Date(2026, 10, 26, 9, 0, 0, 0, cst))
	if event.ThemeID != domain.ProgrammersDay2026ThemeID || active {
		t.Fatalf("活动结束后仍应返回最近一场用于展示，active=false，实际 %q active=%v", event.ThemeID, active)
	}
}

func TestThemeModeValidation(t *testing.T) {
	for _, mode := range []string{domain.ThemeModeAuto, domain.ThemeModeDefault, domain.ProgrammersDay2026ThemeID} {
		if !domain.IsKnownThemeMode(mode) {
			t.Errorf("%q 应为合法皮肤选项", mode)
		}
	}
	for _, mode := range []string{"", "programmers-day-2027", "javascript:alert(1)"} {
		if domain.IsKnownThemeMode(mode) {
			t.Errorf("%q 不应为合法皮肤选项", mode)
		}
	}
}

func TestNormalizeThemeBackgroundURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"https 图床直链", " https://cdn.example.com/a.jpg?x-oss-process=resize ", "https://cdn.example.com/a.jpg?x-oss-process=resize", false},
		{"http 直链", "http://cdn.example.com/a.png", "http://cdn.example.com/a.png", false},
		{"空值表示未设置", "   ", "", false},
		{"去掉 fragment", "https://cdn.example.com/a.jpg#section", "https://cdn.example.com/a.jpg", false},
		{"拒绝 public 相对路径", "/skins/programmers-day-2026/bg-desktop.webp", "", true},
		{"拒绝相对路径", "skins/bg.webp", "", true},
		{"拒绝协议相对地址", "//cdn.example.com/a.jpg", "", true},
		{"拒绝 javascript", "javascript:alert(1)", "", true},
		{"拒绝 data url", "data:image/png;base64,AAAA", "", true},
		{"拒绝无 host", "https:///a.jpg", "", true},
		{"超长地址", "https://cdn.example.com/" + strings.Repeat("a", 2100), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.NormalizeThemeBackgroundURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if got != tc.want {
				t.Errorf("得到 %q, 期望 %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeThemeSettingsDefaultsAndValidation(t *testing.T) {
	s := domain.DefaultThemeSettings(7)
	if s.ThemeMode != domain.ThemeModeAuto || !s.SplashEnabled || !s.BackgroundEnabled || s.BackgroundSource != domain.ThemeBackgroundSourceDefault {
		t.Fatalf("默认设置不符合预期: %+v", s)
	}

	s.ThemeMode = ""
	s.BackgroundSource = ""
	s.CustomBackgroundURL = ""
	if err := domain.NormalizeThemeSettings(&s); err != nil {
		t.Fatalf("空值应回填默认: %v", err)
	}
	if s.ThemeMode != domain.ThemeModeAuto || s.BackgroundSource != domain.ThemeBackgroundSourceDefault {
		t.Errorf("空值回填结果不对: %+v", s)
	}

	bad := domain.ThemeSettings{ThemeMode: "not-a-skin", BackgroundSource: domain.ThemeBackgroundSourceDefault}
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemeModeInvalid {
		t.Errorf("非法皮肤应返回 ErrThemeModeInvalid，实际 %v", err)
	}
	bad = domain.ThemeSettings{ThemeMode: domain.ThemeModeAuto, BackgroundSource: "remote"}
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemeBackgroundSourceInvalid {
		t.Errorf("非法背景来源应返回 ErrThemeBackgroundSourceInvalid，实际 %v", err)
	}
}
