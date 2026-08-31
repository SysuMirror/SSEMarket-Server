package gormrepo

import (
	"context"
	"errors"
	"math"
	"time"

	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type sueRepository struct {
	db *gorm.DB
}

func NewSueRepository(db *gorm.DB) repository.SueRepository {
	return &sueRepository{db: db}
}

func (r *sueRepository) session(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

func (r *sueRepository) ListPending(ctx context.Context) ([]*domain.Sue, error) {
	var list []model.Sue
	if err := r.session(ctx).Where("finish = ?", false).Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.Sue, 0, len(list))
	for i := range list {
		out = append(out, sueToDomain(&list[i]))
	}
	return out, nil
}

func (r *sueRepository) FindByID(ctx context.Context, id int) (*domain.Sue, error) {
	var m model.Sue
	err := r.session(ctx).Where("sueID = ?", id).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return sueToDomain(&m), nil
}

func (r *sueRepository) MarkFinishedByID(ctx context.Context, id int, status string) error {
	return r.session(ctx).Model(&model.Sue{}).Where("sueID = ?", id).
		Updates(map[string]interface{}{"status": status, "finish": true}).Error
}

func (r *sueRepository) MarkFinishedByTarget(ctx context.Context, targetType string, targetID int, status string) error {
	return r.session(ctx).Model(&model.Sue{}).
		Where("targettype = ? AND targetID = ? AND finish = 0", targetType, targetID).
		Updates(map[string]interface{}{"status": status, "finish": true}).Error
}

func (r *sueRepository) LoadTargetSummary(ctx context.Context, targetType string, targetID int) (title, detail string, ok bool) {
	switch targetType {
	case "post":
		var post model.Post
		if r.session(ctx).Where("postID = ?", targetID).First(&post).Error != nil || post.PostID == 0 {
			return "", "", false
		}
		return post.Title, post.Ptext, true
	case "pcomment":
		var pc model.Pcomment
		if r.session(ctx).Where("pcommentID = ?", targetID).First(&pc).Error != nil || pc.PcommentID == 0 {
			return "", "", false
		}
		return "", pc.Pctext, true
	case "ccomment":
		var cc model.Ccomment
		if r.session(ctx).Where("ccommentID = ?", targetID).First(&cc).Error != nil || cc.CcommentID == 0 {
			return "", "", false
		}
		return "", cc.Cctext, true
	default:
		return "", "", false
	}
}

func truncateContent(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func (r *sueRepository) ProcessViolation(ctx context.Context, sue *domain.Sue) (*domain.ViolationResult, error) {
	result := &domain.ViolationResult{
		SueID:      sue.ID,
		ReporterID: sue.UserID,
	}
	switch sue.TargetType {
	case "post":
		result.SueType = "帖子"
		var post model.Post
		if err := r.session(ctx).Where("postID = ?", sue.TargetID).First(&post).Error; err != nil || post.PostID == 0 {
			return nil, domain.ErrNotFound
		}
		result.Content = truncateContent(post.Ptext, 30)
		var user model.User
		if err := r.session(ctx).Where("userID = ?", post.UserID).First(&user).Error; err != nil || user.UserID == 0 {
			return nil, domain.ErrNotFound
		}
		result.TargetUserID = user.UserID
		if err := r.session(ctx).Delete(&post).Error; err != nil {
			return nil, err
		}
	case "pcomment":
		result.SueType = "评论"
		var pc model.Pcomment
		if err := r.session(ctx).Where("pcommentID = ?", sue.TargetID).First(&pc).Error; err != nil || pc.PcommentID == 0 {
			return nil, domain.ErrNotFound
		}
		result.Content = truncateContent(pc.Pctext, 30)
		var targetUser model.User
		if err := r.session(ctx).Where("userID = ?", pc.UserID).First(&targetUser).Error; err != nil || targetUser.UserID == 0 {
			return nil, domain.ErrNotFound
		}
		result.TargetUserID = targetUser.UserID

		var post model.Post
		if err := r.session(ctx).Where("postID = ?", pc.PtargetID).First(&post).Error; err != nil || post.PostID == 0 {
			return nil, domain.ErrNotFound
		}
		var count int64
		r.session(ctx).Model(&model.Ccomment{}).Where("ctargetID = ?", pc.PcommentID).Count(&count)
		r.session(ctx).Model(&post).UpdateColumn("comment_num", gorm.Expr("comment_num - ?", count+1))

		currentTime := time.Now()
		days := int(currentTime.Sub(pc.Time).Hours() / 24)
		weightComment := float64(6)
		if days > 0 {
			deleteHeat := math.Pow(weightComment, math.Pow(0.5, float64(days)))
			r.session(ctx).Model(&post).Update("heat", post.Heat-(deleteHeat+float64(count)))
		} else {
			deleteCcommentHeat := float64(count * int64(weightComment))
			r.session(ctx).Model(&post).Update("heat", post.Heat-(weightComment+deleteCcommentHeat))
		}
		common.InvalidatePostDetailCache(post.PostID)
		if err := r.session(ctx).Delete(&pc).Error; err != nil {
			return nil, err
		}
	case "ccomment":
		result.SueType = "评论"
		var cc model.Ccomment
		if err := r.session(ctx).Where("ccommentID = ?", sue.TargetID).First(&cc).Error; err != nil || cc.CcommentID == 0 {
			return nil, domain.ErrNotFound
		}
		result.Content = truncateContent(cc.Cctext, 30)

		var targetComment model.Pcomment
		if err := r.session(ctx).Where("pcommentID = ?", cc.CtargetID).First(&targetComment).Error; err != nil {
			return nil, domain.ErrNotFound
		}
		var post model.Post
		if err := r.session(ctx).Where("postID = ?", targetComment.PtargetID).First(&post).Error; err != nil || post.PostID == 0 {
			return nil, domain.ErrNotFound
		}
		currentTime := time.Now()
		days := int(currentTime.Sub(cc.Time).Hours() / 24)
		weightComment := float64(6)
		if days > 0 {
			deleteHeat := math.Pow(weightComment, math.Pow(0.5, float64(days)))
			r.session(ctx).Model(&post).Update("heat", post.Heat-deleteHeat)
		} else {
			r.session(ctx).Model(&post).Update("heat", post.Heat-weightComment)
		}
		var targetUser model.User
		if err := r.session(ctx).Where("userID = ?", cc.UserID).First(&targetUser).Error; err != nil || targetUser.UserID == 0 {
			return nil, domain.ErrNotFound
		}
		result.TargetUserID = targetUser.UserID
		r.session(ctx).Model(&post).UpdateColumn("comment_num", gorm.Expr("comment_num - 1"))
		common.InvalidatePostDetailCache(post.PostID)
		if err := r.session(ctx).Delete(&cc).Error; err != nil {
			return nil, err
		}
	default:
		return nil, domain.ErrBadRequest
	}
	return result, nil
}

func (r *sueRepository) CreateNotice(ctx context.Context, receiver int, noticeType, text string, target int) error {
	n := model.Notice{
		Receiver: receiver,
		Type:     noticeType,
		Ntext:    text,
		Time:     time.Now(),
		Read:     false,
		Target:   target,
	}
	return r.session(ctx).Create(&n).Error
}

func (r *sueRepository) Create(ctx context.Context, sue *domain.Sue) error {
	m := model.Sue{
		Targettype: sue.TargetType,
		TargetID:   sue.TargetID,
		UserID:     sue.UserID,
		Reason:     sue.Reason,
		Time:       sue.Time,
		Status:     sue.Status,
		Finish:     sue.Finish,
	}
	return r.session(ctx).Create(&m).Error
}

func (r *sueRepository) PunishUser(ctx context.Context, userID int) (*domain.User, error) {
	var u model.User
	if err := r.session(ctx).Where("userID = ?", userID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	u.Punishnum++
	u.Banend = time.Now().AddDate(0, 0, u.Punishnum)
	if err := r.session(ctx).Model(&u).Updates(map[string]interface{}{
		"punishnum": u.Punishnum,
		"banend":    u.Banend,
	}).Error; err != nil {
		return nil, err
	}
	return userToDomain(&u), nil
}

