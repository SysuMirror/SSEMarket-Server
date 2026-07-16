package util

const AnonymousChatDisplayName = "匿名用户"
const AnonymousThreadSuffix = "·匿名"

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
