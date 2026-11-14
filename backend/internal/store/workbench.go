package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- 待核实：关联候选 ----------

func (s *Store) ListLinkCandidates(ctx context.Context, status string) ([]map[string]any, error) {
	q := `
		SELECT lc.id, lc.document_id, lc.project_id, lc.project_no_hint, lc.reason, lc.status,
			lc.note, lc.created_at, d.source_id, d.source_ref, d.url, d.doc_type,
			COALESCE(di.title,'') AS title
		FROM link_candidates lc
		JOIN documents d ON d.id=lc.document_id
		LEFT JOIN discovery_items di ON di.source_id=d.source_id AND di.source_ref=d.source_ref`
	args := []any{}
	if status != "" {
		q += ` WHERE lc.status=$1 ORDER BY lc.id DESC`
		args = append(args, status)
	} else {
		q += ` ORDER BY lc.id DESC`
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, docID, sourceID int64
		var projectID *int64
		var hint, reason, st, note, ref, url, docType, title string
		var createdAt time.Time
		if err := rows.Scan(&id, &docID, &projectID, &hint, &reason, &st, &note, &createdAt,
			&sourceID, &ref, &url, &docType, &title); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "document_id": docID, "project_id": projectID,
			"project_no_hint": hint, "reason": reason, "status": st, "note": note, "created_at": createdAt,
			"source_id": sourceID, "source_ref": ref, "url": url, "doc_type": docType, "title": title})
	}
	return out, rows.Err()
}

// ResolveLinkCandidate 人工确认关联（可指定/新建项目）、退回补查或拒绝。
func (s *Store) ResolveLinkCandidate(ctx context.Context, id, userID int64, status string, projectID *int64, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var oldOpen int
	if err := tx.QueryRow(ctx,
		`SELECT 1 FROM link_candidates WHERE id=$1 AND status='pending' FOR UPDATE`, id).Scan(&oldOpen); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoRows
		}
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE link_candidates SET status=$2, project_id=COALESCE($3, project_id),
			handled_by=$4, handled_at=now(), note=$5 WHERE id=$1`,
		id, status, projectID, userID, note); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetLinkCandidate(ctx context.Context, id int64) (docID int64, projectID *int64, status string, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT document_id, project_id, status FROM link_candidates WHERE id=$1`, id).Scan(&docID, &projectID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, "", ErrNoRows
	}
	return
}

// ---------- 待核实：更正与逐字段处置 ----------

