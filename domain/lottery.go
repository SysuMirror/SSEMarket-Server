package domain

import "time"

const (
	LotteryModeRandom = "random"
	LotteryModeWish   = "wish"

	LotteryStatusActive = "active"
	LotteryStatusDrawn  = "drawn"
	LotteryStatusClosed = "closed"
)

type LotteryPrizeInput struct {
	Name        string
	Quantity    int
	WishKeyword string
}

type LotteryCreateInput struct {
	UserID    int
	Title     string
	Content   string
	Partition string
	Photos    string
	Tags      []string
	IsPrivate bool
	Mode      string
	DrawAt    time.Time
	Prizes    []LotteryPrizeInput
}

type LotteryPrize struct {
	PrizeID           int
	Name              string
	Quantity          int
	WishKeyword       string
	ParticipantCount  int
	Participated      bool
}

type LotteryPost struct {
	LotteryID        int
	PostID           int
	UserID           int
	Mode             string
	Status           string
	DrawAt           time.Time
	DrawnAt          time.Time
	ParticipantCount int
	Prizes           []LotteryPrize
	Participated     bool
	Winners          []LotteryWinnerItem
}

type LotteryWinnerItem struct {
	UserID   int
	UserName string
	PrizeID  int
	PrizeName string
}

type LotteryParticipateInput struct {
	UserID    int
	PostID    int
	CommentID int
	Content   string
}
