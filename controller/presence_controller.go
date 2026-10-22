package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"loginTest/domain"
	"loginTest/response"
	"loginTest/service"

	"github.com/gin-gonic/gin"
)

// PresenceHeartbeatMsg 心跳请求体。
// Scene 为空表示只报站点在线；TargetID 仅在需要统计「某个内容」时传。
type PresenceHeartbeatMsg struct {
	Scene    string `json:"scene"`
	TargetID int    `json:"targetID"`
}

// PresenceHeartbeat 客户端保活心跳（POST /api/auth/presence/heartbeat）。
//
// 登录用户每 60 秒左右调用一次（与前端新帖轮询同频）；服务端记录活跃并返回当前站点在线人数与
// 同内容在线人数。失败时仍返回 200 + 零值，避免弱功能影响页面。
func PresenceHeartbeat(c *gin.Context) {
	viewer, ok := viewerFromAuth(c)
	if !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "权限不足")
		return
	}
	var req PresenceHeartbeatMsg
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.Response(c, http.StatusBadRequest, 400, nil, "参数解析错误")
		return
	}
	scene, valid := domain.NormalizePresenceScene(req.Scene)
	if !valid {
		response.Response(c, http.StatusBadRequest, 400, nil, "场景不合法")
		return
	}
	targetID := req.TargetID
	if targetID < 0 {
		targetID = 0
	}
	// 只有需要按内容统计的场景才认 targetID；home 场景忽略它。
	if scene == "" || scene == domain.PresenceSceneHome {
		targetID = 0
	}
	snapshot := service.Presence.Heartbeat(c.Request.Context(), viewer.ID, scene, targetID)
	response.Success(c, presenceSnapshotData(snapshot), "")
}

// PresenceStats 读取当前在线人数（GET /api/auth/presence/stats?scene=post&targetID=1）。
func PresenceStats(c *gin.Context) {
	if _, ok := viewerFromAuth(c); !ok {
		response.Response(c, http.StatusUnauthorized, 401, nil, "权限不足")
		return
	}
	scene, valid := domain.NormalizePresenceScene(c.Query("scene"))
	if !valid {
		response.Response(c, http.StatusBadRequest, 400, nil, "场景不合法")
		return
	}
	targetID := parsePositiveQueryInt(c.Query("targetID"))
	if scene == "" || scene == domain.PresenceSceneHome {
		targetID = 0
	}
	snapshot := service.Presence.Snapshot(c.Request.Context(), scene, targetID)
	response.Success(c, presenceSnapshotData(snapshot), "")
}

func presenceSnapshotData(s domain.PresenceSnapshot) gin.H {
	return gin.H{
		"online":           s.Online,
		"viewers":          s.Viewers,
		"scene":            s.Scene,
		"targetID":         s.TargetID,
		"windowSeconds":    s.WindowSeconds,
		"heartbeatSeconds": s.HeartbeatSeconds,
	}
}

func parsePositiveQueryInt(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
