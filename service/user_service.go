package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"loginTest/cache"
	"loginTest/common"
	"loginTest/core"
	"loginTest/domain"
	"loginTest/repository"
	"loginTest/util"

	"github.com/brianvoe/gofakeit"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const maxValidateFail = 5

type userService struct {
	users repository.UserRepository
	// coins 投币换来的主页经验要参与每小时 score 重算，否则重算会把投币经验冲掉。
	// 允许为 nil（单测与未开通币体系的场景）。
	coins CoinService
}

func NewUserService(users repository.UserRepository, coins CoinService) UserService {
	return &userService{users: users, coins: coins}
}

func (s *userService) GetByID(ctx context.Context, id int) (*domain.User, error) {
	if id <= 0 {
		return nil, domain.ErrBadRequest
	}
	key := util.GetUserInfoKey(id)
	return MultiLevelGet(ctx, key, func() (*domain.User, error) {
		return s.users.FindByID(ctx, id)
	}, cache.ProfileUserInfo)
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

// IssueTokensForUser 为已存在用户发放令牌（SSO 登录/绑定路径复用）。
func (s *userService) IssueTokensForUser(ctx context.Context, userID int) (*domain.AuthTokens, error) {
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, u)
}

// PlatformCreateUserInput 平台中转建号入参（跳过邮箱验证码与 CDKey）。
type PlatformCreateUserInput struct {
	Name     string
	Email    string
	Password string
	Phone    string
}

type MirrorProvisionInput struct {
	Name     string
	Email    string
	Password string
}

// MirrorProvision 供 Mirror 服务端在 UniSSO 验证后调用：已有映射账号直接复用。
func (s *userService) MirrorProvision(ctx context.Context, in MirrorProvisionInput) (*domain.User, *domain.AuthTokens, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if !strings.Contains(email, "@") || len(in.Password) < 6 {
		return nil, nil, &WorkbenchValidationError{"email 或 password 不合法"}
	}

	if u, _, err := s.users.FindAuthByEmail(ctx, email); err == nil {
		tokens, err := s.issueTokens(ctx, u)
		if err != nil {
			return nil, nil, err
		}
		return u, tokens, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, nil, err
	}

	return s.PlatformCreateUser(ctx, PlatformCreateUserInput{
		Name:     in.Name,
		Email:    email,
		Password: in.Password,
	})
}

// PlatformCreateUser 平台（SM2 验签通过的中转请求）创建用户。
// 与 Register 的差异：不需要邮箱验证码、不需要 CDKey；其余校验（邮箱/手机/密码）保持一致。
// 返回创建的用户（含 userID）与登录令牌（可直接使用）。
func (s *userService) PlatformCreateUser(ctx context.Context, in PlatformCreateUserInput) (*domain.User, *domain.AuthTokens, error) {
	email := strings.TrimSpace(in.Email)
	password := in.Password
	name := strings.TrimSpace(in.Name)
	phone := strings.TrimSpace(in.Phone)

	if !strings.Contains(email, "@") {
		return nil, nil, &WorkbenchValidationError{"email 不合法"}
	}
	if len(password) < 6 {
		return nil, nil, &WorkbenchValidationError{"password 不能少于 6 位"}
	}
	if name == "" {
		name = gofakeit.Name()
	}
	if phone == "" {
		for {
			phone = util.GenerateRandomDigits(11)
			ok, err := s.users.ExistsByPhone(ctx, phone)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				break
			}
		}
	} else {
		phone = util.NormalizePhone(phone)
		if len(phone) != 11 {
			return nil, nil, &WorkbenchValidationError{"phone 必须为 11 位数字"}
		}
		if ok, err := s.users.ExistsByPhone(ctx, phone); err == nil && ok {
			return nil, nil, &WorkbenchValidationError{"该手机号已存在"}
		}
	}
	if ok, err := s.users.ExistsByEmail(ctx, email); err != nil {
		return nil, nil, err
	} else if ok {
		return nil, nil, &WorkbenchValidationError{"该邮箱已存在"}
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, domain.ErrInternal
	}
	u, err := s.users.Create(ctx, name, phone, email, string(hashed))
	if err != nil {
		return nil, nil, err
	}
	tokens, err := s.issueTokens(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	log.Printf("[platform] 平台建号成功: userID=%d email=%s invoker 记录于 access log", u.ID, email)
	return u, tokens, nil
}

