package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"procurewatch/internal/service"
	"procurewatch/internal/store"
)

type Server struct {
	st *store.Store
	wb *service.Workbench
}

func NewServer(st *store.Store, wb *service.Workbench) *Server {
	return &Server{st: st, wb: wb}
}

func (s *Server) Register(r *gin.Engine, auth *Auth) {
	api := r.Group("/api")
	api.POST("/auth/login", auth.Login)

	authed := api.Group("")
	authed.Use(auth.Middleware())
	authed.GET("/auth/me", auth.Me)

	authed.GET("/sources", s.listSources)
	authed.GET("/sources/:id/discoveries", s.listDiscoveries)
	authed.GET("/sources/:id/jobs", s.sourceJobs)

	admin := authed.Group("")
	admin.Use(RequireRole("admin"))
	admin.POST("/sources/:id/pause", s.pauseSource)
	admin.POST("/sources/:id/resume", s.resumeSource)
	admin.POST("/sources/:id/list-jobs", s.triggerListJob)
	admin.POST("/sources/:id/manual-detail", s.manualDetail)

	authed.GET("/projects", s.listProjects)
	authed.GET("/projects/:id", s.getProject)
	authed.GET("/projects/:id/timeline", s.projectTimeline)
	authed.GET("/projects/:id/packages", s.listPackages)
	authed.POST("/projects/:id/export", s.exportProject)

	authed.GET("/documents/:id/versions", s.listVersions)
	authed.GET("/versions/:id/raw", s.versionRaw)

	authed.GET("/candidates", s.listCandidates)
	authed.POST("/candidates/:id/confirm", s.confirmCandidate)
	authed.POST("/candidates/:id/reject", s.rejectCandidate)
	authed.POST("/candidates/:id/recheck", s.recheckCandidate)

	authed.GET("/corrections", s.listCorrections)
	authed.GET("/corrections/:id", s.correctionDetail)
	authed.POST("/corrections/:id/fields/confirm", s.confirmField)
	authed.POST("/corrections/:id/fields/return", s.returnField)
	authed.POST("/corrections/:id/return", s.returnCorrection)
}

func idParam(c *gin.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return id
}

