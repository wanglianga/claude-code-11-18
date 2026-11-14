package worker

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"procurewatch/internal/config"
	"procurewatch/internal/fetcher"
	"procurewatch/internal/service"
	"procurewatch/internal/store"
)

// Worker 运行三类调度循环：列表采集、详情下载、定期复查。
// 列表与详情分开排队；所有回写前校验租约，中断恢复后旧执行者不能覆盖新结果。
type Worker struct {
	cfg    config.Config
	st     *store.Store
	fetch  *fetcher.Client
	proc   *service.Processor
	tokenSeq atomic.Uint64
}

func New(cfg config.Config, st *store.Store) *Worker {
	return &Worker{cfg: cfg, st: st, fetch: fetcher.NewClient(), proc: service.NewProcessor(st)}
}

func (w *Worker) Run(ctx context.Context) {
	go w.listLoop(ctx)
	for i := 0; i < 3; i++ {
		go w.detailLoop(ctx)
	}
	go w.reviewLoop(ctx)
	log.Printf("worker %s started: list loop + 3 detail loops + review loop", w.cfg.WorkerID)
}

// newClaimToken 为每次领取生成唯一执行者令牌；同一进程重新领取过期任务也会得到不同令牌。
// 该令牌直接写入作业的 worker_id，回写前据此判断租约是否仍属于本次领取。
func (w *Worker) newClaimToken() string {
	n := w.tokenSeq.Add(1)
	return formatToken(w.cfg.WorkerID, 0, n, time.Now())
}

func formatToken(worker string, jobID int64, n uint64, t time.Time) string {
	return worker + "-j" + itoa(jobID) + "-n" + itoa64(n) + "-t" + itoa64(uint64(t.UnixNano()))
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func itoa64(i uint64) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
