package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"procurewatch/internal/sampledata"
	"procurewatch/internal/store"
)

// Seed 幂等初始化：测试账号与三个样本来源。
func Seed(ctx context.Context, st *store.Store, sampleBaseURL string) error {
	users := []struct{ username, display, role, password string }{
		{"admin", "系统管理员", "admin", "admin123"},
		{"buyer", "采购专员", "buyer", "buyer123"},
	}
	for _, u := range users {
		if _, _, _, _, err := st.GetUserByUsername(ctx, u.username); err == nil {
			continue
		} else if err != store.ErrNoRows {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := st.Pool().Exec(ctx,
			`INSERT INTO users(username, password_hash, display_name, role) VALUES($1,$2,$3,$4)
			 ON CONFLICT (username) DO NOTHING`,
			u.username, string(hash), u.display, u.role); err != nil {
			return err
		}
	}

	sources := []struct {
		code, name, mode, listPath, tpl, marker string
		interval, concurrency, retries           int
	}{
		{"page-site", "样本站A·页码分页", "page", "/page-site/list", "/page-site/notice/{ref}", sampledata.PageEndMarker, 150, 2, 3},
		{"cursor-site", "样本站B·游标分页", "cursor", "/cursor-site/list", "/cursor-site/notice/{ref}", sampledata.CursorEndMarker, 150, 2, 3},
		{"revise-site", "样本站C·原址修订", "page", "/revise-site/list", "/revise-site/notice/{ref}", sampledata.ReviseEndMarker, 150, 2, 3},
	}
	for _, s := range sources {
		var exists bool
		if err := st.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sources WHERE code=$1)`, s.code).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := st.Pool().Exec(ctx, `
			INSERT INTO sources(code, name, base_url, list_mode, list_path, detail_path_tpl,
				interval_ms, max_concurrency, max_retries, end_marker)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			s.code, s.name, sampleBaseURL, s.mode, s.listPath, s.tpl,
			s.interval, s.concurrency, s.retries, s.marker); err != nil {
			return err
		}
	}
	return nil
}
