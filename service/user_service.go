package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"loginTest/common"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"

	"github.com/brianvoe/gofakeit"
	"golang.org/x/crypto/bcrypt"
)

const maxValidateFail = 5

type userService struct {
	users repository.UserRepository
}

func NewUserService(users repository.UserRepository) UserService {
	return &userService{users: users}
}

func (s *userService) GetByID(ctx context.Context, id int) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}

func (s *userService) Login(ctx context.Context, in domain.LoginInput) (*domain.AuthTokens, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return nil, domain.ErrBadRequest
	}
	if len(in.Password) < 6 {
		return nil, domain.ErrBadRequest
	}

	u, hash, err := s.users.FindAuthByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return nil, domain.ErrUnauthorized
	}

	return s.issueTokens(ctx, u)
}

func (s *userService) Register(ctx context.Context, in domain.RegisterInput) (*domain.AuthTokens, error) {
	name := in.Name
	phone := in.Phone
	password := in.Password
	email := strings.TrimSpace(in.Email)

	if phone == "" {
		for {
			phone = util.GenerateRandomDigits(11)
			ok, err := s.users.ExistsByPhone(ctx, phone)
			if err != nil {
				return nil, err
			}
			if !ok {
				break
			}
		}
	}
	if len(phone) != 11 {
		return nil, domain.ErrBadRequest
	}
	if len(password) < 6 {
		return nil, domain.ErrBadRequest
	}
	if name == "" {
		name = gofakeit.Name()
	}
	if ok, _ := s.users.ExistsByPhone(ctx, phone); ok {
		return nil, domain.ErrConflict
	}
	if !strings.Contains(email, "@") {
		return nil, domain.ErrBadRequest
	}
	if ok, _ := s.users.ExistsByEmail(ctx, email); ok {
		return nil, domain.ErrConflict
	}
	if err := s.verifyEmailCode(ctx, email, in.ValiCode); err != nil {
		return nil, err
	}
	if err := s.users.UseCDKey(ctx, in.CDKey); err != nil {
		if errors.Is(err, domain.ErrBadRequest) {
			return nil, domain.ErrBadRequest
		}
		return nil, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.ErrInternal
	}
	u, err := s.users.Create(ctx, name, phone, email, string(hashed))
	if err != nil {
		return nil, err
	}

	tokens, err := s.issueTokens(ctx, u)
	if err != nil {
		return nil, err
	}
	s.clearEmailCode(ctx, email)
	return tokens, nil
}

func (s *userService) issueTokens(ctx context.Context, u *domain.User) (*domain.AuthTokens, error) {
	modelUser := DomainUserToModel(u)
	access, err := common.ReleaseToken(modelUser)
	if err != nil {
		return nil, domain.ErrInternal
	}
	refresh, err := common.ReleaseRefreshToken(modelUser)
	if err != nil {
		return nil, domain.ErrInternal
	}
	expires := time.Now().Add(30 * 24 * time.Hour).Unix()
	if err := s.users.SaveRefreshToken(ctx, u.ID, refresh, expires); err != nil {
		return nil, err
	}
	CacheUserAfterAuth(ctx, modelUser)
	return &domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         u,
	}, nil
}

func (s *userService) verifyEmailCode(ctx context.Context, email, valiCode string) error {
	if core.MyRedis == nil {
		return domain.ErrInternal
	}
	codeKey := "vcode:" + email
	failKey := "vcode_fail:" + email
	correct, err := core.MyRedis.Get(ctx, codeKey).Result()
	if err != nil || correct != valiCode {
		failCountStr, _ := core.MyRedis.Get(ctx, failKey).Result()
		failCount := 0
		if failCountStr != "" {
			if v, convErr := strconv.Atoi(failCountStr); convErr == nil {
				failCount = v
			}
		}
		failCount++
		_ = core.MyRedis.Set(ctx, failKey, strconv.Itoa(failCount), 5*time.Minute).Err()
		if failCount >= maxValidateFail {
			_ = core.MyRedis.Del(ctx, codeKey).Err()
		}
		return domain.ErrUnauthorized
	}
	return nil
}

