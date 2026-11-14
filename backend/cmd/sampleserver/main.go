package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"procurewatch/internal/sampledata"
)

// 样本站点：单进程承载三个来源，各自独立限速/并发统计与故障注入。
type siteState struct {
	mu          sync.Mutex
	lastRequest time.Time
	minGap      time.Duration
	inFlight    int
	maxFlight   int
	totalReqs   int64
	limited429  int64
}

type server struct {
	mu sync.Mutex

	pageMu   sync.Mutex
	pageData map[string]sampledata.Notice

	cursorMu   sync.Mutex
	cursorData map[string]sampledata.Notice

	reviseMu    sync.Mutex
	reviseStage string
	reviseData  map[string]map[string]sampledata.Notice

	// 故障注入：ref -> 剩余 403 次数
	failMu       sync.Mutex
	failLeft     map[string]int
	requestCount map[string]int

	sites map[string]*siteState
}

func newServer() *server {
	s := &server{
		pageData:     sampledata.PageSiteNotices(),
		cursorData:   sampledata.CursorSiteNotices(),
		reviseStage:  "stage1",
		reviseData:   sampledata.ReviseSiteNotices(),
		failLeft:     map[string]int{},
		requestCount: map[string]int{},
		sites: map[string]*siteState{
			"page-site":   {minGap: 120 * time.Millisecond, maxFlight: 4},
			"cursor-site": {minGap: 120 * time.Millisecond, maxFlight: 4},
			"revise-site": {minGap: 120 * time.Millisecond, maxFlight: 4},
		},
	}
	// 默认故障：B-001 前两次请求拒绝访问，验证有限次重试
	s.failLeft["B-001"] = 2
	return s
}

// enter 执行站点级间隔与并发限制；返回 false 时已写出拒绝响应。
func (s *server) enter(site string, w http.ResponseWriter) bool {
	st := s.sites[site]
	st.mu.Lock()
	st.totalReqs++
	now := time.Now()
	if st.inFlight >= st.maxFlight {
		st.limited429++
		st.mu.Unlock()
		http.Error(w, `{"error":"concurrency limit"}`, http.StatusTooManyRequests)
		return false
	}
	if !st.lastRequest.IsZero() && now.Sub(st.lastRequest) < st.minGap {
		st.limited429++
		st.mu.Unlock()
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"site interval not reached"}`, http.StatusTooManyRequests)
		return false
	}
	st.inFlight++
	st.lastRequest = now
	st.mu.Unlock()
	return true
}

func (s *server) leave(site string) {
	st := s.sites[site]
	st.mu.Lock()
	st.inFlight--
	st.mu.Unlock()
}

// check403 故障注入：命中剩余次数则返回 403。
func (s *server) check403(ref string, w http.ResponseWriter) bool {
	s.failMu.Lock()
	s.requestCount[ref]++
	left := s.failLeft[ref]
	if left > 0 {
		s.failLeft[ref] = left - 1
		s.failMu.Unlock()
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":"access denied (injected)"}`, http.StatusForbidden)
		return true
	}
	s.failMu.Unlock()
	return false
}

func (s *server) handlePageList(w http.ResponseWriter, r *http.Request) {
	if !s.enter("page-site", w) {
		return
	}
	defer s.leave("page-site")
	pages := sampledata.PageSitePages()
	page := 0
	fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
	if page < 1 {
		page = 1
	}
	var entries []sampledata.ListEntry
	end := false
	if page > len(pages) {
		entries = []sampledata.ListEntry{}
		end = true // 空列表 + 样本站点结束标志 => 到尾
	} else {
		entries = pages[page-1]
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sampledata.RenderList(entries, sampledata.PageEndMarker, end, ""))
}

func (s *server) handlePageNotice(w http.ResponseWriter, r *http.Request, ref string) {
	if !s.enter("page-site", w) {
		return
	}
	defer s.leave("page-site")
	if s.check403(ref, w) {
		return
	}
	s.pageMu.Lock()
	n, ok := s.pageData[ref]
	s.pageMu.Unlock()
	if !ok {
		http.Error(w, `{"error":"notice not found"}`, http.StatusNotFound) // 失效链接，不代表撤标
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sampledata.RenderDetail(n))
}

