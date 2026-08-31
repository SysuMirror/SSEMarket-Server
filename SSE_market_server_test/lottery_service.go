package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"loginTest/common"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/repository"
)

type lotteryService struct {
	lotteries repository.LotteryRepository
	posts     repository.PostRepository
}

func NewLotteryService(lotteries repository.LotteryRepository, posts repository.PostRepository) LotteryService {
	return &lotteryService{lotteries: lotteries, posts: posts}
}

func (s *lotteryService) Create(ctx context.Context, in domain.LotteryCreateInput) (int, error) {
	title := strings.TrimSpace(in.Title)
	content := strings.TrimSpace(in.Content)
	if title == "" || content == "" || in.Partition == "" {
		return 0, domain.ErrBadRequest
	}
	if utf8.RuneCountInString(title) > 30 {
		return 0, domain.ErrBadRequest
	}
	if utf8.RuneCountInString(content) > config.PostContentMaxRunes {
		return 0, domain.ErrBadRequest
	}
	if in.Mode != domain.LotteryModeRandom && in.Mode != domain.LotteryModeWish {
		return 0, domain.ErrBadRequest
	}
	if len(in.Prizes) == 0 {
		return 0, domain.ErrBadRequest
	}
	for _, p := range in.Prizes {
		if strings.TrimSpace(p.Name) == "" || p.Quantity <= 0 {
			return 0, domain.ErrBadRequest
		}
		if in.Mode == domain.LotteryModeWish && strings.TrimSpace(p.WishKeyword) == "" {
			return 0, domain.ErrBadRequest
		}
	}
	if in.DrawAt.Before(time.Now()) {
		return 0, domain.ErrBadRequest
	}

	tagString := strings.Join(in.Tags, ",")
	post := &domain.Post{
		UserID:    in.UserID,
		Title:     title,
		Content:   content,
		Partition: in.Partition,
		Photos:    in.Photos,
		Tag:       tagString,
		IsPrivate: in.IsPrivate,
		PostTime:  time.Now(),
	}
	if err := s.posts.Create(ctx, post); err != nil {
		return 0, err
	}
	common.AddPostID(post.ID)
	InvalidateHomeBrowse()

	lotteryID, err := s.lotteries.CreateLottery(ctx, post.ID, in.UserID, in.Mode, in.DrawAt)
	if err != nil {
		return 0, err
	}
	if err := s.lotteries.CreatePrizes(ctx, lotteryID, in.Prizes); err != nil {
		return 0, err
	}
	return post.ID, nil
}

func (s *lotteryService) IsLotteryPost(ctx context.Context, postID int) (bool, error) {
	if postID <= 0 {
		return false, nil
	}
	return s.lotteries.ExistsByPostID(ctx, postID)
}

func (s *lotteryService) GetDetail(ctx context.Context, postID, viewerUserID int) (*domain.LotteryPost, error) {
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return nil, err
	}
	if !canViewPost(post, viewerUserID) {
		return nil, domain.ErrNotFound
	}
	rec, err := s.lotteries.FindByPostID(ctx, postID)
	if err != nil {
		return nil, err
	}
	prizes, err := s.lotteries.FindPrizes(ctx, rec.LotteryID)
	if err != nil {
		return nil, err
	}
	count, err := s.lotteries.CountParticipants(ctx, rec.LotteryID)
	if err != nil {
		return nil, err
	}
	participated := false
	if viewerUserID != 0 {
		participated, _ = s.lotteries.HasParticipated(ctx, rec.LotteryID, viewerUserID)
	}
	out := &domain.LotteryPost{
		LotteryID:        rec.LotteryID,
		PostID:           rec.PostID,
		UserID:           rec.UserID,
		Mode:             rec.Mode,
		Status:           rec.Status,
		DrawAt:           rec.DrawAt,
		DrawnAt:          rec.DrawnAt,
		ParticipantCount: count,
		Participated:     participated,
	}
	for _, p := range prizes {
		prize := domain.LotteryPrize{
			PrizeID:     p.PrizeID,
			Name:        p.Name,
			Quantity:    p.Quantity,
			WishKeyword: p.WishKeyword,
		}
		if rec.Mode == domain.LotteryModeWish {
			prize.ParticipantCount, _ = s.lotteries.CountParticipantsByPrize(ctx, rec.LotteryID, p.PrizeID)
			if viewerUserID != 0 {
				prize.Participated, _ = s.lotteries.HasParticipatedForPrize(ctx, rec.LotteryID, viewerUserID, p.PrizeID)
			}
		}
		out.Prizes = append(out.Prizes, prize)
	}
	if rec.Status == domain.LotteryStatusDrawn || rec.Status == domain.LotteryStatusClosed {
		winners, err := s.lotteries.ListWinners(ctx, rec.LotteryID)
		if err != nil {
			return nil, err
		}
		for _, w := range winners {
			out.Winners = append(out.Winners, domain.LotteryWinnerItem{
				UserID:    w.UserID,
				UserName:  w.UserName,
				PrizeID:   w.PrizeID,
				PrizeName: w.PrizeName,
			})
		}
	}
	return out, nil
}

