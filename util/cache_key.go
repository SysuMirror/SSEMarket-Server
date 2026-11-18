package util

import (
	"fmt"
	"math/rand"
	"time"
)

// 缓存全局前缀
const (
	// 原有基础缓存前缀
	PrefixPostDetail      = "post:detail:"       // 帖子详情（ShowDetails）
	PrefixHotPost         = "post:hot:list"      // 热点帖子列表
	PrefixUserInfo        = "user:info:"         // 用户信息
	PrefixCommentList     = "comment:list:"      // 帖子评论列表（主评论+子评论）
	PrefixPostBrowse      = "post:browse:"       // 帖子浏览列表（home/history/save/rating）
	PrefixProductList     = "product:list:"      // 商品列表（home/个人商品）
	PrefixPostBrowseCount = "post:browse:count:" // 帖子浏览数独立Key
	PrefixUserSession     = "user:session:"      // JWT 用户快照缓存
	// 在线状态（presence）：全站在线 ZSET + 按场景/内容聚合的在线 ZSET
	PrefixPresence        = "presence:"
	PrefixPresenceOnline  = PrefixPresence + "online"
	PrefixPresenceCurrent = PrefixPresence + "current:"
)

// GetPresenceCurrentKey 记录某用户「当前所在场景」的 Key。
// 切换帖子/小说/音乐/简历时据此把用户从上一个内容的在线集合里移除，
// 避免同一个用户在同一段时间内被多个内容重复计数。
func GetPresenceCurrentKey(userID int) string {
	return PrefixPresenceCurrent + fmt.Sprintf("%d", userID)
}

// GetPresenceSceneKey 生成某场景/内容的在线 ZSET Key（scene 为空或 targetID<=0 时返回空串）。
func GetPresenceSceneKey(scene string, targetID int) string {
	if scene == "" || targetID <= 0 {
		return ""
	}
	return PrefixPresence + "scene:" + scene + ":" + fmt.Sprintf("%d", targetID)
}

// GetPostBrowseCountKey 生成帖子浏览数缓存Key（适配ShowDetails函数，postID为int）
func GetPostBrowseCountKey(postID int) string {
	return PrefixPostBrowseCount + fmt.Sprintf("%d", postID)
}

// GetPostDetailKey 生成帖子详情缓存Key（适配ShowDetails函数，postID为int）
func GetPostDetailKey(postID int) string {
	return PrefixPostDetail + fmt.Sprintf("%d", postID)
}

// GetHotPostKey 生成热点帖子列表缓存Key
func GetHotPostKey() string {
	return PrefixHotPost
}

// GetUserInfoKey 生成用户信息缓存Key（userID为int）
func GetUserInfoKey(userID int) string {
	return PrefixUserInfo + fmt.Sprintf("%d", userID)
}

// GetCommentListKey 生成帖子评论列表缓存Key
// 参数：postID 帖子ID，postType 帖子类型（post/评分贴）- 两者共同决定评论列表唯一性
func GetCommentListKey(postID int, postType string) string {
	return PrefixCommentList + fmt.Sprintf("%d:%s", postID, postType)
}

// PublicFeedGenKey 公共 Feed 缓存世代号 Redis Key。
// 失效时 INCR 此 key，所有带 g{gen} 的旧列表 key 自动作废（无需 scan DEL）。
const PublicFeedGenKey = PrefixPostBrowse + "public:gen"

// GetHomeBrowsePage1Key 首页帖子列表第 1 页缓存 Key（旧方案，已由 PublicFeed 取代，保留兼容清理）。
func GetHomeBrowsePage1Key() string {
	return PrefixPostBrowse + "home:page1"
}

// BrowseFeedKeyInput 公共 Feed 缓存 key 的筛选维度（刻意不含 ViewerUserID）。
type BrowseFeedKeyInput struct {
	Searchsort string
	Partition  string
	OrderBy    string
	Offset     int
	Limit      int
}

// GetRandomExpire 随机过期时间（防缓存雪崩）
func GetRandomExpire(base time.Duration) time.Duration {
	rand.Seed(time.Now().UnixNano())
	// 随机偏移1~300秒，避免大量Key同时过期
	offset := time.Duration(rand.Intn(300)) * time.Second
	return base + offset
}
