package domain_test

import (
	"strings"
	"testing"
	"time"

	"loginTest/domain"
)

func cstTime(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, domain.ThemeLocation())
}

func TestThemeWallClockInterpretsBeijingWallTime(t *testing.T) {
	// MySQL DATETIME 读出来可能带 UTC，但分量是管理员填的北京时间墙钟。
	raw := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	got := domain.ThemeWallClock(raw)
	if got.Format("2006-01-02 15:04:05 -07:00") != "2026-10-24 00:00:00 +08:00" {
		t.Errorf("ThemeWallClock = %s, 期望北京时间 2026-10-24 00:00:00 +08:00", got.Format(time.RFC3339))
	}
}

func TestPickFeaturedThemeEvent(t *testing.T) {
	events := []domain.ThemeEvent{
		{ThemeID: "past", Name: "已结束", StartsAt: cstTime(2026, 10, 20, 0, 0), EndsAt: cstTime(2026, 10, 21, 0, 0)},
		{ThemeID: "current", Name: "进行中", StartsAt: cstTime(2026, 10, 24, 0, 0), EndsAt: cstTime(2026, 10, 25, 0, 0)},
		{ThemeID: "future", Name: "未开始", StartsAt: cstTime(2026, 11, 1, 0, 0), EndsAt: cstTime(2026, 11, 2, 0, 0)},
	}

	event, active := domain.PickFeaturedThemeEvent(events, cstTime(2026, 10, 24, 12, 0))
	if event == nil || event.ThemeID != "current" || !active {
		t.Fatalf("活动期间应返回 current 且 active=true，实际 %+v active=%v", event, active)
	}

	// 活动开始前：返回最近已结束的一期，active=false（前端据此显示「已结束/即将开始」）
	event, active = domain.PickFeaturedThemeEvent(events, cstTime(2026, 10, 23, 12, 0))
	if event == nil || event.ThemeID != "past" || active {
		t.Fatalf("活动前应返回最近已结束的 past 且 active=false，实际 %+v active=%v", event, active)
	}

	// 只有未来活动：返回最早将开始的一期
	event, active = domain.PickFeaturedThemeEvent(events[2:], cstTime(2026, 10, 23, 12, 0))
	if event == nil || event.ThemeID != "future" || active {
		t.Fatalf("只有未来活动时应返回 future 且 active=false，实际 %+v active=%v", event, active)
	}

	if event, _ := domain.PickFeaturedThemeEvent(nil, time.Now()); event != nil {
		t.Errorf("没有活动时应返回 nil，实际 %+v", event)
	}
}

func TestPickFeaturedThemeEventOverlapPrefersLatestStart(t *testing.T) {
	events := []domain.ThemeEvent{
		{ThemeID: "old", StartsAt: cstTime(2026, 10, 24, 0, 0), EndsAt: cstTime(2026, 10, 26, 0, 0)},
		{ThemeID: "new", StartsAt: cstTime(2026, 10, 25, 0, 0), EndsAt: cstTime(2026, 10, 26, 0, 0)},
	}
	event, active := domain.PickFeaturedThemeEvent(events, cstTime(2026, 10, 25, 12, 0))
	if event == nil || event.ThemeID != "new" || !active {
		t.Fatalf("重叠活动应取开始时间最晚的一期，实际 %+v active=%v", event, active)
	}
}

func TestThemeEventWindow(t *testing.T) {
	event := domain.ThemeEvent{
		ThemeID:  domain.ProgrammersDay2026ThemeID,
		StartsAt: cstTime(2026, 10, 24, 0, 0),
		EndsAt:   cstTime(2026, 10, 25, 0, 0),
	}
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"前一天深夜", cstTime(2026, 10, 23, 23, 0), false},
		{"活动开始", cstTime(2026, 10, 24, 0, 0), true},
		{"活动当天中午", cstTime(2026, 10, 24, 12, 0), true},
		{"活动结束前 1 分钟", time.Date(2026, 10, 24, 23, 59, 0, 0, domain.ThemeLocation()), true},
		{"活动结束（次日 0 点）", cstTime(2026, 10, 25, 0, 0), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := event.ActiveAt(tc.now); got != tc.want {
				t.Errorf("ActiveAt(%s) = %v, 期望 %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestThemeModeValidation(t *testing.T) {
	for _, mode := range []string{domain.ThemeModeAuto, domain.ThemeModeDefault, domain.ProgrammersDay2026ThemeID, "halloween-2026"} {
		if !domain.IsValidThemeMode(mode) {
			t.Errorf("%q 应为合法皮肤选项", mode)
		}
	}
	for _, mode := range []string{"", "Programmers-Day", "programmers day", "programmers/day", "-bad", strings.Repeat("a", 65), "javascript:alert(1)"} {
		if domain.IsValidThemeMode(mode) {
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
	if s.PostCardOpacity != domain.ThemePostCardOpacityDefault {
		t.Fatalf("默认卡片透明度应为 %d，实际 %d", domain.ThemePostCardOpacityDefault, s.PostCardOpacity)
	}

	s.ThemeMode = ""
	s.BackgroundSource = ""
	s.CustomBackgroundURL = ""
	s.PostCardOpacity = 0 // 旧客户端不传该字段
	if err := domain.NormalizeThemeSettings(&s); err != nil {
		t.Fatalf("空值应回填默认: %v", err)
	}
	if s.ThemeMode != domain.ThemeModeAuto || s.BackgroundSource != domain.ThemeBackgroundSourceDefault {
		t.Errorf("空值回填结果不对: %+v", s)
	}
	if s.PostCardOpacity != domain.ThemePostCardOpacityDefault {
		t.Errorf("卡片透明度空值应回填默认 %d，实际 %d", domain.ThemePostCardOpacityDefault, s.PostCardOpacity)
	}

	bad := domain.ThemeSettings{ThemeMode: "Not A Skin", BackgroundSource: domain.ThemeBackgroundSourceDefault}
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemeModeInvalid {
		t.Errorf("非法皮肤应返回 ErrThemeModeInvalid，实际 %v", err)
	}
	bad = domain.ThemeSettings{ThemeMode: domain.ThemeModeAuto, BackgroundSource: "remote"}
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemeBackgroundSourceInvalid {
		t.Errorf("非法背景来源应返回 ErrThemeBackgroundSourceInvalid，实际 %v", err)
	}
	bad = domain.ThemeSettings{ThemeMode: domain.ThemeModeAuto, BackgroundSource: domain.ThemeBackgroundSourceDefault, PostCardOpacity: 10}
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemePostCardOpacityInvalid {
		t.Errorf("卡片透明度低于下限应返回 ErrThemePostCardOpacityInvalid，实际 %v", err)
	}
	bad.PostCardOpacity = 120
	if err := domain.NormalizeThemeSettings(&bad); err != domain.ErrThemePostCardOpacityInvalid {
		t.Errorf("卡片透明度高于上限应返回 ErrThemePostCardOpacityInvalid，实际 %v", err)
	}
}
