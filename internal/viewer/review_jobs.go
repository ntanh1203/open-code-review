// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type reviewJob struct {
	ID         string     `json:"id"`
	RepoDir    string     `json:"repo_dir"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	SessionURL string     `json:"session_url,omitempty"`
	Started    time.Time  `json:"started"`
	Finished   *time.Time `json:"finished,omitempty"`
}

type reviewRequest struct {
	RepoDir string `json:"repo_dir"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type reviewJobs struct {
	mu    sync.Mutex
	items map[string]reviewJob
	order []string
	slots chan struct{}
	token string
	root  string
	run   func(context.Context, reviewRequest) (string, error)
	ctx   context.Context
}

var sessionLine = regexp.MustCompile(`(?m)^\[ocr\] Session: ([0-9a-f-]+)(?: \(retry with: --resume [0-9a-f-]+\))?$`)

func newReviewJobs(root string) *reviewJobs {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("cannot generate viewer CSRF token: " + err.Error())
	}
	j := &reviewJobs{items: make(map[string]reviewJob), slots: make(chan struct{}, 2), token: hex.EncodeToString(key), root: root}
	j.run = j.runReview
	j.ctx = context.Background()
	return j
}

func (j *reviewJobs) runReview(ctx context.Context, req reviewRequest) (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, binary, "review", "--repo", req.RepoDir, "--from", req.From, "--to", req.To)
	cmd.Dir = req.RepoDir
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("review failed: %s", lastReviewLine(output.String()))
	}
	match := sessionLine.FindStringSubmatch(output.String())
	if len(match) < 2 {
		return "", errors.New("review ended without a session ID")
	}
	for _, repo := range mustDiscoverRepos(j.root) {
		if _, err := os.Stat(filepath.Join(j.root, repo.EncodedPath, match[1]+".jsonl")); err == nil {
			return "/r/" + url.PathEscape(repo.EncodedPath) + "/" + match[1], nil
		}
	}
	return "", errors.New("review session not found")
}

func mustDiscoverRepos(root string) []RepoInfo {
	repos, _ := DiscoverRepos(root)
	return repos
}

func lastReviewLine(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "Error:") {
			return strings.TrimSpace(line)
		}
	}
	return "check provider configuration or repository refs"
}

func (j *reviewJobs) authorized(r *http.Request) bool {
	return localRequest(r) && sameOrigin(r) && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Viewer-Token")), []byte(j.token)) == 1
}

func (j *reviewJobs) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if !j.authorized(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		j.create(w, r)
		return
	}
	j.mu.Lock()
	if id := r.PathValue("id"); id != "" {
		job, ok := j.items[id]
		j.mu.Unlock()
		if !ok {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(job)
		return
	}
	jobs := make([]reviewJob, 0, len(j.order))
	for i := len(j.order) - 1; i >= 0; i-- {
		jobs = append(jobs, j.items[j.order[i]])
	}
	j.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

func localRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback()
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" { // Non-browser clients must still know the CSRF token.
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func (j *reviewJobs) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req reviewRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := validateReviewRequest(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		http.Error(w, "cannot create job", http.StatusInternalServerError)
		return
	}
	job := reviewJob{ID: hex.EncodeToString(idBytes), RepoDir: req.RepoDir, From: req.From, To: req.To, Status: "queued", Started: time.Now()}
	j.mu.Lock()
	if len(j.order) >= 32 {
		j.mu.Unlock()
		http.Error(w, "review queue full; restart viewer to clear history", http.StatusTooManyRequests)
		return
	}
	j.items[job.ID] = job
	j.order = append(j.order, job.ID)
	j.mu.Unlock()
	go j.execute(job.ID, req)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func validateReviewRequest(req *reviewRequest) error {
	if !filepath.IsAbs(req.RepoDir) || strings.ContainsRune(req.RepoDir, 0) || filepath.Clean(req.RepoDir) != req.RepoDir {
		return errors.New("source folder must be an absolute path")
	}
	path, err := filepath.EvalSymlinks(req.RepoDir)
	if err != nil {
		return errors.New("source folder not found")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errors.New("source folder must be a directory")
	}
	req.RepoDir = path
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	root, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(root)) != path {
		return errors.New("source folder must be a Git repository root")
	}
	for _, ref := range []*string{&req.From, &req.To} {
		if *ref == "" || len(*ref) > 256 || strings.HasPrefix(*ref, "-") || strings.ContainsAny(*ref, "\x00\n\r") {
			return errors.New("invalid branch name")
		}
		if !isCommit(path, *ref) {
			// A branch fetched but never checked out exists only as origin/<name>.
			if !isCommit(path, "origin/"+*ref) {
				return fmt.Errorf("branch not found: %s", *ref)
			}
			*ref = "origin/" + *ref
		}
	}
	return nil
}

func isCommit(repo, ref string) bool {
	return exec.Command("git", "-C", repo, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}").Run() == nil
}

func (j *reviewJobs) execute(id string, req reviewRequest) {
	select {
	case j.slots <- struct{}{}:
		defer func() { <-j.slots }()
	case <-j.ctx.Done():
		return
	}
	j.mu.Lock()
	job := j.items[id]
	job.Status = "running"
	j.items[id] = job
	j.mu.Unlock()
	link, err := j.run(j.ctx, req)
	j.mu.Lock()
	job = j.items[id]
	now := time.Now()
	job.Finished = &now
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	} else {
		job.Status = "completed"
		job.SessionURL = link
	}
	j.items[id] = job
	j.mu.Unlock()
}
