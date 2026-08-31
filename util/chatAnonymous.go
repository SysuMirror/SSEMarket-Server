package util

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/spf13/viper"
)

const AnonymousChatDisplayName = "匿名用户"
const AnonymousThreadSuffix = "·匿名"

// anonIDBase ensures pseudo IDs never collide with real (sequential) user IDs.
const anonIDBase = 1_000_000_001

// AnonPeerID 返回 (viewerID, realPeerID) 对应的稳定伪 ID。
// 伪 ID 用于匿名会话中对接收方隐藏发送方真实身份：后端在 JSON 响应里
// 用伪 ID 替换真实 ID，前端无感知地用作会话 key。真实 ID 仅留在数据库。
func AnonPeerID(viewerID, realPeerID int) int {
	h := hmac.New(sha256.New, []byte(viper.GetString("crypto.jwtKey")))
	fmt.Fprintf(h, "anonpeer:%d:%d", viewerID, realPeerID)
	sum := h.Sum(nil)
	n := binary.BigEndian.Uint32(sum[:4])
	return anonIDBase + int(n%1_000_000_000)
}

// IsAnonPeerID 判断一个 ID 是否落在伪 ID 区间。
func IsAnonPeerID(id int) bool {
	return id >= anonIDBase
}

// ChatPeerDisplay 返回会话列表/顶栏展示用的昵称与头像。
func ChatPeerDisplay(realName, realAvatar string, threadAnonymous, peerMasked bool) (displayName, displayAvatar string) {
	if threadAnonymous {
		if peerMasked {
			return AnonymousChatDisplayName, ""
		}
		return realName + AnonymousThreadSuffix, realAvatar
	}
	return realName, realAvatar
}