func (s *userService) issueTokens(ctx context.Context, u *domain.User) (*domain.AuthTokens, error) {
	access, err := common.ReleaseToken(*u)
	if err != nil {
		return nil, domain.ErrInternal
	}
	refresh, err := common.ReleaseRefreshToken(*u)
	if err != nil {
		return nil, domain.ErrInternal
	}
	expires := time.Now().Add(30 * 24 * time.Hour).Unix()
	if err := s.users.SaveRefreshToken(ctx, u.ID, refresh, expires); err != nil {
		return nil, err
	}
	CacheUserAfterAuth(ctx, u)
	return &domain.AuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         u,
	}, nil
}

func (s *userService) verifyEmailCode(ctx context.Context, email, valiCode string) error {
	rds := core.MyRedis
	if rds == nil {
		return domain.ErrInternal
	}
	email = strings.ToLower(strings.TrimSpace(email))
	codeKey := "vcode:" + email
	failKey := "vcode_fail:" + email
	correct, err := rds.Get(ctx, codeKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return domain.ErrInternal
	}
	if err != nil || correct != valiCode {
		failCountStr, failErr := rds.Get(ctx, failKey).Result()
		if failErr != nil && !errors.Is(failErr, redis.Nil) {
			return domain.ErrInternal
		}
		failCount := 0
		if failCountStr != "" {
			if v, convErr := strconv.Atoi(failCountStr); convErr == nil {
				failCount = v
			}
		}
		failCount++
		if err := rds.Set(ctx, failKey, strconv.Itoa(failCount), 5*time.Minute).Err(); err != nil {
			return domain.ErrInternal
		}
		if failCount >= maxValidateFail {
			if err := rds.Del(ctx, codeKey).Err(); err != nil {
				return domain.ErrInternal
			}
		}
		return domain.ErrUnauthorized
	}
	return nil
}

func (s *userService) clearEmailCode(ctx context.Context, email string) {
	if core.MyRedis == nil {
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))
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

func (s *userService) ModifyEmail(ctx context.Context, in domain.ModifyEmailInput) (*domain.User, error) {
	if in.UserID <= 0 {
		return nil, domain.ErrUnauthorized
	}
	newEmail := strings.ToLower(strings.TrimSpace(in.NewEmail))
	at := strings.IndexByte(newEmail, '@')
	if newEmail == "" || at <= 0 || at != strings.LastIndexByte(newEmail, '@') || at == len(newEmail)-1 {
		return nil, domain.ErrBadRequest
	}
	if len(newEmail) > 255 {
		return nil, domain.ErrBadRequest
	}

	u, err := s.users.FindByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(u.Email), newEmail) {
		return nil, domain.ErrBadRequest
	}

	exists, err := s.users.ExistsByEmail(ctx, newEmail)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, domain.ErrConflict
	}
	used, err := s.users.ExistsUsedEmail(ctx, u.ID, newEmail)
	if err != nil {
		return nil, err
	}
	if used {
		return nil, domain.ErrEmailUsed
	}
	if err := s.verifyEmailCode(ctx, newEmail, in.ValiCode); err != nil {
		return nil, err
	}
	if err := s.users.UpdateEmailWithHistory(ctx, u.ID, u.Email, newEmail); err != nil {
		return nil, err
	}

	s.clearEmailCode(ctx, newEmail)
	InvalidateUserSession(ctx, u.ID)
	InvalidateUserInfo(u.ID)
	u.Email = newEmail
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
	app, err := s.users.FindActiveOAuth2AppByRedirectURI(ctx, in.AppID, in.RedirectURI)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", domain.ErrBadRequest
		}
		return "", err
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
	_, err := s.users.FindActiveOAuth2AppBySecret(ctx, in.AppID, in.AppSecret)
	if err != nil {
		return nil, err
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
	InvalidateUserInfo(u.ID)
	return nil
}

