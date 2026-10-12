package common

import (
	"fmt"
	"log"
	"loginTest/config"
	"loginTest/model"
	"loginTest/observability"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

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

	db.AutoMigrate(&model.User{})
	ensureUserEmailUniqueIndex(db)
	db.AutoMigrate(&model.UsedEmail{})
	db.AutoMigrate(&model.Post{})
	db.AutoMigrate(&model.Plike{})
	db.AutoMigrate(&model.Psave{})
	db.AutoMigrate(&model.Cclike{})
	db.AutoMigrate(&model.Ccdeny{})
	db.AutoMigrate(&model.Pclike{})
	db.AutoMigrate(&model.Pcdeny{})
	db.AutoMigrate(&model.Pcomment{})
	db.AutoMigrate(&model.Ccomment{})
	db.AutoMigrate(&model.Pbrowse{})
	db.AutoMigrate(&model.Admin{})
	db.AutoMigrate(&model.Feedback{})
	db.AutoMigrate(&model.Notice{})
	db.Exec("ALTER TABLE notices MODIFY COLUMN type ENUM('pcomment','ccomment','punish','feedback','sue','post','lottery_win','rating_comment','post_watch')")
	db.AutoMigrate(&model.Sue{})
	db.AutoMigrate(&model.CDKey{})
	db.AutoMigrate(&model.Tag{})
	db.AutoMigrate(&model.ChatMsg{})
	ensureChatMsgAnonymousColumn(db)
	ensureChatMsgThreadAnonymousColumn(db)
	ensureChatMsgRecalledColumn(db)
	db.AutoMigrate(&model.OAuth2App{})
	db.AutoMigrate(&model.OAuth2Code{})
	db.AutoMigrate(&model.OAuth2Token{})
	db.AutoMigrate(&model.OAuth2AuthRecord{})
	db.AutoMigrate(&model.UserAPIToken{})
	db.AutoMigrate(&model.Pvote{})
	db.AutoMigrate(&model.PostWatch{})
	db.AutoMigrate(&model.LotteryPost{})
	db.AutoMigrate(&model.LotteryPrize{})
	db.AutoMigrate(&model.LotteryParticipant{})
	db.AutoMigrate(&model.LotteryWinner{})
	ensureLotteryParticipantUniqueIndex(db)
	db.AutoMigrate(&model.Resume{})
	db.AutoMigrate(&model.WorkbenchApp{})
	db.AutoMigrate(&model.Music{})
	db.AutoMigrate(&model.Video{})
	db.AutoMigrate(&model.Novel{})
	db.AutoMigrate(&model.NovelChapter{})
	db.AutoMigrate(&model.NovelComment{})
	db.AutoMigrate(&model.NovelCommentLike{})
	db.AutoMigrate(&model.NovelBookshelf{})
	db.AutoMigrate(&model.NovelProgress{})
	db.AutoMigrate(&model.NovelLedger{})
	db.AutoMigrate(&model.NovelRating{})
	// 站内币体系：流水 + 投币记录（与小说的经验/积分互不相通）。
	db.AutoMigrate(&model.CoinLedger{})
	db.AutoMigrate(&model.CoinThrow{})
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
