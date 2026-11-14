package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"procurewatch/internal/store"
)

// Workbench 承载采购专员的人工处置动作。
// 纪律：未确认内容不替换采用结论；被取消（superseded）的更正不得再确认；逐字段独立处置。
type Workbench struct {
	st        *store.Store
	proc      *Processor
	reviewDue time.Duration
}

func NewWorkbench(st *store.Store, proc *Processor, reviewDue time.Duration) *Workbench {
	return &Workbench{st: st, proc: proc, reviewDue: reviewDue}
}

// ConfirmField 确认更正中的单个字段，立即形成新的采用修订；其余字段不受影响。
func (wb *Workbench) ConfirmField(ctx context.Context, corrID int64, field, pkg string, userID int64) error {
	docID, projectID, status, err := wb.st.GetCorrection(ctx, corrID)
	if err != nil {
		return err
	}
	if projectID == nil {
		return errors.New("更正尚未关联项目，无法确认")
	}
	if status == "superseded" {
		return errors.New("该更正已被取消通知取代，其内容不得再确认；如需调整请处理取消通知载明的取值")
	}
	newValue, _, decision, err := wb.st.GetFieldChange(ctx, corrID, field, pkg)
	if err != nil {
		return err
	}
	if decision == "returned" {
		return errors.New("该字段已退回补查，不能确认；如需采用请先重新发起更正")
	}
	if err := wb.st.SetFieldDecision(ctx, corrID, field, pkg, "confirmed", userID); err != nil {
		return err
	}
	versionID, err := wb.st.LatestVersionID(ctx, docID)
	if err != nil {
		return err
	}
	kind := "correction"
	if docType, derr := wb.st.DocTypeOf(ctx, docID); derr == nil && docType == "cancel_correction" {
		kind = "cancel_correction"
	}
	corrIDVal := corrID
	if err := wb.st.AdoptValue(ctx, *projectID, pkg, field, newValue,
		versionID, docID, &corrIDVal, kind, userID); err != nil {
		return err
	}
	return wb.st.RefreshCorrectionStatus(ctx, corrID, userID)
}

// ReturnField 退回单个字段补查；不改动当前采用值。
func (wb *Workbench) ReturnField(ctx context.Context, corrID int64, field, pkg string, userID int64) error {
	if _, _, _, err := wb.st.GetCorrection(ctx, corrID); err != nil {
		return err
	}
	if _, _, _, err := wb.st.GetFieldChange(ctx, corrID, field, pkg); err != nil {
		return err
	}
	if err := wb.st.SetFieldDecision(ctx, corrID, field, pkg, "returned", userID); err != nil {
		return err
	}
	return wb.st.RefreshCorrectionStatus(ctx, corrID, userID)
}

// ReturnCorrection 整份更正退回补查。
func (wb *Workbench) ReturnCorrection(ctx context.Context, corrID, userID int64, note string) error {
	if _, _, status, err := wb.st.GetCorrection(ctx, corrID); err != nil {
		return err
	} else if status == "superseded" {
		return errors.New("已被取消的更正无需退回")
	}
	return wb.st.ReturnCorrection(ctx, corrID, userID, note)
}

// ConfirmCandidate 确认关联候选归属到指定项目（已存在项目），随后重新处理该文档。
func (wb *Workbench) ConfirmCandidate(ctx context.Context, candidateID, projectID, userID int64, note string) error {
	docID, _, status, err := wb.st.GetLinkCandidate(ctx, candidateID)
	if err != nil {
		return err
	}
	if status != "pending" {
		return errors.New("候选已处理，不能重复确认")
	}
	pid := projectID
	if err := wb.st.ResolveLinkCandidate(ctx, candidateID, userID, "confirmed", &pid, note); err != nil {
		return err
	}
	return wb.proc.ProcessLatestVersion(ctx, docID, wb.reviewDue)
}

// ConfirmCandidateNewProject 确认候选并新建项目后关联。
func (wb *Workbench) ConfirmCandidateNewProject(ctx context.Context, candidateID int64, projectNo, title string, userID int64, note string) (int64, error) {
	docID, _, status, err := wb.st.GetLinkCandidate(ctx, candidateID)
	if err != nil {
		return 0, err
	}
	if status != "pending" {
		return 0, errors.New("候选已处理，不能重复确认")
	}
	if projectNo == "" {
		return 0, errors.New("新建项目必须给出采购编号")
	}
	pid, err := wb.st.CreateProject(ctx, projectNo, title)
	if err != nil {
		return 0, fmt.Errorf("创建项目失败（编号可能已存在）: %w", err)
	}
	if err := wb.st.ResolveLinkCandidate(ctx, candidateID, userID, "confirmed", &pid, note); err != nil {
		return 0, err
	}
	if err := wb.proc.ProcessLatestVersion(ctx, docID, wb.reviewDue); err != nil {
		return pid, err
	}
	return pid, nil
}

// RejectCandidate / RecheckCandidate
func (wb *Workbench) RejectCandidate(ctx context.Context, candidateID, userID int64, note string) error {
	docID, _, status, err := wb.st.GetLinkCandidate(ctx, candidateID)
	if err != nil {
		return err
	}
	if status != "pending" {
		return errors.New("候选已处理")
	}
	_ = docID
	return wb.st.ResolveLinkCandidate(ctx, candidateID, userID, "rejected", nil, note)
}

func (wb *Workbench) RecheckCandidate(ctx context.Context, candidateID, userID int64, note string) error {
	_, _, status, err := wb.st.GetLinkCandidate(ctx, candidateID)
	if err != nil {
		return err
	}
	if status != "pending" {
		return errors.New("候选已处理")
	}
	return wb.st.ResolveLinkCandidate(ctx, candidateID, userID, "recheck", nil, note)
}