// SetCustomTitle 满级(Lv8, score>=5000)用户自定义称号;1~12 字符(rune 计),自动 trim。
func (s *userService) SetCustomTitle(ctx context.Context, userID int, title string) error {
	title = strings.TrimSpace(title)
	runes := []rune(title)
	if len(runes) == 0 || len(runes) > 12 {
		return domain.ErrBadRequest
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.Score < 5000 {
		return domain.ErrForbidden
	}
	if err := s.users.UpdateCustomTitle(ctx, userID, title); err != nil {
		return err
	}
	InvalidateUserSession(ctx, u.ID)
	InvalidateUserInfo(u.ID)
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
	// 投币换来的主页经验：口径与投币当场加分的口径完全一致（投币总数 × 10），
	// 所以每小时重算不会把投币经验冲掉。整表一次取回，不在按用户的循环里逐个查。
	thrownByUser := map[int]int{}
	if s.coins != nil {
		totals, err := s.coins.ThrownTotalsAll(ctx)
		if err != nil {
			log.Printf("CalculateAndSaveScores thrown totals: %v", err)
		} else {
			thrownByUser = totals
		}
	}
	for _, user := range users {
		if user.ID == 0 {
			continue
		}
		// 职位小号（开发人员匿名 @）不参与经验重算：它的 score=5000（满级）与称号是
		// staff_aliases 给的固定口径，按发帖/评论重算会在几小时内把它刷成 0。
		if domain.IsStaffShellEmail(user.Email) {
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
		browseBonus := 0
		postBrowseNums, err := s.users.ListPostBrowseNumsByUserID(ctx, user.ID)
		if err == nil {
			for _, n := range postBrowseNums {
				browseBonus += int(math.Ceil(float64(n) * 0.01))
			}
		}
		pvoteCount, _ := s.users.CountPvotesByPostIDs(ctx, postIDs)
		// 投币经验与发帖/评论/点赞等项相加，之后仍然只由 score 这一条线决定主页等级。
		// 举报成功/处罚项已在 online 9247516 移除，不再恢复。
		totalScore := len(postIDs)*10 + len(pcommentIDs)*5 + len(ccommentIDs)*5 + totalLikeNum +
			int(psaveCount)*3 + int(pvoteCount)*3 + int(commentedCount+repliedCount)*2 + browseBonus +
			thrownByUser[user.ID]*domain.CoinExpPerCoin
		if totalScore != user.Score {
			// 分数落库后同步失效用户信息缓存,否则页面等级最长滞后 1h+ 才更新
			_ = s.users.UpdateScore(ctx, user.ID, totalScore)
			InvalidateUserInfo(user.ID)
		}
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

// ---- User API Token (PAT) ----

// apiTokenScopes 当前支持的所有 scope，与智能体 MCP 工具一一对应。
// 顺序也用于前端展示。
var apiTokenScopes = []domain.UserAPITokenScope{
	{Name: "agent:chat", Description: "小秋通用对话：凭 token 即可直接调用智能体对话工具"},
	{Name: "forum:read", Description: "论坛检索：搜索帖子、查看评论"},
	{Name: "forum:write", Description: "论坛发帖：以你的身份发布新帖"},
	{Name: "web:search", Description: "联网搜索：调用小秋的站外搜索能力（当日配额制）"},
}

const (
	apiTokenPrefix       = "sk_xq_" // token 明文前缀，便于识别
	apiTokenRandomLen    = 32       // 随机部分字节数；hex 后 64 字符
	apiTokenMaxPerUser   = 10       // 每个用户最多 10 个活跃 PAT
	apiTokenPrefixShow   = 8        // 前缀展示长度（含 sk_xq_）
	apiTokenDefaultPage  = 1
	apiTokenDefaultLimit = 20
)

// ListAPITokenScopes 返回当前支持的所有 scope 定义。
func (s *userService) ListAPITokenScopes(ctx context.Context) []domain.UserAPITokenScope {
	return apiTokenScopes
}

// CreateAPIToken 创建 PAT。明文 token 仅在此返回一次。
func (s *userService) CreateAPIToken(ctx context.Context, in domain.UserAPITokenCreateInput) (*domain.UserAPITokenCreateResult, error) {
	if in.UserID <= 0 {
		return nil, domain.ErrBadRequest
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, domain.ErrBadRequest
	}
	if len(name) > 64 {
		name = name[:64]
	}
	// 校验 scope：必须是白名单中的合法值
	validSet := make(map[string]struct{}, len(apiTokenScopes))
	for _, sc := range apiTokenScopes {
		validSet[sc.Name] = struct{}{}
	}
	cleaned := make([]string, 0, len(in.Scopes))
	seen := make(map[string]struct{}, len(in.Scopes))
	for _, sc := range in.Scopes {
		sc = strings.TrimSpace(sc)
		if sc == "" {
			continue
		}
		if _, ok := validSet[sc]; !ok {
			return nil, domain.ErrBadRequest
		}
		if _, dup := seen[sc]; dup {
			continue
		}
		seen[sc] = struct{}{}
		cleaned = append(cleaned, sc)
	}
	if len(cleaned) == 0 {
		return nil, domain.ErrBadRequest
	}
	// 限流：每用户最多 apiTokenMaxPerUser 个活跃 token
	count, err := s.users.CountUserAPITokens(ctx, in.UserID)
	if err != nil {
		log.Printf("[pat] count user tokens: %v", err)
		return nil, domain.ErrInternal
	}
	if count >= apiTokenMaxPerUser {
		return nil, domain.ErrTooManyRequests
	}

	// 生成明文 token：sk_xq_ + 32 字节随机 hex
	raw := make([]byte, apiTokenRandomLen)
	if _, err := rand.Read(raw); err != nil {
		return nil, domain.ErrInternal
	}
	plainText := apiTokenPrefix + hex.EncodeToString(raw)
	hash := sha256Hex(plainText)
	prefixShow := plainText
	if len(prefixShow) > apiTokenPrefixShow {
		prefixShow = prefixShow[:apiTokenPrefixShow]
	}
	scopesStr := strings.Join(cleaned, ",")

	tokenID, err := s.users.CreateUserAPIToken(ctx, in.UserID, hash, prefixShow, name, scopesStr)
	if err != nil {
		log.Printf("[pat] create token: %v", err)
		return nil, domain.ErrInternal
	}
	return &domain.UserAPITokenCreateResult{
		TokenID:   tokenID,
		PlainText: plainText,
		Prefix:    prefixShow,
		Name:      name,
		Scopes:    scopesStr,
		IsActive:  true,
		CreatedAt: time.Now(),
	}, nil
}

// ListAPITokens 分页列出用户的 PAT（不含明文）。
func (s *userService) ListAPITokens(ctx context.Context, userID, page, pageSize int) (*domain.UserAPITokenPage, error) {
	if userID <= 0 {
		return nil, domain.ErrBadRequest
	}
	if page <= 0 {
		page = apiTokenDefaultPage
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = apiTokenDefaultLimit
	}
	offset := (page - 1) * pageSize
	total, err := s.users.CountUserAPITokens(ctx, userID)
	if err != nil {
		log.Printf("[pat] count: %v", err)
		return nil, domain.ErrInternal
	}
	records, err := s.users.ListUserAPITokens(ctx, userID, offset, pageSize)
	if err != nil {
		log.Printf("[pat] list: %v", err)
		return nil, domain.ErrInternal
	}
	totalPages := int64(0)
	if pageSize > 0 {
		totalPages = (total + int64(pageSize) - 1) / int64(pageSize)
	}
	return &domain.UserAPITokenPage{
		Records:    records,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// RevokeAPIToken 软删 PAT。tokenID 必须属于 userID，防越权。
func (s *userService) RevokeAPIToken(ctx context.Context, tokenID, userID int) error {
	if tokenID <= 0 || userID <= 0 {
		return domain.ErrBadRequest
	}
	if err := s.users.RevokeUserAPIToken(ctx, tokenID, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		log.Printf("[pat] revoke: %v", err)
		return domain.ErrInternal
	}
	return nil
}

// DeleteAPIToken 硬删已吊销的 PAT 记录。活跃 token 必须先吊销才能删除。
func (s *userService) DeleteAPIToken(ctx context.Context, tokenID, userID int) error {
	if tokenID <= 0 || userID <= 0 {
		return domain.ErrBadRequest
	}
	if err := s.users.DeleteUserAPIToken(ctx, tokenID, userID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		log.Printf("[pat] delete: %v", err)
		return domain.ErrInternal
	}
	return nil
}

// VerifyAPIToken 验证明文 token，返回 user_id + scope。供主站内部或调试使用；
// 智能体侧直连 MySQL 自行实现验证逻辑，不调此方法。
func (s *userService) VerifyAPIToken(ctx context.Context, plainToken string) (*domain.UserAPITokenVerify, error) {
	if plainToken == "" {
		return nil, domain.ErrUnauthorized
	}
	hash := sha256Hex(plainToken)
	res, err := s.users.FindUserAPITokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		log.Printf("[pat] verify: %v", err)
		return nil, domain.ErrInternal
	}
	// 异步回写 last_used_at，不阻塞请求
	go func(tokenID int) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.users.TouchUserAPITokenLastUsed(bgCtx, tokenID, time.Now()); err != nil {
			log.Printf("[pat] touch last_used_at: %v", err)
		}
	}(res.TokenID)
	return res, nil
}

// sha256Hex 返回 SHA-256 十六进制小写。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
