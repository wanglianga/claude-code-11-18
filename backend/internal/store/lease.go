package store

import (
	"context"
)

// CheckDetailLease 详情作业回写前校验租约仍属于当前执行者；
// 中断后任务被重新领取时 worker_id 已变更，旧执行者回传将被拒绝，不能覆盖新结果。
func (s *Store) CheckDetailLease(ctx context.Context, jobID int64, token string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM detail_jobs WHERE id=$1 AND worker_id=$2 AND lease_until > now())`,
		jobID, token).Scan(&ok)
	return ok, err
}

func (s *Store) CheckReviewLease(ctx context.Context, queueID int64, token string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM review_queue WHERE id=$1 AND worker_id=$2 AND lease_until > now())`,
		queueID, token).Scan(&ok)
	return ok, err
}

func (s *Store) DetailJobSource(ctx context.Context, jobID int64) (sourceID int64, ref, url string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT source_id, source_ref, url FROM detail_jobs WHERE id=$1`, jobID).Scan(&sourceID, &ref, &url)
	return
}

func (s *Store) ReviewTarget(ctx context.Context, queueID int64) (projectID, docID int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT project_id, document_id FROM review_queue WHERE id=$1`, queueID).Scan(&projectID, &docID)
	return
}

func (s *Store) ProjectUpdatedAt(ctx context.Context, projectID int64) (string, error) {
	var ts string
	err := s.pool.QueryRow(ctx,
		`SELECT to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') FROM projects WHERE id=$1`, projectID).Scan(&ts)
	return ts, err
}
