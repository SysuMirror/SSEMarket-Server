package domain

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 全站皮肤（主题）体系。
//
// 后端只负责两件事：
//  1. 从 theme_events 表读取活动时间窗，回答「现在是否处于某期节日活动内」；
//  2. 保存某个用户的皮肤选择、开场画面与背景设置（user_theme_settings）。
//
// 活动时间窗不写死在代码里：管理员直接 INSERT theme_events 一行即可，
// 想加场/改期/停用都只动数据。真正的视觉资源（图片、配色、贴纸）仍在前端
// src/skins/ 中，后端不存图片路径。
//
// 皮肤选择口径（theme_mode）：
//   - auto：跟随活动时间窗——活动期间自动使用活动皮肤，结束后自动回到默认；
//   - default：强制默认皮肤（活动期间也不换）；
//   - <themeID>：强制使用某套皮肤（活动结束后用户仍可手动保留）。
const (
	ThemeModeAuto    = "auto"
	ThemeModeDefault = "default"

	// ProgrammersDay2026ThemeID 2026 程序员节皮肤的稳定 ID，前后端必须一致。
	// 时间窗不在代码里，只是常用 ID 的别名，方便测试/文档引用。
	ProgrammersDay2026ThemeID = "programmers-day-2026"

	// 背景来源：default = 当前皮肤自带背景（默认皮肤没有则用网站底色）；
	// custom = 用户自己上传的图床图片。
	ThemeBackgroundSourceDefault = "default"
	ThemeBackgroundSourceCustom  = "custom"

	// 帖子卡片不透明度（百分比）：有全站背景时让背景图透出来。
	// 下限 30 防止卡片完全透明影响阅读；100 = 完全不透明（等价关闭该效果）。
	ThemePostCardOpacityDefault = 70
	ThemePostCardOpacityMin     = 30
	ThemePostCardOpacityMax     = 100
)

// themeLocation 活动时间窗按北京时间计算，不依赖服务器本地时区。
var themeLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// ThemeLocation 返回活动时间窗使用的时区（北京时间）。
func ThemeLocation() *time.Location {
	return themeLocation
}

// ThemeWallClock 把数据库 DATETIME 的「墙钟分量」解释成北京时间。
//
// MySQL DATETIME 不带时区，驱动按服务器 loc 读出来的 time.Time 可能带 UTC 等
// 位置；这里只取年月日时分秒，重新装到 +08:00，保证管理员按北京时间填写即可。
func ThemeWallClock(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), themeLocation)
}

// ThemeEvent 一期节日皮肤活动（theme_events 表的一行）。
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

// Valid 基础校验：必须有皮肤 ID 且结束时间晚于开始时间。
func (e ThemeEvent) Valid() bool {
	return strings.TrimSpace(e.ThemeID) != "" && e.EndsAt.After(e.StartsAt)
}

// PickFeaturedThemeEvent 从多期活动里挑出「当前要展示的一期」。
//
//   - 有进行中的活动：取开始时间最晚的一期（多期重叠时以最近的为准），ok=true；
//   - 没有进行中的：取最近已结束的一期；连已结束的都没有则取最近将开始的一期，ok=false；
//   - 没有任何活动：返回 nil。
//
// 返回的 bool 表示这一期当前是否 active，供前端展示状态；前端仍会用
// startsAt/endsAt 自行跨零点切换，不依赖单次接口结果。
func PickFeaturedThemeEvent(events []ThemeEvent, now time.Time) (*ThemeEvent, bool) {
	if len(events) == 0 {
		return nil, false
	}

	var active *ThemeEvent
	for i := range events {
		if !events[i].Valid() || !events[i].ActiveAt(now) {
			continue
		}
		if active == nil || events[i].StartsAt.After(active.StartsAt) {
			active = &events[i]
		}
	}
	if active != nil {
		return active, true
	}

	var latestEnded *ThemeEvent
	var nextUpcoming *ThemeEvent
	for i := range events {
		e := &events[i]
		if !e.Valid() {
			continue
		}
		if !e.StartsAt.After(now) {
			// 已开始但不在活动窗口内（已结束）
			if latestEnded == nil || e.StartsAt.After(latestEnded.StartsAt) {
				latestEnded = e
			}
			continue
		}
		if nextUpcoming == nil || e.StartsAt.Before(nextUpcoming.StartsAt) {
			nextUpcoming = e
		}
	}
	if latestEnded != nil {
		return latestEnded, false
	}
	if nextUpcoming != nil {
		return nextUpcoming, false
	}
	return nil, false
}

// themeIDPattern 皮肤 ID 只允许小写字母/数字/连字符（如 programmers-day-2026）。
// 不校验它是否真的存在于活动表：用户手动保留的皮肤在活动行删除后仍应能保存。
var themeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// IsValidThemeMode 校验用户的皮肤选择：auto / default / 合法皮肤 ID。
func IsValidThemeMode(mode string) bool {
	switch mode {
	case ThemeModeAuto, ThemeModeDefault:
		return true
	}
	return themeIDPattern.MatchString(mode)
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
	// CustomSkinID 用户皮肤库中当前选中的自定义皮肤；>0 时覆盖其它背景来源。
	CustomSkinID int
	// PostCardOpacity 帖子卡片不透明度（30-100）；只在开启全站背景时有视觉效果。
	PostCardOpacity int
	UpdatedAt       time.Time
}

// DefaultThemeSettings 没有保存过设置时的默认值：
// 跟随活动（活动当天自动换肤）、开场画面开、背景开、用皮肤自带背景、卡片 70% 不透明。
func DefaultThemeSettings(userID int) ThemeSettings {
	return ThemeSettings{
		UserID:            userID,
		ThemeMode:         ThemeModeAuto,
		SplashEnabled:     true,
		BackgroundEnabled: true,
		BackgroundSource:  ThemeBackgroundSourceDefault,
		PostCardOpacity:   ThemePostCardOpacityDefault,
	}
}

// IsValidThemePostCardOpacity 帖子卡片不透明度是否在允许范围内。
func IsValidThemePostCardOpacity(opacity int) bool {
	return opacity >= ThemePostCardOpacityMin && opacity <= ThemePostCardOpacityMax
}

// 外观设置相关业务错误。
var (
	ErrThemeModeInvalid             = errors.New("皮肤选项不合法")
	ErrThemeBackgroundSourceInvalid = errors.New("背景来源不合法")
	ErrThemeBackgroundURLInvalid    = errors.New("背景图片地址不合法，只支持 http/https 图片直链")
	ErrThemePostCardOpacityInvalid  = errors.New("帖子卡片透明度需在 30-100 之间")
	ErrThemeCustomSkinInvalid       = errors.New("自定义皮肤不存在或不在你的皮肤库中")
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
	if !IsValidThemeMode(s.ThemeMode) {
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
	if s.CustomSkinID < 0 {
		return ErrThemeCustomSkinInvalid
	}

	// 旧客户端不传该字段时为 0：回退默认值，保持向后兼容。
	if s.PostCardOpacity == 0 {
		s.PostCardOpacity = ThemePostCardOpacityDefault
	}
	if !IsValidThemePostCardOpacity(s.PostCardOpacity) {
		return ErrThemePostCardOpacityInvalid
	}
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
