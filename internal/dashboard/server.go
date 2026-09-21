// SPDX-License-Identifier: Apache-2.0

package dashboard

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
)

//go:embed templates/*.html static/*
var content embed.FS

// Server is the dashboard HTTP server.
type Server struct {
	db            *DB
	worker        *Worker
	webhookSecret string
	templates     map[string]*template.Template
	mux           *http.ServeMux
}

// NewServer creates and configures the dashboard server.
func NewServer(db *DB, worker *Worker, webhookSecret string) (*Server, error) {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"severityColor": func(s string) string {
			switch strings.ToLower(s) {
			case "critical":
				return "#dc2626"
			case "high":
				return "#ea580c"
			case "medium":
				return "#ca8a04"
			case "low":
				return "#65a30d"
			default:
				return "#6b7280"
			}
		},
		"statusColor": func(s string) string {
			switch s {
			case "done":
				return "#16a34a"
			case "running":
				return "#2563eb"
			case "pending":
				return "#ca8a04"
			case "failed":
				return "#dc2626"
			default:
				return "#6b7280"
			}
		},
		"commentStatusIcon": func(s string) string {
			switch s {
			case "fixed":
				return "check-circle"
			case "intentional":
				return "info"
			default:
				return "alert-circle"
			}
		},
		"pct": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a * 100 / b
		},
	}

	// Parse layout once, then clone for each page template.
	layoutTmpl, err := template.New("layout").Funcs(funcMap).ParseFS(content, "templates/layout.html")
	if err != nil {
		return nil, err
	}
	pages := []string{"repos.html", "reviews.html", "review_detail.html", "stats.html"}

	s := &Server{
		db:            db,
		worker:        worker,
		webhookSecret: webhookSecret,
		templates:     make(map[string]*template.Template),
		mux:           http.NewServeMux(),
	}
	for _, p := range pages {
		clone, cloneErr := layoutTmpl.Clone()
		if cloneErr != nil {
			return nil, cloneErr
		}
		pt, parseErr := clone.ParseFS(content, "templates/"+p)
		if parseErr != nil {
			return nil, parseErr
		}
		s.templates[p] = pt
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	// Pages
	s.mux.HandleFunc("GET /{$}", s.pageRepos)
	s.mux.HandleFunc("GET /repo/{repoID}", s.pageReviews)
	s.mux.HandleFunc("GET /repo/{repoID}/review/{reviewID}", s.pageReviewDetail)
	s.mux.HandleFunc("GET /stats", s.pageStats)

	// API
	s.mux.HandleFunc("POST /api/webhook/gitlab", s.HandleGitLabWebhook)
	s.mux.HandleFunc("POST /api/repo", s.apiCreateRepo)
	s.mux.HandleFunc("GET /api/repo/{repoID}/reviews", s.apiListReviews)
	s.mux.HandleFunc("POST /api/repo/{repoID}/review", s.apiTriggerReview)
	s.mux.HandleFunc("GET /api/review/{reviewID}", s.apiGetReview)
	s.mux.HandleFunc("POST /api/review/{reviewID}/re-review", s.apiReReview)
	s.mux.HandleFunc("PUT /api/comment/{commentID}/fixed", s.apiMarkFixed)
	s.mux.HandleFunc("PUT /api/comment/{commentID}/intentional", s.apiMarkIntentional)
	s.mux.HandleFunc("GET /api/review/{reviewID}/audit", s.apiListAudit)
	s.mux.HandleFunc("GET /api/stats", s.apiGetStats)

	// Static
	s.mux.Handle("GET /static/", http.FileServerFS(content))
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// --- Page handlers ---

func (s *Server) pageRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.db.ListRepos()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "repos.html", map[string]interface{}{"Repos": repos})
}

func (s *Server) pageReviews(w http.ResponseWriter, r *http.Request) {
	repoID, err := pathInt(r, "repoID")
	if err != nil {
		http.Error(w, "invalid repo id", 400)
		return
	}
	repo, err := s.db.GetRepo(repoID)
	if err != nil {
		http.Error(w, "repo not found", 404)
		return
	}
	reviews, err := s.db.ListReviews(repoID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "reviews.html", map[string]interface{}{"Repo": repo, "Reviews": reviews})
}

func (s *Server) pageReviewDetail(w http.ResponseWriter, r *http.Request) {
	repoID, _ := pathInt(r, "repoID")
	reviewID, err := pathInt(r, "reviewID")
	if err != nil {
		http.Error(w, "invalid review id", 400)
		return
	}
	review, err := s.db.GetReview(reviewID)
	if err != nil {
		http.Error(w, "review not found", 404)
		return
	}
	comments, err := s.db.ListComments(reviewID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	review.Comments = comments

	runs, _ := s.db.ListReviewRuns(reviewID)
	review.Runs = runs

	open, fixed, intentional, _ := s.db.CountCommentsByStatus(reviewID)
	total := open + fixed + intentional

	audit, _ := s.db.ListAudit(reviewID)

	// Filter by severity if requested.
	severityFilter := r.URL.Query().Get("severity")

	s.render(w, "review_detail.html", map[string]interface{}{
		"RepoID":         repoID,
		"Review":         review,
		"Comments":       comments,
		"Runs":           runs,
		"Audit":          audit,
		"Open":           open,
		"Fixed":          fixed,
		"Intentional":    intentional,
		"Total":          total,
		"SeverityFilter": severityFilter,
	})
}

func (s *Server) pageStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, "stats.html", map[string]interface{}{"Stats": stats})
}

func (s *Server) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, ok := s.templates[name]
	if !ok {
		log.Printf("[render] template %q not found", name)
		http.Error(w, "template not found", 500)
		return
	}
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[render] %s: %v", name, err)
		http.Error(w, "template error", 500)
	}
}

// intParam parses query param as int with fallback.
func intParam(r *http.Request, name string, fallback int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