func (s *Store) ListCorrections(ctx context.Context, status string) ([]map[string]any, error) {
	q := `
		SELECT c.id, c.document_id, c.project_id, c.target_document_id, c.correction_no,
			c.scope_all_packages, c.scope_package_nos, c.status, c.contradictory_note, c.created_at,
			d.source_id, d.source_ref, d.url, COALESCE(di.title,'')
		FROM corrections c
		JOIN documents d ON d.id=c.document_id
		LEFT JOIN discovery_items di ON di.source_id=d.source_id AND di.source_ref=d.source_ref`
	args := []any{}
	if status != "" {
		// 待办视图：pending 同时纳入矛盾待核实
		statuses := []string{}
		for _, x := range strings.Split(status, ",") {
			x = strings.TrimSpace(x)
			if x == "" {
				continue
			}
			if x == "pending" {
				statuses = append(statuses, "pending", "contradictory")
			} else {
				statuses = append(statuses, x)
			}
		}
		if len(statuses) > 0 {
			q += ` WHERE c.status = ANY($1)`
			args = append(args, statuses)
		}
	}
	q += ` ORDER BY c.id DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, docID, sourceID int64
		var projectID, target *int64
		var corrNo, st, note, ref, url, title string
		var scopeAll bool
		var scopePkgs []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &docID, &projectID, &target, &corrNo, &scopeAll, &scopePkgs, &st,
			&note, &createdAt, &sourceID, &ref, &url, &title); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "document_id": docID, "project_id": projectID,
			"target_document_id": target, "correction_no": corrNo, "scope_all_packages": scopeAll,
			"scope_package_nos": rawJSON(scopePkgs), "status": st, "contradictory_note": note,
			"created_at": createdAt, "source_id": sourceID, "source_ref": ref, "url": url, "title": title})
	}
	return out, rows.Err()
}

// CorrectionDetail 返回更正头信息与字段变更（并列快照旧值、新值、原文位置与片段）。
func (s *Store) CorrectionDetail(ctx context.Context, corrID int64) (map[string]any, []map[string]any, error) {
	var docID int64
	var projectID, target *int64
	var corrNo, st, note string
	var scopeAll bool
	var scopePkgs []byte
	err := s.pool.QueryRow(ctx, `
		SELECT document_id, project_id, target_document_id, correction_no, scope_all_packages,
			scope_package_nos, status, contradictory_note
		FROM corrections WHERE id=$1`, corrID).
		Scan(&docID, &projectID, &target, &corrNo, &scopeAll, &scopePkgs, &st, &note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNoRows
	}
	if err != nil {
		return nil, nil, err
	}
	head := map[string]any{"id": corrID, "document_id": docID, "project_id": projectID,
		"target_document_id": target, "correction_no": corrNo, "scope_all_packages": scopeAll,
		"scope_package_nos": rawJSON(scopePkgs), "status": st, "contradictory_note": note}

	rows, err := s.pool.Query(ctx, `
		SELECT fc.id, fc.field_name, fc.package_no, fc.old_value, fc.new_value,
			fc.source_locator, fc.source_excerpt, fc.decision
		FROM correction_field_changes fc WHERE fc.correction_id=$1
		ORDER BY fc.package_no, fc.field_name`, corrID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	changes := []map[string]any{}
	for rows.Next() {
		var id int64
		var field, pkg, oldV, newV, locator, excerpt, decision string
		if err := rows.Scan(&id, &field, &pkg, &oldV, &newV, &locator, &excerpt, &decision); err != nil {
			return nil, nil, err
		}
		changes = append(changes, map[string]any{"id": id, "field_name": field, "package_no": pkg,
			"old_value": oldV, "new_value": newV, "source_locator": locator,
			"source_excerpt": excerpt, "decision": decision})
	}
	return head, changes, rows.Err()
}

func (s *Store) GetCorrection(ctx context.Context, corrID int64) (docID int64, projectID *int64, status string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT document_id, project_id, status FROM corrections WHERE id=$1`, corrID).
		Scan(&docID, &projectID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, "", ErrNoRows
	}
	return
}

// SetFieldDecision 逐字段确认或退回补查。
func (s *Store) SetFieldDecision(ctx context.Context, corrID int64, field, pkg, decision string, userID int64) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE correction_field_changes SET decision=$3, decided_by=$4, decided_at=now()
		WHERE correction_id=$1 AND field_name=$2 AND package_no=$5`,
		corrID, field, decision, userID, pkg)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNoRows
	}
	return nil
}

// RefreshCorrectionStatus 依据字段处置结果汇总更正状态；未确认内容不替换当前采用结论。
func (s *Store) RefreshCorrectionStatus(ctx context.Context, corrID int64, userID int64) error {
	var total, confirmed, returned int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*),
			count(*) FILTER (WHERE decision='confirmed'),
			count(*) FILTER (WHERE decision='returned')
		FROM correction_field_changes WHERE correction_id=$1`, corrID).
		Scan(&total, &confirmed, &returned); err != nil {
		return err
	}
	newStatus := ""
	switch {
	case total == 0:
		return nil
	case confirmed == total:
		newStatus = "confirmed"
	case returned == total:
		newStatus = "returned"
	default:
		newStatus = "pending" // 仍有字段待处理
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE corrections SET status=$2, handled_by=$3, handled_at=now(),
			contradictory_note='' WHERE id=$1`, corrID, newStatus, userID)
	return err
}

func (s *Store) SnapshotOldValues(ctx context.Context, corrID, projectID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE correction_field_changes fc SET old_value = COALESCE(
			(SELECT a.field_value FROM adopted_revisions a
			 WHERE a.project_id=$2 AND a.package_no=fc.package_no
			   AND a.field_name=fc.field_name AND a.is_current=TRUE), '')
		WHERE fc.correction_id=$1 AND fc.old_value=''`, corrID, projectID)
	return err
}

