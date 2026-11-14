package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// LatestVersionPayload 返回文档最新版本的正文与解析结果。
func (s *Store) LatestVersionPayload(ctx context.Context, docID int64) (
	raw string, payload []byte, parsedOK, truncated bool, parseErr string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT raw_html, parsed_payload, parsed_ok, truncated, parse_error
		FROM document_versions WHERE document_id=$1
		ORDER BY id DESC LIMIT 1`, docID).
		Scan(&raw, &payload, &parsedOK, &truncated, &parseErr)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, false, false, "", ErrNoRows
	}
	return
}

func (s *Store) LatestVersionID(ctx context.Context, docID int64) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM document_versions WHERE document_id=$1 ORDER BY id DESC LIMIT 1`, docID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoRows
	}
	return id, err
}

func (s *Store) DocumentSource(ctx context.Context, docID int64) (int64, error) {
	var sourceID int64
	err := s.pool.QueryRow(ctx, `SELECT source_id FROM documents WHERE id=$1`, docID).Scan(&sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoRows
	}
	return sourceID, err
}

func (s *Store) FindDocumentIDByRef(ctx context.Context, sourceID int64, ref string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM documents WHERE source_id=$1 AND source_ref=$2`, sourceID, ref).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoRows
	}
	return id, err
}

func (s *Store) AttachProcessNote(ctx context.Context, docID int64, note string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE documents SET process_note=$2 WHERE id=$1`, docID, note)
	return err
}

// FindConflictingFieldChange 判断同一项目+标包+字段是否已存在取值不同且仍有效的更正字段。
func (s *Store) FindConflictingFieldChange(ctx context.Context, projectID int64, pkg, field, newValue string, excludeCorrID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM correction_field_changes fc
			JOIN corrections c ON c.id=fc.correction_id
			WHERE c.project_id=$1 AND fc.package_no=$2 AND fc.field_name=$3
			  AND fc.new_value <> $4 AND fc.decision <> 'returned'
			  AND c.status <> 'superseded' AND fc.correction_id <> $5)`,
		projectID, pkg, field, newValue, excludeCorrID).Scan(&exists)
	return exists, err
}

// SupersedeCorrectionByDoc 将被取消通知引用的原更正标记为 superseded，但其字段内容与历史采用修订保留。
func (s *Store) SupersedeCorrectionByDoc(ctx context.Context, targetDocID, cancelDocID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE corrections SET status='superseded'
		 WHERE document_id=$1 AND status <> 'superseded'`, targetDocID)
	return err
}
