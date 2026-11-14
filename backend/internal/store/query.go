package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- 项目查询与时间线 ----------

func (s *Store) ListProjects(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.project_no, p.title, p.status, p.status_reason,
			p.last_checked_at, p.check_note,
			(SELECT COUNT(*) FROM packages pk WHERE pk.project_id=p.id) AS pkg_count,
			(SELECT COUNT(*) FROM adopted_revisions a WHERE a.project_id=p.id AND a.is_current) AS adopted_count
		FROM projects p ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, pkgCount, adoptedCount int64
		var projectNo *string
		var title, status, reason, note string
		var lastCheck *time.Time
		if err := rows.Scan(&id, &projectNo, &title, &status, &reason, &lastCheck, &note, &pkgCount, &adoptedCount); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "project_no": projectNo, "title": title, "status": status,
			"status_reason": reason, "last_checked_at": lastCheck, "check_note": note,
			"package_count": pkgCount, "adopted_count": adoptedCount,
		})
	}
	return out, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id int64) (map[string]any, error) {
	var projectNo *string
	var title, status, reason, note string
	var lastCheck *time.Time
	err := s.pool.QueryRow(ctx, `SELECT project_no, title, status, status_reason, last_checked_at, check_note
		FROM projects WHERE id=$1`, id).Scan(&projectNo, &title, &status, &reason, &lastCheck, &note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "project_no": projectNo, "title": title, "status": status,
		"status_reason": reason, "last_checked_at": lastCheck, "check_note": note}, nil
}

func (s *Store) ListPackages(ctx context.Context, projectID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, package_no, title FROM packages WHERE project_id=$1 ORDER BY package_no`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var no, title string
		if err := rows.Scan(&id, &no, &title); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "package_no": no, "title": title})
	}
	return out, rows.Err()
}

// ProjectTimeline 汇总：文档版本、采用修订、更正、复查记录，供前端呈现时间线与差异。
func (s *Store) ProjectTimeline(ctx context.Context, projectID int64) (map[string]any, error) {
	docs := []map[string]any{}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.source_id, d.source_ref, d.url, d.doc_type, d.first_seen_at, d.last_fetched_at,
			(SELECT count(*) FROM document_versions v WHERE v.document_id=d.id) AS version_count
		FROM documents d
		WHERE d.id IN (
			SELECT document_id FROM corrections WHERE project_id=$1
			UNION SELECT source_document_id FROM adopted_revisions WHERE project_id=$1
			UNION SELECT document_id FROM review_runs WHERE project_id=$1
			UNION SELECT document_id FROM link_candidates WHERE project_id=$1)
		ORDER BY d.first_seen_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sourceID, versionCount int64
		var ref, url, docType string
		var first, last time.Time
		if err := rows.Scan(&id, &sourceID, &ref, &url, &docType, &first, &last, &versionCount); err != nil {
			return nil, err
		}
		docs = append(docs, map[string]any{"id": id, "source_id": sourceID, "source_ref": ref,
			"url": url, "doc_type": docType, "first_seen_at": first, "last_fetched_at": last,
			"version_count": versionCount})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	revRows, err := s.pool.Query(ctx, `
		SELECT a.id, a.package_no, a.field_name, a.field_value, a.change_kind, a.is_current,
			a.adopted_at, a.source_document_id, a.source_version_id, a.correction_id
		FROM adopted_revisions a WHERE a.project_id=$1
		ORDER BY a.package_no, a.field_name, a.adopted_at, a.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer revRows.Close()
	revisions := []map[string]any{}
	for revRows.Next() {
		var id, docID, versionID int64
		var pkgNo, field, value, kind string
		var current bool
		var adoptedAt time.Time
		var corrID *int64
		if err := revRows.Scan(&id, &pkgNo, &field, &value, &kind, &current, &adoptedAt, &docID, &versionID, &corrID); err != nil {
			return nil, err
		}
		revisions = append(revisions, map[string]any{"id": id, "package_no": pkgNo, "field_name": field,
			"field_value": value, "change_kind": kind, "is_current": current, "adopted_at": adoptedAt,
			"source_document_id": docID, "source_version_id": versionID, "correction_id": corrID})
	}
	if err := revRows.Err(); err != nil {
		return nil, err
	}

	corrRows, err := s.pool.Query(ctx, `
		SELECT c.id, c.document_id, c.target_document_id, c.correction_no, c.scope_all_packages,
			c.scope_package_nos, c.status, c.contradictory_note, c.created_at
		FROM corrections c WHERE c.project_id=$1 ORDER BY c.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer corrRows.Close()
	corrections := []map[string]any{}
	for corrRows.Next() {
		var id, docID int64
		var target *int64
		var corrNo, status, note string
		var scopeAll bool
		var scopePkgs []byte
		var createdAt time.Time
		if err := corrRows.Scan(&id, &docID, &target, &corrNo, &scopeAll, &scopePkgs, &status, &note, &createdAt); err != nil {
			return nil, err
		}
		corrections = append(corrections, map[string]any{"id": id, "document_id": docID,
			"target_document_id": target, "correction_no": corrNo, "scope_all_packages": scopeAll,
			"scope_package_nos": rawJSON(scopePkgs), "status": status, "contradictory_note": note,
			"created_at": createdAt})
	}
	if err := corrRows.Err(); err != nil {
		return nil, err
	}

	runRows, err := s.pool.Query(ctx, `
		SELECT id, document_id, result, fingerprint, note, checked_at
		FROM review_runs WHERE project_id=$1 ORDER BY checked_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer runRows.Close()
	runs := []map[string]any{}
	for runRows.Next() {
		var id, docID int64
		var result, fp, note string
		var at time.Time
		if err := runRows.Scan(&id, &docID, &result, &fp, &note, &at); err != nil {
			return nil, err
		}
		runs = append(runs, map[string]any{"id": id, "document_id": docID, "result": result,
			"fingerprint": fp, "note": note, "checked_at": at})
	}
	if err := runRows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"documents": docs, "revisions": revisions,
		"corrections": corrections, "review_runs": runs}, nil
}

func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}
