package gormrepo

import (
	"loginTest/domain"
	"loginTest/model"
)

func postToDomain(p *model.Post) *domain.Post {
	if p == nil {
		return nil
	}
	return &domain.Post{
		ID:         p.PostID,
		UserID:     p.UserID,
		Partition:  p.Partition,
		Title:      p.Title,
		Content:    p.Ptext,
		CommentNum: p.CommentNum,
		LikeNum:    p.LikeNum,
		VoteNum:    p.VoteNum,
		BrowseNum:  p.BrowseNum,
		PostTime:   p.PostTime,
		Heat:       p.Heat,
		Photos:     p.Photos,
		Tag:         p.Tag,
		PostKind:    p.PostKind,
		QuotePrice:  p.QuotePrice,
		QuestStatus: p.QuestStatus,
		IsPrivate:   p.IsPrivate,
		IsAnonymous: p.IsAnonymous,
		IsPinned:        p.IsPinned,
		PinnedAt:        p.PinnedAt,
		PinnedExpiresAt: p.PinnedExpiresAt,
	}
}

func userToDomain(u *model.User) *domain.User {
	if u == nil {
		return nil
	}
	return &domain.User{
		ID:                         u.UserID,
		Phone:                      u.Phone,
		Email:                      u.Email,
		Name:                       u.Name,
		Intro:                      u.Intro,
		AvatarURL:                  u.AvatarURL,
		Identity:                   u.Identity,
		Score:                      u.Score,
		Experience:                 u.Experience,
		NovelPoints:                u.NovelPoints,
		Coins:                      u.Coins,
		BanEnd:                     u.Banend,
		PunishNum:                  u.Punishnum,
		HideProfilePosts:           u.HideProfilePosts,
		ISEmailNotificationBlocked: u.ISEmailNotificationBlocked,
		UserType:                   u.UserType,
	}
}

func userToModel(u *domain.User) model.User {
	if u == nil {
		return model.User{}
	}
	return model.User{
		UserID:                     u.ID,
		Phone:                      u.Phone,
		Email:                      u.Email,
		Name:                       u.Name,
		Intro:                      u.Intro,
		AvatarURL:                  u.AvatarURL,
		Identity:                   u.Identity,
		Score:                      u.Score,
		// Save 是整行写入，成长值与币余额必须带上，否则改一次资料就把它们清零。
		Experience:                 u.Experience,
		NovelPoints:                u.NovelPoints,
		Coins:                      u.Coins,
		Banend:                     u.BanEnd,
		Punishnum:                  u.PunishNum,
		HideProfilePosts:           u.HideProfilePosts,
		ISEmailNotificationBlocked: u.ISEmailNotificationBlocked,
		UserType:                   u.UserType,
	}
}

func ratingCategoryToDomain(c *model.RatingCategory) *domain.RatingCategory {
	if c == nil {
		return nil
	}
	return &domain.RatingCategory{
		ID:          c.CategoryID,
		Name:        c.Name,
		Description: c.Description,
		CreatorID:   c.CreatorID,
		PostCount:   c.PostCount,
		IsOfficial:  c.IsOfficial,
		SortOrder:   c.SortOrder,
		CreatedAt:   c.CreatedAt,
	}
}

func ratingPostToDomain(p *model.RatingPost) *domain.RatingPost {
	if p == nil {
		return nil
	}
	return &domain.RatingPost{
		ID:         p.RatingPostID,
		CategoryID: p.CategoryID,
		UserID:     p.UserID,
		Title:      p.Title,
		Content:    p.Content,
		CommentNum: p.CommentNum,
		LikeNum:    p.LikeNum,
		BrowseNum:  p.BrowseNum,
		Photos:     p.Photos,
		CreatedAt:  p.CreatedAt,
	}
}

func ratingCommentToDomain(c *model.RatingComment) *domain.RatingComment {
	if c == nil {
		return nil
	}
	return &domain.RatingComment{
		ID:           c.CommentID,
		RatingPostID: c.RatingPostID,
		UserID:       c.UserID,
		Content:      c.Content,
		StarNum:      c.StarNum,
		LikeNum:      c.LikeNum,
		CreatedAt:    c.CreatedAt,
	}
}

