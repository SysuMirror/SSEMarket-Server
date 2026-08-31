package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func lotteryDrawnAt(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

type lotteryRepository struct {
	db *gorm.DB
}

func NewLotteryRepository(db *gorm.DB) repository.LotteryRepository {
	return &lotteryRepository{db: db}
}

func (r *lotteryRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *lotteryRepository) CreateLottery(ctx context.Context, postID, userID int, mode string, drawAt time.Time) (int, error) {
	row := model.LotteryPost{
		PostID:    postID,
		UserID:    userID,
		Mode:      mode,
		Status:    domain.LotteryStatusActive,
		DrawAt:    drawAt,
		CreatedAt: time.Now(),
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.LotteryID, nil
}

func (r *lotteryRepository) CreatePrizes(ctx context.Context, lotteryID int, prizes []domain.LotteryPrizeInput) error {
	for _, p := range prizes {
		row := model.LotteryPrize{
			LotteryID:   lotteryID,
			Name:        p.Name,
			Quantity:    p.Quantity,
			WishKeyword: p.WishKeyword,
		}
		if err := r.session(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *lotteryRepository) FindByPostID(ctx context.Context, postID int) (*repository.LotteryRecord, error) {
	var row model.LotteryPost
	err := r.session(ctx).Where("postID = ?", postID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &repository.LotteryRecord{
		LotteryID: row.LotteryID,
		PostID:    row.PostID,
		UserID:    row.UserID,
		Mode:      row.Mode,
		Status:    row.Status,
		DrawAt:    row.DrawAt,
		DrawnAt:   lotteryDrawnAt(row.DrawnAt),
	}, nil
}

func (r *lotteryRepository) FindPrizes(ctx context.Context, lotteryID int) ([]repository.LotteryPrizeRecord, error) {
	var rows []model.LotteryPrize
	if err := r.session(ctx).Where("lotteryID = ?", lotteryID).Order("prizeID ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]repository.LotteryPrizeRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.LotteryPrizeRecord{
			PrizeID:     row.PrizeID,
			LotteryID:   row.LotteryID,
			Name:        row.Name,
			Quantity:    row.Quantity,
			WishKeyword: row.WishKeyword,
		})
	}
	return out, nil
}

func (r *lotteryRepository) CountParticipants(ctx context.Context, lotteryID int) (int, error) {
	var count int64
	err := r.session(ctx).Model(&model.LotteryParticipant{}).Where("lotteryID = ?", lotteryID).Count(&count).Error
	return int(count), err
}

func (r *lotteryRepository) CountParticipantsByPrize(ctx context.Context, lotteryID, prizeID int) (int, error) {
	var count int64
	err := r.session(ctx).Model(&model.LotteryParticipant{}).
		Where("lotteryID = ? AND prizeID = ?", lotteryID, prizeID).
		Count(&count).Error
	return int(count), err
}

func (r *lotteryRepository) HasParticipatedForPrize(ctx context.Context, lotteryID, userID, prizeID int) (bool, error) {
	var row model.LotteryParticipant
	err := r.session(ctx).
		Where("lotteryID = ? AND userID = ? AND prizeID = ?", lotteryID, userID, prizeID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.ParticipantID != 0, nil
}

func (r *lotteryRepository) HasParticipated(ctx context.Context, lotteryID, userID int) (bool, error) {
	var row model.LotteryParticipant
	err := r.session(ctx).Where("lotteryID = ? AND userID = ?", lotteryID, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return row.ParticipantID != 0, nil
}

func (r *lotteryRepository) AddParticipant(ctx context.Context, lotteryID, userID, prizeID int, entryType string, commentID int) error {
	q := r.session(ctx).Where("lotteryID = ? AND userID = ?", lotteryID, userID)
	if prizeID > 0 {
		// 许愿抽奖：同一用户对同一奖品仅计一次参与
		q = q.Where("prizeID = ?", prizeID)
	}
	var existing model.LotteryParticipant
	err := q.First(&existing).Error
	if err == nil && existing.ParticipantID != 0 {
		return domain.ErrConflict
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	row := model.LotteryParticipant{
		LotteryID: lotteryID,
		UserID:    userID,
		PrizeID:   prizeID,
		EntryType: entryType,
		CommentID: commentID,
		CreatedAt: time.Now(),
	}
	if err := r.session(ctx).Create(&row).Error; err != nil {
		if isLotteryDuplicateEntry(err) {
			return domain.ErrConflict
		}
		return err
	}
	return nil
}

func isLotteryDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return strings.Contains(mysqlErr.Message, "lottery_user_prize")
	}
	return false
}

func (r *lotteryRepository) ListParticipants(ctx context.Context, lotteryID int) ([]repository.LotteryParticipantRecord, error) {
	var rows []model.LotteryParticipant
	if err := r.session(ctx).Where("lotteryID = ?", lotteryID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]repository.LotteryParticipantRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.LotteryParticipantRecord{
			ParticipantID: row.ParticipantID,
			LotteryID:     row.LotteryID,
			UserID:        row.UserID,
			PrizeID:       row.PrizeID,
			EntryType:     row.EntryType,
			CommentID:     row.CommentID,
		})
	}
	return out, nil
}

func (r *lotteryRepository) ListWinners(ctx context.Context, lotteryID int) ([]repository.LotteryWinnerRecord, error) {
	var rows []model.LotteryWinner
	if err := r.session(ctx).Where("lotteryID = ?", lotteryID).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	prizeMap := make(map[int]string)
	var prizes []model.LotteryPrize
	_ = r.session(ctx).Where("lotteryID = ?", lotteryID).Find(&prizes)
	for _, p := range prizes {
		prizeMap[p.PrizeID] = p.Name
	}
	out := make([]repository.LotteryWinnerRecord, 0, len(rows))
	for _, row := range rows {
		name := ""
		var user model.User
		if err := r.session(ctx).Select("name").Where("userID = ?", row.UserID).First(&user).Error; err == nil {
			name = user.Name
		}
		out = append(out, repository.LotteryWinnerRecord{
			WinnerID:  row.WinnerID,
			LotteryID: row.LotteryID,
			UserID:    row.UserID,
			PrizeID:   row.PrizeID,
			UserName:  name,
			PrizeName: prizeMap[row.PrizeID],
		})
	}
	return out, nil
}

func (r *lotteryRepository) ListDueForDraw(ctx context.Context, now time.Time) ([]repository.LotteryRecord, error) {
	var rows []model.LotteryPost
	err := r.session(ctx).
		Where("status = ? AND draw_at <= ?", domain.LotteryStatusActive, now).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]repository.LotteryRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.LotteryRecord{
			LotteryID: row.LotteryID,
			PostID:    row.PostID,
			UserID:    row.UserID,
			Mode:      row.Mode,
			Status:    row.Status,
			DrawAt:    row.DrawAt,
			DrawnAt:   lotteryDrawnAt(row.DrawnAt),
		})
	}
	return out, nil
}

