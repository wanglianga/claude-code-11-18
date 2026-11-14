package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"procurewatch/internal/api"
	"procurewatch/internal/config"
	"procurewatch/internal/db"
	"procurewatch/internal/service"
	"procurewatch/internal/store"
	"procurewatch/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// 容器启动时数据库可能尚未就绪，带重试连接。
	var pool *pgxpool.Pool
	var err error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err = db.Connect(ctx, cfg.DatabaseURL)
		if err == nil {
			break
		}
		log.Printf("waiting for database (attempt %d/30): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if pool == nil {
		log.Fatalf("connect database after retries: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	st := store.New(pool)
	if err := service.Seed(ctx, st, cfg.SampleBaseURL); err != nil {
		log.Fatalf("seed: %v", err)
	}

	proc := service.NewProcessor(st)
	wb := service.NewWorkbench(st, proc, cfg.ReviewInterval)

	// 启动采集 worker（列表/详情/复查三队列）
	w := worker.New(cfg, st)
	w.Run(ctx)

	// 启动引导：为每个启用来源入队一次全量列表作业（幂等，已有未完成作业则复用）
	go bootstrapListJobs(ctx, st)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	auth := api.NewAuth(st)
	srv := api.NewServer(st, wb)
	srv.Register(r, auth)

	// 生产形态：托管前端构建产物；开发形态由 Vite (5173) 代理 /api
	distDir := os.Getenv("FRONTEND_DIST")
	if distDir == "" {
		distDir = "/app/frontend/dist"
	}
	if fi, err := os.Stat(distDir); err == nil && fi.IsDir() {
		r.NoRoute(func(c *gin.Context) {
			p := filepath.Join(distDir, filepath.Clean(c.Request.URL.Path))
			if _, err := os.Stat(p); err != nil {
				c.File(filepath.Join(distDir, "index.html"))
				return
			}
			c.File(p)
		})
	}

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("procurewatch api listening on %s (sample base: %s)", cfg.HTTPAddr, cfg.SampleBaseURL)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func bootstrapListJobs(ctx context.Context, st *store.Store) {
	// 等待来源种子可见
	time.Sleep(2 * time.Second)
	ids, err := st.AllEnabledSourceIDs(ctx)
	if err != nil {
		log.Printf("bootstrap sources: %v", err)
		return
	}
	for _, id := range ids {
		if _, err := st.EnqueueListJob(ctx, id, "full"); err != nil {
			log.Printf("bootstrap list job source %d: %v", id, err)
		}
	}
}
