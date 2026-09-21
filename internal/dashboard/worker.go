// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// SPDX-License-Identifier: Apache-2.0

package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Worker processes review jobs sequentially.
type Worker struct {
	db     *DB
	queue  chan ReviewJob
	wg     sync.WaitGroup
	ocrBin string // path to ocr binary; empty = "ocr" from PATH
}

// NewWorker creates a worker with a buffered job queue.
func NewWorker(db *DB, ocrBin string, queueSize int) *Worker {
	if queueSize <= 0 {
		queueSize = 32
	}
	return &Worker{
		db:     db,
		queue:  make(chan ReviewJob, queueSize),
		ocrBin: ocrBin,
	}
}

// Start begins the background worker goroutine.
func (w *Worker) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for job := range w.queue {
			w.process(job)
		}
	}()
}

// Stop closes the queue and waits for the worker to finish.
func (w *Worker) Stop() {
	close(w.queue)
	w.wg.Wait()
}

// Enqueue adds a review job.
func (w *Worker) Enqueue(job ReviewJob) {
	w.queue <- job
}

func (w *Worker) process(job ReviewJob) {
	log.Printf("[worker] processing review %d for repo %d", job.ReviewID, job.RepoID)

	if err := w.db.UpdateReviewStatus(job.ReviewID, "running"); err != nil {
		log.Printf("[worker] update status: %v", err)
		return
	}

	review, err := w.db.GetReview(job.ReviewID)
	if err != nil {
		log.Printf("[worker] get review: %v", err)
		w.db.UpdateReviewStatus(job.ReviewID, "failed")
		return
	}

	repo, err := w.db.GetRepo(job.RepoID)
	if err != nil {
		log.Printf("[worker] get repo: %v", err)
		w.db.UpdateReviewStatus(job.ReviewID, "failed")
		return
	}

	// Ensure repo is cloned and up-to-date.
	if err := w.ensureRepo(repo); err != nil {
		log.Printf("[worker] ensure repo: %v", err)
		w.db.UpdateReviewStatus(job.ReviewID, "failed")
		return
	}

	// Fetch latest refs.
	if err := w.gitFetch(repo.ClonePath); err != nil {
		log.Printf("[worker] git fetch: %v", err)
		w.db.UpdateReviewStatus(job.ReviewID, "failed")
		return
	}

	// Run ocr review.
	ocrJSON, err := w.runOCR(repo.ClonePath, review.TargetBranch, review.SourceBranch)
	if err != nil {
		log.Printf("[worker] ocr review: %v", err)
		w.db.UpdateReviewStatus(job.ReviewID, "failed")
		return
	}

	// Parse output.
	var output OCROutput
	if err := json.Unmarshal(ocrJSON, &output); err != nil {
		log.Printf("[worker] parse ocr json: %v", err)
		w.db.UpdateReviewResult(job.ReviewID, "failed", string(ocrJSON), "", 0, 0, 0, 0, 0, 0)
		return
	}

	// Count severities.
	var critical, high, medium, low int
	for _, c := range output.Comments {
		switch strings.ToLower(c.Severity) {
		case "critical":
			critical++
		case "high":
			high++
		case "medium":
			medium++
		case "low":
			low++
		}
	}

	// Update review record.
	w.db.UpdateReviewResult(
		job.ReviewID, "done", string(ocrJSON), output.SessionID,
		output.Summary.FilesReviewed, len(output.Comments),
		critical, high, medium, low,
	)

	// Insert comments.
	for _, oc := range output.Comments {
		c := &Comment{
			ReviewID:       job.ReviewID,
			Path:           oc.Path,
			Content:        oc.Content,
			StartLine:      oc.StartLine,
			EndLine:        oc.EndLine,
			Category:       oc.Category,
			Severity:       oc.Severity,
			SuggestionCode: oc.SuggestionCode,
			ExistingCode:   oc.ExistingCode,
		}
		if _, err := w.db.InsertComment(c); err != nil {
			log.Printf("[worker] insert comment: %v", err)
		}
	}

	// Record run.
	runNum, _ := w.db.LatestRunNumber(job.ReviewID)
	open, _, _, _ := w.db.CountCommentsByStatus(job.ReviewID)
	w.db.InsertReviewRun(&ReviewRun{
		ReviewID:      job.ReviewID,
		RunNumber:     runNum + 1,
		Status:        "done",
		CommentsCount: len(output.Comments),
		OpenCount:     open,
		OCRJSON:       string(ocrJSON),
	})

	log.Printf("[worker] review %d done: %d comments (%d critical, %d high, %d medium, %d low)",
		job.ReviewID, len(output.Comments), critical, high, medium, low)
}

func (w *Worker) ensureRepo(repo *Repo) error {
	if _, err := os.Stat(filepath.Join(repo.ClonePath, ".git")); err == nil {
		return nil // already cloned
	}
	if err := os.MkdirAll(filepath.Dir(repo.ClonePath), 0o755); err != nil {
		return err
	}
	// Build clone URL with token for auth.
	cloneURL := w.buildCloneURL(repo)
	cmd := exec.Command("git", "clone", "--no-checkout", cloneURL, repo.ClonePath)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w *Worker) buildCloneURL(repo *Repo) string {
	// https://oauth2:<token>@gitlab.mebisoft.vn/group/project.git
	u := strings.TrimPrefix(repo.GitLabURL, "https://")
	u = strings.TrimPrefix(u, "http://")
	return fmt.Sprintf("https://oauth2:%s@%s/%s.git", repo.APIToken, u, repo.ProjectName)
}

func (w *Worker) gitFetch(dir string) error {
	cmd := exec.Command("git", "fetch", "--all", "--prune")
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (w *Worker) runOCR(dir, targetBranch, sourceBranch string) ([]byte, error) {
	bin := w.ocrBin
	if bin == "" {
		bin = "ocr"
	}
	args := []string{
		"review",
		"--from", fmt.Sprintf("origin/%s", targetBranch),
		"--to", fmt.Sprintf("origin/%s", sourceBranch),
		"--format", "json",
		"--audience", "agent",
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("ocr review: %w (output: %s)", err, string(out))
	}
	return out, nil
}
