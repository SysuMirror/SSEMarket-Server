package common

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"loginTest/config"
	"loginTest/domain"
	"loginTest/model"
	"loginTest/observability"
	"time"

	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// autoMigrate 执行 AutoMigrate 并把 error 打日志。
// 此前 AutoMigrate 的 error 被静默吞掉，线上出现过「代码有新列、库里没有」
// （Error 1054: Unknown column 'coins'）却无人知晓的情况；迁移失败必须可见。
// 手动兜底 SQL 见 docs/migrations/20261012_coin.sql。
func autoMigrate(db *gorm.DB, models ...any) {
	if err := db.AutoMigrate(models...); err != nil {
		log.Printf("[db] automigrate %T failed: %v", models[0], err)
	}
}

func InitDB() *gorm.DB {
	host := viper.GetString("datasource.host")
	port := viper.GetString("datasource.port")
	database := viper.GetString("datasource.database")
	username := viper.GetString("datasource.username")
	password := viper.GetString("datasource.password")
	charset := viper.GetString("datasource.charset")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=true&loc=Local",
		username, password, host, port, database, charset)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("failed to connect database, err: " + err.Error())
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB, err: " + err.Error())
	}
	maxOpen := config.DatasourcePoolMaxOpen()
	maxIdle := config.DatasourcePoolMaxIdle()
	connLifetime := config.DatasourcePoolConnMaxLifetime()
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(connLifetime)
	log.Printf("[db] pool max_open=%d max_idle=%d conn_max_lifetime=%s", maxOpen, maxIdle, connLifetime)

	autoMigrate(db, &model.User{})
	// 全站皮肤：活动时间窗（一行一期活动，由管理员直接 INSERT）+ 用户外观设置。
	autoMigrate(db, &model.ThemeEvent{})
	autoMigrate(db, &model.UserThemeSetting{})
	// 勋章系统：勋章目录（管理员 INSERT 图床地址）+ 发放记录 + 用户展示偏好。
	autoMigrate(db, &model.Badge{})
	autoMigrate(db, &model.UserBadge{})
	autoMigrate(db, &model.UserBadgePref{})
	ensureUserEmailUniqueIndex(db)
	autoMigrate(db, &model.UsedEmail{})
	autoMigrate(db, &model.Post{})
	autoMigrate(db, &model.Plike{})
	autoMigrate(db, &model.Psave{})
	autoMigrate(db, &model.Cclike{})
	autoMigrate(db, &model.Ccdeny{})
	autoMigrate(db, &model.Pclike{})
	autoMigrate(db, &model.Pcdeny{})
	autoMigrate(db, &model.Pcomment{})
	autoMigrate(db, &model.Ccomment{})
	autoMigrate(db, &model.Pbrowse{})
	autoMigrate(db, &model.Admin{})
	autoMigrate(db, &model.Feedback{})
	autoMigrate(db, &model.Notice{})
	db.Exec("ALTER TABLE notices MODIFY COLUMN type ENUM('pcomment','ccomment','punish','feedback','sue','post','lottery_win','rating_comment','post_watch')")
	autoMigrate(db, &model.Sue{})
	autoMigrate(db, &model.CDKey{})
	autoMigrate(db, &model.Tag{})
	autoMigrate(db, &model.ChatMsg{})
	ensureChatMsgAnonymousColumn(db)
	ensureChatMsgThreadAnonymousColumn(db)
	ensureChatMsgRecalledColumn(db)
	autoMigrate(db, &model.OAuth2App{})
	autoMigrate(db, &model.OAuth2Code{})
	autoMigrate(db, &model.OAuth2Token{})
	autoMigrate(db, &model.OAuth2AuthRecord{})
	autoMigrate(db, &model.UserAPIToken{})
	autoMigrate(db, &model.Pvote{})
	autoMigrate(db, &model.PostWatch{})
	autoMigrate(db, &model.LotteryPost{})
	autoMigrate(db, &model.LotteryPrize{})
	autoMigrate(db, &model.LotteryParticipant{})
	autoMigrate(db, &model.LotteryWinner{})
	ensureLotteryParticipantUniqueIndex(db)
	autoMigrate(db, &model.Resume{})
	autoMigrate(db, &model.WorkbenchApp{})
	autoMigrate(db, &model.Music{})
	autoMigrate(db, &model.Video{})
	autoMigrate(db, &model.Novel{})
	autoMigrate(db, &model.NovelChapter{})
	autoMigrate(db, &model.NovelComment{})
	autoMigrate(db, &model.NovelCommentLike{})
	autoMigrate(db, &model.NovelBookshelf{})
	autoMigrate(db, &model.NovelProgress{})
	autoMigrate(db, &model.NovelLedger{})
	autoMigrate(db, &model.NovelRating{})
	// 站内币体系：流水 + 投币记录（与小说的经验/积分互不相通）。
	autoMigrate(db, &model.CoinLedger{})
	autoMigrate(db, &model.CoinThrow{})
	// 职位账号（开发人员匿名 @）：管理员手工 INSERT 的映射表 + 启动同步派生小号
	autoMigrate(db, &model.StaffAlias{})
	SyncStaffAliases(db)
	RunRatingMigration(db)
	db.Exec("UPDATE resumes SET view_num = download_num * 3 WHERE view_num = 0 AND download_num > 0")

	if err := observability.RegisterGORMMetrics(db); err != nil {
		log.Printf("[observability] register gorm metrics: %v", err)
	}

	DB = db
	return db
}

