# 分层优化：消除 model↔domain 循环转换 & service 层直接 DB 访问

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 controller→model 循环引用，让 middleware 注入 domain.User；将 service 层直接 DB 操作迁移到 repository 层。

**Architecture:** middleware/strict_auth 注入 domain.User → controller 全部使用 domain.User 类型断言 → service 不再暴露 DomainUserToModel → rank_stats/hot_data 的 DB 操作迁入 repository 接口。

**Tech Stack:** Go, Gin, GORM

---

## 变更文件清单

| 文件 | 操作 | 职责 |
|------|------|------|
| `middleware/strict_auth.go` | 修改 | loadUserFromToken 返回 domain.User，c.Set("user", domainUser) |
| `middleware/user_session.go` | 修改 | LoadUserSession 返回 domain.User，StoreUserSession 接受 domain.User |
| `service/session_cache.go` | 修改 | LoadUserSession 返回 domain.User，StoreUserSession 接受 domain.User，删除 DomainUserToModel |
| `controller/userController.go` | 修改 | 所有 model.User 类型断言改为 domain.User |
| `controller/postController.go` | 修改 | viewerFromAuth 返回 domain.User，model.User 断言改为 domain.User |
| `controller/commentController.go` | 修改 | viewerFromAuth 已返回 domain.User（无需改动，确认） |
| `controller/rating_controller.go` | 修改 | requireLoginUser 返回 domain.User，ratingCommentCreateMsg 参数改为 domain.User |
| `controller/resumeController.go` | 修改 | requireLoginUser 返回 domain.User，viewerIsOwnerDomain/shouldMaskAnonymousDomain/resumeToResponse 参数改为 domain.User |
| `controller/chat.go` | 修改 | model.User 断言改为 domain.User |
| `controller/fileController.go` | 修改 | 7 处 model.User 断言改为 domain.User |
| `controller/noticeController.go` | 修改 | model.User/model.Admin 断言改为 domain.User/domain.Admin |
| `controller/managerController.go` | 修改 | model.Admin 断言改为 domain.Admin，model.Admin 构造改为 domain.Admin |
| `controller/voteController.go` | 修改 | 无 model 引用（确认无需改动） |
| `controller/shopController.go` | 修改 | 无 model 引用（确认无需改动） |
| `controller/sueController.go` | 修改 | 无 model 引用（确认无需改动） |
| `dto/user_dto.go` | 修改 | ToUserDto 参数改为 domain.User |
| `dto/admin_dto.go` | 修改 | ToAdminDto 参数改为 domain.Admin |
| `middleware/oauthMiddleware.go` | 修改 | c.Set("oauth2_user", domainUser)，删除 DomainUserToModel 调用 |
| `repository/rank.go` | 新建 | 排行统计 repository 接口 |
| `repository/gorm/rank.go` | 新建 | 排行统计 GORM 实现 |
| `repository/hot_data.go` | 新建 | 热度数据 repository 接口 |
| `repository/gorm/hot_data.go` | 新建 | 热度数据 GORM 实现 |
| `service/rank_stats_service.go` | 修改 | 删除直接 DB 访问，改用 repository |
| `service/domain_services.go` | 修改 | 删除 OnPostDeleted 直接 DB 操作，改用 repository |
| `service/hot_data_service.go` | 修改 | 删除直接 DB 访问，改用 repository |
| `service/registry.go` | 修改 | 注入新 repository |

---

### Task 1: middleware 层注入 domain.User

**Files:**
- Modify: `middleware/strict_auth.go`
- Modify: `middleware/user_session.go`
- Modify: `service/session_cache.go`

- [ ] **Step 1: 修改 service/session_cache.go**

将 `LoadUserSession` 返回值从 `model.User` 改为 `domain.User`，`StoreUserSession` 参数从 `model.User` 改为 `domain.User`，删除 `DomainUserToModel` 函数。

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"loginTest/core"
	"loginTest/domain"
	"loginTest/util"
)

const userSessionTTL = 5 * time.Minute

func userSessionKey(userID int) string {
	return util.PrefixUserSession + strconv.Itoa(userID)
}

// LoadUserSession 从 Redis 读取用户快照，miss 时回源 DB 并回填（TTL 5min）。
func LoadUserSession(ctx context.Context, userID int) (*domain.User, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if core.MyRedis != nil {
		data, err := core.MyRedis.Get(ctx, userSessionKey(userID)).Bytes()
		if err == nil {
			var user domain.User
			if json.Unmarshal(data, &user) == nil && user.ID != 0 {
				return &user, nil
			}
		}
	}
	if Users == nil {
		return nil, fmt.Errorf("user service unavailable")
	}
	u, err := Users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	_ = StoreUserSession(ctx, u)
	return u, nil
}