// ---------- 导出 ----------

// ExportSnapshot 导出当前采用结论及其所用公告修订，并在导出前复核撤回/不可访问/待核实事项。
func (s *Store) ExportSnapshot(ctx context.Context, projectID, userID int64) (map[string]any, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.package_no, a.field_name, a.field_value, a.change_kind, a.adopted_at,
			a.source_document_id, a.source_version_id, a.correction_id,
			d.source_ref, d.url, v.fingerprint
		FROM adopted_revisions a
		JOIN documents d ON d.id=a.source_document_id
		JOIN document_versions v ON v.id=a.source_version_id
		WHERE a.project_id=$1 AND a.is_current=TRUE
		ORDER BY a.package_no, a.field_name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	docIDs := map[int64]bool{}
	for rows.Next() {
		var id, docID, versionID int64
		var corrID *int64
		var pkg, field, value, kind, ref, url, fp string
		var adoptedAt time.Time
		if err := rows.Scan(&id, &pkg, &field, &value, &kind, &adoptedAt, &docID, &versionID, &corrID,
			&ref, &url, &fp); err != nil {
			return nil, err
		}
		docIDs[docID] = true
		items = append(items, map[string]any{"package_no": pkg, "field_name": field,
			"field_value": value, "change_kind": kind, "adopted_at": adoptedAt,
			"source_ref": ref, "source_url": url, "source_version_id": versionID,
			"source_fingerprint": fp, "correction_id": corrID})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 导出前核验：所用公告最近一次复查结论
	warnings := []string{}
	if project["status"] == "withdrawn" {
		warnings = append(warnings, "项目已被明确撤标（"+asString(project["status_reason"])+"），不得作为有效采购执行")
	}
	for docID := range docIDs {
		var result, note string
		err := s.pool.QueryRow(ctx, `
			SELECT result, note FROM review_runs WHERE document_id=$1
			ORDER BY checked_at DESC, id DESC LIMIT 1`, docID).Scan(&result, &note)
		if err == nil {
			if result == "inaccessible" {
				warnings = append(warnings, fmt.Sprintf("所用公告文档 %d 最近核查为不可访问，需人工复核后再执行", docID))
			}
			if result == "parse_failed" {
				warnings = append(warnings, fmt.Sprintf("所用公告文档 %d 最近核查解析失败，原件已留存", docID))
			}
		}
	}
	var pendingCorr int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM corrections WHERE project_id=$1 AND status IN ('pending','contradictory')`,
		projectID).Scan(&pendingCorr); err != nil {
		return nil, err
	}
	if pendingCorr > 0 {
		warnings = append(warnings, fmt.Sprintf("存在 %d 份未处理完的更正/矛盾通知，导出值仅含已确认的采用结论", pendingCorr))
	}
	verifyNote := "导出前核验通过：所用公告未发现撤标或不可访问记录"
	if len(warnings) > 0 {
		verifyNote = "导出前核验存在提示"
	}
	payload := map[string]any{"project": project, "items": items, "warnings": warnings}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO export_runs(project_id, requested_by, verify_note, payload) VALUES($1,$2,$3,$4)`,
		projectID, userID, verifyNote, mustJSON(payload))
	if err != nil {
		return nil, err
	}
	payload["verify_note"] = verifyNote
	return payload, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
