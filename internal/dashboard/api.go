// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// SPDX-License-Identifier: Apache-2.0

package dashboard

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// --- JSON API handlers ---

func (s *Server) apiCreateRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GitLabURL     string `json:"gitlab_url"`
		ProjectID     int64  `json:"project_id"`
		ProjectName   string `json:"project_name"`
		ClonePath     string `json:"clone_path"`
		DefaultBranch string `json:"default_branch"`
		APIToken      string `json:"api_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.GitLabURL == "" || req.ProjectID == 0 || req.ProjectName == "" || req.APIToken == "" {
		jsonError(w, "gitlab_url, project_id, project_name, api_token required", http.StatusBadRequest)
		return
	}
	if req.ClonePath == "" {
		req.ClonePath = fmt.Sprintf("/data/repos/%s", req.ProjectName)
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "develop"
	}
	repo := &Repo{
		GitLabURL:     req.GitLabURL,
		ProjectID:     req.ProjectID,
		ProjectName:   req.ProjectName,
		ClonePath:     req.ClonePath,
		DefaultBranch: req.DefaultBranch,
		APIToken:      req.APIToken,
	}
	id, err := s.db.InsertRepo(repo)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]interface{}{"id": id})
}

func (s *Server) apiListReviews(w http.ResponseWriter, r *http.Request) {
	repoID, err := pathInt(r, "repoID")
	if err != nil {
		jsonError(w, "invalid repo id", http.StatusBadRequest)
		return
	}
	reviews, err := s.db.ListReviews(repoID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if reviews == nil {
		reviews = []*Review{}
	}
	jsonOK(w, reviews)
}

func (s *Server) apiTriggerReview(w http.ResponseWriter, r *http.Request) {
	repoID, err := pathInt(r, "repoID")
	if err != nil {
		jsonError(w, "invalid repo id", http.StatusBadRequest)
		return
	}
	var req struct {
		MRIID        int64  `json:"mr_iid"`
		MRTitle      string `json:"mr_title"`
		MRURL        string `json:"mr_url"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Author       string `json:"author"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.SourceBranch == "" {
		jsonError(w, "source_branch required", http.StatusBadRequest)
		return
	}

	repo, err := s.db.GetRepo(repoID)
	if err != nil {
		jsonError(w, "repo not found", http.StatusNotFound)
		return
	}
	if req.TargetBranch == "" {
		req.TargetBranch = repo.DefaultBranch
	}

	review := &Review{
		RepoID:       repoID,
		MRIID:        req.MRIID,
		MRTitle:      req.MRTitle,
		MRURL:        req.MRURL,
		SourceBranch: req.SourceBranch,
		TargetBranch: req.TargetBranch,
		Author:       req.Author,
		Status:       "pending",
		TriggeredBy:  "manual",
	}
	reviewID, err := s.db.InsertReview(review)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.worker.Enqueue(ReviewJob{ReviewID: reviewID, RepoID: repoID})

	s.db.InsertAudit(&AuditEntry{
		ReviewID: reviewID,
		Action:   "trigger_review",
		Actor:    req.Author,
		Detail:   fmt.Sprintf("manual trigger for branch %s", req.SourceBranch),
	})

	jsonOK(w, map[string]interface{}{"review_id": reviewID, "status": "queued"})
}

func (s *Server) apiGetReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := pathInt(r, "reviewID")
	if err != nil {
		jsonError(w, "invalid review id", http.StatusBadRequest)
		return
	}
	review, err := s.db.GetReview(reviewID)
	if err != nil {
		jsonError(w, "review not found", http.StatusNotFound)
		return
	}
	comments, err := s.db.ListComments(reviewID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	review.Comments = comments

	runs, _ := s.db.ListReviewRuns(reviewID)
	review.Runs = runs

	jsonOK(w, review)
}

func (s *Server) apiReReview(w http.ResponseWriter, r *http.Request) {
	reviewID, err := pathInt(r, "reviewID")
	if err != nil {
		jsonError(w, "invalid review id", http.StatusBadRequest)
		return
	}
	review, err := s.db.GetReview(reviewID)
	if err != nil {
		jsonError(w, "review not found", http.StatusNotFound)
		return
	}

	// Create a new review for the same MR.
	newReview := &Review{
		RepoID:       review.RepoID,
		MRIID:        review.MRIID,
		MRTitle:      review.MRTitle,
		MRURL:        review.MRURL,
		SourceBranch: review.SourceBranch,
		TargetBranch: review.TargetBranch,
		Author:       review.Author,
		Status:       "pending",
		TriggeredBy:  "re-review",
	}
	newID, err := s.db.InsertReview(newReview)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Carry over intentional comments so they persist.
	oldComments, _ := s.db.ListComments(reviewID)
	for _, c := range oldComments {
		if c.Status == "intentional" {
			clone := &Comment{
				ReviewID:          newID,
				Path:              c.Path,
				Content:           c.Content,
				StartLine:         c.StartLine,
				EndLine:           c.EndLine,
				Category:          c.Category,
				Severity:          c.Severity,
				SuggestionCode:    c.SuggestionCode,
				ExistingCode:      c.ExistingCode,
				Status:            "intentional",
				IntentionalReason: c.IntentionalReason,
				ResolvedBy:        c.ResolvedBy,
				ResolvedAt:        c.ResolvedAt,
			}
			s.db.InsertComment(clone)
		}
	}

	s.worker.Enqueue(ReviewJob{ReviewID: newID, RepoID: review.RepoID})

	s.db.InsertAudit(&AuditEntry{
		ReviewID: reviewID,
		Action:   "re_review",
		Detail:   fmt.Sprintf("new review %d created", newID),
	})

	jsonOK(w, map[string]interface{}{"review_id": newID, "status": "queued"})
}

func (s *Server) apiMarkFixed(w http.ResponseWriter, r *http.Request) {
	commentID, err := pathInt(r, "commentID")
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	var req struct {
		Actor string `json:"actor"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Actor == "" {
		jsonError(w, "actor required", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkCommentFixed(commentID, req.Actor); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit.
	s.db.InsertAudit(&AuditEntry{
		CommentID: commentID,
		Action:    "mark_fixed",
		Actor:     req.Actor,
	})

	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) apiMarkIntentional(w http.ResponseWriter, r *http.Request) {
	commentID, err := pathInt(r, "commentID")
	if err != nil {
		jsonError(w, "invalid comment id", http.StatusBadRequest)
		return
	}
	var req struct {
		Actor  string `json:"actor"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Actor == "" {
		jsonError(w, "actor required", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) < 10 {
		jsonError(w, "reason must be at least 10 characters", http.StatusBadRequest)
		return
	}
	if err := s.db.MarkCommentIntentional(commentID, req.Actor, reason); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.db.InsertAudit(&AuditEntry{
		CommentID: commentID,
		Action:    "mark_intentional",
		Actor:     req.Actor,
		Detail:    reason,
	})

	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) apiGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, stats)
}

func (s *Server) apiListAudit(w http.ResponseWriter, r *http.Request) {
	reviewID, err := pathInt(r, "reviewID")
	if err != nil {
		jsonError(w, "invalid review id", http.StatusBadRequest)
		return
	}
	entries, err := s.db.ListAudit(reviewID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []*AuditEntry{}
	}
	jsonOK(w, entries)
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// pathInt extracts a named integer from the URL path.
// Expects mux patterns like /api/repo/{repoID}/reviews.
func pathInt(r *http.Request, name string) (int64, error) {
	v := r.PathValue(name)
	if v == "" {
		// Fallback: try to parse from URL segments.
		return 0, fmt.Errorf("missing path value %q", name)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func init() {
	// Suppress unused import warning.
	_ = log.Printf
}
