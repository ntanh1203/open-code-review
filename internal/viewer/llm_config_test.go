// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
)

func TestLLMConfigEndpoint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	seed := `{"provider":"old","mcp_servers":{"gh":{"command":"gh"}},"custom_providers":{"old":{"protocol":"anthropic","url":"https://old.example","api_key":"secret-1"}}}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	jobs := newReviewJobs(t.TempDir())
	call := func(method, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://localhost:5483/api/llm-config", strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Viewer-Token", token)
		w := httptest.NewRecorder()
		jobs.llmConfig(w, r)
		return w
	}

	if w := call(http.MethodGet, "wrong", ""); w.Code != http.StatusForbidden {
		t.Fatalf("no token = %d", w.Code)
	}
	w := call(http.MethodGet, jobs.token, "")
	var view llmConfigView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Provider != "old" || !view.KeySet || view.URL != "https://old.example" {
		t.Fatalf("get = %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "secret-1") {
		t.Fatal("api key leaked to the browser")
	}

	for _, bad := range []string{
		`{"provider":"","model":"m"}`,
		`{"provider":"a.b","model":"m"}`,
		`{"provider":"old","model":""}`,
		`{"provider":"old","model":"m","url":"ftp://x"}`,
		`{"provider":"newgw","model":"m"}`,
		`not json`,
	} {
		if w := call(http.MethodPost, jobs.token, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", bad, w.Code)
		}
	}

	// Empty api_key keeps the stored one; unrelated sections survive.
	if w := call(http.MethodPost, jobs.token, `{"provider":"old","url":"https://new.example","model":"m1"}`); w.Code != http.StatusOK {
		t.Fatalf("save = %d %s", w.Code, w.Body)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"secret-1"`, `"mcp_servers"`, `"https://new.example"`, `"m1"`, `"anthropic"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("config missing %s:\n%s", want, data)
		}
	}

	// A new custom provider gets a protocol so the resolver accepts it.
	if w := call(http.MethodPost, jobs.token, `{"provider":"gw","url":"https://gw.example/v1","api_key":"k2","model":"m2"}`); w.Code != http.StatusOK {
		t.Fatalf("custom = %d %s", w.Code, w.Body)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `"protocol": "openai"`) || !strings.Contains(string(data), `"k2"`) {
		t.Errorf("custom provider not stored:\n%s", data)
	}
	if ep, err := llm.ResolveEndpoint(path); err != nil || ep.URL != "https://gw.example/v1" || ep.Model != "m2" || ep.Token != "k2" {
		t.Fatalf("review resolver rejects saved config: %+v %v", ep, err)
	}

	// A preset needs no URL; clearing it falls back to the preset base URL.
	w = call(http.MethodPost, jobs.token, `{"provider":"openai","api_key":"k3","model":"gpt-x"}`)
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Provider != "openai" || view.URL != "" || view.Model != "gpt-x" || !view.KeySet {
		t.Fatalf("preset = %d %s", w.Code, w.Body)
	}
}

func TestLLMConfigRejectsBrokenFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".opencodereview", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	jobs := newReviewJobs(t.TempDir())
	for _, broken := range []string{"{", "null"} {
		if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
			t.Fatal(err)
		}
		body := strings.NewReader(`{"provider":"openai","model":"gpt-4o"}`)
		r := httptest.NewRequest(http.MethodPost, "http://localhost:5483/api/llm-config", body)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Viewer-Token", jobs.token)
		w := httptest.NewRecorder()
		jobs.llmConfig(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("broken config %q = %d", broken, w.Code)
		}
		if data, _ := os.ReadFile(path); string(data) != broken {
			t.Fatalf("broken config %q was overwritten", broken)
		}
	}
}