func productToDomain(p *model.Product) *domain.Product {
	if p == nil {
		return nil
	}
	return &domain.Product{
		ID:          p.ProductID,
		UserID:      p.UserID,
		Name:        p.Name,
		Description: p.Description,
		PostTime:    p.PostTime,
		Photos:      p.Photos,
		IsSold:      p.ISSold,
		Price:       p.Price,
		IsAnonymous: p.ISAnonymous,
		IsDelete:    p.ISDelete,
	}
}

func resumeToDomain(r *model.Resume) *domain.Resume {
	if r == nil {
		return nil
	}
	return &domain.Resume{
		ID:          r.ResumeID,
		UserID:      r.UserID,
		Title:       r.Title,
		Description: r.Description,
		FileName:    r.FileName,
		CosKey:      r.CosKey,
		FileSize:    r.FileSize,
		MimeType:    r.MimeType,
		DownloadNum: r.DownloadNum,
		ViewNum:     r.ViewNum,
		IsAnonymous: r.IsAnonymous,
		IsDelete:    r.IsDelete,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func adminToDomain(a *model.Admin) *domain.Admin {
	if a == nil {
		return nil
	}
	return &domain.Admin{
		ID:       a.AdminID,
		Account:  a.Account,
		Password: a.Password,
	}
}

func tagToDomain(t *model.Tag) *domain.Tag {
	if t == nil {
		return nil
	}
	return &domain.Tag{
		ID:    t.TagID,
		Name:  t.Name,
		Value: t.Value,
		Type:  t.Type,
		Num:   t.Num,
	}
}

func feedbackToDomain(f *model.Feedback) *domain.Feedback {
	if f == nil {
		return nil
	}
	return &domain.Feedback{
		ID:         f.FeedbackID,
		Text:       f.Ftext,
		Attachment: f.Attachment,
		Time:       f.Time,
		Status:     f.Status,
	}
}

func postToModel(p *domain.Post) model.Post {
	if p == nil {
		return model.Post{}
	}
	return model.Post{
		PostID:     p.ID,
		UserID:     p.UserID,
		Partition:  p.Partition,
		Title:      p.Title,
		Ptext:      p.Content,
		LikeNum:    p.LikeNum,
		CommentNum: p.CommentNum,
		BrowseNum:  p.BrowseNum,
		Heat:       p.Heat,
		PostTime:   p.PostTime,
		Photos:     p.Photos,
		Tag:        p.Tag,
		IsPrivate:  p.IsPrivate,
		IsAnonymous: p.IsAnonymous,
	}
}

func pcommentToDomain(p *model.Pcomment) *domain.Pcomment {
	if p == nil {
		return nil
	}
	return &domain.Pcomment{
		ID:        p.PcommentID,
		PostID:    p.PtargetID,
		UserID:    p.UserID,
		Content:   p.Pctext,
		LikeNum:   p.LikeNum,
		DenyNum:   p.DenyNum,
		CreatedAt: p.Time,
	}
}

func ccommentToDomain(c *model.Ccomment) *domain.Ccomment {
	if c == nil {
		return nil
	}
	return &domain.Ccomment{
		ID:             c.CcommentID,
		PcommentID:     c.CtargetID,
		UserID:         c.UserID,
		Content:        c.Cctext,
		LikeNum:        c.LikeNum,
		DenyNum:        c.DenyNum,
		UserTargetName: c.UserTargetName,
		CreatedAt:      c.Time,
	}
}

func chatMsgToDomain(m *model.ChatMsg) *domain.ChatHistoryItem {
	if m == nil {
		return nil
	}
	return &domain.ChatHistoryItem{
		ChatMsgID:       m.ChatMsgID,
		TargetUserID:    m.TargetUserID,
		SenderUserID:    m.SenderUserID,
		Content:         m.Content,
		IsAnonymous:     m.IsAnonymous,
		ThreadAnonymous: m.ThreadAnonymous,
		IsRecalled:      m.IsRecalled,
		CreatedAt:       m.CreatedAt,
	}
}

func sueToDomain(s *model.Sue) *domain.Sue {
	if s == nil {
		return nil
	}
	return &domain.Sue{
		ID:         s.SueID,
		TargetType: s.Targettype,
		TargetID:   s.TargetID,
		UserID:     s.UserID,
		Reason:     s.Reason,
		Time:       s.Time,
		Status:     s.Status,
		Finish:     s.Finish,
	}
}
