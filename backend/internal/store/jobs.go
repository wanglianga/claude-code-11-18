package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- list jobs ----------

func (s *Store) EnqueueListJob(ctx context.Context, sourceID int64, kind string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`INSERT INTO list_jobs(source_id, kind, status) VALUES($1,$2,'pending')
		 ON CONFLICT DO NOTHING RETURNING id`, sourceID, kind).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// 已有未完成作业：复用
		err = s.pool.QueryRow(ctx,
			`SELECT id FROM list_jobs WHERE source_id=$1 AND status IN ('pending','running')`, sourceID).Scan(&id)
	}
	return id, err
}

// ClaimListJob 领取或重新领取过期列表作业。重新领取会递增 epoch，使旧执行者的回写失效。
// 仅返回 enabled=TRUE 来源的作业（暂停来源停止新领取）。
func (s *Store) ClaimListJob(ctx context.Context, workerID string, lease time.Duration) (int64, int64, int, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE list_jobs j SET status='running', worker_id=$1, lease_until=now()+$2::interval,
			epoch = CASE WHEN j.status='running' THEN j.epoch+1 ELSE j.epoch END,
			started_at = COALESCE(j.started_at, now())
		FROM sources src
		WHERE j.source_id=src.id AND src.enabled=TRUE
		  AND j.id = (
			SELECT j2.id FROM list_jobs j2 JOIN sources s2 ON s2.id=j2.source_id
			WHERE s2.enabled=TRUE AND (j2.status='pending'
				OR (j2.status='running' AND j2.lease_until < now()))
			ORDER BY j2.id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING j.id, j.source_id, j.epoch`, workerID, lease)
	var id, sourceID int64
	var epoch int
	err := row.Scan(&id, &sourceID, &epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, 0, ErrNoRows
	}
	return id, sourceID, epoch, err
}

func (s *Store) GetListJobEpoch(ctx context.Context, jobID int64) (int, string, error) {
	var epoch int
	var worker string
	err := s.pool.QueryRow(ctx, `SELECT epoch, worker_id FROM list_jobs WHERE id=$1`, jobID).Scan(&epoch, &worker)
	return epoch, worker, err
}

// SaveListProgress 仅在 epoch 未变化时写入断点；旧执行者回写被拒绝。
func (s *Store) SaveListProgress(ctx context.Context, jobID int64, epoch int, offset []byte) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE list_jobs SET last_offset=$3 WHERE id=$1 AND epoch=$2`, jobID, epoch, string(offset))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errors.New("list job epoch changed: stale worker write rejected")
	}
	return nil
}

func (s *Store) FinishListJob(ctx context.Context, jobID int64, epoch int, status, errMsg string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE list_jobs SET status=$3, error=$4, finished_at=now(), lease_until=NULL
		WHERE id=$1 AND epoch=$2`, jobID, epoch, status, errMsg)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errors.New("list job epoch changed: stale worker finish rejected")
	}
	return nil
}

// ---------- discovery & detail jobs ----------

// UpsertDiscovery 发现即保存；返回是否为新发现及 discovery id。详情失败不会删除该记录。
func (s *Store) UpsertDiscovery(ctx context.Context, sourceID int64, ref, url, title string) (bool, int64, error) {
	var id int64
	var inserted bool
	err := s.pool.QueryRow(ctx, `
		INSERT INTO discovery_items(source_id, source_ref, url, title)
		VALUES($1,$2,$3,$4)
		ON CONFLICT (source_id, source_ref)
		DO UPDATE SET title=EXCLUDED.title
		RETURNING id, (xmax = 0) AS inserted`, sourceID, ref, url, title).Scan(&id, &inserted)
	if err != nil {
		return false, 0, err
	}
	return inserted, id, nil
}

// EnqueueDetailForDiscovery 为已发现但未入队的条目创建详情作业（列表与详情分开排队）。
func (s *Store) EnqueueDetailForDiscovery(ctx context.Context, sourceID int64) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO detail_jobs(source_id, source_ref, url, status)
		SELECT d.source_id, d.source_ref, d.url, 'pending'
		FROM discovery_items d
		WHERE d.source_id=$1 AND d.detail_status='pending'
		  AND NOT EXISTS (SELECT 1 FROM detail_jobs j WHERE j.source_id=d.source_id AND j.source_ref=d.source_ref)`,
		sourceID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) EnqueueManualDetail(ctx context.Context, sourceID int64, ref string) error {
	var url string
	err := s.pool.QueryRow(ctx, `SELECT url FROM discovery_items WHERE source_id=$1 AND source_ref=$2`,
		sourceID, ref).Scan(&url)
	if err != nil {
		return err
	}
	// 手动补抓同样入队等待，受站点并发/间隔限制，无法绕开限额。
	_, err = s.pool.Exec(ctx, `
		INSERT INTO detail_jobs(source_id, source_ref, url, status, manual, max_attempts)
		VALUES($1,$2,$3,'pending',TRUE,
			(SELECT max_retries FROM sources WHERE id=$1))
		ON CONFLICT (source_id, source_ref)
		DO UPDATE SET status=CASE WHEN detail_jobs.status='running' THEN detail_jobs.status ELSE 'pending' END,
			manual=TRUE, last_error=''`, sourceID, ref, url)
	return err
}

