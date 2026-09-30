// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/open-code-review/internal/llm"
)

var translationMu sync.Mutex

type translationRequest struct {
	Repo    string `json:"repo"`
	Session string `json:"session"`
	Finding string `json:"finding"`
}

// ponytail: concurrent first reads can duplicate one LLM call; use per-key coordination if costs grow.
func translateCached(ctx context.Context, path, id, original string, translate func(context.Context, string) (string, error)) (string, error) {
	translationMu.Lock()
	entries := make(map[string]string)
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			translationMu.Unlock()
			return "", err
		}
	} else if !os.IsNotExist(err) {
		translationMu.Unlock()
		return "", err
	}
	if value := entries[id]; value != "" {
		translationMu.Unlock()
		return value, nil
	}
	translationMu.Unlock()
	translated, err := translate(ctx, original)
	if err != nil {
		return "", err
	}
	if translated == "" {
		return "", errors.New("empty translation")
	}
	translationMu.Lock()
	defer translationMu.Unlock()
	data, err = os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if value := entries[id]; value != "" {
		return value, nil
	}
	entries[id] = translated
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	data, err = json.Marshal(entries)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".translations-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return translated, nil
}

func translateVietnamese(ctx context.Context, content string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	endpoint, err := llm.ResolveEndpoint(filepath.Join(home, ".opencodereview", "config.json"))
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, err := llm.NewLLMClient(endpoint, nil, nil).CompletionsWithCtx(ctx, llm.ChatRequest{
		Model: endpoint.Model,
		Messages: []llm.Message{
			{Role: "system", Content: "Translate the code review finding into Vietnamese. Preserve code identifiers, quoted symbols, and inline code unchanged. Output only the translation."},
			{Role: "user", Content: content},
		},
		MaxTokens: 2048,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(response.Content()), nil
}

func (j *reviewJobs) translateFinding(w http.ResponseWriter, r *http.Request) {
	if !j.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var req translationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || unsafeSegment(req.Repo) || unsafeSegment(req.Session) || req.Repo == "" || req.Session == "" || req.Finding == "" {
		http.Error(w, "invalid translation request", http.StatusBadRequest)
		return
	}
	session, err := LoadSession(j.root, req.Repo, req.Session)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	for _, finding := range session.Comments {
		if finding.MarkID != req.Finding {
			continue
		}
		path := filepath.Join(filepath.Dir(j.root), "translations", req.Repo, req.Session+".json")
		value, err := translateCached(r.Context(), path, req.Finding, finding.Content, translateVietnamese)
		if err != nil {
			http.Error(w, fmt.Sprintf("translation unavailable: %v", err), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"translation": value})
		return
	}
	http.Error(w, "finding not found", http.StatusNotFound)
}
