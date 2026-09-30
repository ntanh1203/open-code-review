// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func pickFolder(ctx context.Context, platform string) (string, error) {
	if platform != "darwin" {
		return "", errors.New("folder picker requires macOS; paste the path instead")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Select a Git repository")`)
	output, err := cmd.Output()
	if err != nil {
		return "", errors.New("folder selection cancelled or unavailable")
	}
	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

func (j *reviewJobs) pickFolder(w http.ResponseWriter, r *http.Request) {
	if !j.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path, err := pickFolder(j.ctx, runtime.GOOS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"path": path})
}
