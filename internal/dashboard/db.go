// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// SPDX-License-Identifier: Apache-2.0

package dashboard

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection.
type DB struct {
	db *sql.DB
}

// OpenDB opens (or creates) a SQLite database at path.
func OpenDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	d := &DB{db: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

func (d *DB) migrate() error {
	_, err := d.db.Exec(`
CREATE TABLE IF NOT EXISTS repos (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    gitlab_url TEXT NOT NULL,
    project_id INTEGER NOT NULL,
    project_name TEXT NOT NULL,
    clone_path TEXT NOT NULL,
    default_branch TEXT DEFAULT 'develop',
    api_token TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id INTEGER REFERENCES repos(id),
    mr_iid INTEGER NOT NULL,
    mr_title TEXT,
    mr_url TEXT,
    source_branch TEXT,
    target_branch TEXT,
    author TEXT,
    status TEXT DEFAULT 'pending',
    ocr_json TEXT,
    session_id TEXT,
    files_reviewed INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    critical_count INTEGER DEFAULT 0,
    high_count INTEGER DEFAULT 0,
    medium_count INTEGER DEFAULT 0,
    low_count INTEGER DEFAULT 0,
    triggered_by TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id INTEGER REFERENCES reviews(id),
    path TEXT NOT NULL,
    content TEXT NOT NULL,
    start_line INTEGER DEFAULT 0,
    end_line INTEGER DEFAULT 0,
    category TEXT,
    severity TEXT,
    suggestion_code TEXT,
    existing_code TEXT,
    status TEXT DEFAULT 'open',
    intentional_reason TEXT,
    resolved_by TEXT,
    resolved_at DATETIME
);

CREATE TABLE IF NOT EXISTS review_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id INTEGER REFERENCES reviews(id),
    run_number INTEGER DEFAULT 1,
    status TEXT,
    comments_count INTEGER DEFAULT 0,
    open_count INTEGER DEFAULT 0,
    ocr_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    review_id INTEGER,
    comment_id INTEGER,
    action TEXT,
    actor TEXT,
    detail TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_reviews_repo ON reviews(repo_id);
CREATE INDEX IF NOT EXISTS idx_reviews_mr ON reviews(repo_id, mr_iid);
CREATE INDEX IF NOT EXISTS idx_comments_review ON comments(review_id);
CREATE INDEX IF NOT EXISTS idx_audit_review ON audit_log(review_id);
`)
	return err
}

// --- Repos ---

func (d *DB) InsertRepo(r *Repo) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO repos (gitlab_url, project_id, project_name, clone_path, default_branch, api_token) VALUES (?,?,?,?,?,?)`,
		r.GitLabURL, r.ProjectID, r.ProjectName, r.ClonePath, r.DefaultBranch, r.APIToken,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListRepos() ([]*Repo, error) {
	rows, err := d.db.Query(`SELECT id, gitlab_url, project_id, project_name, clone_path, default_branch, created_at FROM repos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []*Repo
	for rows.Next() {
		r := &Repo{}
		if err := rows.Scan(&r.ID, &r.GitLabURL, &r.ProjectID, &r.ProjectName, &r.ClonePath, &r.DefaultBranch, &r.CreatedAt); err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func (d *DB) GetRepo(id int64) (*Repo, error) {
	r := &Repo{}
	err := d.db.QueryRow(
		`SELECT id, gitlab_url, project_id, project_name, clone_path, default_branch, api_token, created_at FROM repos WHERE id=?`, id,
	).Scan(&r.ID, &r.GitLabURL, &r.ProjectID, &r.ProjectName, &r.ClonePath, &r.DefaultBranch, &r.APIToken, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// --- Reviews ---

func (d *DB) InsertReview(r *Review) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO reviews (repo_id, mr_iid, mr_title, mr_url, source_branch, target_branch, author, status, triggered_by) VALUES (?,?,?,?,?,?,?,?,?)`,
		r.RepoID, r.MRIID, r.MRTitle, r.MRURL, r.SourceBranch, r.TargetBranch, r.Author, r.Status, r.TriggeredBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateReviewResult(id int64, status, ocrJSON, sessionID string, filesReviewed, commentsCount, critical, high, medium, low int) error {
	now := time.Now()
	_, err := d.db.Exec(
		`UPDATE reviews SET status=?, ocr_json=?, session_id=?, files_reviewed=?, comments_count=?, critical_count=?, high_count=?, medium_count=?, low_count=?, finished_at=? WHERE id=?`,
		status, ocrJSON, sessionID, filesReviewed, commentsCount, critical, high, medium, low, now, id,
	)
	return err
}

func (d *DB) UpdateReviewStatus(id int64, status string) error {
	_, err := d.db.Exec(`UPDATE reviews SET status=? WHERE id=?`, status, id)
	return err
}

func (d *DB) ListReviews(repoID int64) ([]*Review, error) {
	rows, err := d.db.Query(
		`SELECT id, repo_id, mr_iid, mr_title, mr_url, source_branch, target_branch, author, status, session_id, files_reviewed, comments_count, critical_count, high_count, medium_count, low_count, triggered_by, created_at, finished_at FROM reviews WHERE repo_id=? ORDER BY created_at DESC`,
		repoID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reviews []*Review
	for rows.Next() {
		r := &Review{}
		if err := rows.Scan(&r.ID, &r.RepoID, &r.MRIID, &r.MRTitle, &r.MRURL, &r.SourceBranch, &r.TargetBranch, &r.Author, &r.Status, &r.SessionID, &r.FilesReviewed, &r.CommentsCount, &r.CriticalCount, &r.HighCount, &r.MediumCount, &r.LowCount, &r.TriggeredBy, &r.CreatedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

func (d *DB) GetReview(id int64) (*Review, error) {
	r := &Review{}
	err := d.db.QueryRow(
		`SELECT r.id, r.repo_id, r.mr_iid, r.mr_title, r.mr_url, r.source_branch, r.target_branch, r.author, r.status, r.session_id, r.files_reviewed, r.comments_count, r.critical_count, r.high_count, r.medium_count, r.low_count, r.triggered_by, r.created_at, r.finished_at, p.project_name FROM reviews r JOIN repos p ON r.repo_id=p.id WHERE r.id=?`, id,
	).Scan(&r.ID, &r.RepoID, &r.MRIID, &r.MRTitle, &r.MRURL, &r.SourceBranch, &r.TargetBranch, &r.Author, &r.Status, &r.SessionID, &r.FilesReviewed, &r.CommentsCount, &r.CriticalCount, &r.HighCount, &r.MediumCount, &r.LowCount, &r.TriggeredBy, &r.CreatedAt, &r.FinishedAt, &r.RepoName)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// FindLatestReview returns the latest review for a given MR.
func (d *DB) FindLatestReview(repoID, mrIID int64) (*Review, error) {
	r := &Review{}
	err := d.db.QueryRow(
		`SELECT id, repo_id, mr_iid, mr_title, mr_url, source_branch, target_branch, author, status, session_id, files_reviewed, comments_count, critical_count, high_count, medium_count, low_count, triggered_by, created_at, finished_at FROM reviews WHERE repo_id=? AND mr_iid=? ORDER BY created_at DESC LIMIT 1`,
		repoID, mrIID,
	).Scan(&r.ID, &r.RepoID, &r.MRIID, &r.MRTitle, &r.MRURL, &r.SourceBranch, &r.TargetBranch, &r.Author, &r.Status, &r.SessionID, &r.FilesReviewed, &r.CommentsCount, &r.CriticalCount, &r.HighCount, &r.MediumCount, &r.LowCount, &r.TriggeredBy, &r.CreatedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// --- Comments ---

func (d *DB) InsertComment(c *Comment) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO comments (review_id, path, content, start_line, end_line, category, severity, suggestion_code, existing_code) VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ReviewID, c.Path, c.Content, c.StartLine, c.EndLine, c.Category, c.Severity, c.SuggestionCode, c.ExistingCode,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListComments(reviewID int64) ([]*Comment, error) {
	rows, err := d.db.Query(
		`SELECT id, review_id, path, content, start_line, end_line, category, severity, suggestion_code, existing_code, status, intentional_reason, resolved_by, resolved_at FROM comments WHERE review_id=? ORDER BY severity='critical' DESC, severity='high' DESC, severity='medium' DESC, id`,
		reviewID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var comments []*Comment
	for rows.Next() {
		c := &Comment{}
		if err := rows.Scan(&c.ID, &c.ReviewID, &c.Path, &c.Content, &c.StartLine, &c.EndLine, &c.Category, &c.Severity, &c.SuggestionCode, &c.ExistingCode, &c.Status, &c.IntentionalReason, &c.ResolvedBy, &c.ResolvedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (d *DB) MarkCommentFixed(id int64, actor string) error {
	now := time.Now()
	_, err := d.db.Exec(`UPDATE comments SET status='fixed', resolved_by=?, resolved_at=? WHERE id=?`, actor, now, id)
	return err
}

func (d *DB) MarkCommentIntentional(id int64, actor, reason string) error {
	now := time.Now()
	_, err := d.db.Exec(`UPDATE comments SET status='intentional', intentional_reason=?, resolved_by=?, resolved_at=? WHERE id=?`, reason, actor, now, id)
	return err
}

func (d *DB) ReopenComment(id int64) error {
	_, err := d.db.Exec(`UPDATE comments SET status='open', intentional_reason=NULL, resolved_by=NULL, resolved_at=NULL WHERE id=?`, id)
	return err
}

func (d *DB) CountCommentsByStatus(reviewID int64) (open, fixed, intentional int, err error) {
	rows, err := d.db.Query(`SELECT status, COUNT(*) FROM comments WHERE review_id=? GROUP BY status`, reviewID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var c int
		if err = rows.Scan(&s, &c); err != nil {
			return
		}
		switch s {
		case "open":
			open = c
		case "fixed":
			fixed = c
		case "intentional":
			intentional = c
		}
	}
	err = rows.Err()
	return
}

// --- Review Runs ---

func (d *DB) InsertReviewRun(run *ReviewRun) (int64, error) {
	res, err := d.db.Exec(
		`INSERT INTO review_runs (review_id, run_number, status, comments_count, open_count, ocr_json) VALUES (?,?,?,?,?,?)`,
		run.ReviewID, run.RunNumber, run.Status, run.CommentsCount, run.OpenCount, run.OCRJSON,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListReviewRuns(reviewID int64) ([]*ReviewRun, error) {
	rows, err := d.db.Query(
		`SELECT id, review_id, run_number, status, comments_count, open_count, created_at FROM review_runs WHERE review_id=? ORDER BY run_number`,
		reviewID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []*ReviewRun
	for rows.Next() {
		r := &ReviewRun{}
		if err := rows.Scan(&r.ID, &r.ReviewID, &r.RunNumber, &r.Status, &r.CommentsCount, &r.OpenCount, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (d *DB) LatestRunNumber(reviewID int64) (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT COALESCE(MAX(run_number),0) FROM review_runs WHERE review_id=?`, reviewID).Scan(&n)
	return n, err
}

// --- Audit Log ---

func (d *DB) InsertAudit(e *AuditEntry) error {
	_, err := d.db.Exec(
		`INSERT INTO audit_log (review_id, comment_id, action, actor, detail) VALUES (?,?,?,?,?)`,
		e.ReviewID, e.CommentID, e.Action, e.Actor, e.Detail,
	)
	return err
}

func (d *DB) ListAudit(reviewID int64) ([]*AuditEntry, error) {
	rows, err := d.db.Query(
		`SELECT id, review_id, comment_id, action, actor, detail, created_at FROM audit_log WHERE review_id=? ORDER BY created_at DESC`,
		reviewID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.ReviewID, &e.CommentID, &e.Action, &e.Actor, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// --- Stats ---

func (d *DB) GetStats() (*Stats, error) {
	s := &Stats{
		SeverityCounts: make(map[string]int),
		CategoryCounts: make(map[string]int),
	}

	d.db.QueryRow(`SELECT COUNT(*) FROM reviews`).Scan(&s.TotalReviews)
	d.db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&s.TotalComments)
	d.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE status='open'`).Scan(&s.OpenComments)
	d.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE status='fixed'`).Scan(&s.FixedComments)
	d.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE status='intentional'`).Scan(&s.IntentionalComments)

	// Severity counts
	rows, err := d.db.Query(`SELECT severity, COUNT(*) FROM comments GROUP BY severity`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var v int
			rows.Scan(&k, &v)
			s.SeverityCounts[k] = v
		}
	}

	// Category counts
	rows2, err := d.db.Query(`SELECT category, COUNT(*) FROM comments GROUP BY category`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var k string
			var v int
			rows2.Scan(&k, &v)
			s.CategoryCounts[k] = v
		}
	}

	// Top files
	rows3, err := d.db.Query(`SELECT path, COUNT(*) as c FROM comments GROUP BY path ORDER BY c DESC LIMIT 20`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			f := FileStats{}
			rows3.Scan(&f.Path, &f.Count)
			s.TopFiles = append(s.TopFiles, f)
		}
	}

	// Member activity
	rows4, err := d.db.Query(`SELECT resolved_by, SUM(CASE WHEN status='fixed' THEN 1 ELSE 0 END), SUM(CASE WHEN status='intentional' THEN 1 ELSE 0 END) FROM comments WHERE resolved_by IS NOT NULL AND resolved_by != '' GROUP BY resolved_by ORDER BY COUNT(*) DESC`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			m := MemberStats{}
			rows4.Scan(&m.Name, &m.FixedCount, &m.IntentionalCount)
			s.MemberActivity = append(s.MemberActivity, m)
		}
	}

	return s, nil
}
