package domain

import "time"

// 排行榜口径：投票功能下线后，帖子榜与作者榜都按「收到的币数」排。
//
// 币数口径（与前端展示完全一致）：
//   - 帖子：coin_num（真实投币）+ vote_num（历史投票折算）；
//   - 小说：coin_num（小说没有历史投票，直接就是币）；
//   - 作者：自己帖子与小说收到的币数之和。

// PostCoinRankItem 帖子投币榜单项。
type PostCoinRankItem struct {
	PostID    int
	Title     string
	Partition string
	CoinNum   int
	PostTime  time.Time
}

// UserCoinRankItem 作者投币榜单项：ReceivedCoins 是其所有内容收到的币总数。
type UserCoinRankItem struct {
	UserID        int
	UserName      string
	AvatarURL     string
	ReceivedCoins int
}
