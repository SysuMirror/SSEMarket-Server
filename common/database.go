package common

import (
	"fmt"
	"log"
	"loginTest/model"
	"time"

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
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	db.AutoMigrate(&model.User{})
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
	db.AutoMigrate(&model.Sue{})
	db.AutoMigrate(&model.CDKey{})
	db.AutoMigrate(&model.Tag{})
	db.AutoMigrate(&model.ChatMsg{})
	db.AutoMigrate(&model.OAuth2App{})
	db.AutoMigrate(&model.OAuth2Code{})
	db.AutoMigrate(&model.OAuth2Token{})
	db.AutoMigrate(&model.OAuth2AuthRecord{})
	db.AutoMigrate(&model.Pvote{})
	db.AutoMigrate(&model.Resume{})
	RunRatingMigration(db)
	db.Exec("UPDATE resumes SET view_num = download_num * 3 WHERE view_num = 0 AND download_num > 0")

	DB = db
	return db
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
