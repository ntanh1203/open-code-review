// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// SPDX-License-Identifier: Apache-2.0

package dashboard

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// GitLabMREvent is the subset of a GitLab merge_request webhook payload we need.
type GitLabMREvent struct {
	ObjectKind string `json:"object_kind"` // "merge_request"
	Project    struct {
		ID     int64  `json:"id"`
		WebURL string `json:"web_url"`
		PathNS string `json:"path_with_namespace"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int64  `json:"iid"`
		Title        string `json:"title"`
		URL          string `json:"url"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Action       string `json:"action"` // open, update, reopen, ...
		State        string `json:"state"`  // opened, merged, closed
	} `json:"object_attributes"`
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

// HandleGitLabWebhook processes incoming GitLab MR webhook events.
func (s *Server) HandleGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify webhook secret if configured.
	if s.webhookSecret != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.webhookSecret)) != 1 {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	}

	var event GitLabMREvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	if event.ObjectKind != "merge_request" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ignored: not a merge_request event")
		return
	}

	// Only process open/update/reopen actions on opened MRs.
	action := event.ObjectAttributes.Action
	if action != "open" && action != "update" && action != "reopen" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ignored: action=%s", action)
		return
	}

	// Find repo by GitLab project ID.
	repo, err := s.findRepoByProjectID(event.Project.ID)
	if err != nil {
		log.Printf("[webhook] repo not found for project %d: %v", event.Project.ID, err)
		http.Error(w, "repo not configured", http.StatusNotFound)
		return
	}

	// Check if there's already a running review for this MR.
	existing, err := s.db.FindLatestReview(repo.ID, event.ObjectAttributes.IID)
	if err == nil && existing.Status == "running" {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "review already running")
		return
	}

	// Create review record.
	review := &Review{
		RepoID:       repo.ID,
		MRIID:        event.ObjectAttributes.IID,
		MRTitle:      event.ObjectAttributes.Title,
		MRURL:        event.ObjectAttributes.URL,
		SourceBranch: event.ObjectAttributes.SourceBranch,
		TargetBranch: event.ObjectAttributes.TargetBranch,
		Author:       event.User.Username,
		Status:       "pending",
		TriggeredBy:  "webhook",
	}

	reviewID, err := s.db.InsertReview(review)
	if err != nil {
		log.Printf("[webhook] insert review: %v", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	// Enqueue job.
	s.worker.Enqueue(ReviewJob{ReviewID: reviewID, RepoID: repo.ID})

	log.Printf("[webhook] queued review %d for MR !%d (%s -> %s)",
		reviewID, event.ObjectAttributes.IID, event.ObjectAttributes.SourceBranch, event.ObjectAttributes.TargetBranch)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"review_id": reviewID,
		"status":    "queued",
	})
}

func (s *Server) findRepoByProjectID(projectID int64) (*Repo, error) {
	repos, err := s.db.ListRepos()
	if err != nil {
		return nil, err
	}
	for _, r := range repos {
		if r.ProjectID == projectID {
			full, err := s.db.GetRepo(r.ID)
			if err != nil {
				return nil, err
			}
			return full, nil
		}
	}
	return nil, sql.ErrNoRows
}
