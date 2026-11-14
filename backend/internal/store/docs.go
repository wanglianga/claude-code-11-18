package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------- documents & versions ----------

// UpsertDocument 获取文档行（不存在则创建），更新最后抓取时间。
func (s *Store) UpsertDocument(ctx context.Context, sourceID int64, ref, url, docType string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO documents(source_id, source_ref, url, doc_type)
		VALUES($1,$2,$3,$4)
		ON CONFLICT (source_id, source_ref)
		DO UPDATE SET last_fetched_at=now(),
			doc_type=CASE WHEN $4='notice' THEN documents.doc_type ELSE $4 END
		RETURNING id`, sourceID, ref, url, docType).Scan(&id)
	return id, err
}

// InsertVersion 保留每次正文与指纹；同指纹版本已存在则返回其 ID 与 false。
// 解析失败/截断也落库（parsed_ok=false + parse_error），原件不丢。
func (s *Store) InsertVersion(ctx context.Context, docID, fetchRunID int64, fingerprint, rawHTML string,
	parsedOK, truncated bool, parseErr string, payload []byte) (int64, bool, error) {
	var id int64
	var created bool
	err := s.pool.QueryRow(ctx, `
		INSERT INTO document_versions(document_id, fetch_run_id, fingerprint, raw_html, parsed_ok,
			parse_error, truncated, parsed_payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE(NULLIF($8,'')::jsonb,'{}'::jsonb))
		ON CONFLICT (document_id, fingerprint)
		DO UPDATE SET fetch_run_id=EXCLUDED.fetch_run_id
		RETURNING id, (xmax = 0) AS created`,
		docID, fetchRunID, fingerprint, rawHTML, parsedOK, parseErr, truncated, string(payload)).Scan(&id, &created)
	if err != nil {
		return 0, false, err
	}
	return id, created, nil
}

func (s *Store) GetDocumentByRef(ctx context.Context, sourceID int64, ref string) (int64, string, error) {
	var id int64
	var docType string
	err := s.pool.QueryRow(ctx, `SELECT id, doc_type FROM documents WHERE source_id=$1 AND source_ref=$2`,
		sourceID, ref).Scan(&id, &docType)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrNoRows
	}
	return id, docType, err
}

func (s *Store) ListVersions(ctx context.Context, docID int64) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, fingerprint, parsed_ok, parse_error, truncated, fetched_at,
		length(raw_html) AS raw_len FROM document_versions WHERE document_id=$1 ORDER BY id`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var fp, parseErr string
		var ok, truncated bool
		var fetchedAt time.Time
		var rawLen int64
		if err := rows.Scan(&id, &fp, &ok, &parseErr, &truncated, &fetchedAt, &rawLen); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "fingerprint": fp, "parsed_ok": ok,
			"parse_error": parseErr, "truncated": truncated, "fetched_at": fetchedAt, "raw_len": rawLen})
	}
	return out, rows.Err()
}

func (s *Store) GetVersionRaw(ctx context.Context, versionID int64) (string, error) {
	var raw string
	err := s.pool.QueryRow(ctx, `SELECT raw_html FROM document_versions WHERE id=$1`, versionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoRows
	}
	return raw, err
}

func (s *Store) GetDocumentURL(ctx context.Context, docID int64) (string, error) {
	var url string
	err := s.pool.QueryRow(ctx, `SELECT url FROM documents WHERE id=$1`, docID).Scan(&url)
	return url, err
}

// ---------- projects & packages ----------

func (s *Store) FindProjectByNo(ctx context.Context, projectNo string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM projects WHERE project_no=$1`, projectNo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoRows
	}
	return id, err
}

func (s *Store) CreateProject(ctx context.Context, projectNo, title string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO projects(project_no, title) VALUES(NULLIF($1,''), $2) RETURNING id`,
		projectNo, title).Scan(&id)
	return id, err
}

func (s *Store) EnsurePackage(ctx context.Context, projectID int64, pkgNo, title string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO packages(project_id, package_no, title) VALUES($1,$2,$3)
		ON CONFLICT (project_id, package_no) DO UPDATE SET title=EXCLUDED.title`,
		projectID, pkgNo, title)
	return err
}

func (s *Store) SetProjectStatus(ctx context.Context, projectID int64, status, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE projects SET status=$2, status_reason=$3, updated_at=now() WHERE id=$1`,
		projectID, status, reason)
	return err
}

