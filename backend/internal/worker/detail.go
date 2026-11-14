package worker

import (
	"context"
	"log"

	"procurewatch/internal/parser"
	"procurewatch/internal/store"
)

// detailLoop 处理详情下载；某份详情失败不丢失发现记录，也不阻塞其他来源。
func (w *Worker) detailLoop(ctx context.Context) {
	for {
		if !sleep(ctx, w.cfg.PollInterval) {
			return
		}
		token := w.newClaimToken()
		jobID, sourceID, ref, url, maxAttempts, err := w.st.ClaimDetailJob(ctx, token, w.cfg.LeaseDuration)
		if err == store.ErrNoRows {
			continue
		}
		if err != nil {
			log.Printf("detail claim error: %v", err)
			continue
		}
		w.runDetailJob(ctx, jobID, sourceID, ref, url, maxAttempts, token)
	}
}

func (w *Worker) runDetailJob(ctx context.Context, jobID, sourceID int64, ref, url string, maxAttempts int, token string) {
	src, err := w.st.GetSourceRow(ctx, sourceID)
	if err != nil {
		_ = w.st.MarkDetailFailed(ctx, jobID, "来源配置读取失败: "+err.Error(), false)
		return
	}
	res := w.fetch.Fetch(ctx, sourceID, src.IntervalMs, src.MaxConcurrency, src.MaxRetries, url)

	// 回写前校验租约：任务若已被重新领取，旧执行者回传一律拒绝，不能覆盖新结果
	ok, err := w.st.CheckDetailLease(ctx, jobID, token)
	if err != nil {
		log.Printf("detail job %d lease check error: %v", jobID, err)
		return
	}
	if !ok {
		log.Printf("detail job %d: 租约已失效（被重新领取），旧执行者结果丢弃", jobID)
		return
	}

	if !res.Accessible {
		// 失效链接记录为不可访问；明确撤标才能改变项目状态，这里不动项目状态
		if merr := w.st.MarkDetailFailed(ctx, jobID, res.Error, true); merr != nil {
			log.Printf("detail job %d ref %s: 标记不可访问失败: %v", jobID, ref, merr)
		}
		log.Printf("detail job %d ref %s: 不可访问（%s，尝试 %d 次）", jobID, ref, res.Error, res.Attempts)
		return
	}

	detail, truncated, parseErrMsg := parser.ParseDetail(res.Body)
	parsedOK := parseErrMsg == ""
	docType := detail.DocType
	if docType == "" {
		docType = "notice"
	}
	docID, err := w.st.UpsertDocument(ctx, sourceID, ref, url, docType)
	if err != nil {
		_ = w.st.MarkDetailFailed(ctx, jobID, err.Error(), false)
		return
	}
	payload := detail.JSON()
	versionID, created, err := w.st.InsertVersion(ctx, docID, jobID, res.Fingerprint, res.Body,
		parsedOK, truncated, parseErrMsg, payload)
	if err != nil {
		_ = w.st.MarkDetailFailed(ctx, jobID, err.Error(), false)
		return
	}
	if err := w.st.MarkDetailSucceeded(ctx, jobID); err != nil {
		log.Printf("detail job %d mark succeeded error: %v", jobID, err)
		return
	}
	log.Printf("detail job %d ref %s: 版本 %d 已留存（新版本=%v，解析成功=%v，截断=%v）",
		jobID, ref, versionID, created, parsedOK, truncated)

	// 业务处理独立于下载成败：半份/解析失败只留存原件与错误，不产生采用结论
	if err := w.proc.ProcessLatestVersion(ctx, docID, w.cfg.ReviewInterval); err != nil {
		log.Printf("process doc %d error: %v", docID, err)
	}
}