// StoreUserSession 写入 Redis 用户快照。
func StoreUserSession(ctx context.Context, user *domain.User) error {
	if core.MyRedis == nil || user.ID == 0 {
		return nil
	}
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return core.MyRedis.Set(ctx, userSessionKey(user.ID), data, userSessionTTL).Err()
}

// InvalidateUserSession 使用户 session 缓存失效。
func InvalidateUserSession(ctx context.Context, userID int) {
	if core.MyRedis == nil || userID == 0 {
		return
	}
	_ = core.MyRedis.Del(ctx, userSessionKey(userID)).Err()
}

// CacheUserAfterAuth 登录/注册成功后预热 session 缓存。
func CacheUserAfterAuth(ctx context.Context, user *domain.User) {
	_ = StoreUserSession(ctx, user)
}
```

- [ ] **Step 2: 修改 middleware/user_session.go**

```go
package middleware

import (
	"context"

	"loginTest/domain"
	"loginTest/service"
)

// LoadUserSession 委托 Service 层 session 缓存。
func LoadUserSession(ctx context.Context, userID int) (*domain.User, error) {
	return service.LoadUserSession(ctx, userID)
}

// StoreUserSession 委托 Service 层 session 缓存。
func StoreUserSession(ctx context.Context, user *domain.User) error {
	return service.StoreUserSession(ctx, user)
}

// InvalidateUserSession 委托 Service 层 session 缓存。
func InvalidateUserSession(ctx context.Context, userID int) {
	service.InvalidateUserSession(ctx, userID)
}

// RefreshUserSession 登录成功后写入 session 缓存。
func RefreshUserSession(ctx context.Context, user *domain.User) {
	service.CacheUserAfterAuth(ctx, user)
}
```

- [ ] **Step 3: 修改 middleware/strict_auth.go**

将 `loadUserFromToken` 返回 `*domain.User`，`c.Set("user", user)` 注入 domain.User。

```go
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"loginTest/common"
	"loginTest/domain"

	"github.com/gin-gonic/gin"
)

func parseBearerToken(c *gin.Context) (token string, isWS bool, ok bool) {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		tokenString = c.Query("token")
		isWS = true
	}
	if tokenString == "" {
		return "", isWS, false
	}
	if !isWS {
		if len(tokenString) <= 7 || !strings.HasPrefix(tokenString, "Bearer ") {
			return "", isWS, false
		}
		return tokenString[7:], isWS, true
	}
	if len(tokenString) <= 7 {
		return "", isWS, false
	}
	return tokenString, isWS, true
}

func loadUserFromToken(c *gin.Context, tokenString string, isWS bool) (*domain.User, bool) {
	token, claims, err := common.ParseToken(tokenString)
	if err != nil || !token.Valid || claims.UserID == 0 {
		if isWS {
			fmt.Println(tokenString)
		}
		return nil, false
	}
	user, err := LoadUserSession(c.Request.Context(), claims.UserID)
	if err != nil || user == nil || user.ID == 0 {
		return nil, false
	}
	return user, true
}

// StrictAuthMiddleware Fail-Closed：/api/auth/* 必须登录。
func StrictAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, isWS, ok := parseBearerToken(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			c.Abort()
			return
		}
		user, valid := loadUserFromToken(c, tokenString, isWS)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "权限不足"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

// OptionalAuthMiddleware 可选登录：/api/noauth/* 游客可访问，有 token 则注入用户。
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, isWS, ok := parseBearerToken(c)
		if !ok {
			c.Set("user", nil)
			c.Next()
			return
		}
		user, valid := loadUserFromToken(c, tokenString, isWS)
		if !valid {
			c.Set("user", nil)
		} else {
			c.Set("user", user)
		}
		c.Next()
	}
}
```

- [ ] **Step 4: 验证编译**

Run: `cd /root/SSE_Market/SSE_market_server && go build ./...`
Expected: 编译错误（controller 层还在用 model.User 断言），这是预期的，将在后续 Task 修复

---

### Task 2: controller 层全部改用 domain.User

**Files:**
- Modify: `controller/userController.go`
- Modify: `controller/postController.go`
- Modify: `controller/rating_controller.go`
- Modify: `controller/resumeController.go`
- Modify: `controller/chat.go`
- Modify: `controller/fileController.go`
- Modify: `controller/noticeController.go`
- Modify: `controller/managerController.go`
- Modify: `dto/user_dto.go`
- Modify: `dto/admin_dto.go`

- [ ] **Step 1: 修改 dto/user_dto.go**

```go
package dto

