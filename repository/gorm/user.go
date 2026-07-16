package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *userRepository) FindByID(ctx context.Context, id int) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("userID = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("phone = ?", phone).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) FindAuthByEmail(ctx context.Context, email string) (*domain.User, string, error) {
	var user model.User
	err := r.session(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", domain.ErrNotFound
		}
		return nil, "", err
	}
	return userToDomain(&user), user.Password, nil
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var n int64
	err := r.session(ctx).Model(&model.User{}).Where("email = ?", email).Count(&n).Error
	return n > 0, err
}

func (r *userRepository) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var n int64
	err := r.session(ctx).Model(&model.User{}).Where("phone = ?", phone).Count(&n).Error
	return n > 0, err
}

func (r *userRepository) Create(ctx context.Context, name, phone, email, hashedPassword string) (*domain.User, error) {
	user := model.User{
		Name:     name,
		Phone:    phone,
		Email:    email,
		Password: hashedPassword,
		Banend:   time.Now(),
	}
	if err := r.session(ctx).Create(&user).Error; err != nil {
		return nil, err
	}
	if user.UserID == 0 {
		return nil, domain.ErrInternal
	}
	return userToDomain(&user), nil
}

func (r *userRepository) SaveRefreshToken(ctx context.Context, userID int, token string, expiresAt int64) error {
	rec := model.RefreshToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Unix(expiresAt, 0),
		IsUsed:    false,
		CreatedAt: time.Now(),
	}
	return r.session(ctx).Create(&rec).Error
}

func (r *userRepository) UseCDKey(ctx context.Context, content string) error {
	var key model.CDKey
	err := r.session(ctx).Where("content = ? AND used = 0", content).First(&key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrBadRequest
		}
		return err
	}
	return r.session(ctx).Model(&key).Updates(model.CDKey{
		Used:     true,
		UsedTime: time.Now(),
	}).Error
}

func (r *userRepository) FindByPhoneOrID(ctx context.Context, phone string, userID int) (*domain.User, error) {
	var user model.User
	err := r.session(ctx).Where("phone = ? or userID = ?", phone, userID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return userToDomain(&user), nil
}

func (r *userRepository) Update(ctx context.Context, u *domain.User) error {
	m := userToModel(u)
	return r.session(ctx).Save(&m).Error
}

func (r *userRepository) SoftDeleteAccount(ctx context.Context, userID int) error {
	return r.session(ctx).Model(&model.User{}).Where("userID = ?", userID).Updates(map[string]interface{}{
		"name":       "用户已注销",
		"phone":      "0",
		"email":      "",
		"AvatarURL":  "",
	}).Error
}

func (r *userRepository) UpdatePassword(ctx context.Context, userID int, hashedPassword string) error {
	return r.session(ctx).Model(&model.User{}).Where("userID = ?", userID).Update("password", hashedPassword).Error
}

func (r *userRepository) UpdateAvatarURL(ctx context.Context, userID int, avatarURL string) error {
	return r.session(ctx).Model(&model.User{}).Where("userID = ?", userID).Update("AvatarURL", avatarURL).Error
}

func (r *userRepository) ToggleEmailNotificationBlocked(ctx context.Context, userID int) error {
	var user model.User
	if err := r.session(ctx).Where("userID = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrNotFound
		}
		return err
	}
	blocked := !user.ISEmailNotificationBlocked
	return r.session(ctx).Model(&user).Update("is_email_notification_blocked", blocked).Error
}

func (r *userRepository) UpdateScore(ctx context.Context, userID int, score int) error {
	return r.session(ctx).Model(&model.User{}).Where("userID = ?", userID).Update("Score", score).Error
}

func (r *userRepository) ListAllUsers(ctx context.Context) ([]*domain.User, error) {
	var users []model.User
	if err := r.session(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.User, 0, len(users))
	for i := range users {
		if users[i].UserID == 0 {
			continue
		}
		out = append(out, userToDomain(&users[i]))
	}
	return out, nil
}

func (r *userRepository) FindValidRefreshToken(ctx context.Context, token string, userID int) (*domain.RefreshTokenRecord, error) {
	var rec model.RefreshToken
	err := r.session(ctx).Where("token = ? AND user_id = ? AND is_used = ?", token, userID, false).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if rec.ID == 0 {
		return nil, domain.ErrNotFound
	}
	return &domain.RefreshTokenRecord{
		ID:        rec.ID,
		UserID:    rec.UserID,
		Token:     rec.Token,
		ExpiresAt: rec.ExpiresAt,
		IsUsed:    rec.IsUsed,
	}, nil
}

