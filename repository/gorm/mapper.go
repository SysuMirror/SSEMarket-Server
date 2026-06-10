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
		Tag:        p.Tag,
		IsPrivate:  p.IsPrivate,
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
		BanEnd:                     u.Banend,
		PunishNum:                  u.Punishnum,
		HideProfilePosts:           u.HideProfilePosts,
		ISEmailNotificationBlocked: u.ISEmailNotificationBlocked,
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
		Banend:                     u.BanEnd,
		Punishnum:                  u.PunishNum,
		HideProfilePosts:           u.HideProfilePosts,
		ISEmailNotificationBlocked: u.ISEmailNotificationBlocked,
	}
}
