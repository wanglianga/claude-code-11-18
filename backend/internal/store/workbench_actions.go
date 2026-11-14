package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetFieldChange 读取单条字段变更（确认采用时取新值与适用标包）。
func (s *Store) GetFieldChange(ctx context.Context, corrID int64, field, pkg string) (
	newValue, oldValue string, decision string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT new_value, old_value, decision FROM correction_field_changes
		WHERE correction_id=$1 AND field_name=$2 AND package_no=$3`,
		corrID, field, pkg).Scan(&newValue, &oldValue, &decision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNoRows
	}
	return
}

// CorrectionDocument 返回更正关联的文档 ID。
func (s *Store) CorrectionDocument(ctx context.Context, corrID int64) (int64, error) {
	var docID int64
	err := s.pool.QueryRow(ctx, `SELECT document_id FROM corrections WHERE id=$1`, corrID).Scan(&docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoRows
	}
	return docID, err
}

// ReturnCorrection 整份更正退回补查：字段处置全部置为 returned。
func (s *Store) ReturnCorrection(ctx context.Context, corrID, userID int64, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE correction_field_changes SET decision='returned', decided_by=$2, decided_at=now()
		WHERE correction_id=$1`, corrID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE corrections SET status='returned', handled_by=$2, handled_at=now(),
			contradictory_note=CASE WHEN $3='' THEN contradictory_note ELSE $3 END
		WHERE id=$1`, corrID, userID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
