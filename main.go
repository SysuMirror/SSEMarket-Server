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
	gormrepo "loginTest/repository/gorm"
	"loginTest/route"
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

// awardDueAnniversaries 扫描 registered_at，给已到周年日的用户补发周年勋章。
// 幂等：同一用户同一周年只会插一条，重复执行安全。
func awardDueAnniversaries() {
	if service.Badges == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	n, err := service.Badges.AwardDueAnniversaries(ctx)
	if err != nil {
		log.Printf("[badge] 周年勋章自动发放失败: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[badge] 自动发放周年勋章 %d 枚", n)
	}
}

func startBadgeCron() {
	c := cron.New()
	// 每天 00:10（容器 TZ=Asia/Shanghai）检查一次；错过当天也会在下次自动补齐。
	_, _ = c.AddFunc("10 0 * * *", awardDueAnniversaries)
	c.Start()
}

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
	// 清理放弃发布留下的音乐草稿（未发布且超过24小时）
	_, _ = c.AddFunc("@every 24h", controller.CleanUnpublishedMusic)
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
	// SSO 配置自检：启用统一身份但缺 client/secret 时，所有登录都会 503。
	// 启动即失败，避免「容器起来了但没人能登录」；secret 只放在 go_server.env，不写进镜像。
	if config.SsoEnabled() && (config.SsoClientID() == "" || config.SsoClientSecret() == "" || config.SsoDiscoveryURL() == "") {
		log.Fatalf("[config] SSO enabled but SSO_CLIENT_ID / SSO_CLIENT_SECRET / SSO_DISCOVERY_URL is empty; refusing to start (check /root/SSE_Market/go_server.env)")
	}
	config.InitEmailConfig()
	// 邮件配置自检：缺 SMTP_* 时验证码/通知会全部发不出去，且发件人地址会
	// 退化成非法格式（mail: invalid string）。这里明确告警，便于线上排查；
	// 不 Fatalf，避免因邮件故障拖垮登录等不依赖邮件的主流程。
	if !config.EmailConfigured() {
		log.Printf("[config] 警告：SMTP_EMAIL_USERNAME/PASSWORD/ADDR/HOST 未配置，注册验证码与邮件通知将不可用（检查 /root/SSE_Market/go_server.env）")
	}
	config.InitPprof()

	obsCtx := context.Background()
	shutdownTracing, err := observability.InitTracing(obsCtx)
	if err != nil {
		log.Fatalf("[observability] init tracing: %v", err)
	}

	go Copy()
	db := common.InitDB()
	service.InitServices(db)
	// 周年勋章：启动即补一次，之后每天 00:10 自动扫描到期用户。
	go awardDueAnniversaries()
	go startBadgeCron()
	service.Ranks.RebuildRankStats(context.Background())
	// 启动即全量重算一次积分,后续由 Copy() 内的 @every 1h cron 维持
	go controller.CalculateAndSaveScores()

	rds := core.RedisInit()
	common.InitJWTkey()
	core.InitLocalCache()
	cache.InitManager()
	service.InitHotDataService(gormrepo.NewHotDataRepository(db))
	// UniSSO 健康探测：主模式/兜底模式切换，前端登录入口依赖此状态。
	service.StartSsoHealthMonitor()

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
	r.GET("/metrics", middleware.MetricsGuard(gin.WrapH(promhttp.Handler())))
	r.StaticFS("/uploads", http.Dir("./public/uploads"))
	r.StaticFS("/resized", http.Dir("./public/resized"))
	r.Static("/media/videos", "./public/videos")
	registerPprofRoutes(r)
	route.CollectRoute(r)
	go heat.StartHeat()
	go lottery.StartAutoDraw()
	go pin.StartClearExpired()
	// 推荐流：加载向量索引 + 后台增量构建帖子嵌入（未配置 key 时自动只做时间流）。
	service.Recs.Start(context.Background())

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