func (s *lotteryService) ParticipateButton(ctx context.Context, userID, postID int) error {
	rec, err := s.lotteries.FindByPostID(ctx, postID)
	if err != nil {
		return err
	}
	if rec.Mode != domain.LotteryModeRandom {
		return domain.ErrBadRequest
	}
	if rec.Status != domain.LotteryStatusActive {
		return domain.ErrBadRequest
	}
	if time.Now().After(rec.DrawAt) {
		return domain.ErrBadRequest
	}
	post, err := s.posts.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if !canViewPost(post, userID) {
		return domain.ErrNotFound
	}
	if post.UserID == userID {
		return domain.ErrBadRequest
	}
	return s.lotteries.AddParticipant(ctx, rec.LotteryID, userID, 0, "button", 0)
}

func matchWishPrizeID(content string, prizes []repository.LotteryPrizeRecord) int {
	type candidate struct {
		prizeID int
		keyword string
	}
	candidates := make([]candidate, 0, len(prizes))
	for _, p := range prizes {
		kw := strings.TrimSpace(p.WishKeyword)
		if kw != "" && strings.Contains(content, kw) {
			candidates = append(candidates, candidate{prizeID: p.PrizeID, keyword: kw})
		}
	}
	if len(candidates) == 0 {
		return 0
	}
	// 优先匹配更长、更具体的关键词，避免「苹果」误抢「苹果手机」
	best := candidates[0]
	for _, c := range candidates[1:] {
		if len([]rune(c.keyword)) > len([]rune(best.keyword)) {
			best = c
		}
	}
	return best.prizeID
}

