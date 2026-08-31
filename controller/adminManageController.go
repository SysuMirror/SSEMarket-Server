package controller

import (
	"loginTest/common"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/response"
	"loginTest/service"
	"loginTest/util"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type adminBrowseReq struct {
	Page      int    `json:"page"`
	Size      int    `json:"size"`
	Keyword   string `json:"keyword"`
	Partition string `json:"partition"`
	UserID    int    `json:"user_id"`
	PostID    int    `json:"post_id"`
}

type adminPostIDReq struct {
	PostID int `json:"PostID"`
	UserID int `json:"UserID"`
	Days   int `json:"Days"`
}

type adminKeyReq struct {
	Key  string `json:"Key"`
	Used *bool  `json:"Used"`
	Num  int    `json:"Num"`
}

type adminMuteReq struct {
	UserID  int    `json:"UserID"`
	Name    string `json:"Name"`
	Days    int    `json:"Days"`
	Reason  string `json:"Reason"`
	Release bool   `json:"Release"`
}

// AdminBrowsePosts 管理端帖子列表（分页+筛选）
func AdminBrowsePosts(c *gin.Context) {
	var req adminBrowseReq
	_ = c.ShouldBindJSON(&req)
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Size <= 0 || req.Size > 100 {
		req.Size = 20
	}
	db := common.GetDB().Model(&model.Post{})
	if req.PostID > 0 {
		db = db.Where("postID = ?", req.PostID)
	}
	if req.UserID > 0 {
		db = db.Where("userID = ?", req.UserID)
	}
	if kw := strings.TrimSpace(req.Keyword); kw != "" {
		like := "%" + kw + "%"
		db = db.Where("title LIKE ? OR ptext LIKE ?", like, like)
	}
	if p := strings.TrimSpace(req.Partition); p != "" {
		db = db.Where("`partition` = ?", p)
	}
	var total int64
	db.Count(&total)
	var posts []model.Post
	if err := db.Order("postID desc").Offset((req.Page - 1) * req.Size).Limit(req.Size).Find(&posts).Error; err != nil {
		response.Fail(c, nil, "读取帖子失败")
		return
	}
	out := make([]gin.H, 0, len(posts))
	for _, p := range posts {
		out = append(out, gin.H{
			"postID":       p.PostID,
			"userID":       p.UserID,
			"title":        p.Title,
			"partition":    p.Partition,
			"comment_num":  p.CommentNum,
			"like_num":     p.LikeNum,
			"browse_num":   p.BrowseNum,
			"heat":         p.Heat,
			"post_time":    p.PostTime,
			"tag":          p.Tag,
			"is_private":   p.IsPrivate,
			"is_anonymous": p.IsAnonymous,
			"is_pinned":    p.IsPinned,
			"excerpt":      util.TruncateRunes(p.Ptext, 80),
		})
	}
	response.Success(c, gin.H{"posts": out, "total": total, "page": req.Page, "size": req.Size}, "ok")
}

func AdminBrowsePostNum(c *gin.Context) {
	var total int64
	common.GetDB().Model(&model.Post{}).Count(&total)
	response.Success(c, gin.H{"num": total}, "ok")
}

func AdminGetPostDetail(c *gin.Context) {
	var req adminPostIDReq
	_ = c.ShouldBindJSON(&req)
	if req.PostID == 0 {
		response.Fail(c, nil, "缺少 PostID")
		return
	}
	var post model.Post
	if err := common.GetDB().Where("postID = ?", req.PostID).First(&post).Error; err != nil {
		response.Fail(c, nil, "帖子不存在")
		return
	}
	var user model.User
	_ = common.GetDB().Where("userID = ?", post.UserID).First(&user).Error
	response.Success(c, gin.H{
		"post": gin.H{
			"postID": post.PostID, "userID": post.UserID, "title": post.Title,
			"content": post.Ptext, "partition": post.Partition, "tag": post.Tag,
			"photos": post.Photos, "heat": post.Heat, "like_num": post.LikeNum,
			"comment_num": post.CommentNum, "browse_num": post.BrowseNum,
			"post_time": post.PostTime, "is_pinned": post.IsPinned,
			"is_private": post.IsPrivate, "is_anonymous": post.IsAnonymous,
		},
		"author": gin.H{"userID": user.UserID, "name": user.Name, "email": user.Email, "phone": user.Phone},
	}, "ok")
}

func AdminDeletePostByID(c *gin.Context) {
	var req adminPostIDReq
	_ = c.ShouldBindJSON(&req)
	if req.PostID == 0 {
		response.Fail(c, nil, "缺少 PostID")
		return
	}
	res := common.GetDB().Where("postID = ?", req.PostID).Delete(&model.Post{})
	if res.Error != nil || res.RowsAffected == 0 {
		response.Fail(c, nil, "删除失败或帖子不存在")
		return
	}
	response.Success(c, gin.H{"PostID": req.PostID}, "已删除")
}

func AdminPinPost(c *gin.Context) {
	var req adminPostIDReq
	_ = c.ShouldBindJSON(&req)
	if req.PostID == 0 {
		response.Fail(c, nil, "缺少 PostID")
		return
	}
	globalCount, err := service.Posts.CountActivePinned(c.Request.Context())
	if err != nil {
		response.Fail(c, nil, "置顶失败")
		return
	}
	if globalCount >= int64(domain.MaxPinnedQueueSize) {
		response.Fail(c, nil, domain.ErrGlobalPinnedLimitReached.Error())
		return
	}
	now := time.Now()
	// 旧逻辑未写 pinned_expires_at,ListPinned 要求 expires > NOW(),导致管理员置顶永远不进置顶区:
	// if err := common.GetDB().Model(&model.Post{}).Where("postID = ?", req.PostID).Updates(map[string]interface{}{
	// 	"is_pinned": true, "pinned_at": now,
	// }).Error; err != nil {
	expiresAt := now.Add(30 * 24 * time.Hour) // 管理员优质置顶默认 30 天
	if err := common.GetDB().Model(&model.Post{}).Where("postID = ?", req.PostID).Updates(map[string]interface{}{
		"is_pinned": true, "pinned_at": now, "pinned_expires_at": expiresAt, // pinned_at=入队时刻,FIFO 排序依据
	}).Error; err != nil {
		response.Fail(c, nil, "置顶失败")
		return
	}
	service.InvalidateHomeBrowse()
	response.Success(c, nil, "已置顶（优质）")
}

func AdminUnpinPost(c *gin.Context) {
	var req adminPostIDReq
	_ = c.ShouldBindJSON(&req)
	if req.PostID == 0 {
		response.Fail(c, nil, "缺少 PostID")
		return
	}
	if err := common.GetDB().Model(&model.Post{}).Where("postID = ?", req.PostID).Updates(map[string]interface{}{
		"is_pinned": false, "pinned_at": nil, "pinned_expires_at": nil,
	}).Error; err != nil {
		response.Fail(c, nil, "取消置顶失败")
		return
	}
	service.InvalidateHomeBrowse()
	response.Success(c, nil, "已取消置顶")
}

func AdminListKeys(c *gin.Context) {
	used := c.Query("used")
	db := common.GetDB().Model(&model.CDKey{})
	if used == "1" || used == "true" {
		db = db.Where("used = ?", true)
	} else if used == "0" || used == "false" {
		db = db.Where("used = ?", false)
	}
	var keys []model.CDKey
	if err := db.Order("cdkeyID desc").Limit(500).Find(&keys).Error; err != nil {
		response.Fail(c, nil, "读取邀请码失败")
		return
	}
	response.Success(c, gin.H{"keys": keys}, "ok")
}

func AdminAddKey(c *gin.Context) {
	var req adminKeyReq
	_ = c.ShouldBindJSON(&req)
	content := strings.TrimSpace(req.Key)
	if content == "" {
		content = util.GenerateRandomString(8)
		if len(content) > 9 {
			content = content[:9]
		}
	}
	if len(content) > 9 {
		content = content[:9]
	}
	key := model.CDKey{
		Content:     content,
		Used:        false,
		CreatedTime: time.Now(),
	}
	if err := common.GetDB().Create(&key).Error; err != nil {
		response.Fail(c, nil, "添加失败，可能重复")
		return
	}
	response.Success(c, gin.H{"key": key}, "已添加")
}

func AdminBatchAddKeys(c *gin.Context) {
	var req adminKeyReq
	_ = c.ShouldBindJSON(&req)
	n := req.Num
	if n <= 0 {
		n = 10
	}
	if n > 100 {
		n = 100
	}
	created := make([]model.CDKey, 0, n)
	for i := 0; i < n; i++ {
		content := util.GenerateRandomString(8)
		if len(content) > 9 {
			content = content[:9]
		}
		key := model.CDKey{Content: content, Used: false, CreatedTime: time.Now()}
		if err := common.GetDB().Create(&key).Error; err != nil {
			continue
		}
		created = append(created, key)
	}
	response.Success(c, gin.H{"keys": created, "count": len(created)}, "批量生成完成")
}

func AdminMuteUser(c *gin.Context) {
	var req adminMuteReq
	_ = c.ShouldBindJSON(&req)
	db := common.GetDB()
	var user model.User
	q := db.Model(&model.User{})
	if req.UserID > 0 {
		q = q.Where("userID = ?", req.UserID)
	} else if strings.TrimSpace(req.Name) != "" {
		q = q.Where("name = ?", strings.TrimSpace(req.Name))
	} else {
		response.Fail(c, nil, "需要 UserID 或 Name")
		return
	}
	if err := q.First(&user).Error; err != nil {
		response.Fail(c, nil, "用户不存在")
		return
	}
	days := req.Days
	if days <= 0 {
		days = 7
	}
	banEnd := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	updates := map[string]interface{}{
		"banend":    banEnd,
		"punishnum": user.Punishnum + 1,
	}
	if err := db.Model(&user).Updates(updates).Error; err != nil {
		response.Fail(c, nil, "禁言失败")
		return
	}
	response.Success(c, gin.H{"userID": user.UserID, "banend": banEnd, "days": days}, "已禁言")
}

func AdminReleaseUser(c *gin.Context) {
	var req adminMuteReq
	_ = c.ShouldBindJSON(&req)
	db := common.GetDB()
	var user model.User
	q := db.Model(&model.User{})
	if req.UserID > 0 {
		q = q.Where("userID = ?", req.UserID)
	} else if strings.TrimSpace(req.Name) != "" {
		q = q.Where("name = ?", strings.TrimSpace(req.Name))
	} else {
		response.Fail(c, nil, "需要 UserID 或 Name")
		return
	}
	if err := q.First(&user).Error; err != nil {
		response.Fail(c, nil, "用户不存在")
		return
	}
	if err := db.Model(&user).Update("banend", time.Now().Add(-time.Hour)).Error; err != nil {
		response.Fail(c, nil, "解禁失败")
		return
	}
	response.Success(c, gin.H{"userID": user.UserID}, "已解禁")
}

func AdminListMutedUsers(c *gin.Context) {
	var users []model.User
	if err := common.GetDB().Where("banend > ?", time.Now()).Order("banend desc").Limit(200).Find(&users).Error; err != nil {
		response.Fail(c, nil, "读取失败")
		return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"userID": u.UserID, "name": u.Name, "email": u.Email,
			"phone": u.Phone, "banend": u.Banend, "punishnum": u.Punishnum,
		})
	}
	response.Success(c, gin.H{"users": out}, "ok")
}

