package repository

import (
	"context"
	"time"

	"loginTest/domain"
)

type LotteryRecord struct {
	LotteryID int
	PostID    int
	UserID    int
	Mode      string
	Status    string
	DrawAt    time.Time
	DrawnAt   time.Time
}

type LotteryPrizeRecord struct {
	PrizeID     int
	LotteryID   int
	Name        string
	Quantity    int
	WishKeyword string
}

type LotteryParticipantRecord struct {
	ParticipantID int
	LotteryID     int
	UserID        int
	PrizeID       int
	EntryType     string
	CommentID     int
}

type LotteryWinnerRecord struct {
	WinnerID int
	LotteryID int
	UserID   int
	PrizeID  int
	UserName string
	PrizeName string
}

type LotteryRepository interface {
	CreateLottery(ctx context.Context, postID, userID int, mode string, drawAt time.Time) (int, error)
	CreatePrizes(ctx context.Context, lotteryID int, prizes []domain.LotteryPrizeInput) error
	FindByPostID(ctx context.Context, postID int) (*LotteryRecord, error)
	FindPrizes(ctx context.Context, lotteryID int) ([]LotteryPrizeRecord, error)
	CountParticipants(ctx context.Context, lotteryID int) (int, error)
	CountParticipantsByPrize(ctx context.Context, lotteryID, prizeID int) (int, error)
	HasParticipated(ctx context.Context, lotteryID, userID int) (bool, error)
	HasParticipatedForPrize(ctx context.Context, lotteryID, userID, prizeID int) (bool, error)
	AddParticipant(ctx context.Context, lotteryID, userID, prizeID int, entryType string, commentID int) error
	ListParticipants(ctx context.Context, lotteryID int) ([]LotteryParticipantRecord, error)
	ListWinners(ctx context.Context, lotteryID int) ([]LotteryWinnerRecord, error)
	ListDueForDraw(ctx context.Context, now time.Time) ([]LotteryRecord, error)
	UpdateStatus(ctx context.Context, lotteryID int, status string, drawnAt time.Time) error
	SaveWinners(ctx context.Context, lotteryID int, winners []LotteryWinnerRecord) error
	CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error
	FindUserByID(ctx context.Context, userID int) (name string, err error)
	ExistsByPostID(ctx context.Context, postID int) (bool, error)
}