func (r *userRepository) MarkRefreshTokenUsed(ctx context.Context, id int) error {
	return r.session(ctx).Model(&model.RefreshToken{}).Where("id = ?", id).Update("is_used", true).Error
}

func (r *userRepository) FindActiveOAuth2App(ctx context.Context, appID string) (*domain.OAuth2App, error) {
	var app model.OAuth2App
	err := r.session(ctx).Where("app_id = ? AND is_active = ?", appID, true).First(&app).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.OAuth2App{
		AppID:       app.AppID,
		AppName:     app.AppName,
		AppSecret:   app.AppSecret,
		AppIcon:     app.AppIcon,
		RedirectURI: app.RedirectURI,
		Description: app.Description,
	}, nil
}

func (r *userRepository) CreateOAuth2Code(ctx context.Context, code string, appID string, userID int, scope, redirectURI, state string, expiresAt time.Time) error {
	authCode := model.OAuth2Code{
		Code:        code,
		AppID:       appID,
		UserID:      userID,
		Scope:       scope,
		RedirectURI: redirectURI,
		State:       state,
		ExpiresAt:   expiresAt,
		IsUsed:      false,
		CreatedAt:   time.Now(),
	}
	return r.session(ctx).Create(&authCode).Error
}

func (r *userRepository) CreateOAuth2AuthRecord(ctx context.Context, record *domain.OAuth2AuthRecord) error {
	if record == nil {
		return domain.ErrBadRequest
	}
	rec := model.OAuth2AuthRecord{
		UserID:       record.UserID,
		AppID:        record.AppID,
		AppName:      record.AppName,
		Scope:        record.Scope,
		IPAddress:    record.IPAddress,
		UserAgent:    record.UserAgent,
		Status:       record.Status,
		AuthorizedAt: record.AuthorizedAt,
		ExpiresAt:    record.ExpiresAt,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	return r.session(ctx).Create(&rec).Error
}

func (r *userRepository) FindUnusedOAuth2Code(ctx context.Context, code, appID string) (*domain.OAuth2Code, error) {
	var authCode model.OAuth2Code
	err := r.session(ctx).Where("code = ? AND app_id = ? AND is_used = ?", code, appID, false).First(&authCode).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.OAuth2Code{
		CodeID:      authCode.CodeID,
		Code:        authCode.Code,
		AppID:       authCode.AppID,
		UserID:      authCode.UserID,
		Scope:       authCode.Scope,
		RedirectURI: authCode.RedirectURI,
		State:       authCode.State,
		ExpiresAt:   authCode.ExpiresAt,
		IsUsed:      authCode.IsUsed,
	}, nil
}

func (r *userRepository) MarkOAuth2CodeUsed(ctx context.Context, codeID int) error {
	return r.session(ctx).Model(&model.OAuth2Code{}).Where("code_id = ?", codeID).Update("is_used", true).Error
}

func (r *userRepository) CreateOAuth2Token(ctx context.Context, accessToken, appID string, userID int, scope string, expiresAt time.Time) error {
	token := model.OAuth2Token{
		AccessToken:  accessToken,
		RefreshToken: "",
		AppID:        appID,
		UserID:       userID,
		Scope:        scope,
		ExpiresAt:    expiresAt,
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	return r.session(ctx).Create(&token).Error
}

func (r *userRepository) CountOAuth2AuthRecords(ctx context.Context, userID int) (int64, error) {
	var total int64
	err := r.session(ctx).Model(&model.OAuth2AuthRecord{}).Where("user_id = ?", userID).Count(&total).Error
	return total, err
}

func (r *userRepository) ListOAuth2AuthRecords(ctx context.Context, userID, offset, limit int) ([]domain.OAuth2AuthRecord, error) {
	var records []model.OAuth2AuthRecord
	err := r.session(ctx).Where("user_id = ?", userID).
		Order("authorized_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.OAuth2AuthRecord, 0, len(records))
	for _, rec := range records {
		out = append(out, domain.OAuth2AuthRecord{
			RecordID:     rec.RecordID,
			UserID:       rec.UserID,
			AppID:        rec.AppID,
			AppName:      rec.AppName,
			Scope:        rec.Scope,
			IPAddress:    rec.IPAddress,
			UserAgent:    rec.UserAgent,
			Status:       rec.Status,
			AuthorizedAt: rec.AuthorizedAt,
			ExpiresAt:    rec.ExpiresAt,
		})
	}
	return out, nil
}

// CreateUserAPIToken 创建 PAT 记录，返回 token_id。tokenHash 为 SHA-256(token)。
func (r *userRepository) CreateUserAPIToken(ctx context.Context, userID int, tokenHash, tokenPrefix, name, scopes string) (int, error) {
	now := time.Now()
	rec := model.UserAPIToken{
		UserID:      userID,
		TokenHash:   tokenHash,
		TokenPrefix: tokenPrefix,
		Name:        name,
		Scopes:      scopes,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := r.session(ctx).Create(&rec).Error; err != nil {
		return 0, err
	}
	return rec.TokenID, nil
}

// FindUserAPITokenByHash 按 hash 查找活跃的 PAT，返回智能体验证所需的最小字段集。
func (r *userRepository) FindUserAPITokenByHash(ctx context.Context, tokenHash string) (*domain.UserAPITokenVerify, error) {
	var rec model.UserAPIToken
	err := r.session(ctx).Where("token_hash = ? AND is_active = ?", tokenHash, true).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.UserAPITokenVerify{
		TokenID:  rec.TokenID,
		UserID:   rec.UserID,
		Scopes:   splitScopes(rec.Scopes),
		IsActive: rec.IsActive,
	}, nil
}

// TouchUserAPITokenLastUsed 更新最后使用时间。失败仅记日志，不影响主流程。
func (r *userRepository) TouchUserAPITokenLastUsed(ctx context.Context, tokenID int, lastUsedAt time.Time) error {
	return r.session(ctx).Model(&model.UserAPIToken{}).
		Where("token_id = ?", tokenID).
		Update("last_used_at", lastUsedAt).Error
}

// CountUserAPITokens 统计用户活跃 PAT 数量。
func (r *userRepository) CountUserAPITokens(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.UserAPIToken{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error
	return count, err
}

// ListUserAPITokens 分页列出用户的 PAT（不含明文）。
func (r *userRepository) ListUserAPITokens(ctx context.Context, userID, offset, limit int) ([]domain.UserAPITokenItem, error) {
	var records []model.UserAPIToken
	err := r.session(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.UserAPITokenItem, 0, len(records))
	for _, rec := range records {
		out = append(out, domain.UserAPITokenItem{
			TokenID:    rec.TokenID,
			Prefix:     rec.TokenPrefix,
			Name:       rec.Name,
			Scopes:     rec.Scopes,
			LastUsedAt: rec.LastUsedAt,
			IsActive:   rec.IsActive,
			CreatedAt:   rec.CreatedAt,
		})
	}
	return out, nil
}

// RevokeUserAPIToken 软删（is_active=0），同时校验 user_id 防越权。
func (r *userRepository) RevokeUserAPIToken(ctx context.Context, tokenID, userID int) error {
	res := r.session(ctx).Model(&model.UserAPIToken{}).
		Where("token_id = ? AND user_id = ?", tokenID, userID).
		Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteUserAPIToken 硬删，仅允许删除已吊销（is_active=0）的记录，校验 user_id 防越权。
func (r *userRepository) DeleteUserAPIToken(ctx context.Context, tokenID, userID int) error {
	res := r.session(ctx).
		Where("token_id = ? AND user_id = ? AND is_active = ?", tokenID, userID, false).
		Delete(&model.UserAPIToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 不存在或仍处于活跃状态
		return domain.ErrNotFound
	}
	return nil
}

// splitScopes 将逗号分隔的 scope 字符串拆为切片。
func splitScopes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (r *userRepository) ListPostIDsByUserID(ctx context.Context, userID int) ([]int, error) {
	var ids []int
	err := r.session(ctx).Model(&model.Post{}).Where("userID = ?", userID).Pluck("postID", &ids).Error
	return ids, err
}

func (r *userRepository) GetPostLikeNum(ctx context.Context, postID int) (int, error) {
	var likeNum int
	row := r.session(ctx).Model(&model.Post{}).Where("postID = ?", postID).Select("like_num").Row()
	if err := row.Scan(&likeNum); err != nil {
		return 0, err
	}
	return likeNum, nil
}

func (r *userRepository) ListPcommentIDsByUserID(ctx context.Context, userID int) ([]int, error) {
	var ids []int
	err := r.session(ctx).Model(&model.Pcomment{}).Where("userID = ?", userID).Pluck("pcommentID", &ids).Error
	return ids, err
}

func (r *userRepository) GetPcommentLikeNum(ctx context.Context, pcommentID int) (int, error) {
	var likeNum int
	row := r.session(ctx).Model(&model.Pcomment{}).Where("pcommentID = ?", pcommentID).Select("like_num").Row()
	if err := row.Scan(&likeNum); err != nil {
		return 0, err
	}
	return likeNum, nil
}

func (r *userRepository) CountPsavesByPostIDs(ctx context.Context, postIDs []int) (int64, error) {
	if len(postIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.session(ctx).Model(&model.Psave{}).Where("ptargetID IN (?)", postIDs).Count(&count).Error
	return count, err
}

func (r *userRepository) CountPcommentsOnPostsExcludingUser(ctx context.Context, postIDs []int, userID int) (int64, error) {
	if len(postIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.session(ctx).Model(&model.Pcomment{}).Where("ptargetID IN (?)", postIDs).Not("userID = ?", userID).Count(&count).Error
	return count, err
}

func (r *userRepository) CountCcommentsOnPcommentsExcludingUser(ctx context.Context, pcommentIDs []int, userID int) (int64, error) {
	if len(pcommentIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := r.session(ctx).Model(&model.Ccomment{}).Where("ctargetID IN (?)", pcommentIDs).Not("userID = ?", userID).Count(&count).Error
	return count, err
}

func (r *userRepository) ListCcommentIDsByUserID(ctx context.Context, userID int) ([]int, error) {
	var ids []int
	err := r.session(ctx).Model(&model.Ccomment{}).Where("userID = ?", userID).Pluck("ccommentID", &ids).Error
	return ids, err
}

func (r *userRepository) GetCcommentLikeNum(ctx context.Context, ccommentID int) (int, error) {
	var likeNum int
	row := r.session(ctx).Model(&model.Ccomment{}).Where("ccommentID = ?", ccommentID).Select("like_num").Row()
	if err := row.Scan(&likeNum); err != nil {
		return 0, err
	}
	return likeNum, nil
}

func (r *userRepository) CountSuccessfulSuesByUser(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Sue{}).
		Where("status = ? AND finish = ? AND userID = ?", " ok", 1, userID).
		Select("DISTINCT targettype, targetID").
		Count(&count).Error
	return count, err
}

func (r *userRepository) ListPostSummariesByUserID(ctx context.Context, userID int) ([]domain.PostSummary, error) {
	var posts []model.Post
	if err := r.session(ctx).Where("userID = ?", userID).Find(&posts).Error; err != nil {
		return nil, err
	}
	out := make([]domain.PostSummary, 0, len(posts))
	for _, p := range posts {
		out = append(out, domain.PostSummary{
			ID:         p.PostID,
			LikeNum:    p.LikeNum,
			BrowseNum:  p.BrowseNum,
			CommentNum: p.CommentNum,
			PostTime:   p.PostTime,
		})
	}
	return out, nil
}

func (r *userRepository) CountPsavesByUserID(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Psave{}).Where("userID = ?", userID).Count(&count).Error
	return count, err
}

func (r *userRepository) ListChatMessagesForUser(ctx context.Context, userID int) ([]domain.ChatMessagePeer, error) {
	var msgs []model.ChatMsg
	err := r.session(ctx).Where("senderUserID = ? OR targetUserID = ?", userID, userID).
		Preload("TargetUser").
		Preload("SenderUser").
		Find(&msgs).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChatMessagePeer, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, domain.ChatMessagePeer{
			TargetUserID: m.TargetUserID,
			SenderUserID: m.SenderUserID,
			TargetUser: domain.ChatUserSummary{
				UserID:    m.TargetUser.UserID,
				Name:      m.TargetUser.Name,
				AvatarURL: m.TargetUser.AvatarURL,
				Identity:  m.TargetUser.Identity,
				Score:     m.TargetUser.Score,
			},
			SenderUser: domain.ChatUserSummary{
				UserID:    m.SenderUser.UserID,
				Name:      m.SenderUser.Name,
				AvatarURL: m.SenderUser.AvatarURL,
				Identity:  m.SenderUser.Identity,
				Score:     m.SenderUser.Score,
			},
		})
	}
	return out, nil
}

func (r *userRepository) ListTopActive(ctx context.Context, limit int) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.User
	err := r.session(ctx).
		Order("(received_like_num + received_vote_num) DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.User, 0, len(rows))
	for i := range rows {
		out = append(out, userToDomain(&rows[i]))
	}
	return out, nil
}
