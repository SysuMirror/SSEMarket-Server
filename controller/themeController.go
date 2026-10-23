package controller

import (
	"errors"
	"net/http"
	"time"

	"loginTest/apperr"
	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// 全站皮肤 / 外观设置
//
// GET  /api/auth/theme/state    活动时间窗 + 当前用户设置（noauth，可选登录）
// POST /api/auth/theme/settings 保存当前用户设置（auth）
//
// 时间窗由 domain 的静态注册表给出，前端只消费，不自己判断日期；
// 这样活动开始/结束只需后端改一处，所有客户端同时生效。
// ---------------------------------------------------------------------------

type themeSettingsReq struct {
	ThemeMode           string `json:"themeMode"`
	SplashEnabled       bool   `json:"splashEnabled"`
	BackgroundEnabled   bool   `json:"backgroundEnabled"`
	BackgroundSource    string `json:"backgroundSource"`
	CustomBackgroundURL string `json:"customBackgroundUrl"`
}

func themeEventData(ev domain.ThemeEvent, active bool, now time.Time) gin.H {
	name := ev.Name
	if name == "" {
		name = ev.ThemeID
	}
	return gin.H{
		"themeId":    ev.ThemeID,
		"name":       name,
		"active":     active,
		"startsAt":   ev.StartsAt.Format(time.RFC3339),
		"endsAt":     ev.EndsAt.Format(time.RFC3339),
		"serverTime": now.Format(time.RFC3339),
	}
}

func themeSettingsData(s *domain.ThemeSettings) gin.H {
	if s == nil {
		return nil
	}
	data := gin.H{
		"themeMode":           s.ThemeMode,
		"splashEnabled":       s.SplashEnabled,
		"backgroundEnabled":   s.BackgroundEnabled,
		"backgroundSource":    s.BackgroundSource,
		"customBackgroundUrl": s.CustomBackgroundURL,
	}
	if !s.UpdatedAt.IsZero() {
		data["updatedAt"] = s.UpdatedAt.Format(time.RFC3339)
	}
	return data
}

// ThemeState 返回活动时间窗；已登录（或带有效 token）时一并返回该用户的外观设置。
// 游客拿到 settings=null，前端用本地默认值展示活动皮肤。
func ThemeState(c *gin.Context) {
	// 响应包含当前用户的个性化设置，禁止任何中间层缓存。
	c.Header("Cache-Control", "no-store")
	now := time.Now()

	events, err := service.Themes.ListThemeEvents(c.Request.Context())
	if err != nil {
		apperr.WriteMsg(c, err, "读取活动时间窗失败")
		return
	}
	// 没有配置任何活动时 event=null，前端按「无活动」处理，不影响手动换肤。
	var eventData any
	if event, active := domain.PickFeaturedThemeEvent(events, now); event != nil {
		eventData = themeEventData(*event, active, now)
	}

	settings := (*domain.ThemeSettings)(nil)
	if user, ok := viewerFromAuth(c); ok {
		settings, err = service.Themes.GetUserSettings(c.Request.Context(), user.ID)
		if err != nil {
			apperr.WriteMsg(c, err, "读取外观设置失败")
			return
		}
	}

	data := gin.H{
		"serverTime": now.Format(time.RFC3339),
		"event":      eventData,
		"settings":   themeSettingsData(settings),
	}
	response.Success(c, data, "")
}

// ThemeSaveSettings 保存当前用户的外观设置。
// 只接受 domain 注册过的皮肤 ID / 背景来源，背景图只接受 http(s) 绝对地址。
func ThemeSaveSettings(c *gin.Context) {
	user, ok := requireLoginUser(c)
	if !ok {
		return
	}
	var req themeSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数格式错误")
		return
	}

	saved, err := service.Themes.SaveUserSettings(c.Request.Context(), user.ID, domain.ThemeSettings{
		ThemeMode:           req.ThemeMode,
		SplashEnabled:       req.SplashEnabled,
		BackgroundEnabled:   req.BackgroundEnabled,
		BackgroundSource:    req.BackgroundSource,
		CustomBackgroundURL: req.CustomBackgroundURL,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrThemeModeInvalid),
			errors.Is(err, domain.ErrThemeBackgroundSourceInvalid),
			errors.Is(err, domain.ErrThemeBackgroundURLInvalid):
			response.Response(c, http.StatusBadRequest, 400, nil, err.Error())
		case errors.Is(err, domain.ErrUnauthorized):
			response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		default:
			apperr.WriteMsg(c, err, "保存外观设置失败")
		}
		return
	}
	response.Success(c, gin.H{"settings": themeSettingsData(saved)}, "外观设置已保存")
}
