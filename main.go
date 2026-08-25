package main

import (
	"context"
	"fmt"
	"log"
	"loginTest/cache"
	"loginTest/common"
	"loginTest/config"
	"loginTest/controller"
	"loginTest/core"
	"loginTest/heat"
	"loginTest/lottery"
	"loginTest/middleware"
	"loginTest/observability"
	"loginTest/pin"
	"loginTest/route"
	gormrepo "loginTest/repository/gorm"
	"loginTest/service"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	rpprof "runtime/pprof"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// 核心变量
var (
	r            *gin.Engine
	pprofRunning bool
	pprofMutex   sync.Mutex
)

func Copy() {
	dbHost := viper.GetString("datasource.host")
	dbPort := viper.GetInt("datasource.port")
	dbUser := viper.GetString("datasource.username")
	dbPassword := viper.GetString("datasource.password")
	dbName := viper.GetString("datasource.database")
	backupDir := "/app/database"

	c := cron.New()
	// 每日 04:00 备份一次（原 @every 12h 一天两次，易占满磁盘）
	_, _ = c.AddFunc("0 4 * * *", func() {
		backupFile := fmt.Sprintf("%s/backup_%s.sql", backupDir, time.Now().Format("2006-01-02 15:04:05"))
		cmd := exec.Command("mysqldump", fmt.Sprintf("-h%s", dbHost), fmt.Sprintf("-P%d", dbPort), fmt.Sprintf("-u%s", dbUser), fmt.Sprintf("-p%s", dbPassword), dbName, "--skip-ssl", "--result-file="+backupFile)
		if err := cmd.Run(); err != nil {
			log.Println("备份失败:", err)
			return
		}
		log.Println("备份成功:", backupFile)
	})
	_, _ = c.AddFunc("@every 1h", controller.CalculateAndSaveScores)
	c.Start()
}

// 手动触发pprof采集
func startPprof(c *gin.Context) {
	pprofMutex.Lock()
	if pprofRunning {
		pprofMutex.Unlock()
		c.JSON(400, gin.H{"msg": "pprof 正在采集中"})
		return
	}
	pprofRunning = true
	pprofMutex.Unlock()

	duration, _ := time.ParseDuration(c.DefaultQuery("duration", "30s"))
	saveDir := "/app/pprof"
	_ = os.MkdirAll(saveDir, 0755)
	now := time.Now().Format("20060102_150405")

	go func() {
		defer func() {
			pprofMutex.Lock()
			pprofRunning = false
			pprofMutex.Unlock()
			log.Println("✅ pprof 采集完成")
		}()

		f, _ := os.Create(filepath.Join(saveDir, fmt.Sprintf("cpu_%s.pprof", now)))
		_ = rpprof.StartCPUProfile(f)
		time.Sleep(duration)
		rpprof.StopCPUProfile()
		_ = f.Close()

		f2, _ := os.Create(filepath.Join(saveDir, fmt.Sprintf("heap_%s.pprof", now)))
		_ = rpprof.WriteHeapProfile(f2)
		_ = f2.Close()
	}()

	c.JSON(200, gin.H{
		"msg":      "✅ pprof 已启动",
		"duration": duration.String(),
		"savePath": "/app/pprof",
	})
}

func registerPprofRoutes(r *gin.Engine) {
	guard := middleware.PprofGuard()
	debug := r.Group("/debug/pprof", guard)
	{
		debug.GET("/", gin.WrapF(pprof.Index))
		debug.GET("/cmdline", gin.WrapF(pprof.Cmdline))
		debug.GET("/profile", gin.WrapF(pprof.Profile))
		debug.GET("/symbol", gin.WrapF(pprof.Symbol))
		debug.POST("/symbol", gin.WrapF(pprof.Symbol))
		debug.GET("/trace", gin.WrapF(pprof.Trace))
		debug.GET("/heap", gin.WrapH(pprof.Handler("heap")))
		debug.GET("/allocs", gin.WrapH(pprof.Handler("allocs")))
		debug.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		debug.GET("/block", gin.WrapH(pprof.Handler("block")))
		debug.GET("/mutex", gin.WrapH(pprof.Handler("mutex")))
		debug.GET("/threadcreate", gin.WrapH(pprof.Handler("threadcreate")))
	}
	r.GET("/debug/pprof/start", guard, startPprof)
}

func main() {
	config.InitConfig()
	config.InitEmailConfig()
	config.InitPprof()

	obsCtx := context.Background()
	shutdownTracing, err := observability.InitTracing(obsCtx)
	if err != nil {
		log.Fatalf("[observability] init tracing: %v", err)
	}

	go Copy()
	db := common.InitDB()
	service.InitServices(db)
	service.Ranks.RebuildRankStats(context.Background())

	rds := core.RedisInit()
	common.InitJWTkey()
	core.InitLocalCache()
	cache.InitManager()
	service.InitHotDataService(gormrepo.NewHotDataRepository(db))

	collectorCtx, collectorCancel := context.WithCancel(context.Background())
	observability.StartCollectors(collectorCtx, rds)

	// 缓存预热：完成后再标记 ready 并注册路由接流量
	warmupCtx, warmupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	postRepo := gormrepo.NewPostRepository(db)
	userRepo := gormrepo.NewUserRepository(db)
	hotRepo := gormrepo.NewHotDataRepository(db)
	cache.RunWarmup(warmupCtx, cache.BuildWarmupTasks(cache.WarmupDeps{
		Posts:   postRepo,
		Users:   userRepo,
		HotData: hotRepo,
		LoadBloom: func(ctx context.Context) error {
			_ = ctx
			common.InitBloomFilter()
			return nil
		},
		LoadHotZSet: func(ctx context.Context) error {
			_ = ctx
			heat.RefreshHeat()
			return nil
		},
	}))
	cache.WarmupPostDetailBatch(warmupCtx, postRepo, cache.ExtractTopPostIDsFromZSet(warmupCtx, 100))
	warmupCancel()
	cache.MarkReady()

	r = gin.Default()
	r.Use(middleware.RequestID())
	if viper.GetBool("observability.tracing.enabled") {
		serviceName := viper.GetString("observability.tracing.service_name")
		if serviceName == "" {
			serviceName = "sse_market_server"
		}
		r.Use(otelgin.Middleware(serviceName))
		r.Use(middleware.AttachRequestIDToSpan())
	}
	r.Use(middleware.PrometheusMetrics())
	r.Use(middleware.LoggerToFile())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.StaticFS("/uploads", http.Dir("./public/uploads"))
	r.StaticFS("/resized", http.Dir("./public/resized"))
	registerPprofRoutes(r)
	route.CollectRoute(r)
	go heat.StartHeat()
	go lottery.StartAutoDraw()
	go pin.StartClearExpired()

	port := viper.GetString("server.port")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		log.Printf("🚀 服务启动: %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到关闭信号，开始优雅关闭...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectorCancel()

	if err := shutdownTracing(shutdownCtx); err != nil {
		log.Printf("[observability] shutdown tracing: %v", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 服务关闭: %v", err)
	}

	if service.HotData != nil {
		service.HotData.Shutdown()
	}

	if err := rds.Close(); err != nil {
		log.Printf("Redis 关闭: %v", err)
	}
	common.CloseDB()

	log.Println("服务已优雅关闭")
}
