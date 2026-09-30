// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (j *reviewJobs) branches(w http.ResponseWriter, r *http.Request) {
	if !j.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		RepoDir string `json:"repo_dir"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	path, err := validRepoPath(request.RepoDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fetchCtx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := exec.CommandContext(fetchCtx, "git", "-C", path, "fetch", "origin", "--prune").Run(); err != nil {
		http.Error(w, "git fetch origin failed", http.StatusBadRequest)
		return
	}
	output, err := exec.Command("git", "-C", path, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin").Output()
	if err != nil {
		http.Error(w, "cannot list origin branches", http.StatusBadRequest)
		return
	}
	branches := make([]string, 0)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "origin/") && line != "origin/HEAD" {
			branches = append(branches, line)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(branches)
}

func validRepoPath(value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", errors.New("source folder must be an absolute path")
	}
	path, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", errors.New("source folder not found")
	}
	info, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil || strings.TrimSpace(string(info)) != path {
		return "", fmt.Errorf("source folder must be a Git repository root")
	}
	return path, nil
}