func (s *lotteryService) ParticipateByComment(ctx context.Context, in domain.LotteryParticipateInput) error {
	if in.UserID == 0 || in.PostID == 0 {
		return domain.ErrBadRequest
	}
	rec, err := s.lotteries.FindByPostID(ctx, in.PostID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if rec.Mode != domain.LotteryModeWish {
		return nil
	}
	if rec.Status != domain.LotteryStatusActive {
		return nil
	}
	if time.Now().After(rec.DrawAt) {
		return nil
	}
	if rec.UserID == in.UserID {
		return nil
	}
	prizes, err := s.lotteries.FindPrizes(ctx, rec.LotteryID)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(in.Content)
	matchedPrizeID := matchWishPrizeID(content, prizes)
	if matchedPrizeID == 0 {
		return nil
	}
	err = s.lotteries.AddParticipant(ctx, rec.LotteryID, in.UserID, matchedPrizeID, "comment", in.CommentID)
	if errors.Is(err, domain.ErrConflict) {
		return nil
	}
	return err
}

func (s *lotteryService) HasParticipated(ctx context.Context, userID, postID int) (bool, error) {
	rec, err := s.lotteries.FindByPostID(ctx, postID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return s.lotteries.HasParticipated(ctx, rec.LotteryID, userID)
}

func (s *lotteryService) Draw(ctx context.Context, postID int) error {
	rec, err := s.lotteries.FindByPostID(ctx, postID)
	if err != nil {
		return err
	}
	if rec.Status != domain.LotteryStatusActive {
		return domain.ErrBadRequest
	}
	if time.Now().Before(rec.DrawAt) {
		return domain.ErrBadRequest
	}

	participants, err := s.lotteries.ListParticipants(ctx, rec.LotteryID)
	if err != nil {
		return err
	}
	prizes, err := s.lotteries.FindPrizes(ctx, rec.LotteryID)
	if err != nil {
		return err
	}

	winners := pickWinners(rec.Mode, participants, prizes)
	if err := s.lotteries.SaveWinners(ctx, rec.LotteryID, winners); err != nil {
		return err
	}
	drawnAt := time.Now()
	if err := s.lotteries.UpdateStatus(ctx, rec.LotteryID, domain.LotteryStatusDrawn, drawnAt); err != nil {
		return err
	}
	s.notifyWinners(ctx, rec, winners, prizes)
	return nil
}

func (s *lotteryService) AutoDrawDue(ctx context.Context) {
	due, err := s.lotteries.ListDueForDraw(ctx, time.Now())
	if err != nil {
		return
	}
	for _, rec := range due {
		_ = s.Draw(ctx, rec.PostID)
	}
}

func (s *lotteryService) notifyWinners(ctx context.Context, rec *repository.LotteryRecord, winners []repository.LotteryWinnerRecord, prizes []repository.LotteryPrizeRecord) {
	prizeMap := make(map[int]string)
	for _, p := range prizes {
		prizeMap[p.PrizeID] = p.Name
	}
	for _, w := range winners {
		prizeName := prizeMap[w.PrizeID]
		text := fmt.Sprintf("恭喜你在抽奖帖中获得了「%s」", prizeName)
		_ = s.lotteries.CreateNotice(ctx, w.UserID, rec.UserID, "lottery_win", text, rec.PostID)
	}
}

func pickWinners(mode string, participants []repository.LotteryParticipantRecord, prizes []repository.LotteryPrizeRecord) []repository.LotteryWinnerRecord {
	if len(participants) == 0 {
		return nil
	}
	if mode == domain.LotteryModeWish {
		return pickWishWinners(participants, prizes)
	}
	return pickRandomWinners(participants, prizes)
}

func pickRandomWinners(participants []repository.LotteryParticipantRecord, prizes []repository.LotteryPrizeRecord) []repository.LotteryWinnerRecord {
	shuffled := shuffleParticipants(participants)
	var winners []repository.LotteryWinnerRecord
	used := make(map[int]bool)
	idx := 0
	for _, prize := range prizes {
		for i := 0; i < prize.Quantity && idx < len(shuffled); {
			p := shuffled[idx]
			idx++
			if used[p.UserID] {
				continue
			}
			used[p.UserID] = true
			winners = append(winners, repository.LotteryWinnerRecord{
				UserID:    p.UserID,
				PrizeID:   prize.PrizeID,
				PrizeName: prize.Name,
			})
			i++
		}
	}
	return winners
}

func pickWishWinners(participants []repository.LotteryParticipantRecord, prizes []repository.LotteryPrizeRecord) []repository.LotteryWinnerRecord {
	prizeMap := make(map[int]repository.LotteryPrizeRecord)
	for _, p := range prizes {
		prizeMap[p.PrizeID] = p
	}
	byPrize := make(map[int][]repository.LotteryParticipantRecord)
	for _, p := range participants {
		if p.PrizeID == 0 {
			continue
		}
		byPrize[p.PrizeID] = append(byPrize[p.PrizeID], p)
	}
	var winners []repository.LotteryWinnerRecord
	used := make(map[int]bool)
	for _, prize := range prizes {
		pool := byPrize[prize.PrizeID]
		shuffled := shuffleParticipants(pool)
		count := 0
		for _, p := range shuffled {
			if count >= prize.Quantity {
				break
			}
			if used[p.UserID] {
				continue
			}
			used[p.UserID] = true
			winners = append(winners, repository.LotteryWinnerRecord{
				UserID:    p.UserID,
				PrizeID:   prize.PrizeID,
				PrizeName: prize.Name,
			})
			count++
		}
	}
	return winners
}

func shuffleParticipants(in []repository.LotteryParticipantRecord) []repository.LotteryParticipantRecord {
	out := make([]repository.LotteryParticipantRecord, len(in))
	copy(out, in)
	for i := len(out) - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			break
		}
		j := int(jBig.Int64())
		out[i], out[j] = out[j], out[i]
	}
	return out
}
