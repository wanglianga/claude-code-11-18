package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr       string
	DatabaseURL    string
	SampleBaseURL  string // 样本站点根地址，供初始化来源使用
	WorkerID       string
	LeaseDuration  time.Duration // 作业租约
	PollInterval   time.Duration // 调度轮询
	ReviewRecent   time.Duration // 近期项目窗口
	ReviewInterval time.Duration // 近期项目复查间隔
	StaleNoteDays  int           // 超过该天数未核查则提示不能声称仍有效
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getdur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getint(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func Load() Config {
	return Config{
		HTTPAddr:       getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:    getenv("DATABASE_URL", "postgres://ccdev:ccdev@127.0.0.1:5432/ccdev?sslmode=disable"),
		SampleBaseURL:  getenv("SAMPLE_BASE_URL", "http://127.0.0.1:9090"),
		WorkerID:       getenv("WORKER_ID", "worker-1"),
		LeaseDuration:  getdur("LEASE_DURATION", 90*time.Second),
		PollInterval:   getdur("POLL_INTERVAL", 2*time.Second),
		ReviewRecent:   getdur("REVIEW_RECENT_WINDOW", 72*time.Hour),
		ReviewInterval: getdur("REVIEW_INTERVAL", 24*time.Hour),
		StaleNoteDays:  getint("STALE_NOTE_DAYS", 3),
	}
}
