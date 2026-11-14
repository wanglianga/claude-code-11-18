package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- review queue ----------

// EnqueueReview 为项目文档安排复查（到期时间 dueAt）。开放记录唯一，避免重复排队。
func (s *Store) EnqueueReview(ctx context.Context, projectID, docID int64, dueAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO review_queue(project_id, document_id, due_at)
		VALUES($1,$2,$3)
		ON CONFLICT DO NOTHING`, projectID, docID, dueAt)
	return err
}

func (s *Store) ClaimReview(ctx context.Context, workerID string, lease time.Duration) (int64, int64, int64, string, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE review_queue SET status='running', worker_id=$1, lease_until=now()+$2::interval, attempts=attempts+1
		WHERE id = (
			SELECT id FROM review_queue
			WHERE status='pending' OR (status='running' AND lease_until < now())
			ORDER BY due_at LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING id, project_id, document_id, status`, workerID, lease)
	var id, projectID, docID int64
	var status string
	err := row.Scan(&id, &projectID, &docID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, 0, "", ErrNoRows
	}
	return id, projectID, docID, status, err
}

func (s *Store) FinishReview(ctx context.Context, queueID int64, status, errMsg string,
	projectID, docID int64, versionID *int64, result, fingerprint, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE review_queue SET status=$2, last_error=$3, lease_until=NULL, finished_at=now() WHERE id=$1`,
		queueID, status, errMsg); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO review_runs(project_id, document_id, version_id, result, fingerprint, note)
		VALUES($1,$2,$3,$4,$5,$6)`, projectID, docID, versionID, result, fingerprint, note); err != nil {
		return err
	}
	checkNote := ""
	if result == "inaccessible" {
		checkNote = "最近复查：链接不可访问（不等于撤标，项目状态保持不变）"
	} else if result == "changed" {
		checkNote = "最近复查：正文发生变化，已留存新版本，待采购专员核实"
	} else if result == "parse_failed" {
		checkNote = "最近复查：正文解析失败，原件已保留"
	} else {
		checkNote = "最近复查：正文未变化"
	}
	if _, err := tx.Exec(ctx,
		`UPDATE projects SET last_checked_at=now(), check_note=$2 WHERE id=$1`, projectID, checkNote); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RequeueReview 复查失败后重新排队（带退避）。
func (s *Store) RequeueReview(ctx context.Context, queueID int64, dueAt time.Time, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE review_queue SET status='pending', worker_id='', lease_until=NULL, last_error=$3, due_at=$2 WHERE id=$1`,
		queueID, dueAt, errMsg)
	return err
}
