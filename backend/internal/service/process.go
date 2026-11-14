package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"procurewatch/internal/parser"
	"procurewatch/internal/store"
)

// Processor 负责把一份已入库的文档版本转化为：项目/标包、关联候选、更正与采用结论。
// 关键纪律：缺少依据不自动关联；未确认内容不替换当前采用结论；撤标仅由明确撤标公告触发。
type Processor struct {
	st *store.Store
}

func NewProcessor(st *store.Store) *Processor { return &Processor{st: st} }

// ProcessLatestVersion 取文档最新版本执行业务处理。截断/解析失败的版本只留存，不产生采用结论。
func (p *Processor) ProcessLatestVersion(ctx context.Context, docID int64, reviewDue time.Duration) error {
	raw, payload, parsedOK, truncated, parseErr, err := p.st.LatestVersionPayload(ctx, docID)
	if err != nil {
		return err
	}
	_ = raw
	var detail parser.Detail
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &detail); err != nil {
			return err
		}
	}
	if !parsedOK || truncated {
		// 半份内容/解析失败：保留原件与错误，缺失字段不得视为删除，也不形成采用结论
		return p.st.AttachProcessNote(ctx, docID, "parse-blocked: "+parseErr)
	}
	return p.processDetail(ctx, docID, detail, reviewDue)
}

func (p *Processor) processDetail(ctx context.Context, docID int64, d parser.Detail, reviewDue time.Duration) error {
	sourceID, err := p.st.DocumentSource(ctx, docID)
	if err != nil {
		return err
	}

	// 1. 项目解析：只有显式采购编号才能自动关联；标题相近但编号不同 => 不同项目
	var projectID int64
	if d.ProjectNo == "" {
		// 已人工确认过归属：按确认项目继续处理；仍待确认则保留候选，不自动关联
		if pid, err := p.st.CandidateProject(ctx, docID); err == nil {
			projectID = pid
			for _, pk := range d.Packages {
				if err := p.st.EnsurePackage(ctx, projectID, pk.No, pk.Title); err != nil {
					return err
				}
			}
		} else {
			handled, _ := p.st.HasHandledCandidate(ctx, docID)
			if !handled {
				if err := p.st.UpsertLinkCandidate(ctx, docID, nil, "",
					"公告未标注采购编号，缺少自动关联依据，需采购专员人工确认归属项目"); err != nil {
					return err
				}
			}
			return p.st.AttachProcessNote(ctx, docID, "candidate: missing project number")
		}
	} else {
		pid, err := p.st.FindProjectByNo(ctx, d.ProjectNo)
		if err == store.ErrNoRows {
			pid, err = p.st.CreateProject(ctx, d.ProjectNo, d.Title)
		}
		if err != nil {
			return err
		}
		projectID = pid
		for _, pk := range d.Packages {
			if err := p.st.EnsurePackage(ctx, projectID, pk.No, pk.Title); err != nil {
				return err
			}
		}
	}

	// 2. 按文档类型分流
	switch d.DocType {
	case "notice":
		if err := p.st.ActivateProjectIfOpen(ctx, projectID); err != nil {
			return err
		}
		if err := p.processNotice(ctx, docID, projectID, d); err != nil {
			return err
		}
	case "correction":
		if err := p.st.ActivateProjectIfOpen(ctx, projectID); err != nil {
			return err
		}
		if err := p.processCorrection(ctx, sourceID, docID, projectID, d, false); err != nil {
			return err
		}
	case "cancel_correction":
		if err := p.st.ActivateProjectIfOpen(ctx, projectID); err != nil {
			return err
		}
		if err := p.processCancelCorrection(ctx, sourceID, docID, projectID, d); err != nil {
			return err
		}
	case "cancellation":
		reason := fmt.Sprintf("撤标公告 %s（文档 %d）明确撤销本项目", nz(d.CorrectionNo, d.Title), docID)
		if err := p.st.SetProjectStatus(ctx, projectID, "withdrawn", reason); err != nil {
			return err
		}
	default:
		if err := p.processNotice(ctx, docID, projectID, d); err != nil {
			return err
		}
	}

	// 3. 近期项目定期复查正文
	return p.st.EnqueueReview(ctx, projectID, docID, time.Now().Add(reviewDue))
}

