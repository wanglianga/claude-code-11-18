package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoRows = errors.New("no rows")

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool}}

func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---------- sources ----------

func (s *Store) ListSources(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name, base_url, list_mode, list_path, detail_path_tpl,
		interval_ms, max_concurrency, max_retries, enabled, end_marker, cursor_state, list_status, last_error, updated_at
		FROM sources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, intervalMs, maxConc, maxRetry int64
			code, name, baseURL, mode, listPath, tpl, marker, listStatus, lastErr string
			enabled   bool
			cursor    []byte
			updatedAt any
		)
		if err := rows.Scan(&id, &code, &name, &baseURL, &mode, &listPath, &tpl, &intervalMs, &maxConc, &maxRetry,
			&enabled, &marker, &cursor, &listStatus, &lastErr, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "code": code, "name": name, "base_url": baseURL, "list_mode": mode,
			"list_path": listPath, "detail_path_tpl": tpl, "interval_ms": intervalMs,
			"max_concurrency": maxConc, "max_retries": maxRetry, "enabled": enabled,
			"end_marker": marker, "cursor_state": json.RawMessage(cursor),
			"list_status": listStatus, "last_error": lastErr, "updated_at": updatedAt,
		})
	}
	return out, rows.Err()
}

func (s *Store) GetSource(ctx context.Context, id int64) (map[string]any, error) {
	var (
		intervalMs, maxConc, maxRetry int64
		code, name, baseURL, mode, listPath, tpl, marker, listStatus, lastErr string
		enabled bool
		cursor  []byte
	)
	err := s.pool.QueryRow(ctx, `SELECT code, name, base_url, list_mode, list_path, detail_path_tpl,
		interval_ms, max_concurrency, max_retries, enabled, end_marker, cursor_state, list_status, last_error
		FROM sources WHERE id=$1`, id).Scan(&code, &name, &baseURL, &mode, &listPath, &tpl,
		&intervalMs, &maxConc, &maxRetry, &enabled, &marker, &cursor, &listStatus, &lastErr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "code": code, "name": name, "base_url": baseURL, "list_mode": mode,
		"list_path": listPath, "detail_path_tpl": tpl, "interval_ms": intervalMs,
		"max_concurrency": maxConc, "max_retries": maxRetry, "enabled": enabled,
		"end_marker": marker, "cursor_state": json.RawMessage(cursor),
		"list_status": listStatus, "last_error": lastErr,
	}, nil
}

// SetSourceEnabled 暂停来源仅停止新领取；不动正在执行的任务租约。
func (s *Store) SetSourceEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE sources SET enabled=$2, updated_at=now() WHERE id=$1`, id, enabled)
	return err
}

func (s *Store) UpdateSourceCursor(ctx context.Context, id int64, state []byte, status, lastErr string) error {
	_, err := s.pool.Exec(ctx, `UPDATE sources SET cursor_state=$2, list_status=$3, last_error=$4, updated_at=now() WHERE id=$1`,
		id, state, status, lastErr)
	return err
}

// SourceCounters 管理员视角：列表已发现、详情待取、待复查数量。
func (s *Store) SourceCounters(ctx context.Context) (map[int64]map[string]int64, error) {
	out := map[int64]map[string]int64{}
	rows, err := s.pool.Query(ctx, `
		SELECT d.source_id,
			COUNT(*) FILTER (WHERE d.detail_status='done') AS done,
			COUNT(*) FILTER (WHERE d.detail_status IN ('pending','failed','inaccessible')) AS pending_detail,
			(SELECT COUNT(*) FROM detail_jobs j WHERE j.source_id=d.source_id AND j.status='pending') AS queued,
			(SELECT COUNT(*) FROM review_queue rq JOIN documents doc ON doc.id=rq.document_id
				WHERE doc.source_id=d.source_id AND rq.status='pending') AS review_pending
		FROM discovery_items d GROUP BY d.source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid, done, pending, queued, review int64
		if err := rows.Scan(&sid, &done, &pending, &queued, &review); err != nil {
			return nil, err
		}
		out[sid] = map[string]int64{"detail_done": done, "detail_pending": pending,
			"detail_queued": queued, "review_pending": review}
	}
	return out, rows.Err()
}

// ---------- users ----------

func (s *Store) GetUserByUsername(ctx context.Context, username string) (id int64, hash, displayName, role string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id, password_hash, display_name, role FROM users WHERE username=$1`, username).
		Scan(&id, &hash, &displayName, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", "", "", ErrNoRows
	}
	return
}

func (s *Store) GetUser(ctx context.Context, id int64) (username, displayName, role string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT username, display_name, role FROM users WHERE id=$1`, id).
		Scan(&username, &displayName, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNoRows
	}
	return
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("json marshal: %w", err))
	}
	return b
}
