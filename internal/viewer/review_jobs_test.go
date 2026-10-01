// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewJobsSecurityAndConcurrency(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", repo}, {"-C", repo, "config", "core.hooksPath", "/dev/null"}, {"-C", repo, "commit", "--allow-empty", "-m", "initial"}} {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, output)
		}
	}
	jobs := newReviewJobs(t.TempDir())
	jobs.run = func(_ context.Context, req reviewRequest) (string, error) { return "/r/test/session", nil }
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/reviews", jobs.serve)
	mux.HandleFunc("GET /api/reviews", jobs.serve)
	reqData := reviewRequest{RepoDir: repo, From: "HEAD", To: "HEAD"}
	payload, _ := json.Marshal(reqData)
	request := func(origin, token string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:5483/api/reviews", bytes.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Viewer-Token", token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct{ origin, token string }{{"http://evil.example", jobs.token}, {"http://localhost:5483", "wrong"}} {
		if w := request(tc.origin, tc.token, payload); w.Code != http.StatusForbidden {
			t.Errorf("cross-site request: got %d", w.Code)
		}
	}
	invalid := []reviewRequest{{RepoDir: "relative", From: "HEAD", To: "HEAD"}, {RepoDir: t.TempDir(), From: "HEAD", To: "HEAD"}, {RepoDir: repo, From: "-bad", To: "HEAD"}, {RepoDir: repo, From: "missing-branch", To: "HEAD"}}
	for _, v := range invalid {
		body, _ := json.Marshal(v)
		if w := request("http://localhost:5483", jobs.token, body); w.Code != http.StatusBadRequest {
			t.Errorf("invalid request %+v: got %d: %s", v, w.Code, w.Body.String())
		}
	}
	if w := request("http://localhost:5483", jobs.token, []byte(strings.Repeat("x", 5000))); w.Code != http.StatusBadRequest {
		t.Errorf("oversized body: got %d", w.Code)
	}
	for range 2 {
		if w := request("http://localhost:5483", jobs.token, payload); w.Code != http.StatusAccepted {
			t.Fatalf("valid request: got %d: %s", w.Code, w.Body.String())
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jobs.mu.Lock()
		completed := 0
		for _, job := range jobs.items {
			if job.Status == "completed" && job.SessionURL == "/r/test/session" && job.Finished != nil {
				completed++
			}
		}
		jobs.mu.Unlock()
		if completed == 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("jobs did not finish independently")
}

func TestOriginBranches(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", repo}, {"-C", repo, "config", "core.hooksPath", "/dev/null"}, {"-C", repo, "commit", "--allow-empty", "-m", "initial"}, {"-C", repo, "branch", "develop"}, {"-C", repo, "remote", "add", "origin", repo}, {"-C", repo, "fetch", "origin"}} {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	jobs := newReviewJobs(t.TempDir())
	payload, _ := json.Marshal(map[string]string{"repo_dir": repo})
	post := func(origin, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:5483/api/branches", bytes.NewReader(payload))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", origin)
		r.Header.Set("X-Viewer-Token", token)
		w := httptest.NewRecorder()
		jobs.branches(w, r)
		return w
	}
	if got := post("http://evil.example", jobs.token).Code; got != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", got)
	}
	if got := post("http://localhost:5483", "wrong").Code; got != http.StatusForbidden {
		t.Fatalf("invalid token status = %d", got)
	}
	w := post("http://localhost:5483", jobs.token)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"origin/develop"`) {
		t.Fatalf("branches status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestReviewSessionLine(t *testing.T) {
	output := "[ocr] Session: 01234567-89ab-cdef-0123-456789abcdef (retry with: --resume 01234567-89ab-cdef-0123-456789abcdef)\n"
	match := sessionLine.FindStringSubmatch(output)
	if len(match) != 2 || match[1] != "01234567-89ab-cdef-0123-456789abcdef" {
		t.Fatalf("session match = %q", match)
	}
}

func TestReviewFormPresent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("templates", "repos.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`id="review-repo"`, `id="review-from"`, `value="develop"`, `id="review-to"`, `id="review-language"`} {
		if !bytes.Contains(data, []byte(field)) {
			t.Errorf("missing %s", field)
		}
	}
}

func TestValidateReviewRequestFallsBackToOrigin(t *testing.T) {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", repo}, {"-C", repo, "config", "core.hooksPath", "/dev/null"}, {"-C", repo, "commit", "--allow-empty", "-m", "initial"}, {"-C", repo, "branch", "remote-only"}, {"-C", repo, "remote", "add", "origin", repo}, {"-C", repo, "fetch", "origin"}, {"-C", repo, "branch", "-D", "remote-only"}} {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, output)
		}
	}
	root, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	req := reviewRequest{RepoDir: root, From: "HEAD", To: "remote-only"}
	if err := validateReviewRequest(&req); err != nil || req.From != "HEAD" || req.To != "origin/remote-only" {
		t.Fatalf("got %+v, %v", req, err)
	}
	req.To = "nowhere"
	if err := validateReviewRequest(&req); err == nil {
		t.Fatal("missing branch accepted")
	}
}