// processNotice 处理普通公告：首次出现的字段初始采用；原址换文导致的差异转待核实，不直接替换。
func (p *Processor) processNotice(ctx context.Context, docID, projectID int64, d parser.Detail) error {
	var changed []parser.ParsedField
	for _, f := range d.Fields {
		if f.Field == "__withdrawal__" {
			continue
		}
		if f.Package != "" {
			if err := p.st.EnsurePackage(ctx, projectID, f.Package, f.Package); err != nil {
				return err
			}
		}
		cur, _, err := p.st.CurrentAdoptedValue(ctx, projectID, f.Package, f.Field)
		switch {
		case err == store.ErrNoRows:
			// 新字段（如标包补充设备清单）=> 初始采用
			vid, err := p.st.LatestVersionID(ctx, docID)
			if err != nil {
				return err
			}
			if err := p.st.AdoptValue(ctx, projectID, f.Package, f.Field, f.Value, vid, docID, nil, "initial", 0); err != nil {
				return err
			}
		case err != nil:
			return err
		case cur != f.Value:
			// 原址换文/重复抓取差异：新增原件不等于可执行要求，转待核实
			changed = append(changed, f)
		}
	}
	if len(changed) > 0 {
		corrID, err := p.st.UpsertCorrection(ctx, docID, &projectID, &docID,
			"REV-"+fmt.Sprint(docID), false, nil)
		if err != nil {
			return err
		}
		note := "同址正文发生变化（原址修订），字段差异待采购专员逐字段确认，未确认前不替换当前采用值"
		if err := p.st.MarkCorrectionContradictory(ctx, corrID, note); err != nil {
			return err
		}
		for _, f := range changed {
			if err := p.st.UpsertFieldChange(ctx, corrID, f.Field, f.Package, f.Locator, f.Excerpt, f.Value); err != nil {
				return err
			}
		}
		if err := p.st.SnapshotOldValues(ctx, corrID, projectID); err != nil {
			return err
		}
	}
	return nil
}

// processCorrection 处理更正通知：校验引用对象与作用范围，识别跨通知矛盾，生成逐字段待办。
func (p *Processor) processCorrection(ctx context.Context, sourceID, docID, projectID int64, d parser.Detail, isCancel bool) error {
	var targetDocID *int64
	notes := []string{}

	if d.TargetRef == "" {
		notes = append(notes, "通知未明确引用对象：发布时间晚不代表必然取代早公告，转待核实")
	} else if tid, err := p.st.FindDocumentIDByRef(ctx, sourceID, d.TargetRef); err == nil {
		targetDocID = &tid
	} else {
		notes = append(notes, fmt.Sprintf("引用对象 %s 在本来源尚未采集或不存在，无法自动关联", d.TargetRef))
	}
	if !d.ScopeAll && len(d.ScopePackages) == 0 {
		notes = append(notes, "通知未声明作用范围（全部标包或具体标包），转待核实")
	}

	corrID, err := p.st.UpsertCorrection(ctx, docID, &projectID, targetDocID,
		nz(d.CorrectionNo, "COR-"+fmt.Sprint(docID)), d.ScopeAll, d.ScopePackages)
	if err != nil {
		return err
	}
	if len(notes) > 0 {
		if err := p.st.MarkCorrectionContradictory(ctx, corrID, joinNotes(notes)); err != nil {
			return err
		}
	}

	for _, f := range d.Fields {
		if f.Field == "__withdrawal__" {
			continue
		}
		if !inScope(f, d) {
			continue // 只保存适用标包内的字段；不同标包截止时间分别保存，不全量更新
		}
		// 跨通知矛盾：同一 项目+标包+字段 已存在不同新值的未退回更正
		conflict, err := p.st.FindConflictingFieldChange(ctx, projectID, f.Package, f.Field, f.Value, corrID)
		if err != nil {
			return err
		}
		if conflict {
			if err := p.st.MarkCorrectionContradictory(ctx, corrID,
				"与更早的通知就同一字段给出互相矛盾的取值，转待核实"); err != nil {
				return err
			}
		}
		if err := p.st.UpsertFieldChange(ctx, corrID, f.Field, f.Package, f.Locator, f.Excerpt, f.Value); err != nil {
			return err
		}
	}
	return p.st.SnapshotOldValues(ctx, corrID, projectID)
}

// processCancelCorrection 取消更正：被取消的更正内容保留（标记 superseded），
// 取值以取消通知自身载明内容为准，形成新的采用修订，而不是回退到最早值。
func (p *Processor) processCancelCorrection(ctx context.Context, sourceID, docID, projectID int64, d parser.Detail) error {
	if d.TargetRef != "" {
		if tid, err := p.st.FindDocumentIDByRef(ctx, sourceID, d.TargetRef); err == nil {
			if err := p.st.SupersedeCorrectionByDoc(ctx, tid, docID); err != nil {
				return err
			}
		}
	}
	return p.processCorrection(ctx, sourceID, docID, projectID, d, true)
}

func inScope(f parser.ParsedField, d parser.Detail) bool {
	if f.Package == "" {
		return d.ScopeAll // 项目级字段只有“适用于全部标包”的通知才能更新
	}
	if d.ScopeAll {
		return true
	}
	for _, pk := range d.ScopePackages {
		if pk == f.Package {
			return true
		}
	}
	return false
}

func nz(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func joinNotes(notes []string) string {
	out := ""
	for i, n := range notes {
		if i > 0 {
			out += "；"
		}
		out += n
	}
	return out
}
