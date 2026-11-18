package common

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// ensurePlikeUniqueIndex 确保点赞明细具备 (userID, ptargetID) 唯一索引。
// 历史并发漏洞可能留下重复点赞，这里不在启动路径上自动删数据，
// 避免大表锁表；有重复时先执行 docs/migrations/20261118_plikes_unique_user_target.sql。
func ensurePlikeUniqueIndex(db *gorm.DB) {
	ensureUniqueIndexIfNoDuplicates(db, "plikes", "uk_plikes_user_target", "userID", "ptargetID")
}

// ensureReactionUniqueIndexes 确保浏览与评论反应明细都具备 (userID, targetID) 唯一索引。
// 与点赞相同：有历史重复时只打日志，由迁移 SQL 负责清理。
func ensureReactionUniqueIndexes(db *gorm.DB) {
	ensureUniqueIndexIfNoDuplicates(db, "pbrowses", "uk_pbrowses_user_target", "userID", "ptargetID")
	ensureUniqueIndexIfNoDuplicates(db, "pclikes", "uk_pclikes_user_target", "userID", "pctargetID")
	ensureUniqueIndexIfNoDuplicates(db, "pcdenies", "uk_pcdenies_user_target", "userID", "pctargetID")
	ensureUniqueIndexIfNoDuplicates(db, "cclikes", "uk_cclikes_user_target", "userID", "cctargetID")
	ensureUniqueIndexIfNoDuplicates(db, "ccdenies", "uk_ccdenies_user_target", "userID", "cctargetID")
}

// ensureCommentReactionUniqueIndex 确保合并后的评论反应表具备 (userID, targetType, targetID) 唯一索引。
func ensureCommentReactionUniqueIndex(db *gorm.DB) {
	ensureUniqueIndexIfNoDuplicates(db, "comment_reactions", "uk_comment_reactions_user_target", "userID", "targetType", "targetID")
}

// ensureUniqueIndexIfNoDuplicates 在确认没有重复组合后创建唯一索引。
// 不在启动路径上自动删数据，避免大表锁表；重复数据由迁移 SQL 清理。
func ensureUniqueIndexIfNoDuplicates(db *gorm.DB, table, indexName string, columns ...string) {
	var count int64
	db.Raw(`
		SELECT COUNT(1) FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND index_name = ?
	`, table, indexName).Scan(&count)
	if count > 0 {
		return
	}

	colList := strings.Join(columns, ", ")
	var dup int64
	db.Raw(fmt.Sprintf(`
		SELECT COUNT(1) FROM (
			SELECT %s FROM %s
			GROUP BY %s
			HAVING COUNT(*) > 1
		) AS dups
	`, colList, table, colList)).Scan(&dup)
	if dup > 0 {
		log.Printf("[db] %s 存在 %d 组重复数据，跳过唯一索引 %s 创建；请先执行迁移 SQL", table, dup, indexName)
		return
	}

	if err := db.Exec(fmt.Sprintf(
		"ALTER TABLE %s ADD UNIQUE KEY %s (%s)", table, indexName, colList,
	)).Error; err != nil {
		log.Printf("[db] ensure unique index %s on %s: %v", indexName, table, err)
	}
}
