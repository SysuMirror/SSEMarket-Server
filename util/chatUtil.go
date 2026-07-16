package util

import "fmt"

func GenerateChatViewKey(senderUserID, targetUserID int, threadAnonymous bool) string {
	lane := "direct"
	if threadAnonymous {
		lane = "anon"
	}
	return fmt.Sprintf("%dTo%dIsExist_%s", senderUserID, targetUserID, lane)
}

func GenerateUnreadKey(senderUserID, targetUserID int, threadAnonymous bool) string {
	lane := "direct"
	if threadAnonymous {
		lane = "anon"
	}
	return fmt.Sprintf("%dRecvUnread%d_%s", senderUserID, targetUserID, lane)
}

func GenerateChatViewIndexKey(userID int) string {
	return fmt.Sprintf("chatViewIndex:%d", userID)
}

func GenerateUnreadIndexKey(userID int) string {
	return fmt.Sprintf("chatUnreadIndex:%d", userID)
}