func ensureChatMsgAnonymousColumn(db *gorm.DB) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'chat_msgs'
		  AND column_name = 'isAnonymous'
	`).Scan(&count)
	if count > 0 {
		return
	}
	if err := db.Exec(`
		ALTER TABLE chat_msgs
		ADD COLUMN isAnonymous tinyint(1) NOT NULL DEFAULT 0 AFTER content
	`).Error; err != nil {
		log.Printf("[chat] ensure isAnonymous column: %v", err)
	}
}

func ensureChatMsgThreadAnonymousColumn(db *gorm.DB) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'chat_msgs'
		  AND column_name = 'threadAnonymous'
	`).Scan(&count)
	if count == 0 {
		if err := db.Exec(`
			ALTER TABLE chat_msgs
			ADD COLUMN threadAnonymous tinyint(1) NOT NULL DEFAULT 0 AFTER isAnonymous
		`).Error; err != nil {
			log.Printf("[chat] ensure threadAnonymous column: %v", err)
		}
	}
	if err := db.Exec(`
		UPDATE chat_msgs SET threadAnonymous = isAnonymous
		WHERE isAnonymous = 1 AND threadAnonymous = 0
	`).Error; err != nil {
		log.Printf("[chat] backfill threadAnonymous: %v", err)
	}
}

func ensureChatMsgRecalledColumn(db *gorm.DB) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'chat_msgs'
		  AND column_name = 'isRecalled'
	`).Scan(&count)
	if count == 0 {
		if err := db.Exec(`
			ALTER TABLE chat_msgs
			ADD COLUMN isRecalled tinyint(1) NOT NULL DEFAULT 0 AFTER threadAnonymous
		`).Error; err != nil {
			log.Printf("[chat] ensure isRecalled column: %v", err)
		}
	}
}

func ensureUserEmailUniqueIndex(db *gorm.DB) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'users'
		  AND index_name = 'idx_users_email'
	`).Scan(&count)
	if count > 0 {
		return
	}
	// 历史数据若存在重复邮箱则跳过创建，避免启动失败；由事务内二次校验兜底。
	var dup int64
	db.Raw(`
		SELECT COUNT(1) FROM (
			SELECT email FROM users
			WHERE email <> ''
			GROUP BY email HAVING COUNT(*) > 1
		) AS dups
	`).Scan(&dup)
	if dup > 0 {
		log.Printf("[db] users 存在 %d 组重复邮箱，跳过唯一索引创建", dup)
		return
	}
	if err := db.Exec(`
		ALTER TABLE users ADD UNIQUE INDEX idx_users_email (email)
	`).Error; err != nil {
		log.Printf("[db] ensure unique index idx_users_email: %v", err)
	}
}

