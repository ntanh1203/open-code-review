// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestTranslateFindingCached(t *testing.T) {
	root := t.TempDir()
	calls := 0
	translate := func(_ context.Context, content string) (string, error) {
		calls++
		return "Ban dich: " + content, nil
	}
	for i := 0; i < 2; i++ {
		got, err := translateCached(context.Background(), filepath.Join(root, "translation.json"), "finding-id", "Original", translate)
		if err != nil || got != "Ban dich: Original" {
			t.Fatalf("translation %d = %q, %v", i, got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("LLM calls = %d; want 1", calls)
	}
}

func TestTranslationEndpointRejectsCrossOrigin(t *testing.T) {
	jobs := newReviewJobs(t.TempDir())
	r := httptest.NewRequest(http.MethodPost, "http://localhost:5483/api/translate", strings.NewReader(`{"repo":"repo","session":"session","finding":"id"}`))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Origin", "http://evil.example")
	r.Header.Set("X-Viewer-Token", jobs.token)
	w := httptest.NewRecorder()
	jobs.translateFinding(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", w.Code)
	}
}
