package domain

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// 全站皮肤（主题）体系。
//
// 设计目标：节日皮肤是「可复用的前端资源包 + 一段活动时间窗」，
// 后端只负责回答两件事：
//  1. 现在是否处于某个节日皮肤的活动时间内（ThemeState 接口）；
//  2. 某个用户选了哪套皮肤、开场画面与背景怎么展示（UserThemeSetting 表）。
//
// 真正的视觉资源（图片、配色、贴纸）都在前端仓库里，后端不存图片路径。
// 新增一套皮肤只需要：
//   - 在后端 themeEvents 里加一条活动时间窗（本文件）；
//   - 在前端 src/skins/ 里加一份皮肤定义（见前端 src/skins/README.md）。
//
// 皮肤选择口径（theme_mode）：
//   - auto：跟随活动时间窗——活动期间自动使用活动皮肤，活动结束后自动回到默认；
//   - default：强制默认皮肤（活动期间也不换）；
//   - <themeID>：强制使用某套皮肤（活动结束后用户仍可手动保留这套皮肤）。
const (
	ThemeModeAuto    = "auto"
	ThemeModeDefault = "default"

	// ProgrammersDay2026ThemeID 2026 程序员节皮肤的稳定 ID，前后端必须一致。
	ProgrammersDay2026ThemeID = "programmers-day-2026"

	// 背景来源：default = 当前皮肤自带背景（默认皮肤没有则用网站底色）；
	// custom = 用户自己上传的图床图片。
	ThemeBackgroundSourceDefault = "default"
	ThemeBackgroundSourceCustom  = "custom"
)

// themeLocation 活动时间窗按北京时间计算，不依赖服务器本地时区。
var themeLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// ThemeEvent 一套节日皮肤的活动时间窗。
type ThemeEvent struct {
	ThemeID  string
	Name     string
	StartsAt time.Time
	EndsAt   time.Time
}

// ActiveAt 活动时间窗左闭右开：[StartsAt, EndsAt)。
func (e ThemeEvent) ActiveAt(now time.Time) bool {
	return !now.Before(e.StartsAt) && now.Before(e.EndsAt)
}

// themeEvents 活动时间窗注册表。新增节日皮肤只加一条。
// 注意：时间为北京时间；结束时间取次日 00:00，避免 23:59:59 的边界争议。
var themeEvents = []ThemeEvent{
	{
		ThemeID:  ProgrammersDay2026ThemeID,
		Name:     "2026 程序员节",
		StartsAt: time.Date(2026, 10, 24, 0, 0, 0, 0, themeLocation),
		EndsAt:   time.Date(2026, 10, 25, 0, 0, 0, 0, themeLocation),
	},
}

// ThemeEvents 返回全部已注册的活动（副本），供只读展示/测试使用。
func ThemeEvents() []ThemeEvent {
	out := make([]ThemeEvent, len(themeEvents))
	copy(out, themeEvents)
	return out
}

// FindThemeEvent 按 ID 查找活动；未知 ID 返回 false。
func FindThemeEvent(themeID string) (ThemeEvent, bool) {
	for _, e := range themeEvents {
		if e.ThemeID == themeID {
			return e, true
		}
	}
	return ThemeEvent{}, false
}

// FeaturedThemeEvent 返回当前要展示的那套活动皮肤：
// 优先返回正在进行的活动；若当前没有活动，返回最近一场（用于展示倒计时/已结束状态）。
// 目前只有一套皮肤；未来多套时按开始时间排序即可。
func FeaturedThemeEvent(now time.Time) (ThemeEvent, bool) {
	if len(themeEvents) == 0 {
		return ThemeEvent{}, false
	}
	for _, e := range themeEvents {
		if e.ActiveAt(now) {
			return e, true
		}
	}
	// 没有进行中的活动：返回开始时间最晚的一场（通常是下一场/刚结束的一场）。
	best := themeEvents[0]
	for _, e := range themeEvents[1:] {
		if e.StartsAt.After(best.StartsAt) {
			best = e
		}
	}
	return best, false
}