// ActivateProjectIfOpen 自动关联成功的项目进入 active；已撤标/关闭的不回退。
func (s *Store) ActivateProjectIfOpen(ctx context.Context, projectID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE projects SET status='active', updated_at=now()
		 WHERE id=$1 AND status='candidate'`, projectID)
	return err
}

func (s *Store) TouchProjectCheck(ctx context.Context, projectID int64, note string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE projects SET last_checked_at=now(), check_note=$2, updated_at=now() WHERE id=$1`,
		projectID, note)
	return err
}

// ---------- link candidates ----------

func (s *Store) UpsertLinkCandidate(ctx context.Context, docID int64, projectID *int64, hint, reason string) error {
	if projectID == nil {
		_, err := s.pool.Exec(ctx, `
			INSERT INTO link_candidates(document_id, project_id, project_no_hint, reason)
			VALUES($1,NULL,$2,$3)
			ON CONFLICT DO NOTHING`, docID, hint, reason)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO link_candidates(document_id, project_id, project_no_hint, reason)
		VALUES($1,$2,$3,$4)
		ON CONFLICT DO NOTHING`, docID, *projectID, hint, reason)
	return err
}

// ---------- corrections & field changes ----------

func (s *Store) UpsertCorrection(ctx context.Context, docID int64, projectID, targetDocID *int64,
	corrNo string, scopeAll bool, scopePkgs []string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO corrections(document_id, project_id, target_document_id, correction_no,
			scope_all_packages, scope_package_nos, status)
		VALUES($1,$2,$3,$4,$5,$6,'pending')
		ON CONFLICT (document_id) DO UPDATE SET
			project_id=COALESCE(EXCLUDED.project_id, corrections.project_id),
			target_document_id=EXCLUDED.target_document_id,
			correction_no=EXCLUDED.correction_no,
			scope_all_packages=EXCLUDED.scope_all_packages,
			scope_package_nos=EXCLUDED.scope_package_nos
		RETURNING id`,
		docID, projectID, targetDocID, corrNo, scopeAll, mustJSON(scopePkgs)).Scan(&id)
	return id, err
}

func (s *Store) MarkCorrectionContradictory(ctx context.Context, corrID int64, note string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE corrections SET status='contradictory', contradictory_note=$2 WHERE id=$1 AND status='pending'`,
		corrID, note)
	return err
}

func (s *Store) UpsertFieldChange(ctx context.Context, corrID int64, field, pkgNo, locator, excerpt, newVal string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO correction_field_changes(correction_id, field_name, package_no, new_value, source_locator, source_excerpt)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (correction_id, field_name, package_no)
		DO UPDATE SET new_value=EXCLUDED.new_value, source_locator=EXCLUDED.source_locator,
			source_excerpt=EXCLUDED.source_excerpt`,
		corrID, field, pkgNo, newVal, locator, excerpt)
	return err
}

// ---------- adopted revisions ----------

// CurrentAdoptedValue 读取项目+标包+字段的当前采用值（无则返回 ErrNoRows）。
func (s *Store) CurrentAdoptedValue(ctx context.Context, projectID int64, pkgNo, field string) (string, int64, error) {
	var val string
	var id int64
	err := s.pool.QueryRow(ctx, `
		SELECT field_value, id FROM adopted_revisions
		WHERE project_id=$1 AND package_no=$2 AND field_name=$3 AND is_current=TRUE`,
		projectID, pkgNo, field).Scan(&val, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNoRows
	}
	return val, id, err
}

// AdoptValue 写入新采用结论并作废旧值；correctionID 为空表示初始采用。
// 取消更正（kind=cancel_correction）同样保留为一条新修订，而不是回退最早值。
func (s *Store) AdoptValue(ctx context.Context, projectID int64, pkgNo, field, value string,
	versionID, docID int64, correctionID *int64, kind string, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE adopted_revisions SET is_current=FALSE, superseded_at=now()
		WHERE project_id=$1 AND package_no=$2 AND field_name=$3 AND is_current=TRUE`,
		projectID, pkgNo, field); err != nil {
		return err
	}
	var adoptedBy any
	if userID > 0 {
		adoptedBy = userID // 系统初始采用（userID=0）写 NULL
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO adopted_revisions(project_id, package_no, field_name, field_value,
			source_version_id, source_document_id, correction_id, change_kind, adopted_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		projectID, pkgNo, field, value, versionID, docID, correctionID, kind, adoptedBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