func (s *server) handleCursorList(w http.ResponseWriter, r *http.Request) {
	if !s.enter("cursor-site", w) {
		return
	}
	defer s.leave("cursor-site")
	seq, cursors := sampledata.CursorSitePages()
	cursor := r.URL.Query().Get("cursor")
	idx := 0
	for i, c := range cursors {
		if c == cursor {
			idx = i
			break
		}
	}
	var entries []sampledata.ListEntry
	nextCursor, end := "", false
	if idx >= len(seq) {
		entries = []sampledata.ListEntry{}
		end = true
	} else {
		entries = seq[idx]
		if idx+1 < len(cursors) {
			nextCursor = cursors[idx+1]
		}
	}
	html := sampledata.RenderList(entries, sampledata.CursorEndMarker, end, "")
	// 游标写在结束标志同区，供 fetcher 提取
	inject := fmt.Sprintf(`<div id="cursor-control" data-sample-next-cursor="%s"></div>`, nextCursor)
	html = strings.Replace(html, `<body>`, `<body>`+inject, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func (s *server) handleCursorNotice(w http.ResponseWriter, r *http.Request, ref string) {
	if !s.enter("cursor-site", w) {
		return
	}
	defer s.leave("cursor-site")
	if s.check403(ref, w) {
		return
	}
	s.cursorMu.Lock()
	n, ok := s.cursorData[ref]
	s.cursorMu.Unlock()
	if !ok {
		http.Error(w, `{"error":"notice not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sampledata.RenderDetail(n))
}

func (s *server) handleReviseList(w http.ResponseWriter, r *http.Request) {
	if !s.enter("revise-site", w) {
		return
	}
	defer s.leave("revise-site")
	// 单页列表，本身带结束标志
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sampledata.RenderList(sampledata.ReviseSiteList(), sampledata.ReviseEndMarker, true, ""))
}

func (s *server) handleReviseNotice(w http.ResponseWriter, r *http.Request, ref string) {
	if !s.enter("revise-site", w) {
		return
	}
	defer s.leave("revise-site")
	if s.check403(ref, w) {
		return
	}
	s.reviseMu.Lock()
	n, ok := s.reviseData[s.reviseStage][ref]
	stage := s.reviseStage
	s.reviseMu.Unlock()
	if !ok {
		http.Error(w, `{"error":"notice not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("X-Sample-Stage", stage)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, sampledata.RenderDetail(n))
}

func (s *server) handleControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case r.URL.Path == "/control/state":
		s.reviseMu.Lock()
		stage := s.reviseStage
		s.reviseMu.Unlock()
		s.failMu.Lock()
		fails := map[string]int{}
		for k, v := range s.failLeft {
			fails[k] = v
		}
		reqs := map[string]int{}
		for k, v := range s.requestCount {
			reqs[k] = v
		}
		s.failMu.Unlock()
		siteStats := map[string]any{}
		for k, st := range s.sites {
			st.mu.Lock()
			siteStats[k] = map[string]any{"total": st.totalReqs, "limited429": st.limited429, "in_flight": st.inFlight}
			st.mu.Unlock()
		}
		json.NewEncoder(w).Encode(map[string]any{"stage": stage, "fail_left": fails, "requests": reqs, "sites": siteStats})
	case r.URL.Path == "/control/revise-stage" && r.Method == http.MethodPost:
		stage := r.URL.Query().Get("stage")
		if stage != "stage1" && stage != "stage2" {
			http.Error(w, `{"error":"stage must be stage1|stage2"}`, http.StatusBadRequest)
			return
		}
		s.reviseMu.Lock()
		s.reviseStage = stage
		s.reviseMu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"stage": stage})
	case r.URL.Path == "/control/inject-403" && r.Method == http.MethodPost:
		ref := r.URL.Query().Get("ref")
		times := 2
		fmt.Sscanf(r.URL.Query().Get("times"), "%d", &times)
		if ref == "" {
			http.Error(w, `{"error":"ref required"}`, http.StatusBadRequest)
			return
		}
		s.failMu.Lock()
		s.failLeft[ref] += times
		s.failMu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"ref": ref, "fail_left": s.failLeft[ref]})
	case r.URL.Path == "/control/reset" && r.Method == http.MethodPost:
		s.failMu.Lock()
		s.failLeft = map[string]int{"B-001": 2}
		s.requestCount = map[string]int{}
		s.failMu.Unlock()
		s.reviseMu.Lock()
		s.reviseStage = "stage1"
		s.reviseMu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"reset": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/page-site/list", s.handlePageList)
	mux.HandleFunc("/page-site/notice/", func(w http.ResponseWriter, r *http.Request) {
		s.handlePageNotice(w, r, strings.TrimPrefix(r.URL.Path, "/page-site/notice/"))
	})
	mux.HandleFunc("/cursor-site/list", s.handleCursorList)
	mux.HandleFunc("/cursor-site/notice/", func(w http.ResponseWriter, r *http.Request) {
		s.handleCursorNotice(w, r, strings.TrimPrefix(r.URL.Path, "/cursor-site/notice/"))
	})
	mux.HandleFunc("/revise-site/list", s.handleReviseList)
	mux.HandleFunc("/revise-site/notice/", func(w http.ResponseWriter, r *http.Request) {
		s.handleReviseNotice(w, r, strings.TrimPrefix(r.URL.Path, "/revise-site/notice/"))
	})
	mux.HandleFunc("/control/", s.handleControl)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	return mux
}

func main() {
	addr := flag.String("addr", ":9090", "listen address")
	flag.Parse()
	log.Printf("sample procurement sites listening on %s (page/cursor/revise)", *addr)
	if err := http.ListenAndServe(*addr, newServer().routes()); err != nil {
		log.Fatal(err)
	}
}