import "loginTest/domain"

type UserDto struct {
	UserID    int    `json:"userID"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Identity  string `json:"identity"`
}

func ToUserDto(user domain.User) UserDto {
	return UserDto{
		UserID:    user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		AvatarURL: user.AvatarURL,
		Email:     user.Email,
		Identity:  user.Identity,
	}
}
```

- [ ] **Step 2: 修改 dto/admin_dto.go**

```go
package dto

import "loginTest/domain"

type AdminDto struct {
	Account string `json:"account"`
}

func ToAdminDto(admin domain.Admin) AdminDto {
	return AdminDto{
		Account: admin.Account,
	}
}
```

- [ ] **Step 3: 修改 controller/userController.go**

所有 `model.User` 类型断言改为 `*domain.User`，字段引用从 `user.UserID` 改为 `user.ID`，从 `user.Banend` 改为 `user.BanEnd` 等。删除 `model` import。具体变更点：

1. `userInterface.(model.User)` → `userInterface.(*domain.User)`
2. `user.UserID` → `user.ID`
3. `user.Banend` → `user.BanEnd`
4. `user.Punishnum` → `user.PunishNum`
5. `user.ISEmailNotificationBlocked` → `user.ISEmailNotificationBlocked`（不变）
6. `user.HideProfilePosts` → `user.HideProfilePosts`（不变）
7. `service.DomainUserToModel(u)` → 直接用 `u`（删除转换）
8. `dto.ToUserDto(user.(model.User))` → `dto.ToUserDto(*user.(*domain.User))`
9. `var requestUser = model.User{}` → 需要定义一个本地请求结构体（因为 model.User 有 GORM tag）
10. `c.Bind(&requestUser)` 绑定的结构体保持本地定义，不依赖 model

关键函数变更：

**ModifyPassword** — `service.DomainUserToModel(u)` → 直接返回 u 的信息
**AuthorizeOAuth2App** — `user.UserID` → `user.ID`
**GetInfobyOAuth2** — `user.UserID` → `user.ID`，`user.(model.User)` → `user.(*domain.User)`
**GetOAuth2AuthRecords** — 同上
**Info** — `dto.ToUserDto(user.(model.User))` → `dto.ToUserDto(*user.(*domain.User))`
**GetInfo** — `var requestUser = model.User{}` → 改为本地 struct `type getInfoReq struct { Phone string; UserID int }`

- [ ] **Step 4: 修改 controller/postController.go**

1. `viewerFromAuth` 返回 `*domain.User, bool`：
```go
func viewerFromAuth(c *gin.Context) (*domain.User, bool) {
	value, exists := c.Get("user")
	if !exists {
		return nil, false
	}
	user, ok := value.(*domain.User)
	if !ok || user.ID == 0 {
		return nil, false
	}
	return user, true
}
```

2. 所有 `viewer.UserID` 引用不变（domain.User 已经是 .ID，但 viewerFromAuth 之前返回 model.User 用 .UserID，现在改用 .ID）
3. `GetTags` 中 `user := value.(model.User)` → `user := value.(*domain.User)`，`user.UserID` → `user.ID`
4. `GetAllFeedback` 中 `model.Feedback{...}` 构造改为 `domain.Feedback{...}` 或直接用 service 返回的 domain.Feedback
5. 删除 `model` import（如果不再需要）

- [ ] **Step 5: 修改 controller/rating_controller.go**

1. `requireLoginUser` 返回 `*domain.User, bool`：
```go
func requireLoginUser(c *gin.Context) (*domain.User, bool) {
	userID := GetTokenUserID(c)
	if userID == 0 {
		response.Response(c, http.StatusUnauthorized, 401, nil, "请先登录")
		return nil, false
	}
	u, err := service.Users.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.Response(c, http.StatusUnauthorized, 401, nil, "用户不存在")
		return nil, false
	}
	return u, true
}
```

2. 所有 `user.UserID` → `user.ID`
3. `ratingCommentCreateMsg(msg RatingCommentCreateMsg, user model.User)` → `ratingCommentCreateMsg(msg RatingCommentCreateMsg, user *domain.User)`，`user.Banend` → `user.BanEnd`
4. 删除 `model` import

- [ ] **Step 6: 修改 controller/resumeController.go**

1. `requireLoginUser` 同上改为返回 `*domain.User`
2. `viewerIsOwnerDomain(resume *domain.Resume, viewer model.User)` → `viewer *domain.User`，`viewer.UserID` → `viewer.ID`
3. `shouldMaskAnonymousDomain(resume *domain.Resume, viewer model.User)` → `viewer *domain.User`
4. `resumeToResponse(r *domain.Resume, uploader *domain.User, viewer model.User)` → `viewer *domain.User`，`viewer.UserID` → `viewer.ID`
5. 删除 `model` import

- [ ] **Step 7: 修改 controller/chat.go**

1. `currentUser, _ := ctxUser.(model.User)` → `currentUser, _ := ctxUser.(*domain.User)`，`currentUser.UserID` → `currentUser.ID`
2. `user := userInterface.(model.User)` → `user := userInterface.(*domain.User)`，`user.UserID` → `user.ID`
3. `userFromCtx.(model.User)` → `userFromCtx.(*domain.User)`，`user.UserID` → `user.ID`
4. 删除 `model` import

- [ ] **Step 8: 修改 controller/fileController.go**

所有 `value.(model.User)` → `value.(*domain.User)`，`user.UserID` → `user.ID`，`user.Identity` 不变。

删除 `model` import，添加 `domain` import。

- [ ] **Step 9: 修改 controller/noticeController.go**

1. `user = value.(model.User)` → `user = value.(*domain.User)`，`user.UserID` → `user.ID`，`user.Name` → `user.Name`
2. `admin = value.(model.Admin)` → `admin = value.(*domain.Admin)`，`admin.AdminID` → `admin.ID`
3. 变量类型从 `var user model.User` → `var user *domain.User`，`var admin model.Admin` → `var admin *domain.Admin`

- [ ] **Step 10: 修改 controller/managerController.go**

1. `dto.ToAdminDto(admin.(model.Admin))` → `dto.ToAdminDto(*admin.(*domain.Admin))`
2. `model.Admin{AdminID: admin.ID, ...}` → `domain.Admin{ID: admin.ID, Account: admin.Account, Password: admin.Password}`
3. `var user model.Admin` → `var reqAdmin domain.Admin`（用于 Bind），`user.Account` → `reqAdmin.Account`
4. 删除 `model` import，添加 `domain` import

- [ ] **Step 11: 修改 middleware/oauthMiddleware.go**

1. 删除 `service.DomainUserToModel(u)` 调用，直接用 `u`
2. `c.Set("oauth2_user", user)` → `c.Set("oauth2_user", u)`
3. `model.OAuth2App{...}` 构造改为 `domain.OAuth2App{...}`（字段名需对应）
4. 删除 `model` import

- [ ] **Step 12: 验证编译**

Run: `cd /root/SSE_Market/SSE_market_server && go build ./...`
Expected: 编译通过（service 层 DomainUserToModel 已删除，controller 不再引用）

---

### Task 3: 将 rank_stats 的 DB 操作迁入 repository

**Files:**
- Create: `repository/rank.go`
- Create: `repository/gorm/rank.go`
- Modify: `service/rank_stats_service.go`
- Modify: `service/domain_services.go`
- Modify: `service/registry.go`

- [ ] **Step 1: 创建 repository/rank.go 接口**

```go
package repository

import (
	"context"

	"loginTest/domain"
)

// RankRepository 排行统计持久化接口。
type RankRepository interface {
	AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int)
	AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int)
	AdjustPostVoteNum(ctx context.Context, postID, delta int)
	OnPostDeleted(ctx context.Context, post *domain.Post)
	RebuildRankStats(ctx context.Context)
}
```

- [ ] **Step 2: 创建 repository/gorm/rank.go 实现**

将 `service/rank_stats_service.go` 中的纯 DB 操作搬过来，使用 mapper 转换 domain→model。

```go
package gormrepo

import (
	"context"

	"loginTest/domain"
	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type rankRepository struct {
	db *gorm.DB
}

func NewRankRepository(db *gorm.DB) repository.RankRepository {
	return &rankRepository{db: db}
}

func (r *rankRepository) AdjustUserReceivedLike(_ context.Context, authorUserID, delta int) {
	if authorUserID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.User{}).Where("userID = ?", authorUserID).
		UpdateColumn("received_like_num", gorm.Expr("GREATEST(received_like_num + ?, 0)", delta))
}

func (r *rankRepository) AdjustUserReceivedVote(_ context.Context, authorUserID, delta int) {
	if authorUserID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.User{}).Where("userID = ?", authorUserID).
		UpdateColumn("received_vote_num", gorm.Expr("GREATEST(received_vote_num + ?, 0)", delta))
}

func (r *rankRepository) AdjustPostVoteNum(_ context.Context, postID, delta int) {
	if postID == 0 || delta == 0 {
		return
	}
	r.db.Model(&model.Post{}).Where("postID = ?", postID).
		UpdateColumn("vote_num", gorm.Expr("GREATEST(vote_num + ?, 0)", delta))
}

func (r *rankRepository) OnPostDeleted(_ context.Context, post *domain.Post) {
	if post == nil || post.ID == 0 || post.UserID == 0 {
		return
	}
	var voteCount int64
	r.db.Model(&model.Pvote{}).Where("postID = ?", post.ID).Count(&voteCount)
	r.AdjustUserReceivedLike(nil, post.UserID, -post.LikeNum)
	r.AdjustUserReceivedVote(nil, post.UserID, -int(voteCount))
}

func (r *rankRepository) RebuildRankStats(_ context.Context) {
	r.db.Exec(`
		UPDATE posts p
		LEFT JOIN (
			SELECT postID, COUNT(*) AS cnt FROM pvotes GROUP BY postID
		) v ON p.postID = v.postID
		SET p.vote_num = COALESCE(v.cnt, 0)
	`)

	var users []model.User
	r.db.Select("userID").Find(&users)
	for _, u := range users {
		var likeSum int
		r.db.Model(&model.Post{}).Where("userID = ?", u.UserID).
			Select("COALESCE(SUM(like_num), 0)").Row().Scan(&likeSum)

		var voteSum int64
		r.db.Table("pvotes").
			Joins("INNER JOIN posts ON posts.postID = pvotes.postID").
			Where("posts.userID = ?", u.UserID).
			Count(&voteSum)

		r.db.Model(&model.User{}).Where("userID = ?", u.UserID).Updates(map[string]interface{}{
			"received_like_num": likeSum,
			"received_vote_num": int(voteSum),
		})
	}
}
```

- [ ] **Step 3: 简化 service/rank_stats_service.go**

删除所有直接 DB 操作，改为调用 repository。

```go
package service

import (
	"context"

	"loginTest/domain"
	"loginTest/repository"
)

type rankService struct {
	repo repository.RankRepository
}

func NewRankService(repo repository.RankRepository) RankService {
	return &rankService{repo: repo}
}

func (s *rankService) AdjustUserReceivedLike(ctx context.Context, authorUserID, delta int) {
	s.repo.AdjustUserReceivedLike(ctx, authorUserID, delta)
}

func (s *rankService) AdjustUserReceivedVote(ctx context.Context, authorUserID, delta int) {
	s.repo.AdjustUserReceivedVote(ctx, authorUserID, delta)
}

func (s *rankService) AdjustPostVoteNum(ctx context.Context, postID, delta int) {
	s.repo.AdjustPostVoteNum(ctx, postID, delta)
}

func (s *rankService) OnPostDeleted(ctx context.Context, post *domain.Post) {
	s.repo.OnPostDeleted(ctx, post)
}

func (s *rankService) RebuildRankStats(ctx context.Context) {
	s.repo.RebuildRankStats(ctx)
}
```

- [ ] **Step 4: 删除 service/domain_services.go**

整个文件只有 `rankService` 的包装和 `OnPostDeleted`，已被 repository 替代。删除此文件。

- [ ] **Step 5: 更新 service/registry.go**

```go
func InitServices(db *gorm.DB) {
	postRepo := gormrepo.NewPostRepository(db)
	userRepo := gormrepo.NewUserRepository(db)
	commentRepo := gormrepo.NewCommentRepository(db)
	chatRepo := gormrepo.NewChatRepository(db)
	ratingRepo := gormrepo.NewRatingRepository(db)
	shopRepo := gormrepo.NewShopRepository(db)
	resumeRepo := gormrepo.NewResumeRepository(db)
	noticeRepo := gormrepo.NewNoticeRepository(db)
	adminRepo := gormrepo.NewAdminRepository(db)
	sueRepo := gormrepo.NewSueRepository(db)
	voteRepo := gormrepo.NewVoteRepository(db)
	rankRepo := gormrepo.NewRankRepository(db)

	Users = NewUserService(userRepo)
	Ranks = NewRankService(rankRepo)
	Posts = NewPostService(postRepo, userRepo, sueRepo, Ranks)
	Comments = NewCommentService(commentRepo)
	Chats = NewChatService(chatRepo)
	Ratings = NewRatingService(ratingRepo)
	Shops = NewShopService(shopRepo, userRepo)
	Resumes = NewResumeService(resumeRepo)
	Notices = NewNoticeService(noticeRepo)
	Admins = NewAdminService(adminRepo)
	Sues = NewSueService(sueRepo)
	Votes = NewVoteService(voteRepo, postRepo, Ranks)
}
```

- [ ] **Step 6: 验证编译**

Run: `cd /root/SSE_Market/SSE_market_server && go build ./...`

---

### Task 4: 将 hot_data 的 DB 操作迁入 repository

**Files:**
- Create: `repository/hot_data.go`
- Create: `repository/gorm/hot_data.go`
- Modify: `service/hot_data_service.go`

- [ ] **Step 1: 创建 repository/hot_data.go 接口**

```go
package repository

import "context"

// HotDataRepository 热度数据持久化接口。
type HotDataRepository interface {
	GetBrowseNum(ctx context.Context, postID int) int64
	UpdateBrowseNum(ctx context.Context, postID int, browseNum int64) error
}
```

- [ ] **Step 2: 创建 repository/gorm/hot_data.go 实现**

```go
package gormrepo

import (
	"context"

	"loginTest/model"
	"loginTest/repository"

	"gorm.io/gorm"
)

type hotDataRepository struct {
	db *gorm.DB
}

func NewHotDataRepository(db *gorm.DB) repository.HotDataRepository {
	return &hotDataRepository{db: db}
}

func (r *hotDataRepository) GetBrowseNum(_ context.Context, postID int) int64 {
	var p model.Post
	if err := r.db.Select("browse_num").Where("postID = ?", postID).First(&p).Error; err != nil {
		return 0
	}
	return int64(p.BrowseNum)
}

func (r *hotDataRepository) UpdateBrowseNum(_ context.Context, postID int, browseNum int64) error {
	return r.db.Model(&model.Post{}).
		Where("postID = ?", postID).
		Update("browse_num", browseNum).Error
}
```

- [ ] **Step 3: 修改 service/hot_data_service.go**

将 `browseNumFromDB` 和 `syncToDB` 中的 DB 操作改为调用 repository。删除 `common` 和 `model` import。

在 `HotDataService` struct 中添加 `repo` 字段，`InitHotDataService` 接受 repo 参数。

关键变更：
- `browseNumFromDB` → `s.repo.GetBrowseNum`
- `syncToDB` 中 `db.Model(&model.Post{}).Where(...).Update(...)` → `s.repo.UpdateBrowseNum`
- 删除 `common.GetDB()` 调用
- 删除 `model` import

- [ ] **Step 4: 更新 main.go 或 InitHotDataService 调用点**

确保 `InitHotDataService` 传入 `hotDataRepo`。需要在 `service.InitServices` 之后或其中初始化。

- [ ] **Step 5: 验证编译**

Run: `cd /root/SSE_Market/SSE_market_server && go build ./...`

---

### Task 5: 清理残留引用 & 最终验证

**Files:**
- Modify: `service/post_service.go` (删除 `postToDomainFromModel`、`userToDomainFromModel`)
- Modify: `service/user_service.go` (删除 `DomainUserToModel` 引用，如有)
- Verify: 所有 controller 不再 import `model`

- [ ] **Step 1: 删除 service/post_service.go 中的重复转换函数**

删除 `postToDomainFromModel` 和 `userToDomainFromModel`（与 mapper.go 重复，且 service 层不应直接操作 model）。

- [ ] **Step 2: 检查 service/user_service.go 中是否有 DomainUserToModel 引用**

如果有，删除。service 层应只返回 domain 类型。

- [ ] **Step 3: 全局搜索确认无残留 model 引用**

Run: `cd /root/SSE_Market/SSE_market_server && grep -rn '"loginTest/model"' controller/ middleware/ dto/`
Expected: 0 matches

- [ ] **Step 4: 完整编译验证**

Run: `cd /root/SSE_Market/SSE_market_server && go build ./...`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: eliminate model↔domain circular conversion, move DB access to repository layer"
```