func ensureLotteryParticipantUniqueIndex(db *gorm.DB) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'lottery_participants'
		  AND index_name = 'lottery_user_prize'
	`).Scan(&count)
	if count > 0 {
		return
	}
	if err := db.Exec(`
		ALTER TABLE lottery_participants
		ADD UNIQUE INDEX lottery_user_prize (lotteryID, userID, prizeID)
	`).Error; err != nil {
		log.Printf("[lottery] ensure unique index lottery_user_prize: %v", err)
	}
}

// CloseDB 关闭底层连接池（优雅关闭时调用）。
func CloseDB() {
	if DB == nil {
		return
	}
	sqlDB, err := DB.DB()
	if err != nil {
		log.Printf("close db: %v", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		log.Printf("close db: %v", err)
	}
}

func GetDB() *gorm.DB {
	return DB
}

// SyncStaffAliases 启动时同步职位账号（开发人员匿名 @，表 staff_aliases 由管理员手工 INSERT）。
// 对每行合法的职位：
//  1. alias 与普通用户昵称冲突 → 打日志跳过（不派生小号，@ 该名字仍按普通用户处理）；
//  2. 按 staff-<aliasID>@staff.internal 找职位小号（users 行），没有就建：
//     满级（score=5000，前端 Lv8 阈值）+ 称号=职称 + 随机不可登录口令 + 屏蔽邮件通知；
//  3. 管理员改了 alias/title 后重启即同步到小号昵称/称号，并回写 alias_user_id。
//
// 全程幂等；运行期新增的行由 service.ensureShells 兜底（无需重启）。
func SyncStaffAliases(db *gorm.DB) {
	var rows []model.StaffAlias
	if err := db.Order("id").Find(&rows).Error; err != nil {
		log.Printf("[staff] 读取 staff_aliases 失败: %v", err)
		return
	}
	for i := range rows {
		row := &rows[i]
		if err := domain.ValidateStaffAliasRow(row.Alias, row.Title, row.StaffUserID); err != nil {
			log.Printf("[staff] 跳过非法职位行 id=%d: %v", row.ID, err)
			continue
		}
		email := domain.StaffAliasShellEmail(row.ID)
		var conflict int64
		if err := db.Model(&model.User{}).
			Where("name = ? AND (email IS NULL OR email <> ?)", row.Alias, email).
			Count(&conflict).Error; err != nil {
			log.Printf("[staff] 查重失败 id=%d alias=%q: %v", row.ID, row.Alias, err)
			continue
		}
		if conflict > 0 {
			log.Printf("[staff] alias=%q 与已有用户昵称冲突，未派生职位小号（请管理员换个名字）", row.Alias)
			continue
		}
		var shell model.User
		err := db.Where("email = ?", email).First(&shell).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			shell = model.User{
				Name:     row.Alias,
				Email:    email,
				Password: staffShellPassword(row.ID),
				// 等级自动满级（前端 Lv8 阈值 score>=5000），称号即职称
				Score:       domain.StaffAliasScore,
				CustomTitle: row.Title,
				// 职位小号不参与邮件通知；没有手机号，无短信/找回路径
				ISEmailNotificationBlocked: true,
			}
			if err := db.Create(&shell).Error; err != nil {
				log.Printf("[staff] 派生职位小号失败 id=%d alias=%q: %v", row.ID, row.Alias, err)
				continue
			}
			log.Printf("[staff] 已派生职位小号 alias=%q userID=%d", row.Alias, shell.UserID)
		} else if err != nil {
			log.Printf("[staff] 查询职位小号失败 id=%d: %v", row.ID, err)
			continue
		}
		// 同步昵称 / 称号（管理员改行后重启生效）
		if shell.Name != row.Alias || shell.CustomTitle != row.Title || shell.Score < domain.StaffAliasScore {
			if err := db.Model(&model.User{}).Where("userID = ?", shell.UserID).
				Updates(map[string]any{"name": row.Alias, "custom_title": row.Title, "score": domain.StaffAliasScore}).Error; err != nil {
				log.Printf("[staff] 同步职位小号资料失败 id=%d: %v", row.ID, err)
				continue
			}
		}
		if row.AliasUserID != shell.UserID {
			if err := db.Model(&model.StaffAlias{}).Where("id = ?", row.ID).
				UpdateColumn("alias_user_id", shell.UserID).Error; err != nil {
				log.Printf("[staff] 回写 alias_user_id 失败 id=%d: %v", row.ID, err)
			}
		}
	}
}

// staffShellPassword 一段无人知道的随机串的 bcrypt 哈希：职位小号永远无法登录。
func staffShellPassword(aliasID int) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		raw = []byte(fmt.Sprintf("%d:%d", aliasID, time.Now().UnixNano()))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(raw)), bcrypt.DefaultCost)
	if err != nil {
		// 不是合法 bcrypt 哈希的占位串同样无法通过登录校验
		return fmt.Sprintf("staff-alias-unloginnable-%d", aliasID)
	}
	return string(hash)
}