// IsKnownThemeMode 校验用户的皮肤选择：auto / default / 已注册的皮肤 ID。
func IsKnownThemeMode(mode string) bool {
	switch mode {
	case ThemeModeAuto, ThemeModeDefault:
		return true
	}
	_, ok := FindThemeEvent(mode)
	return ok
}

// IsKnownThemeBackgroundSource 校验背景来源。
func IsKnownThemeBackgroundSource(source string) bool {
	return source == ThemeBackgroundSourceDefault || source == ThemeBackgroundSourceCustom
}

// ThemeSettings 某个用户的外观设置（只存选择，不存皮肤资源路径）。
type ThemeSettings struct {
	UserID              int
	ThemeMode           string
	SplashEnabled       bool
	BackgroundEnabled   bool
	BackgroundSource    string
	CustomBackgroundURL string
	UpdatedAt           time.Time
}

// DefaultThemeSettings 没有保存过设置时的默认值：
// 跟随活动（活动当天自动换肤）、开场画面开、背景开、用皮肤自带背景。
func DefaultThemeSettings(userID int) ThemeSettings {
	return ThemeSettings{
		UserID:            userID,
		ThemeMode:         ThemeModeAuto,
		SplashEnabled:     true,
		BackgroundEnabled: true,
		BackgroundSource:  ThemeBackgroundSourceDefault,
	}
}

// 外观设置相关业务错误。
var (
	ErrThemeModeInvalid             = errors.New("皮肤选项不合法")
	ErrThemeBackgroundSourceInvalid = errors.New("背景来源不合法")
	ErrThemeBackgroundURLInvalid    = errors.New("背景图片地址不合法，只支持 http/https 图片直链")
)

const themeBackgroundURLMaxLen = 2048

// NormalizeThemeSettings 校验并规范化用户提交的外观设置（就地修改）。
func NormalizeThemeSettings(s *ThemeSettings) error {
	if s == nil {
		return ErrBadRequest
	}
	s.ThemeMode = strings.TrimSpace(s.ThemeMode)
	if s.ThemeMode == "" {
		s.ThemeMode = ThemeModeAuto
	}
	if !IsKnownThemeMode(s.ThemeMode) {
		return ErrThemeModeInvalid
	}

	s.BackgroundSource = strings.TrimSpace(s.BackgroundSource)
	if s.BackgroundSource == "" {
		s.BackgroundSource = ThemeBackgroundSourceDefault
	}
	if !IsKnownThemeBackgroundSource(s.BackgroundSource) {
		return ErrThemeBackgroundSourceInvalid
	}

	normalizedURL, err := NormalizeThemeBackgroundURL(s.CustomBackgroundURL)
	if err != nil {
		return err
	}
	s.CustomBackgroundURL = normalizedURL
	return nil
}

// NormalizeThemeBackgroundURL 校验用户自定义背景地址；空字符串表示未设置。
func NormalizeThemeBackgroundURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	return normalizeRemoteImageURL(raw)
}

// normalizeRemoteImageURL 只接受绝对 http/https 图片直链。
//
// 这是「图床图片」与「前端 public 内置图片」的分界线：
// 用户上传的背景一律存图床返回的绝对地址；内置皮肤背景由前端资源注册表解析，
// 绝不接受相对路径/协议相对地址，避免把 public 路径误存进用户设置。
func normalizeRemoteImageURL(raw string) (string, error) {
	if len(raw) > themeBackgroundURLMaxLen {
		return "", ErrThemeBackgroundURLInvalid
	}
	// 控制字符会破坏日志/响应，直接拒绝。
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "", ErrThemeBackgroundURLInvalid
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ErrThemeBackgroundURLInvalid
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrThemeBackgroundURLInvalid
	}
	// 统一去掉 fragment（图片地址里没有意义），保留 query（图床常用缩放参数）。
	u.Fragment = ""
	return u.String(), nil
}
