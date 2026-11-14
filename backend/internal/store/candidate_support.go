package store

import (
	"context"
)

// HasHandledCandidate 文档是否已存在已处理（非 pending）的关联候选，
// 避免重复处理时再次生成待核实候选。
func (s *Store) HasHandledCandidate(ctx context.Context, docID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM link_candidates WHERE document_id=$1 AND status <> 'pending')`,
		docID).Scan(&exists)
	return exists, err
}

// CandidateProject 返回候选已确认的项目 ID（无则返回 ErrNoRows）。
func (s *Store) CandidateProject(ctx context.Context, docID int64) (int64, error) {
	var pid int64
	err := s.pool.QueryRow(ctx,
		`SELECT project_id FROM link_candidates WHERE document_id=$1 AND status='confirmed'
		 ORDER BY handled_at DESC NULLS LAST LIMIT 1`, docID).Scan(&pid)
	if err != nil {
		return 0, ErrNoRows
	}
	return pid, nil
}

// DocTypeOf 返回文档类型。
func (s *Store) DocTypeOf(ctx context.Context, docID int64) (string, error) {
	var t string
	err := s.pool.QueryRow(ctx, `SELECT doc_type FROM documents WHERE id=$1`, docID).Scan(&t)
	return t, err
}
