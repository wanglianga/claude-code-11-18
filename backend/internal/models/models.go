package models

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Source struct {
	ID             int64     `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	BaseURL        string    `json:"base_url"`
	ListMode       string    `json:"list_mode"`
	ListPath       string    `json:"list_path"`
	DetailPathTpl  string    `json:"detail_path_tpl"`
	IntervalMs     int       `json:"interval_ms"`
	MaxConcurrency int       `json:"max_concurrency"`
	MaxRetries     int       `json:"max_retries"`
	Enabled        bool      `json:"enabled"`
	EndMarker      string    `json:"end_marker"`
	CursorState    []byte    `json:"-"`
	ListStatus     string    `json:"list_status"`
	LastError      string    `json:"last_error"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DiscoveryItem struct {
	ID           int64     `json:"id"`
	SourceID     int64     `json:"source_id"`
	SourceRef    string    `json:"source_ref"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	DiscoveredAt time.Time `json:"discovered_at"`
	DetailStatus string    `json:"detail_status"`
}

type ListJob struct {
	ID         int64     `json:"id"`
	SourceID   int64     `json:"source_id"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Epoch      int       `json:"epoch"`
	WorkerID   string    `json:"worker_id"`
	LeaseUntil *time.Time `json:"lease_until"`
	LastOffset []byte    `json:"last_offset"`
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type DetailJob struct {
	ID          int64      `json:"id"`
	SourceID    int64      `json:"source_id"`
	SourceRef   string     `json:"source_ref"`
	URL         string     `json:"url"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	WorkerID    string     `json:"worker_id"`
	LeaseUntil  *time.Time `json:"lease_until"`
	LastError   string     `json:"last_error"`
	Manual      bool       `json:"manual"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Document struct {
	ID            int64     `json:"id"`
	SourceID      int64     `json:"source_id"`
	SourceRef     string    `json:"source_ref"`
	URL           string    `json:"url"`
	DocType       string    `json:"doc_type"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
}

type DocumentVersion struct {
	ID            int64     `json:"id"`
	DocumentID    int64     `json:"document_id"`
	FetchRunID    int64     `json:"fetch_run_id"`
	Fingerprint   string    `json:"fingerprint"`
	RawHTML       string    `json:"raw_html"`
	ParsedOK      bool      `json:"parsed_ok"`
	ParseError    string    `json:"parse_error"`
	Truncated     bool      `json:"truncated"`
	ParsedPayload []byte    `json:"parsed_payload"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type Project struct {
	ID            int64      `json:"id"`
	ProjectNo     *string    `json:"project_no"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	StatusReason  string     `json:"status_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CheckNote     string     `json:"check_note"`
}

type Package struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	PackageNo string `json:"package_no"`
	Title     string `json:"title"`
}

type LinkCandidate struct {
	ID            int64      `json:"id"`
	DocumentID    int64      `json:"document_id"`
	ProjectID     *int64     `json:"project_id"`
	ProjectNoHint string     `json:"project_no_hint"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	HandledBy     *int64     `json:"handled_by"`
	HandledAt     *time.Time `json:"handled_at"`
	Note          string     `json:"note"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Correction struct {
	ID               int64      `json:"id"`
	DocumentID       int64      `json:"document_id"`
	ProjectID        *int64     `json:"project_id"`
	TargetDocumentID *int64     `json:"target_document_id"`
	CorrectionNo     string     `json:"correction_no"`
	ScopeAllPackages bool       `json:"scope_all_packages"`
	ScopePackageNos  []byte     `json:"scope_package_nos"`
	Status           string     `json:"status"`
	ContradictoryNote string    `json:"contradictory_note"`
	HandledBy        *int64     `json:"handled_by"`
	HandledAt        *time.Time `json:"handled_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

type FieldChange struct {
	ID            int64      `json:"id"`
	CorrectionID  int64      `json:"correction_id"`
	FieldName     string     `json:"field_name"`
	PackageNo     string     `json:"package_no"`
	OldValue      string     `json:"old_value"`
	NewValue      string     `json:"new_value"`
	SourceLocator string     `json:"source_locator"`
	SourceExcerpt string     `json:"source_excerpt"`
	Decision      string     `json:"decision"`
	DecidedBy     *int64     `json:"decided_by"`
	DecidedAt     *time.Time `json:"decided_at"`
}

type AdoptedRevision struct {
	ID               int64     `json:"id"`
	ProjectID        int64     `json:"project_id"`
	PackageNo        string    `json:"package_no"`
	FieldName        string    `json:"field_name"`
	FieldValue       string    `json:"field_value"`
	SourceVersionID  int64     `json:"source_version_id"`
	SourceDocumentID int64     `json:"source_document_id"`
	CorrectionID     *int64    `json:"correction_id"`
	ChangeKind       string    `json:"change_kind"`
	AdoptedBy        *int64    `json:"adopted_by"`
	AdoptedAt        time.Time `json:"adopted_at"`
	SupersededAt     *time.Time `json:"superseded_at"`
	IsCurrent        bool      `json:"is_current"`
}

type ReviewQueueItem struct {
	ID         int64      `json:"id"`
	ProjectID  int64      `json:"project_id"`
	DocumentID int64      `json:"document_id"`
	DueAt      time.Time  `json:"due_at"`
	Status     string     `json:"status"`
	WorkerID   string     `json:"worker_id"`
	LeaseUntil *time.Time `json:"lease_until"`
	Attempts   int        `json:"attempts"`
	LastError  string     `json:"last_error"`
}

type ReviewRun struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project_id"`
	DocumentID int64     `json:"document_id"`
	VersionID  *int64    `json:"version_id"`
	Result     string    `json:"result"`
	Fingerprint string   `json:"fingerprint"`
	Note       string    `json:"note"`
	CheckedAt  time.Time `json:"checked_at"`
}
