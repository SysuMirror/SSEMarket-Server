package observability

import (
	"time"

	"gorm.io/gorm"
)

type gormMetricsPlugin struct{}

func (gormMetricsPlugin) Name() string { return "observability:metrics" }

func (gormMetricsPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()
	_ = cb.Query().Before("gorm:query").Register("observability:before_query", func(db *gorm.DB) {
		db.InstanceSet("observability:query_start", time.Now())
	})
	_ = cb.Query().After("gorm:query").Register("observability:after_query", func(db *gorm.DB) {
		start, ok := db.InstanceGet("observability:query_start")
		if !ok {
			return
		}
		t, ok := start.(time.Time)
		if !ok {
			return
		}
		op := "query"
		if db.Statement != nil && db.Statement.Table != "" {
			op = db.Statement.Table
		}
		DBQueryDuration.WithLabelValues(op).Observe(time.Since(t).Seconds())
	})
	return nil
}

// RegisterGORMMetrics 为 GORM 注册查询耗时指标。
func RegisterGORMMetrics(db *gorm.DB) error {
	return db.Use(gormMetricsPlugin{})
}