func (s *userService) clearEmailCode(ctx context.Context, email string) {
	if core.MyRedis == nil {
		return
	}
	_ = core.MyRedis.Del(ctx, "vcode:"+email, "vcode_fail:"+email).Err()
}

func (s *userService) DeleteMe(ctx context.Context, in domain.DeleteMeInput) error {
	u, err := s.users.FindByPhone(ctx, in.Phone)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	if u.Email != in.Email {
		return domain.ErrBadRequest
	}
	if err := s.IdentityValidate(ctx, in.Email, in.ValiCode); err != nil {
		return err
	}
	if err := s.users.SoftDeleteAccount(ctx, u.ID); err != nil {
		return err
	}
	InvalidateUserSession(ctx, u.ID)
	return nil
}

func (s *userService) IdentityValidate(ctx context.Context, email, valiCode string) error {
	if !strings.Contains(email, "@") {
		return domain.ErrBadRequest
	}
	ok, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	if err := s.verifyEmailCode(ctx, email, valiCode); err != nil {
		return err
	}
	s.clearEmailCode(ctx, email)
	return nil
}

func (s *userService) RefreshToken(ctx context.Context, refreshToken string) (*domain.AuthTokens, error) {
	if refreshToken == "" {
		return nil, domain.ErrBadRequest
	}
	token, claims, err := common.ParseRefreshToken(refreshToken)
	if err != nil || !token.Valid {
		return nil, domain.ErrUnauthorized
	}
	userID := claims.UserID
	if userID == 0 {
		return nil, domain.ErrUnauthorized
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	rec, err := s.users.FindValidRefreshToken(ctx, refreshToken, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if time.Now().After(rec.ExpiresAt) {
		_ = s.users.MarkRefreshTokenUsed(ctx, rec.ID)
		return nil, domain.ErrTokenExpired
	}
	if err := s.users.MarkRefreshTokenUsed(ctx, rec.ID); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, u)
}

func (s *userService) ModifyPassword(ctx context.Context, in domain.ModifyPasswordInput) (*domain.User, error) {
	ok, err := s.users.ExistsByEmail(ctx, in.Email)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}
	u, err := s.users.FindByEmail(ctx, in.Email)
	if err != nil {
		return nil, err
	}
	if err := s.IdentityValidate(ctx, in.Email, in.ValiCode); err != nil {
		return nil, err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, domain.ErrInternal
	}
	if err := s.users.UpdatePassword(ctx, u.ID, string(hashed)); err != nil {
		return nil, err
	}
	u, err = s.users.FindByID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	InvalidateUserSession(ctx, u.ID)
	return u, nil
}

func (s *userService) CheckOAuth2App(ctx context.Context, appID string) (*domain.OAuth2App, error) {
	return s.users.FindActiveOAuth2App(ctx, appID)
}

func (s *userService) AuthorizeOAuth2App(ctx context.Context, in domain.OAuth2AuthorizeInput) (string, error) {
	if in.ResponseType != "code" {
		return "", domain.ErrBadRequest
	}
	if in.Scope != "read_basic" {
		return "", domain.ErrBadRequest
	}
	app, err := s.users.FindActiveOAuth2App(ctx, in.AppID)
	if err != nil {
		return "", err
	}
	if app.RedirectURI != in.RedirectURI {
		return "", domain.ErrBadRequest
	}
	code := util.GenerateRandomString(32)
	expiresAt := time.Now().Add(5 * time.Minute)
	if err := s.users.CreateOAuth2Code(ctx, code, in.AppID, in.UserID, in.Scope, in.RedirectURI, in.State, expiresAt); err != nil {
		return "", domain.ErrInternal
	}
	record := &domain.OAuth2AuthRecord{
		UserID:       in.UserID,
		AppID:        in.AppID,
		AppName:      app.AppName,
		Scope:        in.Scope,
		IPAddress:    in.IPAddress,
		UserAgent:    in.UserAgent,
		Status:       "authorized",
		AuthorizedAt: time.Now(),
		ExpiresAt:    expiresAt,
	}
	if err := s.users.CreateOAuth2AuthRecord(ctx, record); err != nil {
		log.Printf("记录OAuth2授权信息失败: %v", err)
	}
	return fmt.Sprintf("%s?code=%s&state=%s", in.RedirectURI, code, in.State), nil
}

