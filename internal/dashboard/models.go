// SPDX-License-Identifier: Apache-2.0

package dashboard

import "time"

// Repo holds a configured GitLab project.
type Repo struct {
	ID            int64     `json:"id"`
	GitLabURL     string    `json:"gitlab_url"`
	ProjectID     int64     `json:"project_id"`
	ProjectName   string    `json:"project_name"`
	ClonePath     string    `json:"clone_path"`
	DefaultBranch string    `json:"default_branch"`
	APIToken      string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}

// Review represents a single MR review run.
type Review struct {
	ID            int64      `json:"id"`
	RepoID        int64      `json:"repo_id"`
	MRIID         int64      `json:"mr_iid"`
	MRTitle       string     `json:"mr_title"`
	MRURL         string     `json:"mr_url"`
	SourceBranch  string     `json:"source_branch"`
	TargetBranch  string     `json:"target_branch"`
	Author        string     `json:"author"`
	Status        string     `json:"status"` // pending/running/done/failed
	OCRJSON       string     `json:"-"`
	SessionID     string     `json:"session_id"`
	FilesReviewed int        `json:"files_reviewed"`
	CommentsCount int        `json:"comments_count"`
	CriticalCount int        `json:"critical_count"`
	HighCount     int        `json:"high_count"`
	MediumCount   int        `json:"medium_count"`
	LowCount      int        `json:"low_count"`
	TriggeredBy   string     `json:"triggered_by"` // webhook/manual
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`

	// Joined fields (not in DB directly).
	RepoName string     `json:"repo_name,omitempty"`
	Comments []*Comment `json:"comments,omitempty"`
	Runs     []*ReviewRun `json:"runs,omitempty"`
}

// Comment is a single review finding.
type Comment struct {
	ID               int64      `json:"id"`
	ReviewID         int64      `json:"review_id"`
	Path             string     `json:"path"`
	Content          string     `json:"content"`
	StartLine        int        `json:"start_line"`
	EndLine          int        `json:"end_line"`
	Category         string     `json:"category"`  // bug/security/performance/maintainability/test/style/documentation/other
	Severity         string     `json:"severity"`  // critical/high/medium/low
	SuggestionCode   string     `json:"suggestion_code,omitempty"`
	ExistingCode     string     `json:"existing_code,omitempty"`
	Status           string     `json:"status"` // open/fixed/intentional
	IntentionalReason string   `json:"intentional_reason,omitempty"`
	ResolvedBy       string     `json:"resolved_by,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}

// ReviewRun tracks each re-review iteration.
type ReviewRun struct {
	ID            int64     `json:"id"`
	ReviewID      int64     `json:"review_id"`
	RunNumber     int       `json:"run_number"`
	Status        string    `json:"status"`
	CommentsCount int       `json:"comments_count"`
	OpenCount     int       `json:"open_count"`
	OCRJSON       string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
}

// AuditEntry records an action taken on a review or comment.
type AuditEntry struct {
	ID        int64     `json:"id"`
	ReviewID  int64     `json:"review_id"`
	CommentID int64     `json:"comment_id,omitempty"`
	Action    string    `json:"action"` // mark_fixed/mark_intentional/re_review/trigger_review
	Actor     string    `json:"actor"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ReviewJob is sent to the worker queue.
type ReviewJob struct {
	ReviewID int64
	RepoID   int64
}

// OCROutput mirrors the top-level JSON from `ocr review --format json`.
type OCROutput struct {
	Status   string     `json:"status"`
	LLM      OCRLLM     `json:"llm"`
	Summary  OCRSummary `json:"summary"`
	Comments []OCRComment `json:"comments"`
	SessionID string    `json:"session_id"`
}

// OCRLLM holds provider info.
type OCRLLM struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// OCRSummary holds review stats.
type OCRSummary struct {
	FilesReviewed int    `json:"files_reviewed"`
	Comments      int    `json:"comments"`
	TotalTokens   int    `json:"total_tokens"`
	Elapsed       string `json:"elapsed"`
}

// OCRComment is a single finding from ocr output.
type OCRComment struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	SuggestionCode string `json:"suggestion_code"`
	ExistingCode   string `json:"existing_code"`
	StartLine      int    `json:"start_line"`
	EndLine        int    `json:"end_line"`
	Category       string `json:"category"`
	Severity       string `json:"severity"`
}

// Stats aggregates dashboard statistics.
type Stats struct {
	TotalReviews   int              `json:"total_reviews"`
	TotalComments  int              `json:"total_comments"`
	OpenComments   int              `json:"open_comments"`
	FixedComments  int              `json:"fixed_comments"`
	IntentionalComments int         `json:"intentional_comments"`
	SeverityCounts map[string]int   `json:"severity_counts"`
	CategoryCounts map[string]int   `json:"category_counts"`
	TopFiles       []FileStats      `json:"top_files"`
	MemberActivity []MemberStats    `json:"member_activity"`
}

// FileStats tracks issues per file.
type FileStats struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// MemberStats tracks member actions.
type MemberStats struct {
	Name            string `json:"name"`
	FixedCount      int    `json:"fixed_count"`
	IntentionalCount int   `json:"intentional_count"`
}
