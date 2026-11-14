package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// SourceRow 是 worker 调度所需的来源配置只读视图。
type SourceRow struct {
	ID             int64
	Code           string
	BaseURL        string
	ListMode       string
	ListPath       string
	DetailPathTpl  string
	IntervalMs     int
	MaxConcurrency int
	MaxRetries     int
	EndMarker      string
	CursorState    []byte
}

func (s *Store) GetSourceRow(ctx context.Context, id int64) (SourceRow, error) {
	var r SourceRow
	err := s.pool.QueryRow(ctx, `SELECT id, code, base_url, list_mode, list_path, detail_path_tpl,
		interval_ms, max_concurrency, max_retries, end_marker, cursor_state
		FROM sources WHERE id=$1`, id).
		Scan(&r.ID, &r.Code, &r.BaseURL, &r.ListMode, &r.ListPath, &r.DetailPathTpl,
			&r.IntervalMs, &r.MaxConcurrency, &r.MaxRetries, &r.EndMarker, &r.CursorState)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNoRows
	}
	return r, err
}

// HeartbeatListJob 列表作业续租；epoch 已变（被重新领取）时返回错误，旧执行者应停止。
func (s *Store) HeartbeatListJob(ctx context.Context, jobID int64, epoch int, lease time.Duration) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE list_jobs SET lease_until=now()+$3::interval WHERE id=$1 AND epoch=$2 AND status='running'`,
		jobID, epoch, lease)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errors.New("list job epoch changed: heartbeat rejected")
	}
	return nil
}

// AllEnabledSourceIDs 返回启用中的来源，供定时触发全量列表作业。
func (s *Store) AllEnabledSourceIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM sources WHERE enabled=TRUE ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