func AdminListFeedback(c *gin.Context) {
	var list []model.Feedback
	if err := common.GetDB().Order("feedbackID desc").Limit(200).Find(&list).Error; err != nil {
		response.Fail(c, nil, "读取反馈失败")
		return
	}
	response.Success(c, gin.H{"feedbacks": list}, "ok")
}

func AdminMarkFeedback(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	status := c.Query("status")
	if status != "ok" && status != "wait" {
		status = "ok"
	}
	if err := common.GetDB().Model(&model.Feedback{}).Where("feedbackID = ?", id).Update("status", status).Error; err != nil {
		response.Fail(c, nil, "更新失败")
		return
	}
	response.Success(c, nil, "已更新")
}

func AdminDashboard(c *gin.Context) {
	db := common.GetDB()
	var users, posts, keysUnused, feedbackWait, sues int64
	db.Model(&model.User{}).Count(&users)
	db.Model(&model.Post{}).Count(&posts)
	db.Model(&model.CDKey{}).Where("used = ?", false).Count(&keysUnused)
	db.Model(&model.Feedback{}).Where("status = ?", "wait").Count(&feedbackWait)
	db.Model(&model.Sue{}).Where("finish = ?", false).Count(&sues)
	response.Success(c, gin.H{
		"users": users, "posts": posts, "keys_unused": keysUnused,
		"feedback_wait": feedbackWait, "sues": sues,
	}, "ok")
}