func fail(c *gin.Context, err error) {
	if errors.Is(err, store.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// ---------- sources ----------

func (s *Server) listSources(c *gin.Context) {
	sources, err := s.st.ListSources(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	counters, _ := s.st.SourceCounters(c.Request.Context())
	for _, src := range sources {
		sid := src["id"].(int64)
		if ct, ok := counters[sid]; ok {
			src["counters"] = ct
		} else {
			src["counters"] = map[string]int64{"detail_done": 0, "detail_pending": 0, "detail_queued": 0, "review_pending": 0}
		}
	}
	c.JSON(http.StatusOK, gin.H{"sources": sources})
}

func (s *Server) pauseSource(c *gin.Context) {
	// 暂停仅停止新领取；正在执行的任务租约不变，仍按原凭证完成
	if err := s.st.SetSourceEnabled(c.Request.Context(), idParam(c), false); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": "来源已暂停：停止新任务领取，执行中的任务仍会完成"})
}

func (s *Server) resumeSource(c *gin.Context) {
	if err := s.st.SetSourceEnabled(c.Request.Context(), idParam(c), true); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) triggerListJob(c *gin.Context) {
	id, err := s.st.EnqueueListJob(c.Request.Context(), idParam(c), "manual")
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"list_job_id": id, "note": "列表作业已入队，仍受站点间隔与并发限制"})
}

func (s *Server) manualDetail(c *gin.Context) {
	var in struct {
		Ref string `json:"ref"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ref 必填"})
		return
	}
	// 手动补抓与自动采集共用 detail_jobs 与同一限流器，绕不开站点限额
	if err := s.st.EnqueueManualDetail(c.Request.Context(), idParam(c), in.Ref); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": "手动补抓已排队，同样受站点间隔/并发限制"})
}

func (s *Server) listDiscoveries(c *gin.Context) {
	rows, err := s.st.Pool().Query(c.Request.Context(), `
		SELECT id, source_ref, url, title, detail_status, discovered_at
		FROM discovery_items WHERE source_id=$1 ORDER BY id`, idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var ref, url, title, status string
		var at any
		if err := rows.Scan(&id, &ref, &url, &title, &status, &at); err != nil {
			fail(c, err)
			return
		}
		out = append(out, map[string]any{"id": id, "source_ref": ref, "url": url,
			"title": title, "detail_status": status, "discovered_at": at})
	}
	c.JSON(http.StatusOK, gin.H{"discoveries": out})
}

func (s *Server) sourceJobs(c *gin.Context) {
	rows, err := s.st.Pool().Query(c.Request.Context(), `
		SELECT id, kind, status, epoch, worker_id, lease_until, last_offset, error, created_at
		FROM list_jobs WHERE source_id=$1 ORDER BY id DESC LIMIT 20`, idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var kind, status, worker, errMsg string
		var epoch int
		var lease *string
		var offset []byte
		var createdAt any
		if err := rows.Scan(&id, &kind, &status, &epoch, &worker, &lease, &offset, &errMsg, &createdAt); err != nil {
			fail(c, err)
			return
		}
		out = append(out, map[string]any{"id": id, "kind": kind, "status": status, "epoch": epoch,
			"worker_id": worker, "lease_until": lease, "last_offset": string(offset), "error": errMsg,
			"created_at": createdAt})
	}
	c.JSON(http.StatusOK, gin.H{"list_jobs": out})
}

// ---------- projects ----------

func (s *Server) listProjects(c *gin.Context) {
	projects, err := s.st.ListProjects(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (s *Server) getProject(c *gin.Context) {
	p, err := s.st.GetProject(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) projectTimeline(c *gin.Context) {
	tl, err := s.st.ProjectTimeline(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, tl)
}

func (s *Server) listPackages(c *gin.Context) {
	pkgs, err := s.st.ListPackages(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"packages": pkgs})
}

func (s *Server) exportProject(c *gin.Context) {
	sess := MustGetSession(c)
	payload, err := s.st.ExportSnapshot(c.Request.Context(), idParam(c), sess.UserID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, payload)
}

// ---------- documents ----------

func (s *Server) listVersions(c *gin.Context) {
	versions, err := s.st.ListVersions(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (s *Server) versionRaw(c *gin.Context) {
	raw, err := s.st.GetVersionRaw(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.String(http.StatusOK, raw)
}

// ---------- candidates ----------

func (s *Server) listCandidates(c *gin.Context) {
	items, err := s.st.ListLinkCandidates(c.Request.Context(), c.Query("status"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"candidates": items})
}

func (s *Server) confirmCandidate(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct {
		ProjectID  int64  `json:"project_id"`
		ProjectNo  string `json:"project_no"`
		Title      string `json:"title"`
		Note       string `json:"note"`
		NewProject bool   `json:"new_project"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}
	if in.NewProject {
		pid, err := s.wb.ConfirmCandidateNewProject(c.Request.Context(), idParam(c), in.ProjectNo, in.Title, sess.UserID, in.Note)
		if err != nil {
			fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "project_id": pid})
		return
	}
	if in.ProjectID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id 必填，或勾选 new_project"})
		return
	}
	if err := s.wb.ConfirmCandidate(c.Request.Context(), idParam(c), in.ProjectID, sess.UserID, in.Note); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "project_id": in.ProjectID})
}

func (s *Server) rejectCandidate(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct{ Note string `json:"note"` }
	c.ShouldBindJSON(&in)
	if err := s.wb.RejectCandidate(c.Request.Context(), idParam(c), sess.UserID, in.Note); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) recheckCandidate(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct{ Note string `json:"note"` }
	c.ShouldBindJSON(&in)
	if err := s.wb.RecheckCandidate(c.Request.Context(), idParam(c), sess.UserID, in.Note); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": "已退回补查"})
}

// ---------- corrections ----------

func (s *Server) listCorrections(c *gin.Context) {
	items, err := s.st.ListCorrections(c.Request.Context(), c.Query("status"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"corrections": items})
}

func (s *Server) correctionDetail(c *gin.Context) {
	head, changes, err := s.st.CorrectionDetail(c.Request.Context(), idParam(c))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"correction": head, "field_changes": changes})
}

func (s *Server) confirmField(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct {
		Field   string `json:"field"`
		Package string `json:"package"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Field == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 必填"})
		return
	}
	if err := s.wb.ConfirmField(c.Request.Context(), idParam(c), in.Field, in.Package, sess.UserID); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": "字段已确认并形成新的采用修订，其他字段不受影响"})
}

func (s *Server) returnField(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct {
		Field   string `json:"field"`
		Package string `json:"package"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Field == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "field 必填"})
		return
	}
	if err := s.wb.ReturnField(c.Request.Context(), idParam(c), in.Field, in.Package, sess.UserID); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "note": "字段已退回补查，当前采用值不变"})
}

func (s *Server) returnCorrection(c *gin.Context) {
	sess := MustGetSession(c)
	var in struct{ Note string `json:"note"` }
	c.ShouldBindJSON(&in)
	if err := s.wb.ReturnCorrection(c.Request.Context(), idParam(c), sess.UserID, in.Note); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