func (s *userService) GetTokenByOAuth2(ctx context.Context, in domain.OAuth2TokenInput) (*domain.OAuth2TokenResult, error) {
	if in.Code == "" || in.AppID == "" || in.AppSecret == "" {
		return nil, domain.ErrBadRequest
	}
	app, err := s.users.FindActiveOAuth2App(ctx, in.AppID)
	if err != nil {
		return nil, err
	}
	if app.AppSecret != in.AppSecret {
		return nil, domain.ErrUnauthorized
	}
	authCode, err := s.users.FindUnusedOAuth2Code(ctx, in.Code, in.AppID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if time.Now().After(authCode.ExpiresAt) {
		return nil, domain.ErrBadRequest
	}
	if err := s.users.MarkOAuth2CodeUsed(ctx, authCode.CodeID); err != nil {
		return nil, err
	}
	accessToken, err := common.ReleaseOAuth2Token(authCode.UserID, in.AppID, authCode.Scope)
	if err != nil {
		return nil, domain.ErrInternal
	}
	tokenExpires := time.Now().Add(5 * time.Minute)
	if err := s.users.CreateOAuth2Token(ctx, accessToken, in.AppID, authCode.UserID, authCode.Scope, tokenExpires); err != nil {
		return nil, domain.ErrInternal
	}
	return &domain.OAuth2TokenResult{
		AccessToken: accessToken,
		Scope:       authCode.Scope,
		ExpiresIn:   300,
	}, nil
}

func (s *userService) ListOAuth2AuthRecords(ctx context.Context, userID, page, pageSize int) (*domain.OAuth2AuthRecordPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	total, err := s.users.CountOAuth2AuthRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	offset := (page - 1) * pageSize
	records, err := s.users.ListOAuth2AuthRecords(ctx, userID, offset, pageSize)
	if err != nil {
		return nil, err
	}
	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	return &domain.OAuth2AuthRecordPage{
		Records:    records,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *userService) ValidateEmailExists(ctx context.Context, email string, mode int) error {
	if email == "" {
		return domain.ErrBadRequest
	}
	ok, err := s.users.ExistsByEmail(ctx, email)
	if err != nil {
		return err
	}
	if mode == 1 && !ok {
		return domain.ErrNotFound
	}
	if mode == 0 && ok {
		return domain.ErrConflict
	}
	return nil
}

func (s *userService) UpdateAvatarByPhone(ctx context.Context, phone, avatarURL string) (*domain.User, error) {
	if len(phone) != 11 {
		return nil, domain.ErrBadRequest
	}
	u, err := s.users.FindByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	if err := s.users.UpdateAvatarURL(ctx, u.ID, avatarURL); err != nil {
		return nil, err
	}
	u.AvatarURL = avatarURL
	InvalidateUserSession(ctx, u.ID)
	return u, nil
}

func (s *userService) GetAvatarByPhone(ctx context.Context, phone string) (*domain.User, error) {
	return s.users.FindByPhone(ctx, phone)
}

func (s *userService) GetInfo(ctx context.Context, phone string, userID int) (*domain.User, error) {
	return s.users.FindByPhoneOrID(ctx, phone, userID)
}

func (s *userService) UpdateUserInfo(ctx context.Context, in domain.UpdateUserInput, tokenUserID int) error {
	if tokenUserID != in.UserID {
		return domain.ErrForbidden
	}
	u, err := s.users.FindByID(ctx, in.UserID)
	if err != nil {
		return err
	}
	u.Name = in.Name
	u.Intro = in.Intro
	u.AvatarURL = in.AvatarURL
	if in.HideProfilePosts != nil {
		u.HideProfilePosts = *in.HideProfilePosts
	}
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	InvalidateUserSession(ctx, u.ID)
	return nil
}

func (s *userService) ToggleEmailNotification(ctx context.Context, userID int) error {
	return s.users.ToggleEmailNotificationBlocked(ctx, userID)
}

func (s *userService) CalculateAndSaveScores(ctx context.Context) {
	log.Println("CalculateAndSaveScores")
	users, err := s.users.ListAllUsers(ctx)
	if err != nil {
		log.Printf("CalculateAndSaveScores list users: %v", err)
		return
	}
	for _, user := range users {
		if user.ID == 0 {
			continue
		}
		postIDs, err := s.users.ListPostIDsByUserID(ctx, user.ID)
		if err != nil {
			continue
		}
		totalLikeNum := 0
		for _, postID := range postIDs {
			likeNum, err := s.users.GetPostLikeNum(ctx, postID)
			if err == nil {
				totalLikeNum += likeNum
			}
		}
		pcommentIDs, err := s.users.ListPcommentIDsByUserID(ctx, user.ID)
		if err != nil {
			continue
		}
		for _, pcommentID := range pcommentIDs {
			likeNum, err := s.users.GetPcommentLikeNum(ctx, pcommentID)
			if err == nil {
				totalLikeNum += likeNum
			}
		}
		psaveCount, _ := s.users.CountPsavesByPostIDs(ctx, postIDs)
		commentedCount, _ := s.users.CountPcommentsOnPostsExcludingUser(ctx, postIDs, user.ID)
		repliedCount, _ := s.users.CountCcommentsOnPcommentsExcludingUser(ctx, pcommentIDs, user.ID)
		ccommentIDs, err := s.users.ListCcommentIDsByUserID(ctx, user.ID)
		if err != nil {
			continue
		}
		for _, ccommentID := range ccommentIDs {
			likeNum, err := s.users.GetCcommentLikeNum(ctx, ccommentID)
			if err == nil {
				totalLikeNum += likeNum
			}
		}
		sueCount, _ := s.users.CountSuccessfulSuesByUser(ctx, user.ID)
		totalScore := len(postIDs)*10 + len(pcommentIDs)*5 + len(ccommentIDs)*5 + totalLikeNum +
			int(psaveCount)*3 + int(commentedCount+repliedCount)*2 + int(sueCount)*20 - user.PunishNum*20
		_ = s.users.UpdateScore(ctx, user.ID, totalScore)
	}
	log.Println("Score updated")
}

func (s *userService) GetStatistics(ctx context.Context, userID int) (*domain.UserStatistics, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	curTime := time.Now()
	durationTime := curTime.Sub(u.BanEnd)
	lastYear := curTime.Year() - 1
	statisticsStartTime := time.Date(lastYear, 1, 1, 0, 0, 0, 0, curTime.Location())
	statisticsEndTime := time.Date(curTime.Year(), 1, 1, 0, 0, 0, 0, curTime.Location())

	posts, err := s.users.ListPostSummariesByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	postIDs := make([]int, 0, len(posts))
	totalPostCnt := len(posts)
	totalPostLikeNum := 0
	thisYearPostCnt := 0
	thisYearLikeNum := 0
	maxPostLikeNum := 0
	maxPostBrowseNum := 0
	maxPostCommentNum := 0
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
		totalPostLikeNum += post.LikeNum
		if post.PostTime.After(statisticsStartTime) && post.PostTime.Before(statisticsEndTime) {
			thisYearPostCnt++
			thisYearLikeNum += post.LikeNum
			if post.LikeNum > maxPostLikeNum {
				maxPostLikeNum = post.LikeNum
			}
			if post.BrowseNum > maxPostBrowseNum {
				maxPostBrowseNum = post.BrowseNum
			}
			if post.CommentNum > maxPostCommentNum {
				maxPostCommentNum = post.CommentNum
			}
		}
	}

	pcommentIDs, err := s.users.ListPcommentIDsByUserID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	pCommentCnt := len(pcommentIDs)
	pCommentLikeNum := 0
	for _, pcommentID := range pcommentIDs {
		likeNum, err := s.users.GetPcommentLikeNum(ctx, pcommentID)
		if err == nil {
			pCommentLikeNum += likeNum
		}
	}
	besavedCount, _ := s.users.CountPsavesByPostIDs(ctx, postIDs)
	commentedCount, _ := s.users.CountPcommentsOnPostsExcludingUser(ctx, postIDs, u.ID)
	repliedCount, _ := s.users.CountCcommentsOnPcommentsExcludingUser(ctx, pcommentIDs, u.ID)
	ccommentIDs, err := s.users.ListCcommentIDsByUserID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	ccommentCnt := len(ccommentIDs)
	ccommentLikeNum := 0
	for _, ccommentID := range ccommentIDs {
		likeNum, err := s.users.GetCcommentLikeNum(ctx, ccommentID)
		if err == nil {
			ccommentLikeNum += likeNum
		}
	}
	savedCount, _ := s.users.CountPsavesByUserID(ctx, u.ID)
	chatMsgs, err := s.users.ListChatMessagesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	chatCount := len(chatMsgs)
	relevantUsers := []domain.ChatUserSummary{}
	existsUserIDs := make(map[int]bool)
	chatUserCnt := make(map[int]int)
	for _, chatMsg := range chatMsgs {
		if userID != chatMsg.TargetUserID {
			if _, ok := existsUserIDs[chatMsg.TargetUserID]; !ok {
				existsUserIDs[chatMsg.TargetUserID] = true
				relevantUsers = append(relevantUsers, chatMsg.TargetUser)
			}
			chatUserCnt[chatMsg.TargetUserID]++
		}
		if userID != chatMsg.SenderUserID {
			if _, ok := existsUserIDs[chatMsg.SenderUserID]; !ok {
				existsUserIDs[chatMsg.SenderUserID] = true
				relevantUsers = append(relevantUsers, chatMsg.SenderUser)
			}
			chatUserCnt[chatMsg.SenderUserID]++
		}
	}
	maxSayUserID := 0
	maxSayUserCnt := 0
	for uid, cnt := range chatUserCnt {
		if cnt > maxSayUserCnt {
			maxSayUserID = uid
			maxSayUserCnt = cnt
		}
	}
	var maxSayUser domain.ChatUserSummary
	for _, peer := range relevantUsers {
		if peer.UserID == maxSayUserID {
			maxSayUser = peer
			break
		}
	}
	return &domain.UserStatistics{
		Name:             u.Name,
		Intro:            u.Intro,
		Score:            u.Score,
		AvatarURL:        u.AvatarURL,
		DurationTime:     durationTime,
		TotalPostCnt:     totalPostCnt,
		TotalPostLikeNum: totalPostLikeNum,
		ThisYearPostCnt:  thisYearPostCnt,
		ThisYearLikeNum:  thisYearLikeNum,
		MaxLikeNum:       maxPostLikeNum,
		MaxBrowseNum:     maxPostBrowseNum,
		MaxCommentNum:    maxPostCommentNum,
		PCommentCnt:      pCommentCnt,
		PCommentLikeNum:  pCommentLikeNum,
		CcommentCnt:      ccommentCnt,
		CcommentLikeNum:  ccommentLikeNum,
		BeSavedCount:     besavedCount,
		BeCommentedCount: commentedCount,
		RepliedCount:     repliedCount,
		SavedCount:       savedCount,
		ChatCount:        chatCount,
		RelevantUsers:    relevantUsers,
		MaxSayUser:       maxSayUser,
		MaxSayUserCnt:    maxSayUserCnt,
	}, nil
}