func (r *lotteryRepository) UpdateStatus(ctx context.Context, lotteryID int, status string, drawnAt time.Time) error {
	updates := map[string]interface{}{"status": status}
	if !drawnAt.IsZero() {
		updates["drawn_at"] = drawnAt
	}
	return r.session(ctx).Model(&model.LotteryPost{}).Where("lotteryID = ?", lotteryID).Updates(updates).Error
}

func (r *lotteryRepository) SaveWinners(ctx context.Context, lotteryID int, winners []repository.LotteryWinnerRecord) error {
	for _, w := range winners {
		row := model.LotteryWinner{
			LotteryID: lotteryID,
			UserID:    w.UserID,
			PrizeID:   w.PrizeID,
			Notified:  false,
			CreatedAt: time.Now(),
		}
		if err := r.session(ctx).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *lotteryRepository) CreateNotice(ctx context.Context, receiver, sender int, noticeType, text string, target int) error {
	notice := model.Notice{
		Receiver: receiver,
		Sender:   sender,
		Type:     noticeType,
		Ntext:    text,
		Time:     time.Now(),
		Read:     false,
		Target:   target,
	}
	return r.session(ctx).Create(&notice).Error
}

func (r *lotteryRepository) ExistsByPostID(ctx context.Context, postID int) (bool, error) {
	var count int64
	err := r.session(ctx).Model(&model.LotteryPost{}).Where("postID = ?", postID).Limit(1).Count(&count).Error
	return count > 0, err
}

func (r *lotteryRepository) FindUserByID(ctx context.Context, userID int) (string, error) {
	var user model.User
	err := r.session(ctx).Select("name").Where("userID = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return user.Name, nil
}
