package domain

import "time"

// 在线状态（presence）口径。
//
// 客户端每 PresenceHeartbeatHint 秒左右上报一次心跳；服务端用 Redis ZSET
// 记录「谁在什么时间出现过」，统计时只把 PresenceWindow 内的成员算作在线。
// 时间窗口取心跳间隔的 ~2.5 倍，容忍一次丢包/页面切后台，又不会让离线用户
// 长时间挂在在线列表里。
const (
	// PresenceWindow 在线判定窗口：最后一次心跳距今超过该窗口即视为离线。
	//
	// 口径是「宁可晚一点显示离线，也不给系统加负担」：
	// 前端 60 秒上报一次（与现有新帖轮询同频），窗口取 2.5 倍，
	// 容忍一次丢包/切后台，在线人数最多延迟约 2.5 分钟更新。
	PresenceWindow = 150 * time.Second
	// PresenceHeartbeatHint 建议的前端心跳间隔（秒），随接口一起下发。
	PresenceHeartbeatHint = 60
	// PresenceKeyTTL Redis key 的过期时间：窗口过后再留一段时间用于兜底清理。
	// 取 5 分钟（约 2 倍窗口），避免大量冷门帖子/简历的在线 key 长期占内存。
	PresenceKeyTTL = 5 * time.Minute
)

// 支持在线统计的场景白名单。targetID 的含义随场景不同：
//   - post   帖子 ID（帖子详情页）
//   - novel  小说 ID（书详情页 / 阅读器，按书聚合）
//   - music  音乐 ID（音乐页正在播放的曲子；0 = 只在音乐页但没在播）
//   - resume 简历 ID（简历详情页；0 = 简历列表页）
//   - home   无 target，仅站点在线人数（主页 / 搜索页等）
//
// 白名单同时起到防止任意 targetID 撑爆 Redis key 空间的作用。
const (
	PresenceSceneHome   = "home"
	PresenceScenePost   = "post"
	PresenceSceneNovel  = "novel"
	PresenceSceneMusic  = "music"
	PresenceSceneResume = "resume"
)

var presenceScenes = map[string]struct{}{
	PresenceSceneHome:   {},
	PresenceScenePost:   {},
	PresenceSceneNovel:  {},
	PresenceSceneMusic:  {},
	PresenceSceneResume: {},
}

// NormalizePresenceScene 校验并归一化场景名；未知场景返回 false。
// 空字符串是合法的，表示只上报站点在线、不关联具体内容。
func NormalizePresenceScene(scene string) (string, bool) {
	if scene == "" {
		return "", true
	}
	if _, ok := presenceScenes[scene]; !ok {
		return "", false
	}
	return scene, true
}

// PresenceSnapshot 一次在线状态查询结果。
type PresenceSnapshot struct {
	// Online 站点当前在线人数（去重后的用户数）。
	Online int `json:"online"`
	// Viewers 当前在同一个场景/内容上的在线人数；scene 为空或 targetID<=0 时为 0。
	Viewers int `json:"viewers"`
	// Scene/TargetID 回显本次统计的场景与目标，便于前端确认心跳对象。
	Scene    string `json:"scene"`
	TargetID int    `json:"targetID"`
	// WindowSeconds 在线判定窗口（秒），前端据此展示「最近 N 秒内活跃」。
	WindowSeconds int `json:"windowSeconds"`
	// HeartbeatSeconds 建议心跳间隔（秒）。
	HeartbeatSeconds int `json:"heartbeatSeconds"`
}