// ClaimDetailJob 领取详情作业，受来源 enabled 与每来源在途并发上限约束。
func (s *Store) ClaimDetailJob(ctx context.Context, workerID string, lease time.Duration) (int64, int64, string, string, int, error) {
	row := s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT j.id FROM detail_jobs j
			JOIN sources s ON s.id=j.source_id
			WHERE s.enabled=TRUE AND (j.status='pending'
				OR (j.status IN ('running','failed') AND j.lease_until < now()))
			  AND (SELECT COUNT(*) FROM detail_jobs r
			       WHERE r.source_id=j.source_id AND r.status='running' AND r.lease_until > now())
			      < s.max_concurrency
			ORDER BY j.id LIMIT 1 FOR UPDATE SKIP LOCKED)
		UPDATE detail_jobs j SET status='running', worker_id=$1, lease_until=now()+$2::interval,
			attempts=j.attempts+1
		WHERE j.id=(SELECT id FROM candidate)
		RETURNING j.id, j.source_id, j.source_ref, j.url, j.max_attempts`, workerID, lease)
	var id, sourceID int64
	var ref, url string
	var maxAttempts int
	err := row.Scan(&id, &sourceID, &ref, &url, &maxAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, "", "", 0, ErrNoRows
	}
	return id, sourceID, ref, url, maxAttempts, err
}

func (s *Store) MarkDetailFailed(ctx context.Context, jobID int64, errMsg string, inaccessible bool) error {
	detailStatus := "failed"
	if inaccessible {
		detailStatus = "inaccessible"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if inaccessible {
		if _, err := tx.Exec(ctx,
			`UPDATE detail_jobs SET status='inaccessible', last_error=$2, lease_until=NULL, updated_at=now() WHERE id=$1`,
			jobID, errMsg); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx,
			`UPDATE detail_jobs SET status=CASE WHEN attempts>=max_attempts THEN 'failed' ELSE 'pending' END,
				last_error=$2, lease_until=NULL, updated_at=now() WHERE id=$1`, jobID, errMsg); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE discovery_items d SET detail_status=$2
		FROM detail_jobs j WHERE j.id=$1 AND d.source_id=j.source_id AND d.source_ref=j.source_ref`,
		jobID, detailStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkDetailSucceeded(ctx context.Context, jobID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE detail_jobs SET status='succeeded', last_error='', lease_until=NULL, updated_at=now() WHERE id=$1`,
		jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE discovery_items d SET detail_status='done'
		FROM detail_jobs j WHERE j.id=$1 AND d.source_id=j.source_id AND d.source_ref=j.source_ref`, jobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
