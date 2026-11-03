package gormrepo

import (
	"context"
	"encoding/binary"
	"math"
	"time"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type recommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) repository.RecommendationRepository {
	return &recommendationRepository{db: db}
}

func (r *recommendationRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *recommendationRepository) GetUserMode(ctx context.Context, userID int) (string, error) {
	var rows []model.UserRecommendationSetting
	// 用 Find 而不是 First：绝大多数用户没有行，First 会以 record not found
	// 打进 GORM 错误日志，首页每次请求都会刷屏。
	res := r.session(ctx).Where("user_id = ?", userID).Limit(1).Find(&rows)
	if res.Error != nil {
		return "", res.Error
	}
	if len(rows) == 0 {
		return domain.RecModeAuto, nil
	}
	switch rows[0].Mode {
	case domain.RecModeOn, domain.RecModeOff:
		return rows[0].Mode, nil
	default:
		return domain.RecModeAuto, nil
	}
}

func (r *recommendationRepository) SaveUserMode(ctx context.Context, userID int, mode string) error {
	row := model.UserRecommendationSetting{
		UserID:    userID,
		Mode:      mode,
		UpdatedAt: time.Now(),
	}
	return r.session(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"mode", "updated_at"}),
	}).Create(&row).Error
}

func (r *recommendationRepository) ListGrayRules(ctx context.Context) ([]domain.RecommendationGrayRule, error) {
	var rows []model.RecommendationGrayRule
	if err := r.session(ctx).Order("uid_min ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.RecommendationGrayRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecommendationGrayRule{
			ID:        row.ID,
			UIDMin:    row.UIDMin,
			UIDMax:    row.UIDMax,
			Enabled:   row.Enabled,
			Note:      row.Note,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

func (r *recommendationRepository) UpsertGrayRule(ctx context.Context, in domain.RecommendGrayRuleInput) (*domain.RecommendationGrayRule, error) {
	now := time.Now()
	row := model.RecommendationGrayRule{
		ID:        in.ID,
		UIDMin:    in.UIDMin,
		UIDMax:    in.UIDMax,
		Enabled:   in.Enabled,
		Note:      in.Note,
		UpdatedAt: now,
	}
	if in.ID > 0 {
		res := r.session(ctx).Model(&model.RecommendationGrayRule{}).Where("id = ?", in.ID).Updates(map[string]any{
			"uid_min":    in.UIDMin,
			"uid_max":    in.UIDMax,
			"enabled":    in.Enabled,
			"note":       in.Note,
			"updated_at": now,
		})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			return nil, domain.ErrNotFound
		}
		var saved model.RecommendationGrayRule
		if err := r.session(ctx).Where("id = ?", in.ID).First(&saved).Error; err != nil {
			return nil, err
		}
		return grayRuleToDomain(saved), nil
	}
	row.CreatedAt = now
	if err := r.session(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return grayRuleToDomain(row), nil
}

func (r *recommendationRepository) DeleteGrayRule(ctx context.Context, id int) error {
	res := r.session(ctx).Where("id = ?", id).Delete(&model.RecommendationGrayRule{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *recommendationRepository) MatchGrayRule(ctx context.Context, userID int) (bool, error) {
	var count int64
	err := r.session(ctx).Model(&model.RecommendationGrayRule{}).
		Where("enabled = ? AND uid_min <= ? AND uid_max >= ?", true, userID, userID).
		Count(&count).Error
	return count > 0, err
}

func (r *recommendationRepository) ListPostMeta(ctx context.Context) ([]domain.RecPostMeta, error) {
	var rows []model.Post
	err := r.session(ctx).Model(&model.Post{}).
		Select("postID, userID, `partition`, title, tag, post_time, like_num, comment_num, coin_num, browse_num, is_private, is_anonymous, is_pinned, pinned_expires_at").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecPostMeta, 0, len(rows))
	for i := range rows {
		p := rows[i]
		out = append(out, domain.RecPostMeta{
			ID:              p.PostID,
			UserID:          p.UserID,
			Partition:       p.Partition,
			Title:           p.Title,
			Tag:             p.Tag,
			PostTime:        p.PostTime,
			LikeNum:         p.LikeNum,
			CommentNum:      p.CommentNum,
			CoinNum:         p.CoinNum,
			BrowseNum:       p.BrowseNum,
			IsPrivate:       p.IsPrivate,
			IsAnonymous:     p.IsAnonymous,
			IsPinned:        p.IsPinned,
			PinnedExpiresAt: p.PinnedExpiresAt,
		})
	}
	return out, nil
}

func (r *recommendationRepository) ListEmbeddings(ctx context.Context) ([]domain.RecEmbedding, error) {
	var rows []model.PostEmbedding
	err := r.session(ctx).Model(&model.PostEmbedding{}).
		Select("post_id, kind, model, dims, content_hash, image_url, embedding").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecEmbedding, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if row.Dims <= 0 || len(row.Embedding) != row.Dims*4 {
			continue
		}
		out = append(out, domain.RecEmbedding{
			PostID:      row.PostID,
			Kind:        row.Kind,
			Model:       row.Model,
			Dims:        row.Dims,
			ContentHash: row.ContentHash,
			ImageURL:    row.ImageURL,
			Vector:      decodeFloat32Vector(row.Embedding),
		})
	}
	return out, nil
}

func (r *recommendationRepository) ListPendingTextPosts(ctx context.Context, limit int) ([]domain.RecPostContent, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.Post
	err := r.session(ctx).Model(&model.Post{}).
		Select("postID, title, ptext, tag, `partition`, photos").
		Joins("LEFT JOIN post_embeddings e ON e.post_id = posts.postID AND e.kind = ?", "text").
		Where("e.id IS NULL AND posts.is_private = ?", false).
		Order("posts.postID DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return modelPostsToContents(rows), nil
}

func (r *recommendationRepository) ListPendingImagePosts(ctx context.Context, limit int) ([]domain.RecPostContent, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []model.Post
	err := r.session(ctx).Model(&model.Post{}).
		Select("postID, title, ptext, tag, `partition`, photos").
		Joins("LEFT JOIN post_embeddings e ON e.post_id = posts.postID AND e.kind = ?", "image").
		Where("e.id IS NULL AND posts.is_private = ?", false).
		Where("(photos IS NOT NULL AND photos <> '') OR ptext LIKE ?", "%![%").
		Order("posts.postID DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return modelPostsToContents(rows), nil
}

func (r *recommendationRepository) UpsertEmbedding(ctx context.Context, postID int, kind, modelName string, dims int, contentHash, imageURL string, vector []float32) error {
	row := model.PostEmbedding{
		PostID:      postID,
		Kind:        kind,
		Model:       modelName,
		Dims:        dims,
		ContentHash: contentHash,
		ImageURL:    imageURL,
		Embedding:   encodeFloat32Vector(vector),
		UpdatedAt:   time.Now(),
	}
	return r.session(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "post_id"}, {Name: "kind"}},
		DoUpdates: clause.AssignmentColumns([]string{"model", "dims", "content_hash", "image_url", "embedding", "updated_at"}),
	}).Create(&row).Error
}

func (r *recommendationRepository) DeleteAllEmbeddings(ctx context.Context) error {
	return r.session(ctx).Where("1 = 1").Delete(&model.PostEmbedding{}).Error
}

func (r *recommendationRepository) CountPosts(ctx context.Context) (int64, error) {
	var count int64
	err := r.session(ctx).Model(&model.Post{}).Count(&count).Error
	return count, err
}

func (r *recommendationRepository) CountEmbeddings(ctx context.Context) (text, image int64, err error) {
	err = r.session(ctx).Model(&model.PostEmbedding{}).Where("kind = ?", "text").Count(&text).Error
	if err != nil {
		return 0, 0, err
	}
	err = r.session(ctx).Model(&model.PostEmbedding{}).Where("kind = ?", "image").Count(&image).Error
	return text, image, err
}

func (r *recommendationRepository) CountPending(ctx context.Context) (text, image int64, err error) {
	err = r.session(ctx).Model(&model.Post{}).
		Joins("LEFT JOIN post_embeddings e ON e.post_id = posts.postID AND e.kind = ?", "text").
		Where("e.id IS NULL AND posts.is_private = ?", false).
		Count(&text).Error
	if err != nil {
		return 0, 0, err
	}
	err = r.session(ctx).Model(&model.Post{}).
		Joins("LEFT JOIN post_embeddings e ON e.post_id = posts.postID AND e.kind = ?", "image").
		Where("e.id IS NULL AND posts.is_private = ?", false).
		Where("(photos IS NOT NULL AND photos <> '') OR ptext LIKE ?", "%![%").
		Count(&image).Error
	return text, image, err
}

func (r *recommendationRepository) ListUserBrowses(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.Pbrowse
	err := r.session(ctx).Where("userID = ?", userID).Order("time DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.PtargetID, At: row.Time, Weight: 1})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserLikes(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.Plike
	err := r.session(ctx).Where("userID = ?", userID).Order("time DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.PtargetID, At: row.Time, Weight: 1})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserComments(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.Pcomment
	err := r.session(ctx).Where("userID = ?", userID).Order("time DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.PtargetID, At: row.Time, Weight: 1, Text: row.Pctext})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserSaves(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.Psave
	// psaves 没有时间列，用 psaveID 倒序近似「最近收藏」；时间衰减在 service 里按中性处理。
	err := r.session(ctx).Where("userID = ?", userID).Order("psaveID DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.PtargetID, Weight: 1})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserCoinThrows(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.CoinThrow
	err := r.session(ctx).Where("user_id = ? AND target_type = ?", userID, domain.CoinTargetPost).
		Order("created_at DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.TargetID, At: row.CreatedAt, Weight: 1})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserAuthoredPosts(ctx context.Context, userID, limit int) ([]domain.RecInteraction, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.Post
	// 自己发的公开帖同样代表稳定兴趣；私密帖没有向量，索引里也拿不到正文，跳过。
	err := r.session(ctx).Model(&model.Post{}).
		Select("postID, post_time").
		Where("userID = ? AND is_private = ?", userID, false).
		Order("post_time DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecInteraction, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecInteraction{PostID: row.PostID, At: row.PostTime})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserChatMessages(ctx context.Context, userID, botUID, limit int) ([]domain.RecChatMessage, error) {
	if limit <= 0 || botUID <= 0 {
		return nil, nil
	}
	var rows []model.ChatMsg
	// 只取用户主动发给小秋的内容，不取机器人回复：机器人措辞偏模板化，容易稀释兴趣。
	err := r.session(ctx).Model(&model.ChatMsg{}).
		Where("senderUserID = ? AND targetUserID = ? AND isRecalled = ?", userID, botUID, false).
		Where("content IS NOT NULL AND content <> ''").
		Order("chatMsgID DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecChatMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecChatMessage{Content: row.Content, At: row.CreatedAt})
	}
	return out, nil
}

func (r *recommendationRepository) ListUserReadEvents(ctx context.Context, userID, limit int) ([]domain.RecReadEvent, error) {
	if limit <= 0 {
		return nil, nil
	}
	var rows []model.PostReadEvent
	err := r.session(ctx).Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecReadEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecReadEvent{PostID: row.PostID, DwellMS: row.DwellMS, At: row.CreatedAt})
	}
	return out, nil
}

func (r *recommendationRepository) BrowsedPostIDs(ctx context.Context, userID, limit int) (map[int]struct{}, error) {
	if limit <= 0 {
		return map[int]struct{}{}, nil
	}
	var ids []int
	err := r.session(ctx).Model(&model.Pbrowse{}).
		Where("userID = ?", userID).
		Order("time DESC").Limit(limit).
		Pluck("ptargetID", &ids).Error
	if err != nil {
		return nil, err
	}
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

func (r *recommendationRepository) GetUserRegisteredAt(ctx context.Context, userID int) (*time.Time, error) {
	var rows []model.User
	res := r.session(ctx).Select("userID, registered_at").Where("userID = ?", userID).Limit(1).Find(&rows)
	if res.Error != nil {
		return nil, res.Error
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0].RegisteredAt, nil
}

func (r *recommendationRepository) CreateReadEvent(ctx context.Context, userID, postID, dwellMS int) error {
	now := time.Now()
	row := model.PostReadEvent{
		UserID:    userID,
		PostID:    postID,
		DwellMS:   dwellMS,
		CreatedAt: now,
		UpdatedAt: now,
	}
	// 同一用户同一帖子只保留一行：点进详情先标记已读(dwell=0)，
	// 之后计时器上报停留时长时取较大值，重复调用幂等。
	return r.session(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "post_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"dwell_ms":   gorm.Expr("GREATEST(dwell_ms, ?)", dwellMS),
			"updated_at": now,
		}),
	}).Create(&row).Error
}

func grayRuleToDomain(row model.RecommendationGrayRule) *domain.RecommendationGrayRule {
	return &domain.RecommendationGrayRule{
		ID:        row.ID,
		UIDMin:    row.UIDMin,
		UIDMax:    row.UIDMax,
		Enabled:   row.Enabled,
		Note:      row.Note,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func modelPostsToContents(rows []model.Post) []domain.RecPostContent {
	out := make([]domain.RecPostContent, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.RecPostContent{
			PostID:    row.PostID,
			Title:     row.Title,
			Ptext:     row.Ptext,
			Tag:       row.Tag,
			Partition: row.Partition,
			Photos:    row.Photos,
		})
	}
	return out
}

func encodeFloat32Vector(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeFloat32Vector(buf []byte) []float32 {
	if len(buf) == 0 || len(buf)%4 != 0 {
		return nil
	}
	out := make([]float32, len(buf)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return out
}
