package worker

import (
	"context"
	"log"
	"time"

	"procurewatch/internal/parser"
	"procurewatch/internal/store"
)

// reviewLoop 定期复查近期项目的正文：原址换文在此被发现；长期未查项目只展示最后核查时间。
func (w *Worker) reviewLoop(ctx context.Context) {
	for {
		if !sleep(ctx, w.cfg.PollInterval) {
			return
		}
		token := w.newClaimToken()
		queueID, projectID, docID, _, err := w.st.ClaimReview(ctx, token, w.cfg.LeaseDuration)
		if err == store.ErrNoRows {
			continue
		}
		if err != nil {
			log.Printf("review claim error: %v", err)
			continue
		}
		w.runReview(ctx, queueID, projectID, docID, token)
	}
}

func (w *Worker) runReview(ctx context.Context, queueID, projectID, docID int64, token string) {
	sourceID, err := w.st.DocumentSource(ctx, docID)
	if err != nil {
		_ = w.st.RequeueReview(ctx, queueID, time.Now().Add(time.Minute), err.Error())
		return
	}
	src, err := w.st.GetSourceRow(ctx, sourceID)
	if err != nil {
		_ = w.st.RequeueReview(ctx, queueID, time.Now().Add(time.Minute), err.Error())
		return
	}
	url, err := w.st.GetDocumentURL(ctx, docID)
	if err != nil {
		_ = w.st.RequeueReview(ctx, queueID, time.Now().Add(time.Minute), err.Error())
		return
	}
	res := w.fetch.Fetch(ctx, sourceID, src.IntervalMs, src.MaxConcurrency, src.MaxRetries, url)

	ok, err := w.st.CheckReviewLease(ctx, queueID, token)
	if err != nil || !ok {
		log.Printf("review %d: 租约失效或校验失败，旧结果丢弃", queueID)
		return
	}

	if !res.Accessible {
		// 不可访问不等于撤标：项目状态保持不变，仅记录并安排后续再次核查
		_ = w.st.FinishReview(ctx, queueID, "inaccessible", res.Error,
			projectID, docID, nil, "inaccessible", res.Fingerprint, "链接不可访问，未发现明确撤标公告，项目状态不变")
		w.maybeScheduleNext(ctx, projectID, docID)
		return
	}

	detail, truncated, parseErrMsg := parser.ParseDetail(res.Body)
	if parseErrMsg != "" || truncated {
		versionID, _, verr := w.st.InsertVersion(ctx, docID, 0, res.Fingerprint, res.Body, false, truncated, parseErrMsg, detail.JSON())
		if verr != nil {
			_ = w.st.RequeueReview(ctx, queueID, time.Now().Add(time.Minute), verr.Error())
			return
		}
		_ = w.st.FinishReview(ctx, queueID, "done", parseErrMsg,
			projectID, docID, &versionID, "parse_failed", res.Fingerprint,
			"正文解析失败或内容不完整，原件与错误已保留，缺失字段不视为删除要求")
		w.maybeScheduleNext(ctx, projectID, docID)
		return
	}

	versionID, created, err := w.st.InsertVersion(ctx, docID, 0, res.Fingerprint, res.Body, true, false, "", detail.JSON())
	if err != nil {
		_ = w.st.RequeueReview(ctx, queueID, time.Now().Add(time.Minute), err.Error())
		return
	}
	result := "unchanged"
	note := "正文指纹未变化"
	if created {
		result = "changed"
		note = "原址正文发生变化（指纹不同），新版本已留存并生成待核实差异，未确认前不替换采用结论"
		if perr := w.proc.ProcessLatestVersion(ctx, docID, w.cfg.ReviewInterval); perr != nil {
			log.Printf("review process doc %d error: %v", docID, perr)
		}
	}
	_ = w.st.FinishReview(ctx, queueID, "done", "",
		projectID, docID, &versionID, result, res.Fingerprint, note)
	w.maybeScheduleNext(ctx, projectID, docID)
}

// maybeScheduleNext 仅对“近期项目”继续安排复查；超出窗口的长期项目不再自动复查，
// 页面只展示最后核查时间，不能声称仍有效。
func (w *Worker) maybeScheduleNext(ctx context.Context, projectID, docID int64) {
	updatedAtStr, err := w.st.ProjectUpdatedAt(ctx, projectID)
	if err != nil {
		return
	}
	updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return
	}
	if time.Since(updatedAt) > w.cfg.ReviewRecent {
		log.Printf("project %d 已超出近期窗口，停止自动复查，仅保留最后核查时间", projectID)
		return
	}
	if err := w.st.EnqueueReview(ctx, projectID, docID, time.Now().Add(w.cfg.ReviewInterval)); err != nil {
		log.Printf("re-enqueue review project %d doc %d error: %v", projectID, docID, err)
	}
}
