// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package viewer

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/alibaba/open-code-review/internal/llm"
)

type llmProviderOption struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	BaseURL string   `json:"base_url"`
	Models  []string `json:"models"`
}

type llmConfigView struct {
	Provider  string              `json:"provider"`
	URL       string              `json:"url"`
	Model     string              `json:"model"`
	KeySet    bool                `json:"key_set"`
	Providers []llmProviderOption `json:"providers"`
}

type llmConfigUpdate struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
}

func llmConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".opencodereview", "config.json"), nil
}

// The file is edited as a generic JSON tree so fields this form does not
// know about (MCP servers, telemetry, extra headers) survive a save.
func readLLMConfig(path string) (map[string]any, error) {
	cfg := map[string]any{}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&cfg); err != nil || cfg == nil {
		return nil, errors.New("config.json must be a JSON object")
	}
	return cfg, nil
}

func childMap(parent map[string]any, key string) map[string]any {
	if m, ok := parent[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	parent[key] = m
	return m
}

func providerSection(name string) string {
	if _, preset := llm.LookupProvider(name); preset {
		return "providers"
	}
	return "custom_providers"
}

// llmConfigMu serializes the read-modify-write so two tabs saving different
// providers cannot drop each other's changes.
var llmConfigMu sync.Mutex

func (j *reviewJobs) llmConfig(w http.ResponseWriter, r *http.Request) {
	if !j.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path, err := llmConfigPath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	llmConfigMu.Lock()
	defer llmConfigMu.Unlock()
	cfg, err := readLLMConfig(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodPost {
		var update llmConfigUpdate
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&update); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := applyLLMConfig(cfg, update); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := writeLLMConfig(path, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(describeLLMConfig(cfg))
}

func applyLLMConfig(cfg map[string]any, u llmConfigUpdate) error {
	u.Provider, u.URL, u.Model = strings.TrimSpace(u.Provider), strings.TrimSpace(u.URL), strings.TrimSpace(u.Model)
	if u.Provider == "" || strings.ContainsAny(u.Provider, " ./\\") {
		return errors.New("provider name is required and cannot contain spaces, dots or slashes")
	}
	if u.Model == "" {
		return errors.New("model is required")
	}
	if u.URL != "" {
		parsed, err := url.Parse(u.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("base URL must be an http or https URL")
		}
	}
	section := providerSection(u.Provider)
	if section == "custom_providers" && u.URL == "" {
		return errors.New("a custom provider needs a base URL")
	}
	if prev, _ := cfg["provider"].(string); prev != u.Provider {
		delete(cfg, "model")
	}
	cfg["provider"] = u.Provider
	entry := childMap(childMap(cfg, section), u.Provider)
	if u.URL == "" {
		delete(entry, "url")
	} else {
		entry["url"] = u.URL
	}
	// ponytail: new custom providers default to the OpenAI chat protocol; edit
	// config.json or use `ocr config provider` for anthropic or responses.
	if _, ok := entry["protocol"]; !ok && section == "custom_providers" {
		entry["protocol"] = llm.ProtocolOpenAIChatCompletions
	}
	entry["model"] = u.Model
	if key := strings.TrimSpace(u.APIKey); key != "" {
		entry["api_key"] = key
	}
	return nil
}

// writeLLMConfig replaces the file atomically so a failed write never leaves a
// truncated config holding the user's credentials behind.
func writeLLMConfig(path string, cfg map[string]any) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func describeLLMConfig(cfg map[string]any) llmConfigView {
	view := llmConfigView{}
	view.Provider, _ = cfg["provider"].(string)
	view.Model, _ = cfg["model"].(string)
	if view.Provider != "" {
		section, _ := cfg[providerSection(view.Provider)].(map[string]any)
		entry, _ := section[view.Provider].(map[string]any)
		view.URL, _ = entry["url"].(string)
		if m, _ := entry["model"].(string); m != "" {
			view.Model = m
		}
		key, _ := entry["api_key"].(string)
		cmd, _ := entry["api_key_cmd"].(string)
		view.KeySet = strings.TrimSpace(key) != "" || strings.TrimSpace(cmd) != ""
	}
	for _, p := range llm.ListProviders() {
		view.Providers = append(view.Providers, llmProviderOption{Name: p.Name, Label: p.DisplayName, BaseURL: p.BaseURL, Models: p.Models})
	}
	custom, _ := cfg["custom_providers"].(map[string]any)
	names := make([]string, 0, len(custom))
	for name := range custom {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		view.Providers = append(view.Providers, llmProviderOption{Name: name, Label: "custom", Models: []string{}})
	}
	return view
}
