package worker

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"procurewatch/internal/parser"
	"procurewatch/internal/store"
)

type listOffset struct {
	Page   int    `json:"page,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Done   bool   `json:"done,omitempty"`
}

// listLoop 串行处理列表作业；整站一次只跑一个列表作业（DB 唯一索引保证）。
func (w *Worker) listLoop(ctx context.Context) {
	for {
		if !sleep(ctx, w.cfg.PollInterval) {
			return
		}
		jobID, sourceID, epoch, err := w.st.ClaimListJob(ctx, w.cfg.WorkerID, w.cfg.LeaseDuration)
		if err == store.ErrNoRows {
			continue
		}
		if err != nil {
			log.Printf("list claim error: %v", err)
			continue
		}
		if err := w.runListJob(ctx, jobID, sourceID, epoch); err != nil {
			log.Printf("list job %d source %d failed: %v", jobID, sourceID, err)
			_ = w.st.FinishListJob(ctx, jobID, epoch, "failed", err.Error())
		}
	}
}

func (w *Worker) runListJob(ctx context.Context, jobID, sourceID int64, epoch int) error {
	src, err := w.st.GetSourceRow(ctx, sourceID)
	if err != nil {
		return err
	}
	var off listOffset
	if len(src.CursorState) > 0 {
		_ = json.Unmarshal(src.CursorState, &off)
	}
	// 作业自身断点优先（中断恢复时从上次保存处继续，不重复翻前面的页）
	if off.Page == 0 && off.Cursor == "" {
		off.Page = 1
	}
	heartbeatStop := make(chan struct{})
	go w.heartbeat(ctx, jobID, epoch, heartbeatStop)
	defer close(heartbeatStop)

	maxPages := 100
	for p := 0; p < maxPages; p++ {
		listURL, nextOff := buildListURL(src, off)
		res := w.fetch.Fetch(ctx, src.ID, src.IntervalMs, src.MaxConcurrency, src.MaxRetries, listURL)
		if !res.Accessible {
			return &SoftStopError{Reason: "列表页获取失败：" + res.Error}
		}
		page := parser.ParseList(res.Body)
		// 结束标志校验：空列表只有携带与来源配置一致的样本结束标志才算到尾
		if len(page.Entries) == 0 {
			if !page.EndMarked || page.Marker != src.EndMarker {
				return &SoftStopError{Reason: "空列表缺少匹配的样本站点结束标志，不能判定到尾，保留断点待下次续抓"}
			}
			nextOff.Done = true
			if err := w.st.SaveListProgress(ctx, jobID, epoch, mustJSON(nextOff)); err != nil {
				return err
			}
			if err := w.st.UpdateSourceCursor(ctx, sourceID, mustJSON(nextOff), "done", ""); err != nil {
				return err
			}
			return w.st.FinishListJob(ctx, jobID, epoch, "succeeded", "")
		}
		if page.Marker != "" && page.Marker != src.EndMarker {
			return &SoftStopError{Reason: "列表页结束标志与来源配置不一致，停止推进"}
		}

		// 当前页发现的来源编号全部保存后才推进游标/页码
		for _, e := range page.Entries {
			detailURL := buildDetailURL(src, e.Ref)
			if inserted, _, err := w.st.UpsertDiscovery(ctx, sourceID, e.Ref, detailURL, e.Title); err != nil {
				return err
			} else if inserted {
				log.Printf("list job %d: discovered %s", jobID, e.Ref)
			}
		}
		// 本页发现落库后，立即为待取详情排队（列表与详情分开排队，互不阻塞其他来源）
		if _, err := w.st.EnqueueDetailForDiscovery(ctx, sourceID); err != nil {
			return err
		}

		// 推进断点
		if src.ListMode == "cursor" {
			nextOff.Cursor = page.NextCursor
			if page.EndMarked && page.NextCursor == "" {
				nextOff.Done = true
			}
		} else {
			nextOff.Page = off.Page + 1
			if page.EndMarked {
				nextOff.Done = true
			}
		}
		if err := w.st.SaveListProgress(ctx, jobID, epoch, mustJSON(nextOff)); err != nil {
			return err // epoch 已变（本作业被重新领取），旧执行者立即停止
		}
		if err := w.st.UpdateSourceCursor(ctx, sourceID, mustJSON(nextOff), "running", ""); err != nil {
			return err
		}

		// 故障注入：模拟进程在某页保存后中断（不完成作业），用于演示过期任务重新领取
		if hitListAbandonFailpoint(src.Code, off) {
			log.Printf("FAILPOINT list-abandon hit on %s: 模拟进程中断，作业 %d 保持 running 等待租约过期重新领取", src.Code, jobID)
			return nil
		}

		if nextOff.Done {
			if err := w.st.UpdateSourceCursor(ctx, sourceID, mustJSON(nextOff), "done", ""); err != nil {
				return err
			}
			return w.st.FinishListJob(ctx, jobID, epoch, "succeeded", "")
		}
		off = nextOff
	}
	return &SoftStopError{Reason: "超过最大翻页数，停止本次采集"}
}

func (w *Worker) heartbeat(ctx context.Context, jobID int64, epoch int, stop chan struct{}) {
	ticker := time.NewTicker(w.cfg.LeaseDuration / 3)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.st.HeartbeatListJob(ctx, jobID, epoch, w.cfg.LeaseDuration); err != nil {
				log.Printf("heartbeat rejected for job %d (epoch %d): %v", jobID, epoch, err)
				return
			}
		}
	}
}

func buildListURL(src store.SourceRow, off listOffset) (string, listOffset) {
	next := off
	if src.ListMode == "cursor" {
		sep := "?"
		if strings.Contains(src.ListPath, "?") {
			sep = "&"
		}
		return strings.TrimRight(src.BaseURL, "/") + src.ListPath + sep + "cursor=" + off.Cursor, next
	}
	if off.Page == 0 {
		next.Page = 1
	}
	sep := "?"
	if strings.Contains(src.ListPath, "?") {
		sep = "&"
	}
	return strings.TrimRight(src.BaseURL, "/") + src.ListPath + sep + "page=" + strconv.Itoa(next.Page), next
}

func buildDetailURL(src store.SourceRow, ref string) string {
	return strings.TrimRight(src.BaseURL, "/") + strings.ReplaceAll(src.DetailPathTpl, "{ref}", ref)
}

// SoftStopError 表示可恢复的停止（保留断点，下次续抓），作业标记 failed 并可重新入队。
type SoftStopError struct{ Reason string }

func (e *SoftStopError) Error() string { return e.Reason }

// FAILPOINT_LIST_ABANDON=code:page，例如 page-site:1
func hitListAbandonFailpoint(code string, off listOffset) bool {
	fp := os.Getenv("FAILPOINT_LIST_ABANDON")
	if fp == "" {
		return false
	}
	parts := strings.SplitN(fp, ":", 2)
	if len(parts) != 2 || parts[0] != code {
		return false
	}
	page, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return off.Page == page
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
